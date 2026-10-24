package appliance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fractalcr/internal/config"
)

var runningPath = "/run/running-config.json"
var startupPath = "/datastore/data0/config/startup-config.json"
var journalPath = "/run/config-rollback.json"
var checkDatastore = dataOnline
var commandLinePath = "/proc/cmdline"

func recoveryBoot() bool {
	data, err := os.ReadFile(commandLinePath)
	if err != nil {
		return false
	}
	for _, word := range strings.Fields(string(data)) {
		if word == "fcr.ignore_startup=1" {
			return true
		}
	}
	return false
}

func loadRunning() (config.Config, error) {
	c, err := config.Load(runningPath)
	if os.IsNotExist(err) && recoveryBoot() {
		return config.Default(), nil
	}
	if os.IsNotExist(err) {
		c, err = config.Load(startupPath)
	}
	if os.IsNotExist(err) {
		return config.Default(), nil
	}
	if err == nil {
		migrateInterfaces(&c)
	}
	return c, err
}

func supported(c config.Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	for name := range c.AllInterfaces() {
		if _, kind, _, _ := config.VirtualInterface(name); kind != "" {
			continue
		}
		if _, err := interfaceByName(name); err != nil {
			return fmt.Errorf("interface %s does not exist", name)
		}
	}
	for n := range c.Datastores {
		if n != "data0" {
			return fmt.Errorf("this image supports datastore data0")
		}
	}
	// Never accept a policy that would silently fall back from Kata to runc.
	if c.Isolation.Installed || c.Isolation.Enabled {
		for _, path := range []string{"/opt/kata/bin/containerd-shim-kata-v2", "/opt/kata/bin/cloud-hypervisor", "/opt/kata/libexec/virtiofsd", "/usr/libexec/fractal-virtiofsd", "/opt/kata/share/kata-containers/kata-containers.img", "/opt/kata/share/kata-containers/vmlinux.container"} {
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("Kata artifact unavailable: %s", path)
			}
		}
	}
	if c.Isolation.Enabled {
		if err := kvmAvailable(); err != nil {
			return err
		}
	}
	return nil
}

// Prepare is invoked by the configuration service after its dependencies are
// ready and before networking or Docker start. The runtime snapshot stays put
// until the old networking worker has removed its owned addresses.
func Prepare() error {
	if err := provisionCloud(); err != nil {
		return fmt.Errorf("initial provisioning: %w", err)
	}
	c, err := loadRunning()
	if err != nil {
		return err
	}
	if err = supported(c); err != nil {
		return err
	}
	if err = syscall.Sethostname([]byte(c.Hostname)); err != nil {
		return err
	}
	files, err := c.Render()
	if err != nil {
		return err
	}
	for _, name := range []string{"kata-containers/factory.json", "containerd.toml"} {
		if err := config.AtomicWrite(filepath.Join("/run", name), files[name], 0600); err != nil {
			return err
		}
	}
	if c.Isolation.Installed {
		if err = config.AtomicWrite("/run/kata-containers/configuration.toml", files["kata-containers/configuration.toml"], 0600); err != nil {
			return err
		}
	}
	if err = config.AtomicWrite("/run/docker-daemon.json", files["docker/daemon.json"], 0600); err != nil {
		return err
	}
	if err = config.AtomicWrite("/run/startup-config.json", c.JSON(), 0600); err != nil {
		return err
	}
	return config.AtomicWrite(runningPath, c.JSON(), 0600)
}

