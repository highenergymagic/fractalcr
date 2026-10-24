package appliance

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fractalcr/internal/config"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Model the actual fixed-size supervisor protocol, including a latched failed
// configuration worker and a manually disabled container engine.
func TestTransactionRollbackAndRecovery(t *testing.T) {
	dir := t.TempDir()
	socketDir, err := os.MkdirTemp("/tmp", "fcr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	oldSocket, oldRunning, oldJournal, oldCheck := controlSocket, runningPath, journalPath, checkDatastore
	defer func() {
		controlSocket = oldSocket
		runningPath = oldRunning
		journalPath = oldJournal
		checkDatastore = oldCheck
	}()
	controlSocket = filepath.Join(socketDir, "control")
	runningPath = filepath.Join(dir, "running.json")
	journalPath = filepath.Join(dir, "journal.json")
	checkDatastore = func() error { return nil }
	listener, err := net.Listen("unixpacket", controlSocket)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	defer func() { listener.Close(); <-done }()
	go func() {
		defer close(done)
		var generation uint32
		var state int32 = 5
		var pid int32
		pending := false
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			request := make([]byte, 40)
			n, e := c.Read(request)
			if e != nil || n != 40 {
				c.Close()
				continue
			}
			switch binary.LittleEndian.Uint32(request[4:]) {
			case 2:
				pid = 0
				if state != 5 {
					state = 0
				}
			case 3:
				pending = true
				state = 0
			case 0:
				if pending {
					pending = false
					generation++
					state = 2
					pid = 123
					cfg, e := config.Load(runningPath)
					if e != nil || cfg.Hostname == "bad" {
						state = 5
						pid = 0
					}
				}
			}
			var response bytes.Buffer
			binary.Write(&response, binary.LittleEndian, uint32(supervisorProtocol))
			binary.Write(&response, binary.LittleEndian, uint32(3))
			binary.Write(&response, binary.LittleEndian, int32(0))
			for i := 0; i < supervisorSlots; i++ {
				var s Status
				if i == 0 {
					copy(s.Name[:], "configuration")
					s.State = state
					s.PID = pid
					s.Enabled = 1
					s.Starts = generation
				}
				if i == 1 {
					copy(s.Name[:], "networking")
					s.State = 2
					s.Enabled = 1
				}
				if i == 2 {
					copy(s.Name[:], "container-engine")
					s.State = 0
					s.Enabled = 0
				}
				binary.Write(&response, binary.LittleEndian, s)
			}
			c.Write(response.Bytes())
			c.Close()
		}
	}()
	old := config.Default()
	old.Hostname = "before"
	next := old.Clone()
	next.Hostname = "after"
	next.Isolation.MemoryMiB = 384
	if err = Apply(old, next); err != nil {
		t.Fatal("could not recover latched configuration failure:", err)
	}
	got, err := config.Load(runningPath)
	if err != nil || got.Hostname != "after" {
		t.Fatal(got, err)
	}
	bad := next.Clone()
	bad.Hostname = "bad"
	bad.Isolation.MemoryMiB = 512
	if err = Apply(next, bad); err == nil || !strings.Contains(err.Error(), "previous configuration restored") {
		t.Fatal("missing rollback", err)
	}
	got, err = config.Load(runningPath)
	if err != nil || got.Hostname != "after" {
		t.Fatal("rollback did not restore file", got, err)
	}
	if _, err = os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatal("successful rollback retained journal")
	}
	config.AtomicWrite(journalPath, old.JSON(), 0600)
	if err = recoverTransaction(); err != nil {
		t.Fatal(err)
	}
	got, err = config.Load(runningPath)
	if err != nil || got.Hostname != "before" {
		t.Fatal("interrupted transaction recovery", got, err)
	}
}

func TestInventoryEditsDoNotRequireRunningServicesOrDatastore(t *testing.T) {
	oldRunning, oldCheck, oldSocket := runningPath, checkDatastore, controlSocket
	defer func() { runningPath, checkDatastore, controlSocket = oldRunning, oldCheck, oldSocket }()
	runningPath = filepath.Join(t.TempDir(), "running.json")
	controlSocket = filepath.Join(t.TempDir(), "absent.sock")
	checkDatastore = func() error { t.Fatal("inventory edit accessed datastore"); return nil }
	old := config.Default()
	next := old.Clone()
	next.Datastores["data0"] = config.Datastore{Filesystem: "btrfs", Preset: "raid1", Disks: []string{"vdb"}}
	if err := Apply(old, next); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(runningPath)
	if err != nil || len(got.Datastores["data0"].Disks) != 1 {
		t.Fatal("definition not retained", err)
	}
	if _, err := formatDatastore("data0", got); err == nil || !strings.Contains(err.Error(), "insufficient disks") {
		t.Fatal("incomplete RAID1 reached format operation", err)
	}
	next.Networks = map[string]config.DockerNetwork{"incomplete": {Driver: "macvlan"}}
	if _, err := networkOperation([]string{"request", "network", "incomplete", "create"}, next); err == nil || !strings.Contains(err.Error(), "parent interface") {
		t.Fatal("incomplete macvlan reached Docker", err)
	}
}

func TestEnablingAAAWithoutDatastoreCannotLockOutBootstrap(t *testing.T) {
	oldRunning, oldCheck := runningPath, checkDatastore
	defer func() { runningPath, checkDatastore = oldRunning, oldCheck }()
	runningPath = filepath.Join(t.TempDir(), "running.json")
	checkDatastore = func() error { return errors.New("offline") }
	old := config.Default()
	next := old.Clone()
	hash, _ := config.HashSecret("synthetic-bootstrap-password")
	next.AAA.Users = map[string]config.User{"admin": {Privilege: 15, Secret: hash}}
	next.AAA.Enabled = true
	if err := Apply(old, next); err == nil || !strings.Contains(err.Error(), "before enabling local login") {
		t.Fatal("AAA activation did not require an accounting datastore", err)
	}
	if _, err := os.Stat(runningPath); !os.IsNotExist(err) {
		t.Fatal("rejected AAA activation changed running configuration")
	}
}
