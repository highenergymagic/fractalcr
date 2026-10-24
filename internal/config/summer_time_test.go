package config

import (
	"strings"
	"testing"
	"time"
)

func TestSummerTimeTransitions(t *testing.T) {
	summer, err := ParseSummerTime("PDT", "recurring", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := Clock{Zone: "PST", OffsetMinutes: -480, Summer: summer}
	for _, tt := range []struct {
		utc, zone string
		hour      int
	}{{"2026-03-08T09:59:59Z", "PST", 1}, {"2026-03-08T10:00:00Z", "PDT", 3}, {"2026-11-01T08:59:59Z", "PDT", 1}, {"2026-11-01T09:00:00Z", "PST", 1}} {
		now, _ := time.Parse(time.RFC3339, tt.utc)
		got := c.LocalTime(now)
		zone, _ := got.Zone()
		if zone != tt.zone || got.Hour() != tt.hour {
			t.Fatal(tt, got)
		}
	}
	summer, err = ParseSummerTime("AEDT", "recurring", strings.Fields("first Sunday October 2:00 first Sunday April 3:00"))
	if err != nil {
		t.Fatal(err)
	}
	c = Clock{Zone: "AEST", OffsetMinutes: 600, Summer: summer}
	for month, want := range map[time.Month]string{time.January: "AEDT", time.July: "AEST", time.December: "AEDT"} {
		now := time.Date(2026, month, 15, 0, 0, 0, 0, time.UTC)
		name, _ := c.LocalTime(now).Zone()
		if name != want {
			t.Fatal(month, name)
		}
		start, end := c.SummerTransitions(now)
		if !end.After(start) {
			t.Fatal(start, end)
		}
		if want == "AEDT" && (now.Before(start) || !now.Before(end)) {
			t.Fatal("active summer period does not contain current time", now, start, end)
		}
	}
	if err = c.Validate(); err != nil {
		t.Fatal(err)
	}
	summer, err = ParseSummerTime("DST", "date", strings.Fields("12 October 2026 2:00 26 April 2027 2:00 30"))
	if err != nil {
		t.Fatal(err)
	}
	c.Summer = summer
	if err = c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{"5 Sunday March 2:00 first Sunday November 2:00", "first Sunday March 24:00 first Sunday November 2:00", "first bad March 2:00 first Sunday November 2:00"} {
		if _, err = ParseSummerTime("DST", "recurring", strings.Fields(args)); err == nil {
			t.Fatal(args)
		}
	}
}
