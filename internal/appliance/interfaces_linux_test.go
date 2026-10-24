package appliance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInterfaceDiscoveryDoesNotRewriteUnchangedIdentity(t *testing.T) {
	file := filepath.Join(t.TempDir(), "interface-names.json")
	before := []byte(`[{"name":"Ethernet0","identity":"port0"}]`)
	writePortIdentities(file, before)
	stamp := time.Unix(123456789, 0)
	if err := os.Chtimes(file, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	writePortIdentities(file, before)
	st, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ModTime().Equal(stamp) {
		t.Fatal("read-only discovery rewrote interface identities")
	}
	after := []byte(`[{"name":"Ethernet0","identity":"port0"},{"name":"Ethernet1","identity":"port1"}]`)
	writePortIdentities(file, after)
	got, err := os.ReadFile(file)
	if err != nil || string(got) != string(after) {
		t.Fatalf("new identity not persisted: %s, %v", got, err)
	}
}
