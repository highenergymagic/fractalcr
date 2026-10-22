package appliance

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestHostKeyRejectsUnsafeFilesBeforeReading(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "key")
	if err := os.WriteFile(regular, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := readHostKey(regular); err != nil || string(data) != "fixture" {
		t.Fatal(err)
	}
	link := filepath.Join(root, "symlink")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readHostKey(link); err == nil {
		t.Fatal("followed host-key symlink")
	}
	fifo := filepath.Join(root, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readHostKey(fifo); err == nil {
		t.Fatal("accepted host-key FIFO")
	}
	if err := os.Chmod(regular, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readHostKey(regular); err == nil {
		t.Fatal("accepted exposed host key")
	}
	os.Chmod(regular, 0600)
	if err := os.Link(regular, filepath.Join(root, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if _, err := readHostKey(regular); err == nil {
		t.Fatal("accepted shared host-key inode")
	}
	large := filepath.Join(root, "large")
	os.WriteFile(large, make([]byte, 16385), 0600)
	if _, err := readHostKey(large); err == nil {
		t.Fatal("accepted unbounded host key")
	}
}

func TestAccountingRejectsSpecialLockFile(t *testing.T) {
	oldCheck, oldAudit := checkDatastore, auditPath
	checkDatastore = func() error { return nil }
	auditPath = filepath.Join(t.TempDir(), "audit.jsonl")
	defer func() { checkDatastore, auditPath = oldCheck, oldAudit }()
	if err := syscall.Mkfifo(auditPath+".lock", 0600); err != nil {
		t.Fatal(err)
	}
	if err := audit("admin", "ssh", "login", ""); err == nil {
		t.Fatal("accepted FIFO accounting lock")
	}
}

func TestAccountingRejectsSharedInode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !privateAccountingFile(f) {
		t.Fatal("rejected owned private file")
	}
	if err := os.Link(path, path+".alias"); err != nil {
		t.Fatal(err)
	}
	if privateAccountingFile(f) {
		t.Fatal("accepted accounting inode with another name")
	}
}
