package appliance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fractalcr/internal/cli"
	"fractalcr/internal/config"
)

func TestIOSEnableConsoleAndVTY(t *testing.T) {
	c := config.Default()
	for _, transport := range []string{"console", "ssh"} {
		s := &loginSession{transport: transport, cli: &cli.Session{Running: c, Privilege: 1, EnforcePrivilege: true}}
		r := s.enable(15, c)
		if transport == "console" {
			if s.cli.Privilege != 15 || r.Challenge || r.Error != "" {
				t.Fatal(r)
			}
			if _, err := s.cli.Execute("disable 5"); err != nil || s.cli.Privilege != 5 {
				t.Fatal(err, s.cli.Privilege)
			}
			if _, err := s.cli.Execute("disable 15"); err == nil {
				t.Fatal("disable raised privilege")
			}
			if _, err := s.cli.Execute("disable"); err != nil || s.cli.Privilege != 1 {
				t.Fatal(err)
			}
		} else if s.cli.Privilege != 1 || r.Error != "No password set" {
			t.Fatal(r)
		}
	}
	s := &loginSession{transport: "console", cli: &cli.Session{Running: c, Privilege: 1}}
	c.AAA.EnableSecret = "configured"
	if r := s.enable(7, c); !r.Secret || s.enableLevel != 7 || s.cli.Privilege != 1 {
		t.Fatal(r)
	}
}

func TestIOSReloadTimes(t *testing.T) {
	now := time.Date(2026, 10, 24, 18, 33, 0, 0, time.UTC)
	for _, tt := range []struct {
		command string
		after   time.Duration
	}{
		{"reload in 10 maintenance", 10 * time.Minute},
		{"reload in 1:05 maintenance", 65 * time.Minute},
		{"reload at 19:00", 27 * time.Minute},
		{"reload at 18:00", 23*time.Hour + 27*time.Minute},
		{"reload at 19:00 oct 25 maintenance", 24*time.Hour + 27*time.Minute},
		{"reload at 19:00 25 october maintenance", 24*time.Hour + 27*time.Minute},
	} {
		p, e := parseReload(strings.Fields(tt.command), now)
		if e != nil || p.At.Sub(now) != tt.after || !p.Scheduled {
			t.Fatal(tt.command, p, e)
		}
	}
	for _, command := range []string{"reload in -1", "reload in 1:60", "reload in 34561", "reload at 25:00", "reload at 19:00 feb 31", "reload at 19:00 dec 25", "reload /noverify", "reload warm"} {
		if _, e := parseReload(strings.Fields(command), now); e == nil {
			t.Fatal("accepted", command)
		}
	}
}

func TestIOSReloadSaveConfirmCancelAndRestart(t *testing.T) {
	old := startupPath
	startupPath = filepath.Join(t.TempDir(), "startup")
	defer func() { startupPath = old }()
	c := config.Default()
	fired := 0
	state := filepath.Join(t.TempDir(), "reload")
	m, e := newReloadManager(state, func() error { fired++; return nil })
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 10, 24, 18, 33, 0, 0, time.UTC)
	s := &loginSession{user: "admin", cli: &cli.Session{Running: c, Privilege: 15, Save: func(c config.Config) error { return config.AtomicWrite(startupPath, c.JSON(), 0600) }}}
	r := s.beginReload([]string{"reload", "in", "5", "maintenance"}, m, now)
	if !strings.Contains(r.Prompt, "Save? [yes/no]") {
		t.Fatal(r)
	}
	r = s.answerReload("maybe", m, now)
	if !r.Challenge {
		t.Fatal(r)
	}
	r = s.answerReload("yes", m, now)
	if r.Prompt != "Proceed with reload? [confirm]" {
		t.Fatal(r)
	}
	if fired != 0 || m.plan != nil {
		t.Fatal("reloaded before confirmation")
	}
	r = s.answerReload("", m, now)
	if r.Error != "" || m.plan == nil {
		t.Fatal(r)
	}
	m, e = newReloadManager(state, func() error { fired++; return nil })
	if e != nil {
		t.Fatal(e)
	}
	m.due(now.Add(4 * time.Minute))
	if fired != 0 {
		t.Fatal("early reload")
	}
	r = s.beginReload([]string{"reload", "cancel"}, m, now)
	if r.Output != "%Reload cancelled." {
		t.Fatal(r)
	}
	m.due(now.Add(6 * time.Minute))
	if fired != 0 {
		t.Fatal("cancelled reload fired")
	}
	r = s.beginReload([]string{"reload"}, m, now)
	if r.Prompt != "Proceed with reload? [confirm]" {
		t.Fatal(r)
	}
	s.answerReload("__cancel", m, now)
	if m.plan != nil || s.pendingReload != nil {
		t.Fatal("Ctrl-C did not cancel")
	}
	s.beginReload([]string{"reload"}, m, now)
	s.answerReload("", m, now)
	m.due(now.Add(2 * time.Second))
	m.due(now.Add(3 * time.Second))
	if fired != 1 {
		t.Fatal("reload count", fired)
	}
	if _, e = os.Stat(state); !os.IsNotExist(e) {
		t.Fatal("reload retained across restart")
	}
	m.fire = func() error { return errors.New("test failure") }
	s.beginReload([]string{"reload"}, m, now)
	s.answerReload("", m, now)
	m.due(now.Add(2 * time.Second))
	if !strings.Contains(m.status(now), "test failure") {
		t.Fatal("lost supervisor failure")
	}
}

