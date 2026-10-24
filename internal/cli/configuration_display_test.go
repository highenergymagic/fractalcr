package cli

import (
	"slices"
	"strings"
	"testing"
)

func TestPrivilegeCatalogCoversConfigurationModes(t *testing.T) {
	for mode := range modeCommands {
		if !slices.Contains(privilegeModes, privilegeMode(mode)) {
			t.Fatalf("mode %s cannot be delegated", mode)
		}
	}
}

func TestDelegatedRunningConfigurationVisibility(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "hostname permitted", "username hidden privilege 15 secret hidden-password", "enable secret hidden-enable",
		"interface GigabitEthernet0", "description restricted-interface", "shutdown", "exit",
		"privilege exec level 5 show running-config", "privilege exec level 5 write terminal", "privilege configure level 5 hostname", "end")
	s.EnforcePrivilege, s.Privilege = true, 5
	for _, cmd := range []string{"show running-config", "write terminal"} {
		out, err := s.Execute(cmd)
		if err != nil || !strings.Contains(out, "hostname permitted") || strings.Contains(out, "username") || strings.Contains(out, "enable secret") || strings.Contains(out, "restricted-interface") || strings.Contains(out, "$9$") {
			t.Fatal(cmd, out, err)
		}
	}
	if out, err := s.Execute("write terminal | include username"); err != nil || out != "" {
		t.Fatal(out, err)
	}
	for _, cmd := range []string{"show running-config view full", "show running-config json"} {
		if _, err := s.Execute(cmd); err == nil {
			t.Fatal("delegated ordinary view exposed full configuration", cmd)
		}
	}
	s.Privilege = 15
	execute(t, s, "conf t", "privilege configure all level 5 interface", "end")
	s.Privilege = 5
	if out, err := s.Execute("show running-config"); err != nil || !strings.Contains(out, "restricted-interface") || strings.Contains(out, "username") {
		t.Fatal(out, err)
	}
	s.Privilege = 15
	execute(t, s, "conf t", "privilege exec level 5 show running-config view full", "end")
	s.Privilege = 5
	if out, err := s.Execute("show running-config view full"); err != nil || !strings.Contains(out, "username hidden") {
		t.Fatal(out, err)
	}
	if out, err := s.Execute("show running-config"); err != nil || strings.Contains(out, "username hidden") {
		t.Fatal("full-view grant changed ordinary-view filtering", out, err)
	}
}

func TestFilePrivilegeIsIndependentFromCommandGrant(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege exec all level 5 copy", "privilege exec all level 5 show version", "end")
	s.EnforcePrivilege, s.Privilege = true, 5
	commands := [][]string{{"copy", "running-config", "startup-config"}, {"show", "version", "|", "redirect", "data0:version"}}
	for _, words := range commands {
		if s.AllowedEXEC(words) {
			t.Fatal("file privilege bypass", words)
		}
	}
	s.Privilege = 15
	execute(t, s, "conf t", "file privilege 5", "end")
	s.Privilege = 5
	for _, words := range commands {
		if !s.AllowedEXEC(words) {
			t.Fatal("file access not granted", words)
		}
	}
	s.Privilege = 15
	execute(t, s, "conf t", "no file privilege 7", "end")
	s.Privilege = 5
	for _, words := range commands {
		if s.AllowedEXEC(words) {
			t.Fatal("file privilege reset ineffective", words)
		}
	}
}
