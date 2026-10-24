package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Clock struct {
	Zone          string      `json:"zone,omitempty"`
	OffsetMinutes int         `json:"offset_minutes,omitempty"`
	Summer        *SummerTime `json:"summer,omitempty"`
}

func (c Clock) Location() *time.Location {
	if c.Zone == "" {
		return time.UTC
	}
	return time.FixedZone(c.Zone, c.OffsetMinutes*60)
}
func (c Clock) Validate() error {
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
