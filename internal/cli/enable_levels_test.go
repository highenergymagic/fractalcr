package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestEnableSecretsAreIsolatedByLevel(t *testing.T) {
	c := config.Default()
	s := &Session{Running: c, Candidate: c.Clone()}
	for _, cmd := range []string{"configure terminal", "enable secret 0 fifteen-test", "enable secret level 7 0 seven-test", "enable secret level 3 three-test"} {
		if _, err := s.Execute(cmd); err != nil {
			t.Fatal(cmd, err)
		}
	}
	for level, password := range map[int]string{15: "fifteen-test", 7: "seven-test", 3: "three-test"} {
		if !config.CheckSecret(s.Running.AAA.EnableVerifier(level), password) {
			t.Fatalf("level %d", level)
		}
	}
	if s.Running.AAA.EnableVerifier(5) != "" {
		t.Fatal("unconfigured level inherited another secret")
	}
	if !strings.Contains(s.Running.CLI(), "enable secret level 7 9 $9$") {
		t.Fatal(s.Running.CLI())
	}
	if _, err := s.Execute("no enable secret level 7"); err != nil {
		t.Fatal(err)
	}
	if s.Running.AAA.EnableVerifier(7) != "" || !config.CheckSecret(s.Running.AAA.EnableVerifier(15), "fifteen-test") {
		t.Fatal("removing one level changed another")
	}
	if _, err := s.Execute("no enable secret level 15"); err != nil {
		t.Fatal(err)
	}
	if s.Running.AAA.EnableSecret != "" {
		t.Fatal("level 15 not removed")
	}
}
