package appliance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fractalcr/internal/config"
)

func TestCalendarAuthoritySourceSelection(t *testing.T) {
	oldSource, oldNTP, oldHardware := clockSourcePath, synchronizedNTP, hardwareCalendarPresent
	t.Cleanup(func() { clockSourcePath, synchronizedNTP, hardwareCalendarPresent = oldSource, oldNTP, oldHardware })
	clockSourcePath = filepath.Join(t.TempDir(), "clock-source")
	synchronizedNTP = func() bool { return false }
	hardwareCalendarPresent = func() bool { return true }
	now := time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)
	for _, source := range []string{"", "hardware calendar", "user configuration", "NVRAM"} {
		if source == "" {
			os.Remove(clockSourcePath)
		} else if err := os.WriteFile(clockSourcePath, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		for _, valid := range []bool{false, true} {
			c := config.Clock{CalendarValid: valid}
			got := configuredClockStatus(c, now, true)
			want := source
			if want == "" {
				want = "hardware calendar"
			}
			authoritative := want == "user configuration" || (want == "hardware calendar" && valid)
			if strings.HasPrefix(got, "*") == authoritative || !strings.HasSuffix(got, "Time source is "+want) {
				t.Fatal(source, valid, got)
			}
		}
	}
	// Trusted calendar policy cannot relabel NTP or promote a saved, stopped clock.
	synchronizedNTP = func() bool { return true }
	if got := configuredClockStatus(config.Clock{CalendarValid: true}, now, true); strings.HasPrefix(got, "*") || !strings.HasSuffix(got, "Time source is NTP") {
		t.Fatal(got)
	}
	synchronizedNTP = func() bool { return false }
	hardwareCalendarPresent = func() bool { return false }
	os.WriteFile(clockSourcePath, []byte("hardware calendar"), 0600)
	if got := configuredClockStatus(config.Clock{CalendarValid: true}, now, true); !strings.HasPrefix(got, "*") {
		t.Fatal("missing hardware became authoritative", got)
	}
	c := config.Default()
	c.Clock.CalendarValid = true
	if err := supported(c); err == nil || !strings.Contains(err.Error(), "requires a hardware calendar") {
		t.Fatal(err)
	}
}
