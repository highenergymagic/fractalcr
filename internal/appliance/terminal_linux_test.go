package appliance

import (
	"strings"
	"testing"
	"time"

	"fractalcr/internal/cli"
	"fractalcr/internal/config"
)

func TestTerminalDisplayUsesSessionState(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	c := config.Default()
	c.AAA.ConsoleTimeout = 605
	s := &loginSession{cli: &cli.Session{Privilege: 15}, authenticated: true, transport: "console", activated: now.Add(-65 * time.Second), terminal: defaultTerminal()}
	s.terminal.Length = 0
	s.terminal.Width = 132
	s.terminal.History = false
	s.terminal.HistorySize = 27
	s.terminal.Editing = false
	s.terminal.TX = 115200
	s.terminal.RX = 115200
	got := terminalDisplay(s, 0, c, now)
	want := "Line 0, Location: \"\", Type: \"\"\nLength: 0 lines, Width: 132 columns\nBaud rate (TX/RX) is 115200/115200\nStatus: Ready, Active\nCapabilities: Enabled\nModem state: Ready\nSpecial Chars: Escape  Hold  Stop  Start  Disconnect  Activation\n                none  none   -     -       none\nTimeouts:      Idle EXEC    Idle Session   Modem Answer  Session   Dispatch\n               00:10:05     never          none          none      not set\nSession limit is not set.\nTime since activation: 00:01:05\nEditing is disabled.\nHistory is disabled, history size is 27.\nAllowed input transports are none.\nAllowed output transports are none.\nPreferred transport is none.\nNo output characters are padded\nNo special data dispatching characters\n"
	if got != want {
		t.Fatalf("terminal output:\n%q\nwant:\n%q", got, want)
	}
	c.AAA.VTY = map[int]config.VTYLine{4: {SSH: true, Timeout: 0}}
	s.transport = "ssh"
	s.vty = 4
	out := terminalDisplay(s, 5, c, now)
	if strings.Contains(out, "Baud rate") || !strings.Contains(out, "Allowed input transports are ssh.") || !strings.Contains(out, "Line 5,") {
		t.Fatal(out)
	}
}

func TestUsersAllWideAndLineSelection(t *testing.T) {
	now := time.Now()
	c := config.Default()
	s := &loginSession{cli: &cli.Session{Running: c, Privilege: 1}, authenticated: true, transport: "ssh", vty: 2, user: "administrator-long", peer: "[2001:db8::1]:456", last: now, terminal: defaultTerminal()}
	sessions := map[int32]*loginSession{42: s}
	normal := sessionUsersOptions(sessions, s, now, false, false)
	if strings.Contains(normal, s.user) || !strings.Contains(normal, "administra") || !strings.Contains(normal, "2001:db8::1") || strings.Contains(normal, ":456") {
		t.Fatal(normal)
	}
	all := sessionUsersOptions(sessions, s, now, true, true)
	if !strings.Contains(all, s.user) || !strings.Contains(all, "0 con 0") || !strings.Contains(all, "16 vty 15") || strings.Count(all, "vty 2") != 1 {
		t.Fatal(all)
	}
	for _, words := range [][]string{{"show", "line", "3"}, {"show", "line", "vty", "2"}} {
		out := showLine(words, sessions, s, [config.VTYCount + 1]uint64{}, now)
		if !strings.HasPrefix(out, "Line 3,") || !strings.Contains(out, "Status: Ready, Active") {
			t.Fatal(out)
		}
	}
}

func TestTerminalMetadataValidation(t *testing.T) {
	for _, state := range []terminalState{{Width: 0}, {Width: 513}, {Width: 80, Length: 513}, {Width: 80, HistorySize: 257}} {
		if (managementRequest{Terminal: &state}).validate() == nil {
			t.Fatalf("accepted %+v", state)
		}
	}
}
