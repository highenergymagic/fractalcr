package appliance

// NoCloud is a provisioning format, not an interpreter. Only the explicitly
// supported initial configuration is imported; scripts and packages are errors.
import (
	"bytes"
	"errors"
	"fmt"
	"fractalcr/internal/config"
	"go.yaml.in/yaml/v3"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

type cloudUser struct {
	Name   string   `yaml:"name"`
	Keys   []string `yaml:"ssh_authorized_keys"`
	Locked *bool    `yaml:"lock_passwd"`
}
type cloudData struct {
	Hostname     string      `yaml:"hostname"`
	Users        []cloudUser `yaml:"users"`
	Keys         []string    `yaml:"ssh_authorized_keys"`
	PasswordAuth *bool       `yaml:"ssh_pwauth"`
}
type cloudMeta struct {
	Instance string `yaml:"instance-id"`
	Hostname string `yaml:"local-hostname"`
}
type cloudNetwork struct {
	Version   int `yaml:"version"`
	Ethernets map[string]struct {
		Match struct {
			MAC string `yaml:"macaddress"`
		} `yaml:"match"`
		DHCP4 bool `yaml:"dhcp4"`
	} `yaml:"ethernets"`
}

func cloudDecode(data []byte, target any) error {
	if len(data) > 64*1024 {
		return errors.New("NoCloud document exceeds 64 KiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("NoCloud requires one YAML document")
	}
	return nil
}

func cloudConfig(metadata, userdata, network []byte, ports []platformPort) (config.Config, error) {
	c := config.Default()
	fail := func() (config.Config, error) {
		return c, errors.New("NoCloud requires an instance ID, key-only users and a unique management interface")
	}
	var meta cloudMeta
	var data cloudData
	if err := cloudDecode(metadata, &meta); err != nil {
		return c, fmt.Errorf("NoCloud meta-data: %w", err)
	}
	if !strings.HasPrefix(string(userdata), "#cloud-config\n") && !strings.HasPrefix(string(userdata), "#cloud-config\r\n") {
		return fail()
	}
	if err := cloudDecode(userdata, &data); err != nil {
		return c, fmt.Errorf("NoCloud user-data: %w", err)
	}
	if len(meta.Instance) == 0 || len(meta.Instance) > 256 || strings.ContainsAny(meta.Instance, "\r\n\x00") || data.PasswordAuth != nil && *data.PasswordAuth {
		return fail()
	}
	if meta.Hostname != "" {
		c.Hostname = meta.Hostname
	}
	if data.Hostname != "" {
		c.Hostname = data.Hostname
	}
	if len(data.Keys) != 0 {
		data.Users = append(data.Users, cloudUser{Name: "admin", Keys: data.Keys})
	}
	c.AAA.Users = map[string]config.User{}
	for _, user := range data.Users {
		if !config.Name.MatchString(user.Name) || len(user.Keys) == 0 || user.Locked != nil && !*user.Locked {
			return fail()
		}
		if _, exists := c.AAA.Users[user.Name]; exists {
			return c, errors.New("duplicate NoCloud user")
		}
		u := config.User{Privilege: 15}
		for _, value := range user.Keys {
			key, err := config.NormalizeSSHKey(value)
			if err != nil {
				return c, err
			}
			u.SSHKeys = append(u.SSHKeys, key)
		}
		c.AAA.Users[user.Name] = u
	}
	if len(c.AAA.Users) == 0 {
		return fail()
	}
	c.AAA.Enabled, c.AAA.SSH = true, true
	selected := ""
	if len(network) != 0 {
		var n cloudNetwork
		if err := cloudDecode(network, &n); err != nil {
			return c, fmt.Errorf("NoCloud network-config: %w", err)
		}
		if n.Version != 2 || len(n.Ethernets) != 1 {
			return fail()
		}
		for _, link := range n.Ethernets {
			mac, err := net.ParseMAC(link.Match.MAC)
			if err != nil || !link.DHCP4 {
				return fail()
			}
			for _, p := range ports {
				if bytes.Equal(p.HardwareAddr, mac) {
					if selected != "" {
						return fail()
					}
					selected = p.Name
				}
			}
		}
	} else if len(ports) == 1 {
		selected = ports[0].Name
	}
	if selected == "" {
		return fail()
	}
	c.Network = &config.Network{Interface: selected, Role: "management", DHCP: true}
	return c, c.Validate()
}

func cloudRead(dir, name string, optional bool) ([]byte, error) {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if optional && os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 65536 {
		return nil, errors.New("invalid NoCloud seed file")
	}
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if len(b) > 65536 {
		return nil, errors.New("NoCloud seed too large")
	}
	return b, err
}

// Probe only labelled local media. No metadata HTTP requests, shell commands,
// filesystem writes to the seed, or retries that could replace saved accounts.
func cloudFilesystem(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b := make([]byte, 512)
	if _, err = f.ReadAt(b, 0); err == nil && b[510] == 0x55 && b[511] == 0xaa {
		for _, offset := range []int{43, 71} {
			if strings.EqualFold(strings.TrimSpace(string(b[offset:offset+11])), "cidata") {
				return "vfat"
			}
		}
	}
	b = make([]byte, 2048)
	if _, err = f.ReadAt(b, 16*2048); err == nil && b[0] == 1 && string(b[1:6]) == "CD001" && strings.EqualFold(strings.TrimSpace(string(b[40:72])), "cidata") {
		return "iso9660"
	}
	return ""
}

func provisionCloud() error {
	if recoveryBoot() {
		return nil
	}
	if _, err := os.Lstat(startupPath); !os.IsNotExist(err) {
		return err
	}
	entries, err := os.ReadDir("/sys/class/block")
	if err != nil {
		return err
	}
	allowed := regexp.MustCompile(`^(sd[a-z]+[0-9]*|vd[a-z]+[0-9]*|nvme[0-9]+n[0-9]+(p[0-9]+)?|sr[0-9]+)$`)
	device, fs := "", ""
	for _, entry := range entries {
		if !allowed.MatchString(entry.Name()) {
			continue
		}
		path := "/dev/" + entry.Name()
		kind := cloudFilesystem(path)
		if kind != "" {
			if device != "" {
				return errors.New("multiple NoCloud seed devices")
			}
			device, fs = path, kind
		}
	}
	if device == "" {
		return nil
	}
	if err = checkDatastore(); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("/run", "nocloud-")
	if err != nil {
		return err
	}
	defer os.Remove(dir)
	if err = syscall.Mount(device, dir, fs, syscall.MS_RDONLY|syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""); err != nil {
		return err
	}
	defer syscall.Unmount(dir, 0)
	meta, err := cloudRead(dir, "meta-data", false)
	if err != nil {
		return err
	}
	user, err := cloudRead(dir, "user-data", false)
	if err != nil {
		return err
	}
	network, err := cloudRead(dir, "network-config", true)
	if err != nil {
		return err
	}
	c, err := cloudConfig(meta, user, network, platformPorts())
	if err != nil {
		return err
	}
	if err = supported(c); err != nil {
		return err
	}
	// One atomic startup file is the completion marker, so power loss cannot
	// produce a provisioned flag without the credentials it promises.
	return config.AtomicWrite(startupPath, c.JSON(), 0600)
}
