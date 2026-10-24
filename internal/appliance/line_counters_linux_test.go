package appliance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLineCountersSurviveManagementRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uses.json")
	c, err := loadLineCounters(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []int{0, 0, 1, 16} {
		if err := c.record(line); err != nil {
			t.Fatal(err)
		}
	}
	restarted, err := loadLineCounters(path)
	if err != nil || restarted.Uses != c.Uses || restarted.Uses[0] != 2 || restarted.Uses[16] != 1 {
		t.Fatal("counters lost across management restart", restarted, err)
	}
	if err := restarted.record(1); err != nil || restarted.Uses[1] != 2 {
		t.Fatal(err, restarted)
	}
	for _, line := range []int{-1, 17} {
		if restarted.record(line) == nil {
			t.Fatal("invalid line accepted", line)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	// Counters reset with runtime state on a complete machine boot.
	fresh, err := loadLineCounters(filepath.Join(t.TempDir(), "uses.json"))
	if err != nil || fresh.Uses[0] != 0 {
		t.Fatal(fresh, err)
	}
}

func TestLineCountersRejectCorruptionAndSaturate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uses.json")
	for _, data := range []string{`{"uses":[1]}`, `{"uses":null}`, `not json`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		c, err := loadLineCounters(path)
		if err == nil || c == nil || c.Uses[0] != 0 {
			t.Fatal("corrupt counter accepted", c, err)
		}
	}
	c := &lineCounters{path: path}
	c.Uses[0] = ^uint64(0)
	if err := c.record(0); err != nil || c.Uses[0] != ^uint64(0) {
		t.Fatal(c, err)
	}
	c.path = t.TempDir() // A directory cannot be replaced with a regular file.
	if c.record(0) == nil {
		t.Fatal("failed persistence reported success")
	}
}
