package appliance

import (
	"fractalcr/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestPhysicalRecoveryIsExplicitAndPreservesSavedConfiguration(t *testing.T) {
	oldRun, oldStartup, oldCommand, oldCheck := runningPath, startupPath, commandLinePath, checkDatastore
	defer func() {
		runningPath, startupPath, commandLinePath, checkDatastore = oldRun, oldStartup, oldCommand, oldCheck
	}()
	dir := t.TempDir()
	runningPath, startupPath, commandLinePath = filepath.Join(dir, "running"), filepath.Join(dir, "startup"), filepath.Join(dir, "cmdline")
	checkDatastore = func() error { return nil }
	original := []byte(`{"aaa":broken`)
	os.WriteFile(startupPath, original, 0600)
	for _, line := range []string{"", "fcr.ignore_startup=10", "other=fcr.ignore_startup=1"} {
		os.WriteFile(commandLinePath, []byte(line), 0600)
		if _, err := loadRunning(); err == nil {
			t.Fatal("malformed config accepted without explicit recovery")
		}
	}
	os.WriteFile(commandLinePath, []byte("console=ttyS0 fcr.ignore_startup=1 quiet"), 0600)
	c, err := loadRunning()
	if err != nil || c.AAA.Enabled {
		t.Fatal(c, err)
	}
	c.Hostname = "repaired"
	if err = Save(c); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(startupPath + ".pre-recovery")
	if err != nil || string(backup) != string(original) {
		t.Fatal("original not preserved", err)
	}
	c.Hostname = "repaired-again"
	if err = Save(c); err != nil {
		t.Fatal(err)
	}
	backup, _ = os.ReadFile(startupPath + ".pre-recovery")
	if string(backup) != string(original) {
		t.Fatal("backup overwritten")
	}
	os.WriteFile(commandLinePath, []byte("console=ttyS0"), 0600)
	c, err = loadRunning()
	if err != nil || c.Hostname != "repaired-again" {
		t.Fatal(c, err)
	}
	// Subsequent commits in recovery remain authoritative instead of resetting
	// the running configuration on every management request.
	os.WriteFile(commandLinePath, []byte("fcr.ignore_startup=1"), 0600)
	config.AtomicWrite(runningPath, c.JSON(), 0600)
	c, err = loadRunning()
	if err != nil || c.Hostname != "repaired-again" {
		t.Fatal(c, err)
	}
}
