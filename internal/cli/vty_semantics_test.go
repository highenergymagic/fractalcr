package cli

import (
	"errors"
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestReportedVTYTranscript(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "hostname object", "user operator priv 15 passw 0 secret")
	if !config.CheckSecret(s.Running.AAA.Users["operator"].Secret, "secret") {
		t.Fatal("short password was not stored as a verifier")
	}
	for _, tc := range []struct {
		command, kind string
		column        int
	}{
		{"conf t", "invalid", 0}, {"linve vty 0", "invalid", 0}, {"line vty", "incomplete", 8},
		{"line vty 16", "invalid", 9}, {"line vty -1", "invalid", 9}, {"line vty +1", "invalid", 9},
		{"line vty 0 16", "invalid", 11}, {"line console 1", "invalid", 13},
	} {
		before := string(s.Running.JSON())
		_, err := s.Execute(tc.command)
		var syntax *SyntaxError
		if !errors.As(err, &syntax) || syntax.Kind != tc.kind || syntax.Column != tc.column {
			t.Fatalf("%q: %#v", tc.command, err)
		}
		if s.Mode != "config" || s.Prompt() != "object(config)# " || before != string(s.Running.JSON()) {
			t.Fatal("rejection changed configuration or mode")
		}
	}
	for _, prefix := range []string{"line vty ", "line vty 0 ", "line con "} {
		help := s.ContextHelp(prefix)
		if strings.Contains(help, "password") || strings.Contains(help, "cleartext") || !strings.Contains(help, "line number") {
			t.Fatalf("%q: %s", prefix, help)
		}
	}
	if help := s.ContextHelp("user operator priv 15 pass "); !strings.Contains(help, "cleartext") {
		t.Fatal(help)
	}
	execute(t, s, "line vty 0 4")
	if s.Prompt() != "object(config-line)# " || s.Object != "0 4" {
		t.Fatal(s.Prompt(), s.Object)
	}
	execute(t, s, "login local", "transport input ssh", "exec-timeout 2 30")
	for i := 0; i < 16; i++ {
		line := s.Running.AAA.Line(i)
		if i <= 4 && (!line.SSH || line.Timeout != 150) {
			t.Fatal(i, line)
		}
		if i > 4 && (line.SSH || line.Timeout != 600) {
			t.Fatal("range escaped", i, line)
		}
	}
	execute(t, s, "line vty 2", "exec-timeout 3", "transport input none")
	if s.Running.AAA.Line(2).Timeout != 180 || s.Running.AAA.Line(2).SSH || !s.Running.AAA.Line(1).SSH {
		t.Fatal("single-line settings leaked")
	}
	before := string(s.Running.JSON())
	if _, err := s.Execute("line vty 8 4"); err == nil || before != string(s.Running.JSON()) || s.Object != "2 2" {
		t.Fatal("reversed range changed selection")
	}
	execute(t, s, "end", "wr me")
	restored, err := New(s.StateDir)
	if err != nil || string(restored.Running.JSON()) != string(s.Running.JSON()) {
		t.Fatal("line configuration did not persist", err)
	}
}

func TestModePrecedenceAndParentTransition(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "network blue", "exit", "container web", "network blue")
	if s.Mode != "container" || s.Running.Containers["web"].Network != "blue" {
		t.Fatal("global network selector masked container network")
	}
	execute(t, s, "hostname changed")
	if s.Mode != "config" || s.Object != "" {
		t.Fatal("parent command did not leave submode")
	}
	execute(t, s, "line console 0")
	if _, err := s.Execute("transport input ssh"); err == nil {
		t.Fatal("VTY command leaked into console")
	}
	if help := s.ContextHelp(""); strings.Contains(help, "hostname") || strings.Contains(help, "transport") {
		t.Fatal("unrelated mode commands in help", help)
	}
	execute(t, s, "line vty 4")
	if _, err := s.QueryHelp("line nonsense "); err == nil {
		t.Fatal("invalid help silently ignored")
	}
}

func TestExplicitPasswordLengthPolicy(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "security passwords min-length 12")
	if _, err := s.Execute("username operator password 0 short"); err == nil || !strings.Contains(err.Error(), "configured minimum") {
		t.Fatal(err)
	}
	execute(t, s, "username operator privilege 15 secret 0 LongSyntheticPassword", "no security passwords min-length", "username operator password 0 short")
	if !config.CheckSecret(s.Running.AAA.Users["operator"].Secret, "short") {
		t.Fatal("removed policy still enforced")
	}
	if _, err := s.Execute("security passwords min-length 257"); err == nil {
		t.Fatal("unbounded policy accepted")
	}
}

func TestSharedPrefixParentCompletion(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "interface eth0")
	for _, tc := range []struct{ input, want string }{{"ip a", "ip address "}, {"ip s", "ip ssh "}, {"hostn", "hostname "}} {
		got, feedback := s.Complete(tc.input)
		if got != tc.want || feedback != "" {
			t.Fatalf("%q: %q %q", tc.input, got, feedback)
		}
	}
	if s.Mode != "interface" {
		t.Fatal("completion changed mode")
	}
	execute(t, s, "container web")
	if got, _ := s.Complete("no c"); got != "no c" {
		t.Fatal("parent hid local ambiguity", got)
	}
}
