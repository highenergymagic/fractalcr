package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

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
