package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LocalInstants resolves a wall-clock reading against the configured transition
// rules. A skipped reading has no instant; a repeated reading has two. Keeping
// both avoids silently scheduling with today's offset on a different date.
func (c Clock) LocalInstants(wall time.Time) []time.Time {
	wall = time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), time.UTC)
	offsets := []int{c.OffsetMinutes}
	if c.Summer != nil {
		offsets = append(offsets, c.OffsetMinutes+c.Summer.Offset)
	}
	var matches []time.Time
	for _, offset := range offsets {
		instant := wall.Add(-time.Duration(offset) * time.Minute)
		local := c.LocalTime(instant)
		reading := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC)
		if reading.Equal(wall) {
			matches = append(matches, local)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Before(matches[j]) })
	return matches
}

type Clock struct {
	CalendarValid     bool        `json:"calendar_valid,omitempty"`
	SaveIntervalHours int         `json:"save_interval_hours,omitempty"`
	Zone              string      `json:"zone,omitempty"`
	OffsetMinutes     int         `json:"offset_minutes,omitempty"`
	Summer            *SummerTime `json:"summer,omitempty"`
}

func (c Clock) Location() *time.Location {
	if c.Zone == "" {
		return time.UTC
	}
	return time.FixedZone(c.Zone, c.OffsetMinutes*60)
}
func (c Clock) Validate() error {
	if c.CalendarValid && c.SaveIntervalHours != 0 {
		return fmt.Errorf("calendar authority and NVRAM-only clock persistence require different platforms")
	}
	if c.SaveIntervalHours != 0 && (c.SaveIntervalHours < 8 || c.SaveIntervalHours > 24) {
		return fmt.Errorf("clock save interval must be 8..24 hours")
	}
	if c.Summer != nil {
		s := c.Summer
		mode := "recurring"
		if s.Start.Year != 0 {
			mode = "date"
		}
		args := strings.Fields(s.Start.cli() + " " + s.End.cli() + " " + strconv.Itoa(s.Offset))
		if s.Default {
			args = nil
		}
		parsed, err := ParseSummerTime(s.Zone, mode, args)
		if err != nil {
			return err
		}
		if *parsed != *s {
			return fmt.Errorf("invalid summer-time state")
		}
	}
	if c.Zone == "" {
		if c.OffsetMinutes != 0 {
			return fmt.Errorf("clock offset requires a timezone")
		}
		return nil
	}
	if len(c.Zone) > 7 || c.OffsetMinutes < -1439 || c.OffsetMinutes > 1439 {
		return fmt.Errorf("invalid clock timezone")
	}
	for _, r := range c.Zone {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '+' || r == '_') {
			return fmt.Errorf("invalid timezone name")
		}
	}
	return nil
}
func (c Clock) CLI() string {
	if c.Zone == "" {
		return ""
	}
	minutes := c.OffsetMinutes
	sign := ""
	if minutes < 0 {
		sign = "-"
		minutes = -minutes
	}
	out := fmt.Sprintf("clock timezone %s %s%d", c.Zone, sign, minutes/60)
	if minutes%60 != 0 {
		out += fmt.Sprintf(" %d", minutes%60)
	}
	return out
}
func ParseTimezone(zone, hours, minutes string) (Clock, error) {
	h, e := strconv.Atoi(hours)
	m, f := strconv.Atoi(minutes)
	if e != nil || f != nil || h < -23 || h > 23 || m < 0 || m > 59 {
		return Clock{}, fmt.Errorf("invalid timezone offset")
	}
	offset := h*60 + m
	if strings.HasPrefix(hours, "-") {
		offset = h*60 - m
	}
	c := Clock{Zone: zone, OffsetMinutes: offset}
	return c, c.Validate()
}

func ParseClockTime(w []string, location *time.Location) (time.Time, error) {
	if len(w) != 4 {
		return time.Time{}, fmt.Errorf("Incomplete command.")
	}
	clock := strings.Split(w[0], ":")
	if len(clock) != 3 {
		return time.Time{}, fmt.Errorf("Invalid time")
	}
	h, e := strconv.Atoi(clock[0])
	m, f := strconv.Atoi(clock[1])
	s, g := strconv.Atoi(clock[2])
	if e != nil || f != nil || g != nil || h < 0 || h > 23 || m < 0 || m > 59 || s < 0 || s > 59 {
		return time.Time{}, fmt.Errorf("Invalid time")
	}
	month := time.Month(0)
	day, err := strconv.Atoi(w[1])
	name := w[2]
	if err != nil {
		day, err = strconv.Atoi(w[2])
		name = w[1]
	}
	for n := time.January; n <= time.December; n++ {
		if len(name) >= 3 && strings.HasPrefix(strings.ToLower(n.String()), strings.ToLower(name)) {
			month = n
			break
		}
	}
	year, ye := strconv.Atoi(w[3])
	if err != nil || ye != nil || year < 1993 || year > 2035 || month == 0 || day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("Invalid date")
	}
	t := time.Date(year, month, day, h, m, s, 0, location)
	if t.Month() != month {
		return time.Time{}, fmt.Errorf("Invalid date")
	}
	return t, nil
}
