package appliance

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"fractalcr/internal/config"
	"fractalcr/internal/filedisplay"
)

func TestMoreNativeConfinementAndWorkingDirectory(t *testing.T) {
	old := fileSystems
	defer func() { fileSystems = old }()
	root, outside := t.TempDir(), t.TempDir()
	fileSystems = func() []applianceFS { return []applianceFS{{"data0", root, false}} }
	if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "folder", "text"), []byte("native file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "private"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &loginSession{workingDirectory: "data0:/folder"}
	for _, command := range []string{"more text", "more /ascii text", "more data0:folder/text"} {
		got, err := s.operations(strings.Fields(command), config.Default())
		if err != nil || got != "native file\n" {
			t.Fatal(command, got, err)
		}
	}
	for _, name := range []string{"data0:escape/private", "data0:pipe", "data0:folder", "data0:missing"} {
		got, err := s.operations([]string{"more", name}, config.Default())
		if err == nil || got != "" || strings.Contains(err.Error(), root) || strings.Contains(err.Error(), outside) {
			t.Fatal(name, got, err)
		}
	}
	file, err := os.Create(filepath.Join(root, "large"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(filedisplay.Limit + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := s.operations([]string{"more", "data0:large"}, config.Default()); err == nil {
		t.Fatal("accepted oversized file")
	}
}