func TestIOSDirectoryOptionsAndConfinement(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root := t.TempDir()
	outside := t.TempDir()
	os.Mkdir(filepath.Join(root, "folder"), 0700)
	os.WriteFile(filepath.Join(root, "prefix-one"), []byte("one"), 0600)
	os.WriteFile(filepath.Join(root, "folder", "nested"), []byte("two"), 0600)
	os.WriteFile(filepath.Join(outside, "sentinel"), []byte("safe"), 0600)
	os.Symlink(outside, filepath.Join(root, "escape"))
	fileSystems = func() []applianceFS { return []applianceFS{{"bootflash", root, false}, {"nvram", outside, true}} }
	for _, command := range []string{"dir /all /recursive flash:", "dir /recursive /all bootflash:", "dir all-filesystems"} {
		out, e := fileOperation(strings.Fields(command))
		if e != nil || !strings.Contains(out, "bytes total") {
			t.Fatal(command, out, e)
		}
		if strings.Contains(command, "recursive") && (!strings.Contains(out, "nested") || strings.Contains(out, "sentinel")) {
			t.Fatal(out)
		}
	}
	out, e := fileOperation([]string{"dir", "bootflash:prefix*ignored"})
	if e != nil || !strings.Contains(out, "prefix-one") || strings.Contains(out, "folder") {
		t.Fatal(out, e)
	}
	out, e = fileOperation([]string{"dir", "bootflash:prefix-one"})
	if e != nil || !strings.Contains(out, "prefix-one") {
		t.Fatal(out, e)
	}
	if _, e = fileOperation([]string{"delete", "/recursive", "/force", "flash:escape"}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(outside, "sentinel")); e != nil {
		t.Fatal("escaped root", e)
	}
	if _, e = fileOperation([]string{"delete", "/recursive", "/force", "nvram:"}); e == nil {
		t.Fatal("deleted protected root")
	}
}

func TestIOSCopyPromptsAndOverwrite(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root := t.TempDir()
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	os.WriteFile(filepath.Join(root, "source"), []byte("original"), 0600)
	s := &loginSession{cli: &cli.Session{Running: config.Default(), Privilege: 15}}
	r := s.beginCopy([]string{"copy", "data0:", "data0:dest"})
	if r.Prompt != "Source filename []? " {
		t.Fatal(r)
	}
	r = s.answerCopy("source")
	if r.Prompt != "Destination filename [dest]? " {
		t.Fatal(r)
	}
	r = s.answerCopy("")
	if r.Error != "" {
		t.Fatal(r)
	}
	os.WriteFile(filepath.Join(root, "source"), []byte("updated"), 0600)
	s.beginCopy([]string{"copy", "data0:source", "data0:dest"})
	r = s.answerCopy("")
	if !strings.Contains(r.Prompt, "overwrite? [confirm]") {
		t.Fatal(r)
	}
	s.answerCopy("__cancel")
	data, _ := os.ReadFile(filepath.Join(root, "dest"))
	if string(data) != "original" {
		t.Fatal("overwrite without confirmation")
	}
	s.beginCopy([]string{"copy", "data0:source", "data0:dest"})
	s.answerCopy("")
	r = s.answerCopy("")
	data, _ = os.ReadFile(filepath.Join(root, "dest"))
	if r.Error != "" || string(data) != "updated" {
		t.Fatal(r, string(data))
	}
	s.beginCopy([]string{"copy", "data0:source", "data0:dest"})
	s.cli.Privilege = 1
	if r = s.answerCopy(""); r.Error == "" || s.pendingCopy != nil {
		t.Fatal("privilege drop retained copy", r)
	}
}

func TestIOSWorkingDirectoryIsolation(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root, outside := t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(root, "folder"), 0700)
	os.WriteFile(filepath.Join(root, "folder", "source"), []byte("hello"), 0600)
	os.Symlink(outside, filepath.Join(root, "escape"))
	fileSystems = func() []applianceFS { return []applianceFS{{"bootflash", root, false}, {"data0", root, false}} }
	s, other := &loginSession{}, &loginSession{}
	c := config.Default()
	s.cli = &cli.Session{Running: c, Candidate: c.Clone(), Platform: s.operations, EnforcePrivilege: true, Privilege: 1}
	if _, err := s.cli.Execute("cd data0:folder"); err != nil {
		t.Fatal(err)
	}
	if out, err := s.cli.Execute("pwd"); err != nil || out != "data0:/folder" {
		t.Fatal(out, err)
	}
	if s.pwd() != "data0:/folder" || other.pwd() != "bootflash:/" {
		t.Fatal(s.pwd(), other.pwd())
	}
	if out, err := s.operations([]string{"dir"}, c); err != nil || !strings.Contains(out, "source") {
		t.Fatal(out, err)
	}
	if _, err := s.operations([]string{"rename", "source", "dest"}, c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "folder", "dest")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.operations([]string{"cd", "../escape"}, c); err == nil {
		t.Fatal("escaped named filesystem")
	}
	if s.pwd() != "data0:/folder" {
		t.Fatal("failed cd changed current directory")
	}
	if _, err := s.operations([]string{"cd", ".."}, c); err != nil {
		t.Fatal(err)
	}
	if s.pwd() != "data0:/" {
		t.Fatal(s.pwd())
	}
	if _, err := s.operations([]string{"delete", "/force", "folder/dest"}, c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.operations([]string{"cd", "folder"}, c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.operations([]string{"cd"}, c); err != nil || s.pwd() != "data0:/" {
		t.Fatal(s.pwd(), err)
	}
}
