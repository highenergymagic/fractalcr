package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestConfigurationImportIsStagedAndRoundTrips(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "hostname restored", "username admin privilege 15 secret 0 test-password",
		"interface Ethernet0", `description two  spaces "quoted"`, "exit", "line con 0", "history size 35", "end")
	original := string(s.Running.JSON())
	called := false
	s.Apply = func(config.Config, config.Config) error { called = true; return nil }
	s.Save = func(config.Config) error { called = true; return nil }
	s.Platform = func([]string, config.Config) (string, error) { called = true; return "", nil }
	for _, data := range [][]byte{[]byte(s.Running.CLI()), s.Running.JSON()} {
		got, err := s.ParseConfiguration(data, config.Default())
		if err != nil || got.CLI() != s.Running.CLI() {
			t.Fatalf("roundtrip: %v\n%s", err, got.CLI())
		}
	}
	got, err := s.ParseConfiguration([]byte("! merge\nhostname merged\nend\n"), s.Running)
	if err != nil || got.Hostname != "merged" || len(got.AAA.Users) != 1 {
		t.Fatalf("merge: %+v %v", got, err)
	}
	if called || string(s.Running.JSON()) != original {
		t.Fatal("parsing touched caller or platform")
	}
	for _, text := range []string{"hostname safe\ndo reload", "hostname safe\ncommit", "abort", "exit\nwrite memory", "end\nhostname after", "hostname safe\nrequest platform debug service-container enable", "hostname safe\ninvalid-command", strings.Repeat("x", config.MaxConfigBytes+1)} {
		if _, err := s.ParseConfiguration([]byte(text), s.Running); err == nil {
			t.Fatalf("accepted %q", text[:min(80, len(text))])
		}
		if called || string(s.Running.JSON()) != original {
			t.Fatal("failed parse changed state")
		}
	}
}

func TestConfigurationImportUsesOriginalAuthorization(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege configure level 5 hostname", "end")
	s.EnforcePrivilege, s.Privilege = true, 5
	if _, err := s.ParseConfiguration([]byte("hostname allowed\nend\n"), s.Running); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"hostname allowed\nusername attacker privilege 15 secret 0 password", "privilege configure all level 5 username\nusername attacker privilege 15 secret 0 password"} {
		if _, err := s.ParseConfiguration([]byte(text), s.Running); err == nil {
			t.Fatal("import escaped delegated policy")
		}
	}
}

func TestConfigurationCopyAliasHelp(t *testing.T) {
	s := session(t)
	s.FileSystems = func() []string { return []string{"data0:", "nvram:"} }
	for _, input := range []string{"copy ", "copy data0:backup "} {
		if help := s.ContextHelp(input); !strings.Contains(help, "system:") {
			t.Fatal(help)
		}
	}
}
