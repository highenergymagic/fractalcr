package appliance

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReloadNotificationsAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reload")
	fired := 0
	m, err := newReloadManager(path, func() error { fired++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	plan, err := parseReload([]string{"reload", "in", "5"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.set(plan); err != nil {
		t.Fatal(err)
	}
	first, notice := m.notifications(0)
	if notice != "\n***\n*** --- SHUTDOWN in 0:05:00 ---\n***\n" {
		t.Fatal(notice)
	}
	_, other := m.notifications(0)
	if other != notice {
		t.Fatal("one terminal consumed another's warning")
	}
	_, notice = m.notifications(first)
	if notice != "" {
		t.Fatal("duplicate warning")
	}
	m.due(now.Add(4 * time.Minute))
	last, notice := m.notifications(first)
	if !strings.Contains(notice, "0:01:00") {
		t.Fatal(notice)
	}
	m.due(now.Add(4*time.Minute + time.Second))
	_, notice = m.notifications(last)
	if notice != "" {
		t.Fatal("repeated minute warning")
	}
	m, err = newReloadManager(path, func() error { fired++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	m.due(now.Add(4*time.Minute + 2*time.Second))
	_, notice = m.notifications(0)
	if notice != "" {
		t.Fatal("restart replayed warning")
	}
	if _, err = m.cancel(); err != nil {
		t.Fatal(err)
	}
	_, notice = m.notifications(0)
	if notice != "\n***\n*** --- SHUTDOWN ABORTED ---\n***\n" {
		t.Fatal(notice)
	}
	m.due(now.Add(6 * time.Minute))
	if fired != 0 {
		t.Fatal("cancelled reload fired")
	}
}
