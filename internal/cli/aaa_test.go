package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestIOSLocalPasswordForms(t *testing.T) {
	const password = "TypedPassword-123!"
	for _, command := range []string{
		"user freya priv 15 password 0 ",
		"username freya privilege 15 password ",
		"user freya priv 15 sec 0 ",
		"username freya privilege 15 secret ",
	} {
		t.Run(command, func(t *testing.T) {
			s := session(t)
			execute(t, s, "conf t", command+password, "commit")
			u := s.Running.AAA.Users["freya"]
			if u.Privilege != 15 || !config.CheckSecret(u.Secret, password) || strings.Contains(string(s.Running.JSON()), password) {
				t.Fatal("password did not authenticate at the requested privilege or leaked into config")
			}
			execute(t, s, "user freya priv 7", "commit")
			if s.Running.AAA.Users["freya"].Secret != u.Secret || s.Running.AAA.Users["freya"].Privilege != 7 {
				t.Fatal("privilege update changed password")
			}
		})
	}
}

func TestIOSPasswordDefaultsAndInputTypes(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "user freya password 0 multiple word password", "enable secret 0 privileged password")
	if u := s.Candidate.AAA.Users["freya"]; u.Privilege != 1 || !config.CheckSecret(u.Secret, "multiple word password") {
		t.Fatal("default privilege or password words lost")
	}
	if !config.CheckSecret(s.Candidate.AAA.EnableSecret, "privileged password") {
		t.Fatal("enable secret type 0 failed")
	}
	before := string(s.Candidate.JSON())
	for _, line := range []string{"user freya priv 15 password 0", "user freya password 7 0822455D0A16", "user freya secret 9 CiscoEncodedSecret", "enable secret 5 CiscoEncodedSecret", "user newuser priv 15"} {
		if _, err := s.Execute(line); err == nil {
			t.Fatalf("accepted %q", line)
		}
		if string(s.Candidate.JSON()) != before {
			t.Fatalf("failed command %q changed configuration", line)
		}
	}
	for _, line := range []string{"user freya priv 15 ", "username freya "} {
		help := s.ContextHelp(line)
		if !strings.Contains(help, "password") || !strings.Contains(help, "secret") {
			t.Fatalf("missing password choices: %s", help)
		}
	}
	if help := s.ContextHelp("user freya priv 15 password "); !strings.Contains(help, "cleartext") || !strings.Contains(help, "<SECRET...>") {
		t.Fatal(help)
	}
}

func TestNumericPasswordAndReadOnlyTimeStatus(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "user freya password 1234567890123456", "commit", "end")
	if !config.CheckSecret(s.Running.AAA.Users["freya"].Secret, "1234567890123456") {
		t.Fatal("numeric plaintext mistaken for an encoding selector")
	}
	s.EnforcePrivilege, s.Privilege = true, 1
	for _, command := range []string{"show clock", "show ntp status", "show ntp associations"} {
		if _, _, err := s.Resolve(command); err != nil {
			t.Fatalf("read-only time status unavailable: %s: %v", command, err)
		}
	}
}

func TestPrivilegeGrammarAndCandidateIsolation(t *testing.T) {
	s := session(t)
	s.EnforcePrivilege = true
	s.Privilege = 1
	if _, _, e := s.Resolve("conf t"); e == nil {
		t.Fatal("configuration exposed")
	}
	if strings.Contains(s.ContextHelp("show "), "running-config") {
		t.Fatal("privileged help exposed")
	}
	if s.Prompt() != "ctr> " {
		t.Fatal(s.Prompt())
	}
	s.Privilege = 15
	execute(t, s, "configure candidate", "username admin privilege 15 secret administrator-password", "aaa new-model", "line console 0", "exec-timeout 5 30", "exit", "commit")
	if !s.Running.AAA.Enabled || s.Running.AAA.ConsoleTimeout != 330 {
		t.Fatal(s.Running.AAA)
	}
	s.Running.Hostname = "changed-elsewhere"
	if _, e := s.Execute("commit"); e == nil {
		t.Fatal("stale candidate overwritten")
	}
}

func TestDisableNeverRaisesPrivilege(t *testing.T) {
	for _, level := range []int{0, 1, 7, 15} {
		s := session(t)
		s.EnforcePrivilege, s.Privilege = true, level
		if _, err := s.Execute("disable"); err != nil {
			t.Fatal(err)
		}
		if s.Privilege > level || s.Privilege > 1 {
			t.Fatalf("disable raised or retained privilege %d from %d", s.Privilege, level)
		}
	}
}
