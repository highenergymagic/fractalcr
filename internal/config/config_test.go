package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMigration(t *testing.T) {
	c, e := Decode([]byte(`{"version":1,"hostname":"router","network":{"interface":"eth0","address":"10.0.2.15/24"}}`))
	if e != nil || c.Isolation.Runtime != "clh" {
		t.Fatal(c, e)
	}
}
func TestRejectInvalid(t *testing.T) {
	for _, doc := range []string{`{"version":2,"hostname":"x"}`, `{"version":1,"hostname":"x","unknown":true}`, `{"version":1,"hostname":"x"} garbage`, `{"version":1,"hostname":"x","network":{"interface":"eth0","address":"bad"}}`, `{"version":1,"hostname":"x","containers":null}`} {
		if _, e := Decode([]byte(doc)); e == nil {
			t.Fatalf("accepted %s", doc)
		}
	}
}

func TestRequiredAndDuplicateFields(t *testing.T) {
	for _, doc := range []string{`{}`, `{"hostname":"x"}`, `{"version":1,"hostname":"x","hostname":"y"}`, `{"version":1,"hostname":"x","network":{"interface":"eth0","address":"192.0.2.1/24","address":"192.0.2.2/24"}}`} {
		if _, err := Decode([]byte(doc)); err == nil {
			t.Fatalf("accepted %s", doc)
		}
	}
}
func TestDaemonConfig(t *testing.T) {
	c := Default()
	c.Isolation.Installed = true
	c.Isolation.Enabled = true
	c.Isolation.ApplyTo = "all-containers"
	files, e := c.Render()
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]any
	json.Unmarshal(files["docker/daemon.json"], &d)
	if d["default-runtime"] != "io.containerd.kata.v2" || d["data-root"] != "/datastore/data0/docker" {
		t.Fatal(d)
	}
}
func TestBoundedLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oversize")
	if e := os.WriteFile(p, make([]byte, MaxConfigBytes+1), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(p); e == nil {
		t.Fatal("accepted oversized config")
	}
}
