package appliance

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fractalcr/internal/config"
)

func clockSaveFixture(t *testing.T) {
	t.Helper()
	a, b, c, d := savedClockPath, restoredClockPath, clockSourcePath, runningPath
	h, n, s, check := hardwareCalendarPresent, networkClockReference, setSoftwareClock, checkDatastore
	t.Cleanup(func() {
		savedClockPath, restoredClockPath, clockSourcePath, runningPath = a, b, c, d
		hardwareCalendarPresent, networkClockReference, setSoftwareClock, checkDatastore = h, n, s, check
	})
	dir := t.TempDir()
	savedClockPath, restoredClockPath = filepath.Join(dir, "saved"), filepath.Join(dir, "restored")
	clockSourcePath, runningPath = filepath.Join(dir, "source"), filepath.Join(dir, "running")
	hardwareCalendarPresent = func() bool { return false }
	checkDatastore = func() error { return nil }
	setSoftwareClock = func(time.Time) error { t.Fatal("unexpected host clock write"); return nil }
}

func TestTrackingReferenceRequiresNetworkTime(t *testing.T) {
	ref := time.Date(2026, 10, 24, 18, 33, 0, 123456789, time.UTC)
	packet := make([]byte, 76)
	binary.BigEndian.PutUint16(packet[24:], 2)
	binary.BigEndian.PutUint32(packet[32:], uint32(ref.Unix()))
	binary.BigEndian.PutUint32(packet[36:], uint32(ref.Nanosecond()))
	for _, hi := range []uint32{0, 0x7fffffff} {
		binary.BigEndian.PutUint32(packet[28:], hi)
		if got, ok := trackingReference(packet); !ok || !got.Equal(ref) {
			t.Fatal(got, ok)
		}
	}
	binary.BigEndian.PutUint32(packet[28:], 0)
	for _, pos := range []int{24, 26, 28, 36} {
		bad := append([]byte(nil), packet...)
		switch pos {
		case 24:
			binary.BigEndian.PutUint16(bad[pos:], 0)
		case 26:
			binary.BigEndian.PutUint16(bad[pos:], 3)
		case 28:
			binary.BigEndian.PutUint32(bad[pos:], 0xffffffff)
		case 36:
			binary.BigEndian.PutUint32(bad[pos:], 1000000000)
		}
		if _, ok := trackingReference(bad); ok {
			t.Fatalf("invalid field at %d accepted", pos)
		}
	}
	if _, ok := trackingReference(packet[:39]); ok {
		t.Fatal("truncated timestamp accepted")
	}
}

func TestSavedClockIntervalAndAuthority(t *testing.T) {
	clockSaveFixture(t)
	c := config.Clock{SaveIntervalHours: 8}
	ref := time.Date(2026, 10, 24, 15, 41, 0, 0, time.UTC)
	synced := false
	networkClockReference = func() (time.Time, bool) { return ref, synced }
	for _, source := range []string{"user configuration", "hardware calendar", "NVRAM"} {
		os.WriteFile(clockSourcePath, []byte(source), 0600)
		if err := saveNetworkClock(c); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(savedClockPath); !os.IsNotExist(err) {
			t.Fatal("non-network source saved")
		}
	}
	synced = true
	if err := saveNetworkClock(c); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(savedClockPath)
	ref = ref.Add(8*time.Hour - time.Second)
	if err := saveNetworkClock(c); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(savedClockPath); string(got) != string(first) {
		t.Fatal("wrote before interval elapsed")
	}
	ref = ref.Add(time.Second)
	if err := saveNetworkClock(c); err != nil {
		t.Fatal(err)
	}
	if got, err := readSavedClock(); err != nil || !got.Equal(ref) {
		t.Fatal(got, err)
	}
	// A corrected external clock replaces an earlier erroneous future value.
	ref = ref.Add(-24 * time.Hour)
	if err := saveNetworkClock(c); err != nil {
		t.Fatal(err)
	}
	if got, _ := readSavedClock(); !got.Equal(ref) {
		t.Fatal("failed to replace corrected time", got)
	}
	st, _ := os.Stat(savedClockPath)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
}

