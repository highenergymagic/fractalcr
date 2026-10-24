package cli

import (
	"fractalcr/internal/config"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandPrivilegeDelegationAndReplay(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege exec level 5 configure terminal", "privilege configure level 5 hostname", "end")
	s.EnforcePrivilege, s.Privilege = true, 5
	execute(t, s, "conf t", "hostname delegated", "end")
	if s.Running.Hostname != "delegated" {
		t.Fatal(s.Running.Hostname)
	}
	execute(t, s, "conf t")
	for _, cmd := range []string{"username attacker privilege 15 password example", "enable secret example", "privilege exec all level 0 show", "no aaa new-model"} {
		if _, err := s.Execute(cmd); err == nil {
			t.Fatalf("delegation escaped scope: %s", cmd)
		}
	}
	if help := s.ContextHelp(""); !strings.Contains(help, "hostname") || strings.Contains(help, "username") {
		t.Fatal(help)
	}
	copy := session(t)
	execute(t, copy, "conf t")
	for _, line := range s.Running.AAA.PrivilegeCLI() {
		execute(t, copy, line)
	}
	if string(copy.Running.JSON()) == "" {
		t.Fatal("missing configuration")
	}
	execute(t, copy, "end")
	copy.EnforcePrivilege, copy.Privilege = true, 5
	execute(t, copy, "conf t", "hostname replayed")
	if _, err := copy.Execute("username attacker privilege 15 password example"); err == nil {
		t.Fatal("replay broadened authority")
	}
	path := filepath.Join(t.TempDir(), "startup-config.json")
	if err := config.AtomicWrite(path, s.Running.JSON(), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Session{Running: loaded, EnforcePrivilege: true, Privilege: 5}
	execute(t, restarted, "conf t", "hostname restarted")
	if _, err := restarted.Execute("username attacker privilege 15 password example"); err == nil {
		t.Fatal("persisted policy broadened authority")
	}
}

func TestCommandPrivilegeParentsResetAndZero(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege exec level 7 show ip route", "end")
	s.EnforcePrivilege, s.Privilege = true, 1
	if _, _, err := s.Resolve("show version"); err == nil {
		t.Fatal("implicit show parent not raised")
	}
	s.Privilege = 15
	execute(t, s, "conf t", "privilege exec level 1 show", "end")
	s.Privilege = 1
	if _, _, err := s.Resolve("sh ver"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Resolve("sh ip route"); err == nil {
		t.Fatal("IP parent override lost")
	}
	s.Privilege = 15
	execute(t, s, "conf t", "no privilege exec level 7 show ip route", "privilege exec level 0 show users", "end")
	s.Privilege = 0
	if _, _, err := s.Resolve("show users"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Resolve("show version"); err == nil {
		t.Fatal("lowering a parent silently lowered a sibling")
	}
	if _, _, err := s.Resolve("show running-config"); err == nil {
		t.Fatal("level zero acquired privileged display")
	}
	s.Privilege = 15
	execute(t, s, "conf t", "privilege exec reset show users", "end")
	if len(s.Running.AAA.CommandPrivileges) != 0 {
		t.Fatal(s.Running.AAA.CommandPrivileges)
	}
}

func TestPrivilegeAllIncludesSubmodesAndRevokesLive(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege exec level 5 configure terminal", "privilege configure all level 5 interface", "end")
	s.EnforcePrivilege, s.Privilege = true, 5
	execute(t, s, "conf t", "interface GigabitEthernet0", "shutdown")
	if !s.Running.AllInterfaces()["GigabitEthernet0"].Shutdown {
		t.Fatal("submode not delegated")
	}
	admin := *s
	admin.Mode = "config"
	admin.Privilege = 15
	execute(t, &admin, "privilege configure all reset interface")
	s.Running = admin.Running.Clone()
	if _, err := s.Execute("no shutdown"); err == nil {
		t.Fatal("revoked submode still writable")
	}
	execute(t, s, "end")
}

func TestPrivilegeHelpCompletionAndInvalidTargets(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t")
	if help := s.ContextHelp("privilege "); !strings.Contains(help, "Global configuration mode") || strings.Contains(help, "Configure exec") {
		t.Fatal(help)
	}
	if help := s.ContextHelp("privilege exec level 5 "); strings.Contains(help, "<cr>") {
		t.Fatal("missing target offered as executable", help)
	}
	if help := s.ContextHelp("priv exec level 5 sh "); !strings.Contains(help, "version") || strings.Contains(help, "<COMMAND...>") {
		t.Fatal(help)
	}
	if help := s.ContextHelp("priv exec level 5 show ip "); !strings.Contains(help, "<cr>") {
		t.Fatal(help)
	}
	if line, bell := s.Complete("priv exec level 5 sh"); line != "priv exec level 5 show " || bell != "" {
		t.Fatal(line, bell)
	}
	execute(t, s, "priv exec level 5 sh ver")
	for _, cmd := range []string{"privilege exec level 5 sh nonexistent", "privilege exec level 5 sho ip nonsense", "privilege exec level 5 request compose arbitrary-tail"} {
		before := string(s.Running.JSON())
		if _, err := s.Execute(cmd); err == nil || string(s.Running.JSON()) != before {
			t.Fatal(cmd, err)
		}
	}
	line := "privilege exec level 5 show nonexistent"
	if _, _, err := s.Resolve(line); err == nil {
		t.Fatal("invalid target accepted")
	} else if syntax, ok := err.(*SyntaxError); !ok || syntax.Column != strings.Index(line, "nonexistent") {
		t.Fatal(err)
	}
}

func TestNativeCommandPrivilegeUsesParsedOperation(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "privilege exec level 5 request container-engine events", "end")
	s.EnforcePrivilege, s.Privilege = true, 5
	for _, cmd := range []string{"request container-engine events", "request container-engine debug enable events"} {
		if _, _, err := s.Resolve(cmd); err != nil {
			t.Fatal(cmd, err)
		}
	}
	for _, cmd := range []string{"request container-engine container rm argument victim", "request container-engine debug enable container rm argument victim", "request compose up"} {
		if _, _, err := s.Resolve(cmd); err == nil {
			t.Fatal("native sibling escaped policy", cmd)
		}
	}
	if help := s.ContextHelp("request container-engine "); !strings.Contains(help, "events") || strings.Contains(help, "prune") || strings.Contains(help, "container ") {
		t.Fatal(help)
	}
	s.Privilege = 15
	execute(t, s, "conf t", "privilege exec reset request container-engine events", "end")
	s.Privilege = 5
	if s.AllowedEXEC([]string{"request", "container-engine", "events"}) {
		t.Fatal("native operation retained revoked grant")
	}
}
