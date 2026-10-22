// Package config implements the appliance configuration model, independent of
// the CLI and the host operating system. Runtime selection is inheritance,
// not an attempt to prevent an administrator from overriding policy.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	Version    int                  `json:"version"`
	Hostname   string               `json:"hostname"`
	AAA        AAA                  `json:"aaa"`
	Network    *Network             `json:"network,omitempty"`
	Datastores map[string]Datastore `json:"datastores"`
	Isolation  Isolation            `json:"container_isolation"`
	Containers map[string]Container `json:"containers"`
}

type Network struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Gateway   string `json:"gateway,omitempty"`
	DNS       string `json:"dns,omitempty"`
}

type Datastore struct {
	Filesystem string   `json:"filesystem"`
	Preset     string   `json:"preset"`
	Disks      []string `json:"disks"`
}

type Isolation struct {
	Enabled   bool   `json:"enabled"`
	Installed bool   `json:"kata_installed"`
	ApplyTo   string `json:"apply_to"`
	Runtime   string `json:"runtime"`
	MemoryMiB int    `json:"memory_mib"`
	BaseOS    string `json:"base_os"`
	VMFactory bool   `json:"vm_factory"`
}

type Container struct {
	Image               string            `json:"image"`
	Isolation           string            `json:"isolation"` // inherit, none, kata
	Command             []string          `json:"command,omitempty"`
	Environment         map[string]string `json:"environment,omitempty"`
	Publish             []string          `json:"publish,omitempty"`
	Restart             string            `json:"restart,omitempty"`
	Volumes             []Volume          `json:"volumes,omitempty"`
	MemoryMiB           int               `json:"memory_mib,omitempty"`
	Pids                int               `json:"pids_limit,omitempty"`
	ReadOnly            bool              `json:"read_only,omitempty"`
	NoNewPrivileges     bool              `json:"no_new_privileges,omitempty"`
	DropAllCapabilities bool              `json:"drop_all_capabilities,omitempty"`
}
type Volume struct {
	Name     string `json:"name"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

var Name = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
var Disk = regexp.MustCompile(`^(nvme[0-9]+n[0-9]+|sd[a-z]+|vd[a-z]+|disk/by-id/[a-zA-Z0-9_.:+-]+)$`)

func Default() Config {
	return Config{Version: 1, Hostname: "ctr", AAA: AAA{ConsoleTimeout: 600, VTYTimeout: 600}, Datastores: map[string]Datastore{},
		Containers: map[string]Container{}, Isolation: Isolation{
			ApplyTo: "explicit", Runtime: "clh", MemoryMiB: 256, BaseOS: "appliance"}}
}

func (c Config) Clone() Config {
	b, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(b, &out)
	return out
}

func (c Config) JSON() []byte {
	b, _ := json.MarshalIndent(c, "", "  ")
	return append(b, '\n')
}

func Decode(b []byte) (Config, error) {
	if err := uniqueFields(b); err != nil {
		return Config{}, err
	}
	var required map[string]json.RawMessage
	if err := json.Unmarshal(b, &required); err != nil {
		return Config{}, err
	}
	if _, ok := required["version"]; !ok {
		return Config{}, errors.New("version is required")
	}
	if _, ok := required["hostname"]; !ok {
		return Config{}, errors.New("hostname is required")
	}
	c := Default()
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, errors.New("trailing JSON data")
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

func Load(path string) (Config, error) {
	f, err := openConfig(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Config{}, err
	}
	if !st.Mode().IsRegular() || st.Size() > 65536 {
		return Config{}, errors.New("configuration must be a regular file of at most 64 KiB")
	}
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return Config{}, err
	}
	if len(b) > 65536 {
		return Config{}, errors.New("configuration too large")
	}
	return Decode(b)
}

func (c Config) Validate() error {
	if err := c.AAA.Validate(); err != nil {
		return err
	}
	if len(c.JSON()) > 65536 {
		return errors.New("configuration exceeds 64 KiB")
	}
	if c.Version != 1 {
		return errors.New("unsupported configuration version")
	}
	if !Name.MatchString(c.Hostname) {
		return errors.New("invalid hostname")
	}
	if strings.ContainsAny(c.Hostname, "_.") {
		return errors.New("hostname permits letters, digits and hyphens only")
	}
	if n := c.Network; n != nil {
		if !Interface.MatchString(n.Interface) {
			return errors.New("invalid interface name")
		}
		ip, network, err := net.ParseCIDR(n.Address)
		if err != nil || ip.To4() == nil {
			return errors.New("address must be IPv4/prefix")
		}
		if prefix, _ := network.Mask.Size(); prefix < 1 {
			return errors.New("address must use IPv4 prefix 1..32")
		}
		for _, a := range []string{n.Gateway, n.DNS} {
			if a != "" && net.ParseIP(a).To4() == nil {
				return errors.New("gateway and DNS must be IPv4 addresses")
			}
		}
	}
	if c.Datastores == nil || c.Containers == nil {
		return errors.New("datastores and containers must be objects")
	}
	used := map[string]string{}
	for _, name := range Keys(c.Datastores) {
		ds := c.Datastores[name]
		if !Name.MatchString(name) {
			return fmt.Errorf("invalid datastore name %q", name)
		}
		if ds.Filesystem != "btrfs" {
			return fmt.Errorf("%s: only btrfs is supported", name)
		}
		if ds.Preset != "single" && ds.Preset != "raid1" {
			return fmt.Errorf("%s: expected single or raid1", name)
		}
		if len(ds.Disks) == 0 || (ds.Preset == "raid1" && len(ds.Disks) < 2) {
			return fmt.Errorf("%s: insufficient disks for %s", name, ds.Preset)
		}
		for _, disk := range ds.Disks {
			if !Disk.MatchString(disk) {
				return fmt.Errorf("invalid disk %q; use a whole disk or disk/by-id identifier", disk)
			}
			if owner, ok := used[disk]; ok {
				return fmt.Errorf("disk %s already belongs to %s", disk, owner)
			}
			used[disk] = name
		}
	}
	i := c.Isolation
	if i.Runtime != "clh" {
		return errors.New("this target supports the clh Kata runtime")
	}
	if i.ApplyTo != "explicit" && i.ApplyTo != "all-containers" {
		return errors.New("invalid isolation apply-to")
	}
	if i.MemoryMiB < 128 || i.MemoryMiB > 1048576 {
		return errors.New("Kata memory must be 128..1048576 MiB")
	}
	if i.VMFactory {
		return errors.New("VM factory is not supported by this clh integration")
	}
	if i.BaseOS != "appliance" && i.BaseOS != "Microsoft/CBL-Mariner" {
		return errors.New("only the image-bundled appliance Kata guest is available; guest OS selection requires a built and validated Kata guest")
	}
	if i.Enabled && !i.Installed {
		return errors.New("select the kata-containers runtime before enabling isolation")
	}
	for name, container := range c.Containers {
		if container.MemoryMiB < 0 || (container.MemoryMiB > 0 && container.MemoryMiB < 4) || container.MemoryMiB > 1048576 {
			return errors.New("container memory must be 4..1048576 MiB")
		}
		if container.Pids < 0 || container.Pids > 1048576 {
			return errors.New("invalid PID limit")
		}
		targets := map[string]bool{}
		for _, v := range container.Volumes {
			if !Name.MatchString(v.Name) || !strings.HasPrefix(v.Target, "/") || v.Target == "/" || strings.Contains(v.Target, "..") || strings.ContainsAny(v.Target, ",\x00\r\n") || targets[v.Target] {
				return errors.New("invalid or duplicate volume mount")
			}
			targets[v.Target] = true
		}
		if !Name.MatchString(name) {
			return fmt.Errorf("invalid container name %q", name)
		}
		if !slices.Contains([]string{"inherit", "none", "kata"}, container.Isolation) {
			return fmt.Errorf("%s: invalid isolation override", name)
		}
		if strings.ContainsAny(container.Image, " \t\r\n\x00") || strings.HasPrefix(container.Image, "-") {
			return fmt.Errorf("%s: invalid image reference", name)
		}
		if container.Restart != "" && !slices.Contains([]string{"no", "always", "unless-stopped", "on-failure"}, container.Restart) {
			return errors.New("invalid restart policy")
		}
		for k, v := range container.Environment {
			if !EnvName.MatchString(k) || strings.ContainsRune(v, 0) {
				return errors.New("invalid environment variable")
			}
		}
		for _, p := range container.Publish {
			if !Port.MatchString(p) {
				return errors.New("publish requires HOSTPORT:CONTAINERPORT[/tcp|udp]")
			}
			ports := strings.Split(strings.Split(p, "/")[0], ":")
			for _, port := range ports {
				v, err := strconv.Atoi(port)
				if err != nil || v < 1 || v > 65535 {
					return errors.New("port must be 1..65535")
				}
			}
		}
		for _, a := range container.Command {
			if strings.ContainsRune(a, 0) {
				return errors.New("invalid command argument")
			}
		}
		if container.Isolation == "kata" && (!i.Installed || !i.Enabled) {
			return fmt.Errorf("%s: Kata override requires system isolation enabled and runtime selected", name)
		}
	}
	return nil
}

func uniqueFields(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	var value func() error
	value = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return errors.New("duplicate or invalid JSON key")
				}
				seen[key] = true
				if err = value(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		case json.Delim('['):
			for d.More() {
				if err = value(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	return value()
}

var Interface = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,14}$`)
var EnvName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var Port = regexp.MustCompile(`^[0-9]{1,5}:[0-9]{1,5}(/tcp|/udp)?$`)

// EffectiveRuntime deliberately allows a native exception to a global Kata
// default. A global enable gates Kata; apply-to selects inheritance only.
func (c Config) EffectiveRuntime(name string) string {
	v := c.Containers[name]
	if v.Isolation == "none" {
		return "runc"
	}
	if c.Isolation.Enabled && (v.Isolation == "kata" || c.Isolation.ApplyTo == "all-containers") {
		return "io.containerd.kata.v2"
	}
	return "runc"
}

func Keys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// AtomicWrite never replaces a good configuration with a partially written one.
// Linux parent-directory fsync makes rename durable across power loss.
func AtomicWrite(path string, b []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(dir)
}
