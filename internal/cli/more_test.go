package cli

import (
	"strings"
	"testing"

	"fractalcr/internal/config"
)

func TestMoreConfigurationAliasesAndPolicy(t *testing.T) {
	s := session(t)
	s.FileSystems = func() []string { return []string{"data0:", "nvram:"} }
	for _, command := range []string{"more ", "more /ascii "} {
		help, err := s.QueryHelp(command)
		if err != nil || !strings.Contains(help, "system:") || !strings.Contains(help, "data0:") {
			t.Fatal(help, err)
		}
	}
	if line, _ := s.Complete("more sys"); line != "more system: " {
		t.Fatal("filesystem completion", line)
	}
	for _, command := range []string{"conf t", "hostname filetest", "end", "write memory"} {
		if _, err := s.Execute(command); err != nil {
			t.Fatal(command, err)
		}
	}
	for _, pair := range [][2]string{{"more system:running-config", "show running-config"}, {"more nvram:startup-config", "show startup-config"}, {"more /ascii system:/running-config", "show running-config"}} {
		got, err := s.Execute(pair[0])
		want, e := s.Execute(pair[1])
		if err != nil || e != nil || got != want {
			t.Fatal(pair, got, err, want, e)
		}
	}
	got, err := s.Execute("more system:running-config | include hostname")
	if err != nil || strings.TrimSpace(got) != "hostname filetest" {
		t.Fatal(got, err)
	}
	s.Platform = func(words []string, _ config.Config) (string, error) { return strings.Join(words, " "), nil }
	got, err = s.Execute("more /asc data0:file")
	if err != nil || got != "more /ascii data0:file" {
		t.Fatal(got, err)
	}
	if _, err := s.Execute("conf t"); err != nil {
		t.Fatal(err)
	}
	got, err = s.Execute("do more data0:file")
	if err != nil || got != "more data0:file" {
		t.Fatal(got, err)
	}
	s.Mode, s.EnforcePrivilege, s.Privilege = "", true, 1
	if _, err := s.Execute("more system:running-config"); err == nil {
		t.Fatal("unprivileged read")
	}
	s.Privilege = 15
	execute(t, s, "conf t", "username hidden secret hidden-password", "file privilege 5", "privilege exec level 5 more", "privilege configure level 5 hostname", "end")
	s.Privilege = 5
	got, err = s.Execute("more system:running-config")
	if err != nil || !strings.Contains(got, "hostname filetest") || strings.Contains(got, "username hidden") {
		t.Fatal("alias visibility", got, err)
	}
	s.Running.AAA.FilePrivilege = nil
	if _, err := s.Execute("more system:running-config"); err == nil {
		t.Fatal("bypassed file access level")
	}
}
