package appliance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBtrfsMaintenanceQualification(t *testing.T) {
	dir := os.Getenv("FCR_BTRFS_QUALIFY")
	if dir == "" {
		t.Skip("requires an isolated disposable Btrfs mount")
	}
	if !strings.HasPrefix(dir, "/machines/fractalcr/cli-storage-") {
		t.Fatal("qualification requires an explicitly isolated /machines/fractalcr/cli-storage-* mount")
	}
	old := datastoreDirectory
	datastoreDirectory = dir
	defer func() { datastoreDirectory = old }()
	f, err := openDatastore()
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err = os.WriteFile(filepath.Join(dir, "fixture"), []byte("snapshot original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshotOperation("create", "before"); err != nil {
		t.Fatal(err)
	}
	if listing, err := snapshotOperation("list", ""); err != nil || !strings.Contains(listing, "before") || !strings.Contains(listing, "read-only") {
		t.Fatal(listing, err)
	}
	defer snapshotOperation("delete", "before")
	if err = os.WriteFile(filepath.Join(dir, "fixture"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".snapshots", "before", "fixture"))
	if err != nil || string(data) != "snapshot original\n" {
		t.Fatal("snapshot did not preserve data", err)
	}
	if err = os.WriteFile(filepath.Join(dir, ".snapshots", "before", "fixture"), []byte("must fail"), 0600); err == nil {
		t.Fatal("snapshot is writable")
	}
	if _, err = snapshotOperation("create", "../escape"); err == nil {
		t.Fatal("snapshot path traversal")
	}
	for _, kind := range []string{"scrub", "balance"} {
		if _, err = maintenanceOperation(kind, "start"); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			maintenance.Lock()
			job := maintenance.jobs[kind]
			maintenance.Unlock()
			if job.State != "running" {
				if job.State != "completed" {
					t.Fatalf("%s: %+v", kind, job)
				}
				break
			}
			if time.Now().After(deadline) {
				_, _ = maintenanceOperation(kind, "cancel")
				t.Fatal("maintenance timed out")
			}
			time.Sleep(100 * time.Millisecond)
		}
		status, err := maintenanceOperation(kind, "status")
		if err != nil || !strings.Contains(status, "completed") {
			t.Fatal(status, err)
		}
		maintenance.Lock()
		delete(maintenance.jobs, kind)
		maintenance.Unlock()
		status, err = maintenanceOperation(kind, "status")
		if err != nil || !strings.Contains(status, "completed") {
			t.Fatal("persistent result lost", status, err)
		}
	}
	t.Log("Native Btrfs snapshot/read-only/delete, scrub, balance, and persisted maintenance results passed")
}
