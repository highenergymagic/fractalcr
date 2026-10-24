package config

import "testing"

func TestLineTerminalValidationAndClone(t *testing.T) {
	c := Default()
	p := DefaultLineTerminal()
	p.HistorySize = 0
	c.AAA.ConsoleTerminal = &p
	c.AAA.VTY = map[int]VTYLine{0: {Terminal: &p}, 1: {Terminal: &p}}
	clone := c.Clone()
	clone.AAA.ConsoleTerminal.HistorySize = 256
	if c.AAA.Terminal(0).HistorySize != 0 {
		t.Fatal("clone aliases console policy")
	}
	if !clone.AAA.Line(0).Equal(clone.AAA.Line(1)) {
		t.Fatal("equal line policies depend on pointer identity")
	}
	for _, bad := range []LineTerminal{{Length: -1}, {Length: 513}, {HistorySize: -1}, {HistorySize: 257}} {
		c.AAA.ConsoleTerminal = &bad
		if c.Validate() == nil {
			t.Fatal("accepted invalid console terminal")
		}
		c.AAA.ConsoleTerminal = nil
		c.AAA.VTY[0] = VTYLine{Terminal: &bad}
		if c.Validate() == nil {
			t.Fatal("accepted invalid VTY terminal")
		}
	}
}
