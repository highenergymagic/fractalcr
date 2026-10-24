package appliance

import (
	"fractalcr/internal/cli"
	"fractalcr/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func delegateCommand(t *testing.T, s *loginSession, commands ...string) {
	t.Helper()
	admin := *s.cli
	admin.Mode = "config"
	admin.Privilege = 15
	admin.EnforcePrivilege = true
	for _, cmd := range commands {
		if _, err := admin.Execute(cmd); err != nil {
			t.Fatal(cmd, err)
		}
	}
	s.cli.Running = admin.Running.Clone()
}

func TestDelegatedFileConfirmationRevokesOnPolicyChange(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root := t.TempDir()
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	s := &loginSession{cli: &cli.Session{Running: config.Default(), Privilege: 5, EnforcePrivilege: true}}
	delegateCommand(t, s, "file privilege 5", "privilege exec level 5 delete")
	os.WriteFile(filepath.Join(root, "keep"), []byte("unchanged"), 0600)
	r := s.beginFile([]string{"delete", "data0:keep"})
	if !r.Challenge {
		t.Fatal(r)
	}
	delegateCommand(t, s, "privilege exec reset delete")
	if r = s.answerFile(""); r.Error != "Authorization failed." || s.pendingFile != nil {
		t.Fatal(r)
	}
	if _, err := os.Stat(filepath.Join(root, "keep")); err != nil {
		t.Fatal(err)
	}
	delegateCommand(t, s, "privilege exec level 5 delete")
	s.beginFile([]string{"delete", "data0:keep"})
	if r = s.answerFile(""); r.Error != "" {
		t.Fatal(r)
	}
	if _, err := os.Stat(filepath.Join(root, "keep")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestDelegatedCopySaveAndRevocation(t *testing.T) {
	saved := 0
	s := &loginSession{cli: &cli.Session{Running: config.Default(), Privilege: 5, EnforcePrivilege: true, Save: func(config.Config) error { saved++; return nil }}}
	delegateCommand(t, s, "file privilege 5", "privilege exec all level 5 copy")
	s.beginCopy([]string{"copy", "running-config", "startup-config"})
	if r := s.answerCopy(""); r.Error != "" || saved != 1 {
		t.Fatal(r, saved)
	}
	s.beginCopy([]string{"copy", "running-config", "startup-config"})
	delegateCommand(t, s, "privilege exec all reset copy")
	if r := s.answerCopy(""); r.Error != "Authorization failed." || saved != 1 {
		t.Fatal(r, saved)
	}
}

func TestReloadAndOutputRecheckCommandPolicy(t *testing.T) {
	s := &loginSession{cli: &cli.Session{Running: config.Default(), Privilege: 5, EnforcePrivilege: true}}
	delegateCommand(t, s, "file privilege 5", "privilege exec all level 5 reload", "privilege exec all level 5 show version")
	words := []string{"reload", "in", "5"}
	if !s.cli.AllowedEXEC(words) {
		t.Fatal("reload not delegated")
	}
	s.pendingReload = &reloadDialog{words: words}
	s.outputCommand = []string{"show", "version", "|", "redirect", "data0:version"}
	if !s.cli.AllowedEXEC(s.outputCommand) {
		t.Fatal("output not delegated")
	}
	delegateCommand(t, s, "privilege exec all reset reload", "privilege exec all reset show version")
	if r := s.answerReload("", nil, time.Now()); r.Error != "Authorization failed." {
		t.Fatal(r)
	}
	if _, err := s.writeOutput(&cli.Filter{Mode: "redirect", Pattern: "data0:version"}, "text"); err == nil {
		t.Fatal("output authorization retained")
	}
}

func TestCopySaveRejectsDirtyTransactionAndRevokedFileAccess(t *testing.T) {
	saved := false
	c := config.Default()
	s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), Privilege: 15, EnforcePrivilege: true, Transactional: true, Save: func(config.Config) error { saved = true; return nil }}}
	s.cli.Candidate.Hostname = "uncommitted"
	s.beginCopy([]string{"copy", "running-config", "startup-config"})
	if r := s.answerCopy(""); r.Error != "uncommitted transaction; commit before saving" || saved {
		t.Fatal(r, saved)
	}
	s.cli.Transactional = false
	delegateCommand(t, s, "file privilege 5", "privilege exec all level 5 copy")
	s.cli.Privilege = 5
	s.beginCopy([]string{"copy", "running-config", "startup-config"})
	delegateCommand(t, s, "no file privilege")
	if r := s.answerCopy(""); r.Error != "Authorization failed." || saved {
		t.Fatal(r, saved)
	}
}

func TestNativeStreamGrantRevocationAndIdleDeadline(t *testing.T) {
	now := time.Now()
	c := config.Default()
	s := &loginSession{cli: &cli.Session{Running: c, Privilege: 5, EnforcePrivilege: true}, authenticated: true, bootstrap: true, transport: "console", last: now}
	delegateCommand(t, s, "privilege exec level 5 request container-engine events")
	s.streamCommand = []string{"request", "container-engine", "events"}
	c = s.cli.Running.Clone()
	request := managementRequest{Action: "stream-check"}
	if r, handled := s.authenticate(request, c, now.Add(time.Second)); !handled || !r.Stream {
		t.Fatal(r, handled)
	}
	s.commandCompleted(request, now.Add(time.Second))
	if s.last != now {
		t.Fatal("stream heartbeat renewed idle deadline")
	}
	delegateCommand(t, s, "privilege exec reset request container-engine events")
	if r, _ := s.authenticate(request, s.cli.Running, now.Add(2*time.Second)); r.Stream || r.Error != "Authorization failed." || s.streamCommand != nil {
		t.Fatal(r)
	}
	delegateCommand(t, s, "privilege exec level 5 request container-engine events")
	s.streamCommand = []string{"request", "container-engine", "events"}
	if r, _ := s.authenticate(managementRequest{Action: "stream-end"}, s.cli.Running, now.Add(3*time.Second)); !r.Stream || s.streamCommand != nil {
		t.Fatal(r)
	}
	if r, _ := s.authenticate(request, s.cli.Running, now.Add(4*time.Second)); r.Stream {
		t.Fatal("completed grant reused", r)
	}
}
