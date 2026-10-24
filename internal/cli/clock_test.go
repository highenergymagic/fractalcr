package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestClockSaveIntervalGrammarAndPersistence(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "clock save interval 24", "clock timezone PST -8")
	if s.Running.Clock.SaveIntervalHours != 24 || !strings.Contains(s.Running.CLI(), "clock save interval 24\n") {
		t.Fatal(s.Running.CLI())
	}
	for _, cmd := range []string{"clock save interval 7", "clock save interval 25", "clock save interval", "no clock save interval"} {
		if _, err := s.Execute(cmd); err == nil {
			t.Fatal(cmd)
		}
	}
	for _, reset := range []string{"no", "default"} {
		execute(t, s, reset+" clock save interval 24")
		if s.Running.Clock.SaveIntervalHours != 0 || strings.Contains(s.Running.CLI(), "clock save interval") {
			t.Fatal(s.Running.Clock)
		}
		execute(t, s, "clock save interval 8")
	}
	c, err := config.Decode(s.Running.JSON())
	if err != nil || c.Clock.SaveIntervalHours != 8 {
		t.Fatal(c.Clock, err)
	}
	s.HardwareCalendar = func() bool { return true }
	if strings.Contains(s.ContextHelp("clock "), "save") {
		t.Fatal("hardware calendar exposed NVRAM-only command")
	}
	if _, _, err := s.Resolve("clock save interval 24"); err == nil {
		t.Fatal("hardware calendar accepted NVRAM-only command")
	}
	for _, hours := range []int{-1, 1, 7, 25} {
		c.Clock.SaveIntervalHours = hours
		if c.Validate() == nil {
			t.Fatal(hours)
		}
	}
}

func TestClockTimezoneAndDateGrammar(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "clock timezone NST -3 30")
	if s.Running.Clock.OffsetMinutes != -210 || s.Running.Clock.Zone != "NST" {
		t.Fatal(s.Running.Clock)
	}
	before := string(s.Running.JSON())
	for _, cmd := range []string{"clock timezone PST -24", "clock timezone BAD 4 60", "clock timezone too-long 0"} {
		if _, err := s.Execute(cmd); err == nil {
			t.Fatal(cmd)
		}
		if string(s.Running.JSON()) != before {
			t.Fatal("failed timezone edit mutated config")
		}
	}
	if !strings.Contains(s.Running.CLI(), "clock timezone NST -3 30") {
		t.Fatal(s.Running.CLI())
	}
	execute(t, s, "no clock timezone", "end")
	if s.Running.Clock != (config.Clock{}) {
		t.Fatal(s.Running.Clock)
	}
	for _, cmd := range []string{"clock set 08:33:00 24 October 2026", "clock set 08:33:00 October 24 2026", "show line console 0", "show line vty 15", "show users all wide"} {
		if _, _, err := s.Resolve(cmd); err != nil {
			t.Fatal(cmd, err)
		}
	}
	execute(t, s, "conf t", "clock summer-time DST date October 12 2026 2:00 April 26 2027 2:00 30")
	if s.Running.Clock.Summer == nil || s.Running.Clock.Summer.Offset != 30 {
		t.Fatal(s.Running.Clock)
	}
	execute(t, s, "no clock summer-time", "end")
	execute(t, s, "conf t", "clock timezone US-Cent -6", "clock summer-time US-CDT recurring first Sunday March 2:00 last Sunday November 2:00")
	help := s.ContextHelp("clock summer-time US-CDT recurring ")
	if !strings.Contains(help, "<1-4>") || !strings.Contains(help, "first") || !strings.Contains(help, "last") || strings.Contains(help, "<WEEK>") {
		t.Fatal(help)
	}
	help = s.ContextHelp("clock summer-time US-CDT recurring first Sunday March ")
	if strings.Contains(help, "reload") || !strings.Contains(help, "hh:mm") {
		t.Fatal(help)
	}
}

func TestClockEXECReachesNativeOperations(t *testing.T) {
	s := session(t)
	s.EnforcePrivilege, s.Privilege = true, 15
	var calls []string
	s.Platform = func(w []string, c config.Config) (string, error) {
		calls = append(calls, strings.Join(w, " "))
		if c.Clock.Zone != s.Running.Clock.Zone {
			t.Fatal("operational command used candidate state")
		}
		return "native clock operation", nil
	}
	commands := []string{"clock read-calendar", "clock update-calendar", "clock set 12:30:00 24 October 2026", "calendar set 12:30:00 October 24 2026"}
	for _, command := range commands {
		if out, err := s.Execute(command); err != nil || out != "native clock operation" {
			t.Fatal(command, out, err)
		}
	}
	execute(t, s, "conf t", "clock timezone NST -3 30")
	if len(calls) != len(commands) {
		t.Fatal("configuration escaped into operational clock API", calls)
	}
	for _, command := range commands {
		if out, err := s.Execute("do " + command); err != nil || out != "native clock operation" {
			t.Fatal(command, out, err)
		}
	}
	execute(t, s, "end")
	s.Privilege = 1
	for _, command := range commands[:2] {
		if _, err := s.Execute(command); err != nil {
			t.Fatal("calendar transfer unavailable in user EXEC", command, err)
		}
	}
	before := len(calls)
	for _, command := range commands[2:] {
		if _, err := s.Execute(command); err == nil {
			t.Fatal("unprivileged clock mutation", command)
		}
	}
	if len(calls) != before {
		t.Fatal("unauthorized command reached clock API")
	}
	s.Privilege = 0
	for _, command := range commands {
		if _, err := s.Execute(command); err == nil {
			t.Fatal("clock operation available at privilege zero", command)
		}
	}
}

func TestNTPUpdateCalendarConfiguration(t *testing.T) {
	s := session(t)
	if s.Running.NTP.UpdateCalendar || strings.Contains(s.Running.CLI(), "ntp update-calendar") {
		t.Fatal("calendar updates must be opt-in")
	}
	execute(t, s, "conf t", "ntp server time.example.org", "ntp update-calendar", "clock timezone UTC 0", "end")
	if !s.Running.NTP.UpdateCalendar || !strings.Contains(s.Running.CLI(), "ntp update-calendar\n") {
		t.Fatal("calendar policy not applied/rendered")
	}
	for _, reset := range []string{"no ntp update-calendar", "default ntp update-calendar"} {
		execute(t, s, "conf t", reset, "end")
		if s.Running.NTP.UpdateCalendar || len(s.Running.NTP.Servers) != 1 {
			t.Fatal("calendar reset changed server configuration")
		}
		execute(t, s, "conf t", "ntp update-calendar", "end")
	}
}
