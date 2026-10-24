package appliance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"fractalcr/internal/cli"
	"fractalcr/internal/config"
)

func TestConfigurationCopiesExportCLIAndImportObjects(t *testing.T) {
	oldFS, oldStartup := fileSystems, startupPath
	defer func() { fileSystems, startupPath = oldFS, oldStartup }()
	root := t.TempDir()
	startupPath = filepath.Join(t.TempDir(), "startup-config.json")
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	c := config.Default()
	c.Hostname = "saved"
	if err := config.AtomicWrite(startupPath, c.JSON(), 0600); err != nil {
		t.Fatal(err)
	}
	c.Hostname = "running"
	for _, source := range []string{"running-config", "system:/running-config", "startup-config", "nvram:/startup-config"} {
		if _, err := copyFileOverwrite([]string{"copy", source, "data0:backup"}, c, true); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(root, "backup"))
		name := "running"
		if configurationURL(source) == "startup-config" {
			name = "saved"
		}
		if !strings.Contains(string(data), "hostname "+name+"\n") || strings.Contains(string(data), `"version"`) {
			t.Fatal("export was not CLI", string(data))
		}
	}
	applied, saved := 0, 0
	s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), Privilege: 15, EnforcePrivilege: true,
		Apply: func(old, next config.Config) error { applied++; return nil },
		Save:  func(next config.Config) error { saved++; return config.AtomicWrite(startupPath, next.JSON(), 0600) }}}
	os.WriteFile(filepath.Join(root, "merge"), []byte("hostname merged\nline con 0\n history size 35\nend\n"), 0600)
	r := s.beginCopy([]string{"copy", "data0:merge", "running-config"})
	if r.Prompt != "Destination filename [running-config]? " {
		t.Fatal(r)
	}
	r = s.answerCopy("")
	if r.Error != "" || applied != 1 || saved != 0 || s.cli.Running.Hostname != "merged" || !strings.Contains(r.Prompt, "merged#") {
		t.Fatal(r, applied, saved)
	}
	s.beginCopy([]string{"copy", "data0:merge", "startup-config"})
	r = s.answerCopy("")
	if r.Error != "" || saved != 1 || applied != 1 {
		t.Fatal(r, saved, applied)
	}
	persisted, err := config.Load(startupPath)
	if err != nil || persisted.Hostname != "merged" || persisted.AAA.Terminal(0).HistorySize != 35 {
		t.Fatal(persisted, err)
	}
	// Parsing and apply errors cannot partially alter the caller or saved data.
	before := string(s.cli.Running.JSON())
	os.WriteFile(filepath.Join(root, "bad"), []byte("hostname should-not-apply\ndo reload\n"), 0600)
	r = s.copyConfiguration("data0:bad", "running-config")
	if r.Error == "" || applied != 1 || string(s.cli.Running.JSON()) != before {
		t.Fatal(r)
	}
	s.cli.Apply = func(config.Config, config.Config) error { return errors.New("fixture apply failed") }
	r = s.copyConfiguration("data0:merge", "running-config")
	if r.Error != "fixture apply failed" || string(s.cli.Running.JSON()) != before {
		t.Fatal(r)
	}
	s.beginCopy([]string{"copy", "data0:merge", "startup-config"})
	s.cli.Privilege = 1
	r = s.answerCopy("")
	if r.Error != "Authorization failed." || saved != 1 {
		t.Fatal(r)
	}
}

func TestConfigurationImportFileConfinement(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root, outside := t.TempDir(), t.TempDir()
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	os.WriteFile(filepath.Join(outside, "secret"), []byte("hostname escaped\n"), 0600)
	os.Symlink(outside, filepath.Join(root, "escape"))
	syscall.Mkfifo(filepath.Join(root, "fifo"), 0600)
	os.WriteFile(filepath.Join(root, "large"), make([]byte, config.MaxConfigBytes+1), 0600)
	for _, source := range []string{"data0:escape/secret", "data0:fifo", "data0:large", "data0:/"} {
		if _, err := configurationInput(source, config.Default()); err == nil {
			t.Fatal("accepted", source)
		}
	}
}
