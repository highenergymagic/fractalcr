package cli

import (
	"errors"
	"strings"
	"testing"

	"fractalcr/internal/config"
)

func TestIOSRunningAndStartupWorkflow(t *testing.T) {
	s := session(t)
	password := "synthetic-test-password"
	applies := 0
	s.Apply = func(old, next config.Config) error { applies++; return nil }
	execute(t, s, "conf t", "user testadmin priv 15 pass 0 "+password,
		"line con 0", "login local", "end", "wr me", "conf t", "hostn thething")
	if s.Prompt() != "thething(config)# " || !s.Running.AAA.Enabled || applies != 3 {
		t.Fatalf("live configuration: %s, applies=%d", s.Prompt(), applies)
	}
	// Running changes are immediate, but only an explicit save changes startup.
	boot, err := New(s.StateDir)
	if err != nil || boot.Running.Hostname != "ctr" || !boot.Running.AAA.Enabled {
		t.Fatalf("startup changed before save: %v", err)
	}
	execute(t, s, "end", "wr me")
	boot, err = New(s.StateDir)
	if err != nil || boot.Running.Hostname != "thething" || !config.CheckSecret(boot.Running.AAA.Users["testadmin"].Secret, password) {
		t.Fatalf("restart did not retain configuration: %v", err)
	}
	if strings.Contains(string(boot.Running.JSON()), password) {
		t.Fatal("plaintext persisted")
	}
}

func TestLiveConfigurationFailureAndOtherSessions(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t")
	s.Apply = func(old, next config.Config) error { return errors.New("platform rejected") }
	if _, err := s.Execute("hostname rejected"); err == nil || s.Running.Hostname != "ctr" || s.Candidate.Hostname != "ctr" {
		t.Fatal("failed command changed configuration")
	}
	s.Apply = nil
	if _, err := s.Execute("aaa new-model"); err == nil || s.Running.AAA.Enabled || s.Candidate.AAA.Enabled {
		t.Fatal("invalid AAA edit was not rolled back")
	}
	// Management refreshes Running before each command; another session's
	// successful edit must survive this session's next command.
	s.Running.Hostname = "other-session"
	execute(t, s, "ntp server time.example.org", "end")
	if s.Running.Hostname != "other-session" {
		t.Fatal("lost concurrent update")
	}
	s.Save = func(config.Config) error { return errors.New("datastore unavailable") }
	output, err := s.Execute("wr me")
	if err == nil || output != "" {
		t.Fatal("failed save reported success")
	}
}

func TestExplicitTransactionCannotSilentlyLoseChanges(t *testing.T) {
	s := session(t)
	execute(t, s, "configure candidate", "hostname pending")
	for _, command := range []string{"end", "exit", "do write memory"} {
		if _, err := s.Execute(command); err == nil || s.Mode != "config" || s.Running.Hostname != "ctr" {
			t.Fatalf("%s silently abandoned or saved the transaction", command)
		}
	}
	execute(t, s, "commit", "end", "wr me")
	if s.Running.Hostname != "pending" {
		t.Fatal("explicit commit did not apply")
	}
}
