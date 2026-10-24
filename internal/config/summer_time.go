package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ClockTransition struct {
	Week   int        `json:"week,omitempty"`
	Day    int        `json:"day"`
	Month  time.Month `json:"month"`
	Year   int        `json:"year,omitempty"`
	Minute int        `json:"minute"`
}
type SummerTime struct {
	Zone    string          `json:"zone"`
	Offset  int             `json:"offset"`
	Default bool            `json:"default,omitempty"`
	Start   ClockTransition `json:"start"`
	End     ClockTransition `json:"end"`
}

func clockMonth(word string) time.Month {
	for m := time.January; m <= time.December; m++ {
		if len(word) >= 3 && strings.HasPrefix(strings.ToLower(m.String()), strings.ToLower(word)) {
			return m
		}
	}
	return 0
}
func parseTransition(w []string, recurring bool) (ClockTransition, error) {
	r := ClockTransition{}
	if len(w) != 4 {
		return r, fmt.Errorf("Incomplete summer-time transition")
	}
	hm := strings.Split(w[3], ":")
	if len(hm) != 2 {
		return r, fmt.Errorf("Invalid transition time")
	}
	h, e := strconv.Atoi(hm[0])
	m, f := strconv.Atoi(hm[1])
	if e != nil || f != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return r, fmt.Errorf("Invalid transition time")
	}
	r.Minute = h*60 + m
	if recurring {
		switch strings.ToLower(w[0]) {
		case "first":
			r.Week = 1
		case "last":
			r.Week = -1
		default:
			r.Week, e = strconv.Atoi(w[0])
			if e != nil || r.Week < 1 || r.Week > 4 {
				return r, fmt.Errorf("Invalid transition week")
			}
		}
		r.Day = -1
		for d := time.Sunday; d <= time.Saturday; d++ {
			if len(w[1]) >= 3 && strings.HasPrefix(strings.ToLower(d.String()), strings.ToLower(w[1])) {
				r.Day = int(d)
				break
			}
		}
		r.Month = clockMonth(w[2])
		if r.Day < 0 || r.Month == 0 {
			return r, fmt.Errorf("Invalid transition weekday or month")
		}
	} else {
		r.Day, e = strconv.Atoi(w[0])
		r.Month = clockMonth(w[1])
		if e != nil {
			r.Month = clockMonth(w[0])
			r.Day, e = strconv.Atoi(w[1])
		}
		r.Year, f = strconv.Atoi(w[2])
		if e != nil || f != nil || r.Year < 1993 || r.Year > 2035 || r.Month == 0 || r.Day < 1 || r.Day > 31 || time.Date(r.Year, r.Month, r.Day, 0, 0, 0, 0, time.UTC).Month() != r.Month {
			return r, fmt.Errorf("Invalid transition date")
		}
	}
	return r, nil
}

func ParseSummerTime(zone, mode string, args []string) (*SummerTime, error) {
	if zone == "" {
		return nil, fmt.Errorf("Summer-time zone is required")
	}
	if err := (Clock{Zone: zone}).Validate(); err != nil {
		return nil, err
	}
	s := &SummerTime{Zone: zone, Offset: 60}
	if mode != "recurring" && mode != "date" {
		return nil, fmt.Errorf("Invalid summer-time mode")
	}
	if len(args) == 0 && mode == "recurring" {
		s.Default = true
		s.Start = ClockTransition{Week: 2, Day: 0, Month: 3, Minute: 120}
		s.End = ClockTransition{Week: 1, Day: 0, Month: 11, Minute: 120}
		return s, nil
	}
	if len(args) != 8 && len(args) != 9 {
		return nil, fmt.Errorf("Both summer-time transitions are required")
	}
	var err error
	s.Start, err = parseTransition(args[:4], mode == "recurring")
	if err != nil {
		return nil, err
	}
	s.End, err = parseTransition(args[4:8], mode == "recurring")
	if err != nil {
		return nil, err
	}
	if len(args) == 9 {
		s.Offset, err = strconv.Atoi(args[8])
		if err != nil || s.Offset < 1 || s.Offset > 1440 {
			return nil, fmt.Errorf("Summer-time offset must be 1..1440 minutes")
		}
	}
	if mode == "date" && !s.End.at(s.End.Year, time.UTC).After(s.Start.at(s.Start.Year, time.UTC)) {
		return nil, fmt.Errorf("Summer-time end must follow start")
	}
	return s, nil
}

func (r ClockTransition) at(year int, zone *time.Location) time.Time {
	day := r.Day
	if r.Year != 0 {
		year = r.Year
	} else {
		first := time.Date(year, r.Month, 1, 0, 0, 0, 0, zone)
		if r.Week == -1 {
			last := first.AddDate(0, 1, -1)
			day = last.Day() - (int(last.Weekday())-r.Day+7)%7
		} else {
			day = 1 + (r.Day-int(first.Weekday())+7)%7 + (r.Week-1)*7
		}
	}
	return time.Date(year, r.Month, day, r.Minute/60, r.Minute%60, 0, 0, zone)
}
func (r ClockTransition) cli() string {
	clock := fmt.Sprintf("%d:%02d", r.Minute/60, r.Minute%60)
	if r.Year != 0 {
		return fmt.Sprintf("%d %s %d %s", r.Day, r.Month.String(), r.Year, clock)
	}
	week := strconv.Itoa(r.Week)
	if r.Week == -1 {
		week = "last"
	}
	return fmt.Sprintf("%s %s %s %s", week, time.Weekday(r.Day).String(), r.Month.String(), clock)
}
func (s SummerTime) CLI() string {
	mode := "recurring"
	if s.Start.Year != 0 {
		mode = "date"
	}
	out := "clock summer-time " + s.Zone + " " + mode
	if !s.Default {
		out += " " + s.Start.cli() + " " + s.End.cli()
		if s.Offset != 60 {
			out += fmt.Sprintf(" %d", s.Offset)
		}
	}
	return out
}
func (c Clock) LocalTime(now time.Time) time.Time {
	standard := c.Location()
	if c.Summer == nil {
		return now.In(standard)
	}
	s := c.Summer
	summer := time.FixedZone(s.Zone, (c.OffsetMinutes+s.Offset)*60)
	year := now.In(standard).Year()
	start, end := s.Start.at(year, standard), s.End.at(year, summer)
	active := !now.Before(start) && now.Before(end)
	if s.Start.Year == 0 && end.Before(start) {
		active = !now.Before(start) || now.Before(end)
	}
	if active {
		return now.In(summer)
	}
	return now.In(standard)
}

// SummerTransitions returns the current or next configured summer-time period.
// The start is expressed in standard time and the end in summer time, as in
// the configuration grammar. Dated rules retain their explicitly selected year.
func (c Clock) SummerTransitions(now time.Time) (time.Time, time.Time) {
	if c.Summer == nil {
		return time.Time{}, time.Time{}
	}
	s := c.Summer
	standard := c.Location()
	summer := time.FixedZone(s.Zone, (c.OffsetMinutes+s.Offset)*60)
	year := now.In(standard).Year()
	start, end := s.Start.at(year, standard), s.End.at(year, summer)
	if s.Start.Year != 0 {
		return start, end
	}
	if !end.After(start) {
		if now.Before(end) {
			start = s.Start.at(year-1, standard)
		} else {
			end = s.End.at(year+1, summer)
		}
	} else if !now.Before(end) {
		start, end = s.Start.at(year+1, standard), s.End.at(year+1, summer)
	}
	return start, end
}
