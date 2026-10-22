package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

type Command struct {
	Purpose string   `json:"purpose"`
	Argv    []string `json:"argv"`
}

// Render produces reviewable runtime inputs. It never executes commands,
// formats storage, restarts Docker, or silently recreates existing containers.
func (c Config) Render() (map[string][]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	runtime := "runc"
	if c.Isolation.Enabled && c.Isolation.ApplyTo == "all-containers" {
		runtime = "io.containerd.kata.v2"
	}
	daemon := map[string]any{
		"data-root":       "/datastore/data0/docker",
		"exec-root":       "/run/docker",
		"storage-driver":  "overlay2",
		"exec-opts":       []string{"native.cgroupdriver=cgroupfs"},
		"features":        map[string]bool{"containerd-snapshotter": false},
		"default-runtime": runtime,
		"hosts":           []string{"unix:///run/docker.sock"},
		"log-driver":      "local",
	}
	if c.Isolation.Installed {
		daemon["runtimes"] = map[string]any{"io.containerd.kata.v2": map[string]any{
			"runtimeType": "/opt/kata/bin/containerd-shim-kata-v2",
			"options":     map[string]string{"ConfigPath": "/run/kata-containers/configuration.toml"},
		}}
	}
	db, _ := json.MarshalIndent(daemon, "", "  ")
	files := map[string][]byte{"docker/daemon.json": append(db, '\n')}
	if c.Isolation.Installed {
		// This is an appliance-owned minimal CLH configuration, not a generic
		// replacement for arbitrary upstream Kata release configurations.
		files["kata-containers/configuration.toml"] = []byte(fmt.Sprintf(`[hypervisor.clh]
path = "/opt/kata/bin/cloud-hypervisor"
kernel = "/opt/kata/share/kata-containers/vmlinux.container"
image = "/opt/kata/share/kata-containers/kata-containers.img"
rootfs_type = "ext4"
disable_selinux = true
disable_guest_selinux = true
kernel_params = "cgroup_no_v1=all systemd.unified_cgroup_hierarchy=1"
default_vcpus = 1
default_memory = %d
shared_fs = "virtio-fs"
virtio_fs_daemon = "/usr/libexec/fractal-virtiofsd"
virtio_fs_extra_args = ["--thread-pool-size=1", "--announce-submounts"]
block_device_driver = "virtio-blk"
disable_image_nvdimm = true
enable_annotations = []

[agent.kata]

[runtime]
internetworking_model = "tcfilter"
disable_guest_seccomp = true
create_container_timeout = 60
`, c.Isolation.MemoryMiB))
	}
	var commands []Command
	for _, name := range Keys(c.Containers) {
		v := c.Containers[name]
		if v.Image == "" {
			continue
		}
		commands = append(commands, Command{
			Purpose: "create " + name + "; existing containers require explicit recreation",
			Argv:    []string{"docker", "create", "--name", name, "--runtime", c.EffectiveRuntime(name), v.Image},
		})
	}
	cb, _ := json.MarshalIndent(commands, "", "  ")
	files["containers.plan.json"] = append(cb, '\n')
	return files, nil
}

func (c Config) Export(dir string) error {
	files, err := c.Render()
	if err != nil {
		return err
	}
	for _, path := range Keys(files) {
		if err := AtomicWrite(filepath.Join(dir, path), files[path], 0600); err != nil {
			return err
		}
	}
	return nil
}
