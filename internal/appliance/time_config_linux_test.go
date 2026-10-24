package appliance

import (
	"encoding/binary"
	"errors"
	"fractalcr/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTimeServiceCalendarPolicy(t *testing.T) {
	c := config.NTP{Servers: []string{"time.example.org"}}
	for _, enabled := range []bool{false, true, false} {
		c.UpdateCalendar = enabled
		text := timeServiceConfiguration(c)
		if strings.Contains(text, "\nrtcsync\n") != enabled {
			t.Fatal("RTC policy not reflected in service configuration", text)
		}
		if !strings.Contains(text, "\nserver time.example.org iburst\n") || !strings.Contains(text, "\nport 0\n") {
			t.Fatal("calendar setting changed source or exposed a network service", text)
		}
	}
}

func TestClockSourceIndependentOfCalendarUpdatePolicy(t *testing.T) {
	oldQuery, oldPath := synchronizedNTP, clockSourcePath
	t.Cleanup(func() { synchronizedNTP, clockSourcePath = oldQuery, oldPath })
	clockSourcePath = filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(clockSourcePath, []byte("user configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 76)
	synchronizedNTP = func() bool { return synchronizedTracking(packet) }
	if _, source := softwareClockState(time.Now()); source != "user configuration" {
		t.Fatal(source)
	}
	binary.BigEndian.PutUint16(packet[24:], 2)
	if mark, source := softwareClockState(time.Now()); mark != "" || source != "NTP" {
		t.Fatal(mark, source)
	}
	binary.BigEndian.PutUint16(packet[26:], 3)
	if synchronizedTracking(packet) {
		t.Fatal("unsynchronized leap status accepted")
	}
	binary.BigEndian.PutUint16(packet[26:], 0)
	binary.BigEndian.PutUint16(packet[24:], 16)
	if synchronizedTracking(packet) || synchronizedTracking(nil) {
		t.Fatal("invalid tracking data accepted")
	}
}

func TestTimeConfigurationOnlyRestartsTimeServiceAndRollsBack(t *testing.T) {
	oldRunning, oldJournal, oldRestart, oldCheck := runningPath, journalPath, restartTimeService, checkDatastore
	t.Cleanup(func() {
		runningPath, journalPath, restartTimeService, checkDatastore = oldRunning, oldJournal, oldRestart, oldCheck
	})
	dir := t.TempDir()
	runningPath, journalPath = filepath.Join(dir, "running"), filepath.Join(dir, "rollback")
	checkDatastore = func() error { t.Fatal("NTP edit entered full platform restart"); return nil }
	old := config.Default()
	next := old.Clone()
	next.NTP.UpdateCalendar = true
	calls := 0
	restartTimeService = func() error {
		calls++
		c, err := config.Load(runningPath)
		if err != nil || c.NTP.UpdateCalendar != true {
			t.Fatal(c, err)
		}
		return nil
	}
	if err := Apply(old, next); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatal("completed transaction retained journal", err)
	}
	restartTimeService = func() error {
		c, err := config.Load(runningPath)
		if err != nil {
			return err
		}
		if c.NTP.UpdateCalendar {
			return errors.New("clock service failed")
		}
		return nil
	}
	if err := Apply(old, next); err == nil || !strings.Contains(err.Error(), "previous configuration restored") {
		t.Fatal(err)
	}
	c, err := config.Load(runningPath)
	if err != nil || c.NTP.UpdateCalendar {
		t.Fatal("failed update not rolled back", c, err)
	}
	restartTimeService = func() error { return errors.New("unavailable") }
	if err := Apply(old, next); err == nil {
		t.Fatal("rollback failure hidden")
	}
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatal("recovery journal lost", err)
	}
}
