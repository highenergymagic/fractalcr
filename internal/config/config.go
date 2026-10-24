// Package config implements the appliance configuration model, independent of
// the CLI and the host operating system. Runtime selection is inheritance,
// not an attempt to prevent an administrator from overriding policy.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/go-connections/nat"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type Config struct {
	Version    int                         `json:"version"`
	Hostname   string                      `json:"hostname"`
	FilePrompt string                      `json:"file_prompt,omitempty"`
	AAA        AAA                         `json:"aaa"`
	Network    *Network                    `json:"network,omitempty"`
	Interfaces map[string]*Network         `json:"interfaces,omitempty"`
	NTP        NTP                         `json:"ntp,omitempty"`
	Engine     Engine                      `json:"engine,omitempty"`
	Networks   map[string]DockerNetwork    `json:"docker_networks,omitempty"`
	Datastores map[string]Datastore        `json:"datastores"`
	Isolation  Isolation                   `json:"container_isolation"`
	Containers map[string]Container        `json:"containers"`
	Volumes    map[string]VolumeDefinition `json:"volumes,omitempty"`
}

type Network struct {
	Parent             string                 `json:"parent,omitempty"`
	BondMode           string                 `json:"bond_mode,omitempty"`
	Members            []string               `json:"members,omitempty"`
	Description        string                 `json:"description,omitempty"`
	MTU                int                    `json:"mtu,omitempty"`
	IPv6Addresses      []string               `json:"ipv6_addresses,omitempty"`
	IPv6Gateway        string                 `json:"ipv6_gateway,omitempty"`
	SecondaryAddresses []string               `json:"secondary_addresses,omitempty"`
	Routes             map[string]StaticRoute `json:"routes,omitempty"`
	Interface          string                 `json:"interface"`
	Address            string                 `json:"address"`
	Gateway            string                 `json:"gateway,omitempty"`
	DNS                string                 `json:"dns,omitempty"`
	Role               string                 `json:"role,omitempty"`
	DHCP               bool                   `json:"dhcp,omitempty"`
	Shutdown           bool                   `json:"shutdown,omitempty"`
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
	ContainerOptions
	Network             string            `json:"network,omitempty"`
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
var DriverName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/@+-]{0,255}$`)
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
	for name, v := range c.Volumes {
		if !Name.MatchString(name) {
			return errors.New("invalid volume name")
		}
		if err := v.Validate(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := c.Engine.Validate(); err != nil {
		return err
	}
	if err := c.AAA.Validate(); err != nil {
		return err
	}
	if len(c.JSON()) > 65536 {
		return errors.New("configuration exceeds 64 KiB")
	}
	if c.Version != 1 {
		return errors.New("unsupported configuration version")
	}
	switch c.FilePrompt {
	case "", "alert", "noisy", "quiet":
	default:
		return errors.New("file prompt must be alert, noisy or quiet")
	}
	if !Name.MatchString(c.Hostname) {
		return errors.New("invalid hostname")
	}
	if strings.ContainsAny(c.Hostname, "_.") {
		return errors.New("hostname permits letters, digits and hyphens only")
	}
	if err := c.validateNetworking(); err != nil {
		return err
	}
	if err := c.validateVirtualInterfaces(); err != nil {
		return err
	}
	for _, n := range c.AllInterfaces() {
		if !Interface.MatchString(n.Interface) {
			return errors.New("invalid interface name")
		}
		if n.Address != "" {
			ip, network, err := net.ParseCIDR(n.Address)
			if err != nil || ip.To4() == nil {
				return errors.New("address must be IPv4/prefix")
			}
			if prefix, _ := network.Mask.Size(); prefix < 1 {
				return errors.New("address must use IPv4 prefix 1..32")
			}
		}
		if n.Gateway != "" && net.ParseIP(n.Gateway).To4() == nil {
			return errors.New("gateway must be an IPv4 address")
		}
		if n.DNS != "" && net.ParseIP(n.DNS) == nil {
			return errors.New("DNS must be an IP address")
		}
		if err := n.ValidateOptions(); err != nil {
			return err
		}
		if err := n.ValidateRoutes(); err != nil {
			return err
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
		// Definitions may be built over several CLI commands. Disk-count
		// requirements are enforced before the explicit format operation.
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
	if i.VMFactory && (!i.Enabled || !i.Installed || c.Engine.StorageBackend != "blockfile") {
		return errors.New("VM factory requires enabled Kata isolation and the blockfile storage backend")
	}
	if i.BaseOS != "appliance" && i.BaseOS != "Microsoft/CBL-Mariner" {
		return errors.New("only the image-bundled appliance Kata guest is available; guest OS selection requires a built and validated Kata guest")
	}
	if i.Enabled && !i.Installed {
		return errors.New("select the kata-containers runtime before enabling isolation")
	}
	for name, container := range c.Containers {
		if container.Network != "" && container.Network != "bridge" && container.Network != "host" && container.Network != "none" {
			if _, ok := c.Networks[container.Network]; !ok {
				return fmt.Errorf("%s: network is not configured", name)
			}
		}
		if container.MemoryMiB < 0 || (container.MemoryMiB > 0 && container.MemoryMiB < 4) || container.MemoryMiB > 1048576 {
			return errors.New("container memory must be 4..1048576 MiB")
		}
		if container.Pids < 0 || container.Pids > 1048576 {
			return errors.New("invalid PID limit")
		}
		if err := container.ContainerOptions.Validate(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if len(container.Networks) > 0 && (container.Network == "host" || container.Network == "none") {
			return fmt.Errorf("%s: host/none networking cannot attach other networks", name)
		}
		for network := range container.Networks {
			if network != "bridge" {
				if _, ok := c.Networks[network]; !ok {
					return fmt.Errorf("%s: attached network %s is not configured", name, network)
				}
			}
		}
		if container.MemoryMiB > 0 && container.MemoryReservationMiB > container.MemoryMiB {
			return fmt.Errorf("%s: memory reservation exceeds memory limit", name)
		}
		if container.MemorySwapMiB > 0 && container.MemoryMiB > container.MemorySwapMiB {
			return fmt.Errorf("%s: combined memory and swap limit is below memory limit", name)
		}
		targets := map[string]bool{}
		for _, v := range container.Volumes {
			if !Name.MatchString(v.Name) || !strings.HasPrefix(v.Target, "/") || v.Target == "/" || strings.Contains(v.Target, "..") || strings.ContainsAny(v.Target, ",\x00\r\n") || targets[v.Target] {
				return errors.New("invalid or duplicate volume mount")
			}
			targets[v.Target] = true
		}
		for target := range container.Binds {
			if targets[target] {
				return fmt.Errorf("duplicate mount target")
			}
			targets[target] = true
		}
		for target := range container.Tmpfs {
			if targets[target] {
				return fmt.Errorf("duplicate mount target")
			}
			targets[target] = true
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
		if _, _, err := nat.ParsePortSpecs(container.Publish); err != nil {
			return fmt.Errorf("invalid published port: %w", err)
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

var Interface = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,47}$`)
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
