package appliance

import (
	"fractalcr/internal/cli"
	"fractalcr/internal/config"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupOffersPartitionedNonUSBDiskWithExplicitErase(t *testing.T) {
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
	usb := filepath.Join(t.TempDir(), "usb", "sdc")
	if err := os.MkdirAll(filepath.Join(usb, "holders"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usb, "ro"), []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(usb, filepath.Join(root, "sdc")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sdb", "ro"), []byte("1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	eligible := setupDisks(root)
	if len(eligible) != 3 || eligible[0] != "nvme0n1" || eligible[1] != "sda" || eligible[2] != "vda" {
		t.Fatal(eligible)
	}
	status := setupDiskStatus(root)
	if status["sda"] != "requires erase (partitions: sda1)" || status["sdb"] != "read-only" || status["sdc"] != "USB media (not available for data0)" {
		t.Fatal(status)
	}
	inventory, err := diskInventory(root)
	if err != nil || !strings.Contains(inventory, "sda1") || !strings.Contains(inventory, "USB media") {
		t.Fatal(inventory, err)
	}
	c := config.Default()
	s := newSetup(c)
	s.stage = "disk"
	s.disks = func() []string { return setupDisks(root) }
	s.diskStatus = func() map[string]string { return setupDiskStatus(root) }
	session := &cli.Session{Running: c, Candidate: c.Clone()}
	if r := s.answer("sdb", session); r.Error == "" || s.stage != "disk" {
		t.Fatal(r)
	}
	if r := s.answer("sda,sda", session); r.Error == "" || s.stage != "disk" {
		t.Fatal(r)
	}
	if r := s.answer("sda,vda", session); r.Error != "" || s.stage != "profile" {
		t.Fatal(r)
	}
	if r := s.answer("raid1", session); r.Error != "" || s.stage != "erase" {
		t.Fatal(r)
	}
	if r := s.answer("ERASE sda", session); r.Error == "" || s.stage != "erase" {
		t.Fatal(r)
	}
	if s.prompt().Prompt != "Type ERASE sda,vda to destroy selected disk contents and initialize data0: " {
		t.Fatal(s.prompt())
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
