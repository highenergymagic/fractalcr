package appliance

import (
	"fractalcr/internal/cli"
	"fractalcr/internal/config"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupExcludesBootAndPartitionedDisks(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"sda", "vda", "nvme0n1", "sdb"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(path, "holders"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "ro"), []byte("0\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for disk, part := range map[string]string{"sda": "sda1", "nvme0n1": "nvme0n1p1"} {
		path := filepath.Join(root, disk, part)
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "partition"), []byte("1\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sdb", "ro"), []byte("1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	eligible := setupDisks(root)
	if len(eligible) != 1 || eligible[0] != "vda" {
		t.Fatal(eligible)
	}
	c := config.Default()
	s := newSetup(c)
	s.stage = "disk"
	s.disks = func() []string { return setupDisks(root) }
	session := &cli.Session{Running: c, Candidate: c.Clone()}
	if r := s.answer("sda", session); r.Error == "" || s.stage != "disk" {
		t.Fatal(r)
	}
	if r := s.answer("vda", session); r.Error != "" || s.stage != "erase" {
		t.Fatal(r)
	}
}

func TestSetupStagesAndCancel(t *testing.T) {
	old := checkDatastore
	checkDatastore = func() error { return nil }
	defer func() { checkDatastore = old }()
	c := config.Default()
	c.AAA.PasswordMinLength = 12
	session := &cli.Session{Running: c, Candidate: c.Clone()}
	s := newSetup(c)
	s.ports = func() []platformPort {
		return []platformPort{{Interface: net.Interface{Name: "test-physical0", Index: 10}, Name: "GigabitEthernet0"}}
	}
	nic := "GigabitEthernet0"
	for _, v := range []string{"yes", "steel", nic, "dhcp", "time.example.org", "admin"} {
		if v == nic {
			if r := s.answer("Vlan100", session); r.Error == "" || s.stage != "interface" {
				t.Fatal("accepted an interface outside the physical inventory")
			}
		}
		r := s.answer(v, session)
		if r.Error != "" || !r.Challenge {
			t.Fatal(r)
		}
	}
	if !s.prompt().Secret {
		t.Fatal("password would be echoed")
	}
	if s.nic != "GigabitEthernet0" {
		t.Fatal("setup did not persist the appliance interface name")
	}
	if r := s.answer("short", session); r.Error == "" {
		t.Fatal("accepted weak-length secret")
	}
	if r := s.answer("test-password-long", session); r.Error != "" {
		t.Fatal(r)
	}
	if s.stage != "confirm" || !s.candidate.AAA.Enabled || !s.candidate.AAA.SSH {
		t.Fatal(s)
	}
	if !config.CheckSecret(s.candidate.AAA.Users["admin"].Secret, "test-password-long") {
		t.Fatal("password hash")
	}
	r := s.answer("no", session)
	if r.Challenge || session.Running.Hostname != "ctr" {
		t.Fatal("cancel applied setup")
	}
}
