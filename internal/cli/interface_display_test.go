package cli

import (
	"strings"
	"testing"

	"fractalcr/internal/config"
)

func TestInterfaceShowSelectorsReachPlatform(t *testing.T) {
	s := session(t)
	s.EnforcePrivilege, s.Privilege = true, 1
	s.InterfaceNames = func() []string { return []string{"GigabitEthernet0", "GigabitEthernet1"} }
	var calls []string
	s.Platform = func(words []string, _ config.Config) (string, error) {
		calls = append(calls, strings.Join(words, " "))
		return "interface output", nil
	}
	for _, command := range []string{"sh int gi0", "sh int description", "show interfaces gi0 description", "sh ip int gi0 brief"} {
		if out, err := s.Execute(command); err != nil || out != "interface output" {
			t.Fatal(command, out, err)
		}
	}
	if len(calls) != 4 || calls[0] != "show interfaces gi0" || calls[3] != "show ip interface gi0 brief" {
		t.Fatal(calls)
	}
	help := s.ContextHelp("show interfaces ")
	if !strings.Contains(help, "GigabitEthernet0") || !strings.Contains(help, "description") {
		t.Fatal(help)
	}
	if _, err := s.Execute("show interfaces gi0 description extra"); err == nil {
		t.Fatal("extra argument accepted")
	}
	s.Privilege = 0
	if _, err := s.Execute("show interfaces gi0"); err == nil {
		t.Fatal("interface display exposed at privilege zero")
	}
}
