package appliance

import (
	"fractalcr/internal/cli"
	"os"
	"path/filepath"
	"testing"
)

func TestOutputRedirectionAtomicAppendAndConfinement(t *testing.T) {
	root := t.TempDir()
	old := fileSystems
	defer func() { fileSystems = old }()
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}, {"nvram", root, true}} }
	s := &loginSession{cli: &cli.Session{Privilege: 15}, workingDirectory: "data0:/"}
	for _, tt := range []struct{ mode, text, want string }{{"redirect", "first\n", ""}, {"append", "second\n", ""}, {"tee-append", "third\n", "third\n"}} {
		out, err := s.writeOutput(&cli.Filter{Mode: tt.mode, Pattern: "output.txt"}, tt.text)
		if err != nil || out != tt.want {
			t.Fatal(out, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(root, "output.txt"))
	if string(data) != "first\nsecond\nthird\n" {
		t.Fatal(string(data))
	}
	s.cli.Privilege = 1
	if _, err := s.writeOutput(&cli.Filter{Mode: "redirect", Pattern: "output.txt"}, "destroy"); err == nil {
		t.Fatal("unprivileged write")
	}
	s.cli.Privilege = 15
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("unchanged"), 0600)
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"data0:escape/secret", "nvram:startup-config.json"} {
		if _, err := s.writeOutput(&cli.Filter{Mode: "redirect", Pattern: url}, "destroy"); err == nil {
			t.Fatal("protected output write", url)
		}
	}
	data, _ = os.ReadFile(filepath.Join(outside, "secret"))
	if string(data) != "unchanged" {
		t.Fatal("escaped volume")
	}
}
