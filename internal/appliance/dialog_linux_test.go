package appliance

import (
	"encoding/json"
	"fractalcr/internal/cli"
	"fractalcr/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagementLiteralAuthenticationInput(t *testing.T) {
	oldCheck, oldAudit := checkDatastore, auditPath
	checkDatastore = func() error { return nil }
	auditPath = filepath.Join(t.TempDir(), "audit.jsonl")
	defer func() { checkDatastore, auditPath = oldCheck, oldAudit }()
	for _, password := range []string{"__tick", "__connect", "__cancel", "__end", "__help password", "__complete password", "__reserved-prefix-password"} {
		t.Run(password, func(t *testing.T) {
			// Include exact former sentinels as well as ordinary prefixed text.
			hash, err := config.HashSecret(password)
			if err != nil {
				t.Fatal(err)
			}
			c := config.Default()
			c.AAA.Enabled = true
			c.AAA.EnableSecret = hash
			c.AAA.Users = map[string]config.User{"__connect": {Secret: hash, Privilege: 1}}
			s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), EnforcePrivilege: true}}
			now := time.Now()
			s.authenticate(managementRequest{Action: "connect"}, c, now)
			r, _ := s.authenticate(managementRequest{Command: "__connect"}, c, now)
			if !r.Secret || s.user != "__connect" {
				t.Fatalf("username treated as event: %+v", r)
			}
			r, _ = s.authenticate(managementRequest{Action: "tick"}, c, now)
			if !r.Secret || s.stage != "password" {
				t.Fatalf("heartbeat lost prompt: %+v", r)
			}
			r, _ = s.authenticate(managementRequest{Command: password}, c, now)
			if r.Error != "" || !s.authenticated || s.cli.Privilege != 1 {
				t.Fatalf("literal password failed: %+v", r)
			}
			s.enable(15, c)
			r, _ = s.authenticate(managementRequest{Command: password}, c, now)
			if r.Error != "" || s.cli.Privilege != 15 {
				t.Fatalf("enable password failed: %+v", r)
			}
			s.stage = "enable"
			r, _ = s.authenticate(managementRequest{Action: "cancel"}, c, now)
			if r.Challenge || s.stage != "" || !s.authenticated {
				t.Fatalf("cancel lost session: %+v", r)
			}
		})
	}
}

func TestIOSFilePromptPolicy(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root := t.TempDir()
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	s := &loginSession{cli: &cli.Session{Running: config.Default(), Privilege: 15}}
	for _, mode := range []string{"", "alert", "quiet", "noisy"} {
		t.Run("delete-"+mode, func(t *testing.T) {
			s.cli.Running.FilePrompt = mode
			os.WriteFile(filepath.Join(root, "delete-me"), []byte("present"), 0600)
			r := s.beginFile([]string{"delete", "data0:delete-me"})
			if mode == "quiet" {
				if r.Challenge || r.Error != "" {
					t.Fatal(r)
				}
				if _, err := os.Stat(filepath.Join(root, "delete-me")); !os.IsNotExist(err) {
					t.Fatal("quiet did not delete", err)
				}
				return
			}
			if mode == "noisy" {
				if r.Prompt != "Filename [data0:delete-me]? " {
					t.Fatal(r)
				}
				r = s.answerFile("")
			}
			if r.Prompt != "Delete data0:delete-me? [confirm]" {
				t.Fatal(r)
			}
			s.cancelDialog()
			if _, err := os.Stat(filepath.Join(root, "delete-me")); err != nil {
				t.Fatal("cancel deleted file", err)
			}
			r = s.beginFile([]string{"delete", "/force", "data0:delete-me"})
			if r.Challenge || r.Error != "" {
				t.Fatal("force prompted", r)
			}
		})
	}
	os.WriteFile(filepath.Join(root, "source"), []byte("new"), 0600)
	os.WriteFile(filepath.Join(root, "dest"), []byte("old"), 0600)
	s.cli.Running.FilePrompt = "quiet"
	r := s.beginCopy([]string{"copy", "data0:source", "data0:dest"})
	data, _ := os.ReadFile(filepath.Join(root, "dest"))
	if r.Challenge || r.Error != "" || string(data) != "new" {
		t.Fatal("quiet overwrite", r, string(data))
	}
	r = s.beginCopy([]string{"copy", "data0:", "data0:another"})
	if r.Prompt != "Source filename []? " {
		t.Fatal("quiet suppressed required input", r)
	}
	r = s.answerCopy("source")
	if r.Challenge || r.Error != "" {
		t.Fatal("quiet copy after required input", r)
	}
	s.cli.Running.FilePrompt = "noisy"
	r = s.beginCopy([]string{"copy", "data0:source", "data0:dest"})
	if r.Prompt != "Source filename [data0:source]? " {
		t.Fatal(r)
	}
	r = s.answerCopy("")
	if r.Prompt != "Destination filename [dest]? " {
		t.Fatal(r)
	}
	r = s.answerCopy("")
	if r.Prompt != "Destination file exists, overwrite? [confirm]" {
		t.Fatal(r)
	}
	s.cli.Privilege = 1
	r = s.answerCopy("")
	if r.Error != "Authorization failed." {
		t.Fatal("dialog retained privilege", r)
	}
	s.cli.Privilege = 15
	r = s.beginFile([]string{"rename", "data0:another", "data0:renamed"})
	if r.Prompt != "Source filename [data0:another]? " {
		t.Fatal(r)
	}
	r = s.answerFile("")
	if r.Prompt != "Destination filename [data0:renamed]? " {
		t.Fatal(r)
	}
	r = s.answerFile("")
	if r.Error != "" || r.Challenge {
		t.Fatal(r)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed")); err != nil {
		t.Fatal(err)
	}
	s.cli.Running.FilePrompt = "quiet"
	r = s.beginFile([]string{"delete", "/recursive", "data0:"})
	if r.Error == "" {
		t.Fatal("quiet bypassed protected-root policy")
	}
}

func TestManagementEventFraming(t *testing.T) {
	for _, r := range []managementRequest{{Command: "__connect"}, {Action: "tick"}, {Action: "help", Command: "show "}, {Action: "cancel"}} {
		data, _ := json.Marshal(r)
		got, err := readManagementRequest(strings.NewReader(string(data) + "\n"))
		if err != nil || got != r {
			t.Fatalf("frame changed: %+v %v", got, err)
		}
	}
	for _, data := range []string{`{"action":"tick","command":"secret"}`, `{"action":"unknown"}`, `{"action":"connect","command":"__tick"}`} {
		if _, err := readManagementRequest(strings.NewReader(data + "\n")); err == nil {
			t.Fatalf("accepted ambiguous frame: %s", data)
		}
	}
}

func TestCopyLiteralControlFilenames(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root := t.TempDir()
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	for _, name := range []string{"__connect", "__tick", "__cancel", "__end"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		s := &loginSession{cli: &cli.Session{Running: config.Default(), Privilege: 15}}
		s.beginCopy([]string{"copy", "data0:", "data0:"})
		if r := s.answerCopy(name); !r.Challenge {
			t.Fatalf("source cancelled: %+v", r)
		}
		r := s.answerCopy(name + "-copy")
		if r.Error != "" || r.Challenge {
			t.Fatalf("copy failed: %+v", r)
		}
		data, err := os.ReadFile(filepath.Join(root, name+"-copy"))
		if err != nil || string(data) != name {
			t.Fatalf("literal file missing: %q %v", data, err)
		}
	}
}
