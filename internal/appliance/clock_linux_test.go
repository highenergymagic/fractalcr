package appliance

import (
	"errors"
	"fractalcr/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClockUsesNativeAPIsAndConfiguredZone(t *testing.T) {
	oldSet, oldRead, oldWrite, oldPath := setSoftwareClock, readHardwareClock, writeHardwareClock, clockSourcePath
	defer func() {
		setSoftwareClock, readHardwareClock, writeHardwareClock, clockSourcePath = oldSet, oldRead, oldWrite, oldPath
	}()
	clockSourcePath = filepath.Join(t.TempDir(), "source")
	var software, hardware time.Time
	setSoftwareClock = func(v time.Time) error { software = v; return nil }
	writeHardwareClock = func(v time.Time) error { hardware = v; return nil }
	readHardwareClock = func() (time.Time, error) { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), nil }
	c := config.Default()
	c.Clock = config.Clock{Zone: "PDT", OffsetMinutes: -420}
	if out, err := Operations([]string{"show", "calendar"}, c); err != nil || out != "05:00:00 PDT Sat Oct 24 2026" {
		t.Fatal(out, err)
	}
	for _, w := range [][]string{{"clock", "set", "08:33:01", "24", "October", "2026"}, {"clock", "set", "08:33:01", "Oct", "24", "2026"}} {
		if _, err := clockOperation(w, c); err != nil {
			t.Fatal(err)
		}
		if software.UTC().Hour() != 15 {
			t.Fatal(software)
		}
	}
	prior := software
	if _, err := clockOperation([]string{"clock", "set", "08:00:00", "31", "February", "2026"}, c); err == nil {
		t.Fatal("invalid date accepted")
	}
	if software != prior {
		t.Fatal("invalid input touched clock")
	}
	if _, err := clockOperation([]string{"clock", "read-calendar"}, c); err != nil || software.Hour() != 12 {
		t.Fatal(err, software)
	}
	if _, err := clockOperation([]string{"calendar", "set", "08:00:00", "24", "October", "2026"}, c); err != nil || hardware.UTC().Hour() != 15 {
		t.Fatal(err, hardware)
	}
	setSoftwareClock = func(time.Time) error { return errors.New("permission denied") }
	before, _ := os.ReadFile(clockSourcePath)
	if _, err := clockOperation([]string{"clock", "read-calendar"}, c); err == nil {
		t.Fatal("failed operation reported success")
	}
	after, _ := os.ReadFile(clockSourcePath)
	if string(before) != string(after) {
		t.Fatal("failed operation changed source")
	}
	readHardwareClock = func() (time.Time, error) {
		return time.Time{}, &os.PathError{Op: "open", Path: "/dev/rtc0", Err: os.ErrNotExist}
	}
	if _, err := clockOperation([]string{"clock", "read-calendar"}, c); err == nil || strings.Contains(err.Error(), "/dev/") {
		t.Fatal("hardware error exposed platform implementation", err)
	}
}

func TestClockSummerTimeDisplay(t *testing.T) {
	summer, err := config.ParseSummerTime("PDT", "recurring", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := config.Clock{Zone: "PST", OffsetMinutes: -480, Summer: summer}
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	got := configuredClockStatus(c, now, true)
	want := "\nSummer time starts 02:00:00 PST Sun Mar 8 2026\nSummer time ends 02:00:00 PDT Sun Nov 1 2026"
	if !strings.HasSuffix(got, want) || !strings.Contains(got, "05:00:00.000 PDT") {
		t.Fatal(got)
	}
	if strings.Contains(configuredClockStatus(c, now, false), "Summer") {
		t.Fatal("detail leaked into base display")
	}
}
