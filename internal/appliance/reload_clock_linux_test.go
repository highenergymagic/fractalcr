package appliance

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fractalcr/internal/cli"
	"fractalcr/internal/config"
)

func TestReloadAcrossSummerTime(t *testing.T) {
	summer, err := config.ParseSummerTime("PDT", "recurring", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := config.Clock{Zone: "PST", OffsetMinutes: -480, Summer: summer}
	for _, tt := range []struct{ now, command, want string }{
		{"2026-10-31T18:00:00Z", "reload at 03:00 1 November", "2026-11-01T11:00:00Z"},
		{"2026-03-07T18:00:00Z", "reload at 03:00 8 March", "2026-03-08T10:00:00Z"},
		{"2026-11-01T08:45:00Z", "reload at 01:30", "2026-11-01T09:30:00Z"},
		{"2026-11-01T07:45:00Z", "reload at 01:30", "2026-11-01T08:30:00Z"},
		{"2003-01-01T18:00:00Z", "reload at 11:00", "2003-01-01T19:00:00Z"},
	} {
		now, _ := time.Parse(time.RFC3339, tt.now)
		want, _ := time.Parse(time.RFC3339, tt.want)
		p, err := parseReload(strings.Fields(tt.command), now, c)
		if err != nil || !p.At.Equal(want) {
			t.Fatalf("%s at %s: got %s, %v; want %s", tt.command, now, p.At, err, want)
		}
	}
	now := time.Date(2026, 3, 8, 8, 0, 0, 0, time.UTC)
	if _, err := parseReload(strings.Fields("reload at 02:30"), now, c); err == nil {
		t.Fatal("accepted nonexistent spring-forward time")
	}
	p, err := parseReload(strings.Fields("reload in 180"), now, c)
	if err != nil || p.At.Sub(now) != 3*time.Hour {
		t.Fatal("relative interval changed across transition", p, err)
	}
}

func TestReloadClockLabelSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reload")
	c := config.Clock{Zone: "PST", OffsetMinutes: -480}
	now := time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)
	p, err := parseReload(strings.Fields("reload in 10"), now, c)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newReloadManager(path, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := m.set(p); err != nil {
		t.Fatal(err)
	}
	want := "Reload scheduled for 10:10:00 PST Sat Oct 24 2026 (in 10 minutes)"
	if got := m.status(now, c); got != want {
		t.Fatal(got)
	}
	m, err = newReloadManager(path, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := m.status(now, c); got != want {
		t.Fatal(got)
	}
}

func TestReloadRequiresClockSourceNotRecentYear(t *testing.T) {
	old, oldStartup := softwareClockState, startupPath
	defer func() { softwareClockState, startupPath = old, oldStartup }()
	startupPath = filepath.Join(t.TempDir(), "startup")
	c := config.Default()
	s := &loginSession{cli: &cli.Session{Running: c, Privilege: 15}}
	m, err := newReloadManager(filepath.Join(t.TempDir(), "reload"), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	softwareClockState = func(time.Time) (string, string) { return "*", "none" }
	if r := s.beginReload([]string{"reload", "at", "19:00"}, m, time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)); r.Error != "The clock has not been set" {
		t.Fatal(r)
	}
	softwareClockState = func(time.Time) (string, string) { return "", "user configuration" }
	if r := s.beginReload([]string{"reload", "at", "19:00"}, m, time.Date(2003, 1, 1, 18, 0, 0, 0, time.UTC)); r.Error != "" || !r.Challenge {
		t.Fatal(r)
	}
}