func TestSavedClockRestoresOnceBeforeNetworking(t *testing.T) {
	clockSaveFixture(t)
	c := config.Clock{SaveIntervalHours: 24}
	ref := time.Date(2026, 10, 24, 18, 33, 0, 0, time.UTC)
	networkClockReference = func() (time.Time, bool) { return ref, true }
	if err := saveNetworkClock(c); err != nil {
		t.Fatal(err)
	}
	writes := 0
	setSoftwareClock = func(v time.Time) error {
		writes++
		if !v.Equal(ref) {
			t.Fatal(v)
		}
		return nil
	}
	oldSync := synchronizedNTP
	synchronizedNTP = func() bool { return false }
	t.Cleanup(func() { synchronizedNTP = oldSync })
	bootTime := time.Date(1993, 3, 1, 0, 0, 0, 0, time.UTC)
	for range 2 {
		if err := restoreSavedClock(c, bootTime); err != nil {
			t.Fatal(err)
		}
	}
	if writes != 1 {
		t.Fatal("restored more than once", writes)
	}
	if mark, source := softwareClockState(ref); mark != "*" || source != "NVRAM" {
		t.Fatal("stale saved clock reported synchronized", mark, source)
	}
	os.Remove(restoredClockPath)
	os.WriteFile(runningPath, []byte("{}"), 0600)
	if err := restoreSavedClock(c, bootTime); err != nil || writes != 1 {
		t.Fatal("runtime configuration restored stale boot time", err, writes)
	}
	os.Remove(runningPath)
	if err := restoreSavedClock(c, ref.Add(time.Hour)); err != nil || writes != 1 {
		t.Fatal("rewound newer firmware clock", err, writes)
	}
}

func TestSavedClockDevicePolicyAndFailures(t *testing.T) {
	clockSaveFixture(t)
	c := config.Clock{SaveIntervalHours: 8}
	networkClockReference = func() (time.Time, bool) { t.Fatal("unexpected NTP query"); return time.Time{}, false }
	if err := saveNetworkClock(config.Clock{}); err != nil {
		t.Fatal(err)
	}
	hardwareCalendarPresent = func() bool { return true }
	if err := saveNetworkClock(c); err != nil {
		t.Fatal(err)
	}
	if err := restoreSavedClock(c, time.Time{}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Clock = c
	if err := supported(cfg); err == nil || !strings.Contains(err.Error(), "without a hardware calendar") {
		t.Fatal("hardware platform accepted NVRAM-only clock policy", err)
	}
	hardwareCalendarPresent = func() bool { return false }
	for _, data := range []string{"invalid", strings.Repeat("x", 257), `{"version":2,"time":"2026-10-24T00:00:00Z"}`, `{"version":1,"time":"0001-01-01T00:00:00Z"}`} {
		os.WriteFile(savedClockPath, []byte(data), 0600)
		if _, err := readSavedClock(); err == nil {
			t.Fatal("invalid record accepted")
		}
		os.Remove(restoredClockPath)
		if err := restoreSavedClock(c, time.Time{}); err != nil {
			t.Fatal("damaged record prevented recovery", err)
		}
	}
	os.Remove(savedClockPath)
	os.Symlink(clockSourcePath, savedClockPath)
	if _, err := readSavedClock(); err == nil {
		t.Fatal("followed symlink")
	}
	os.Remove(savedClockPath)
	ref := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	networkClockReference = func() (time.Time, bool) { return ref, true }
	checkDatastore = func() error { return errors.New("offline") }
	if err := saveNetworkClock(c); err == nil {
		t.Fatal("offline datastore accepted")
	}
	checkDatastore = func() error { return nil }
	if err := saveNetworkClock(c); err != nil {
		t.Fatal(err)
	}
	os.Remove(restoredClockPath)
	setSoftwareClock = func(time.Time) error { return errors.New("clock write failed") }
	if err := restoreSavedClock(c, time.Time{}); err == nil {
		t.Fatal("clock write failure hidden")
	}
	if _, err := os.Stat(restoredClockPath); !os.IsNotExist(err) {
		t.Fatal("failed restoration marked complete")
	}
}
