package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFactoryRequiresBlockStorageAndIsolation(t *testing.T) {
	c := Default()
	c.Isolation.VMFactory = true
	if c.Validate() == nil {
		t.Fatal("accepted factory without prerequisites")
	}
	c.Isolation.Installed = true
	c.Isolation.Enabled = true
	c.Engine.StorageBackend = "blockfile"
	files, err := c.Render()
	if err != nil {
		t.Fatal(err)
	}
	var daemon map[string]any
	if err := json.Unmarshal(files["docker/daemon.json"], &daemon); err != nil {
		t.Fatal(err)
	}
	if daemon["storage-driver"] != "blockfile" || daemon["containerd"] != "/run/fractal-containerd/containerd.sock" {
		t.Fatal(daemon)
	}
	profile := string(files["kata-containers/configuration.toml"])
	if !strings.Contains(profile, `shared_fs = "none"`) || !strings.Contains(profile, "enable_template = true") {
		t.Fatal(profile)
	}
	c.Isolation.VMFactory = false
	files, err = c.Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["kata-containers/configuration.toml"]), `shared_fs = "virtio-fs"`) {
		t.Fatal("cold-start sharing lost")
	}
}