func dataOnline() error {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/datastore/data0", &st); err != nil {
		return err
	}
	if uint64(st.Type) != 0x9123683e {
		return fmt.Errorf("datastore data0 is offline")
	}
	return nil
}
func Save(c config.Config) error {
	if err := checkDatastore(); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if recoveryBoot() {
		// Preserve the original even when it cannot be parsed. Never overwrite
		// an earlier recovery backup or truncate the startup file on failure.
		original, err := os.ReadFile(startupPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			backup, err := os.OpenFile(startupPath+".pre-recovery", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil && !os.IsExist(err) {
				return err
			}
			if err == nil {
				_, err = backup.Write(original)
				if err == nil {
					err = backup.Sync()
				}
				closeErr := backup.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
			}
		}
	}
	return config.AtomicWrite(startupPath, c.JSON(), 0600)
}

func applyConfiguration(c config.Config) error {
	if _, err := control(2, "configuration"); err != nil {
		return err
	}
	if err := waitState(func(s map[string]Status) bool {
		return s["configuration"].PID == 0 && s["networking"].PID == 0 && s["container-engine"].PID == 0 &&
			(s["configuration"].State == 0 || s["configuration"].State == 5)
	}); err != nil {
		return err
	}
	if err := config.AtomicWrite(runningPath, c.JSON(), 0600); err != nil {
		return err
	}
	// Restart clears failure latches in enabled consumers as well as the
	// configuration worker. Stopped consumers retain their disabled state.
	before, err := control(3, "configuration")
	if err != nil {
		return err
	}
	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		s, err := control(0, "")
		if err != nil {
			return err
		}
		if s["configuration"].Ready() && (s["networking"].Enabled == 0 || s["networking"].Ready()) && (s["time-sync"].Enabled == 0 || s["networking"].Enabled == 0 || s["time-sync"].Ready()) && (s["container-engine"].Enabled == 0 || s["networking"].Enabled == 0 || s["container-engine"].Ready()) {
			return nil
		}
		if s["configuration"].Starts > before["configuration"].Starts {
			for _, name := range []string{"configuration", "networking", "container-storage", "vm-factory", "container-engine", "time-sync"} {
				if s[name].Enabled != 0 && s[name].Blocked == 0 && s[name].State == 5 {
					return fmt.Errorf("%s failed; see show logging %s", name, name)
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("configuration readiness timed out")
}

func Apply(old, next config.Config) error {
	if err := supported(next); err != nil {
		return err
	}
	if (old.Engine.StorageBackend == "blockfile") != (next.Engine.StorageBackend == "blockfile") {
		if err := emptyEngineForStorageChange(); err != nil {
			return err
		}
	}
	// Bootstrap inventory can be edited before storage is initialized, but
	// enabling authentication there would make mandatory accounting unavailable
	// and lock the operator out of the commands needed to initialize it.
	if next.AAA.Enabled && !old.AAA.Enabled {
		if err := checkDatastore(); err != nil {
			return fmt.Errorf("initialize datastore data0 before enabling local login: %w", err)
		}
	}
	// Container definitions are declarative inventory. Explicit create/remove
	// operations control their lifecycle; editing a definition never destroys one.
	oldPlatform, nextPlatform := old.Clone(), next.Clone()
	oldPlatform.Containers = nil
	nextPlatform.Containers = nil
	oldPlatform.Volumes = nil
	nextPlatform.Volumes = nil
	oldPlatform.Networks = nil
	nextPlatform.Networks = nil
	oldPlatform.Datastores = nil
	nextPlatform.Datastores = nil
	// AAA is consumed live by management and SSH; it must not restart workloads.
	oldPlatform.AAA = config.AAA{}
	nextPlatform.AAA = config.AAA{}
	oldPlatform.Hostname = ""
	nextPlatform.Hostname = ""
	oldPlatform.FilePrompt = ""
	nextPlatform.FilePrompt = ""
	if string(oldPlatform.JSON()) == string(nextPlatform.JSON()) {
		if old.Hostname != next.Hostname {
			if err := syscall.Sethostname([]byte(next.Hostname)); err != nil {
				return err
			}
		}
		if err := config.AtomicWrite(runningPath, next.JSON(), 0600); err != nil {
			if old.Hostname != next.Hostname {
				if rollback := syscall.Sethostname([]byte(old.Hostname)); rollback != nil {
					return fmt.Errorf("saving running configuration failed: %v; hostname rollback failed: %v", err, rollback)
				}
			}
			return err
		}
		return nil
	}
	if err := checkDatastore(); err != nil {
		return err
	}
	if err := config.AtomicWrite(journalPath, old.JSON(), 0600); err != nil {
		return err
	}
	if err := applyConfiguration(next); err != nil {
		rollback := applyConfiguration(old)
		if rollback != nil {
			return fmt.Errorf("apply failed: %v; rollback failed: %v; recovery journal retained", err, rollback)
		}
		os.Remove(journalPath)
		return fmt.Errorf("apply failed; previous configuration restored: %w", err)
	}
	return os.Remove(journalPath)
}

func recoverTransaction() error {
	c, err := config.Load(journalPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = applyConfiguration(c); err != nil {
		return err
	}
	return os.Remove(journalPath)
}

func showConfigFile(path string) (string, error) {
	c, err := config.Load(path)
	if err != nil {
		return "", err
	}
	return string(c.JSON()), nil
}

func configDiff(a, b config.Config) string {
	if string(a.JSON()) == string(b.JSON()) {
		return "No pending changes."
	}
	return "Running:\n" + string(a.JSON()) + "Candidate:\n" + string(b.JSON())
}

func safeImage(ref string) error {
	if ref == "" || strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\r\n\x00") {
		return errors.New("invalid image reference")
	}
	return nil
}

// Check exact regular files before parsing; do not follow a FIFO or symlink in
// the data volume. config.Load enforces the same bound for startup and candidates.
func readJSON(path string, v any) error {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
