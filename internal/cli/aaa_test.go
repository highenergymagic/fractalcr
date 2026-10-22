package cli

import (
	"strings"
	"testing"
)

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
	execute(t, s, "conf t", "username admin privilege 15 secret administrator-password", "aaa new-model", "line console 0", "exec-timeout 5 30", "exit", "commit")
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
