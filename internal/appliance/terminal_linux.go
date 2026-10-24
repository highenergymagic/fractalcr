package appliance

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fractalcr/internal/config"
)

// These values are reported by the native terminal, not inferred from command
// text. In particular, a session can change geometry without changing config.
type terminalState struct {
	Length      uint `json:"length"`
	Width       uint `json:"width"`
	HistorySize uint `json:"history_size"`
	History     bool `json:"history"`
	Editing     bool `json:"editing"`
	Monitor     bool `json:"monitor"`
	TX          uint `json:"tx"`
	RX          uint `json:"rx"`
}

func defaultTerminal() terminalState {
	return terminalState{Length: 24, Width: 80, HistorySize: 10, History: true, Editing: true}
}

// Push line-policy changes as objects. Unchanged policy fields must not reset
// a user's EXEC terminal overrides on every heartbeat or unrelated edit.
func (s *loginSession) syncLineTerminal() *terminalState {
	t := s.cli.Running.AAA.Terminal(terminalLine(s))
	old, initial := s.lineTerminal, !s.lineTerminalSet
	if !initial && old == t {
		return nil
	}
	if initial || old.Length != t.Length {
		s.terminal.Length = uint(t.Length)
	}
	if initial || old.HistorySize != t.HistorySize {
		s.terminal.HistorySize = uint(t.HistorySize)
	}
	if initial || old.History != t.History {
		s.terminal.History = t.History
	}
	if initial || old.Editing != t.Editing {
		s.terminal.Editing = t.Editing
	}
	s.lineTerminal, s.lineTerminalSet = t, true
	state := s.terminal
	return &state
}

func terminalLine(s *loginSession) int {
	if s.transport == "ssh" {
		return s.vty + 1
	}
	return 0
}

func lineSession(sessions map[int32]*loginSession, number int) *loginSession {
	for _, s := range sessions {
		if s.authenticated && terminalLine(s) == number {
			return s
		}
	}
	return nil
}

func terminalDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

func terminalDisplay(s *loginSession, number int, c config.Config, now time.Time) string {
	state := defaultTerminal()
	policy := c.AAA.Terminal(number)
	state.Length, state.HistorySize = uint(policy.Length), uint(policy.HistorySize)
	state.History, state.Editing = policy.History, policy.Editing
	if s != nil {
		state = s.terminal
	}
	timeout := c.AAA.ConsoleTimeout
	transports := "none"
	if number > 0 {
		timeout = c.AAA.Line(number - 1).Timeout
		if c.AAA.Line(number - 1).SSH {
			transports = "ssh"
		}
	}
	idle := "never"
	if timeout > 0 {
		idle = terminalDuration(time.Duration(timeout) * time.Second)
	}
	enabled := func(v bool) string {
		if v {
			return "enabled"
		}
		return "disabled"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Line %d, Location: \"\", Type: \"\"\nLength: %d lines, Width: %d columns\n", number, state.Length, state.Width)
	if number == 0 && (state.TX != 0 || state.RX != 0) {
		fmt.Fprintf(&b, "Baud rate (TX/RX) is %d/%d\n", state.TX, state.RX)
	}
	status := "Ready"
	if s != nil && s.authenticated {
		status += ", Active"
	}
	fmt.Fprintf(&b, "Status: %s\n", status)
	caps := []string{}
	if s != nil && s.cli.Privilege > 1 {
		caps = append(caps, "Enabled")
	}
	if state.Monitor {
		caps = append(caps, "Monitor")
	}
	if len(caps) == 0 {
		caps = append(caps, "none")
	}
	fmt.Fprintf(&b, "Capabilities: %s\n", strings.Join(caps, ", "))
	b.WriteString("Modem state: Ready\nSpecial Chars: Escape  Hold  Stop  Start  Disconnect  Activation\n                none  none   -     -       none\n")
	fmt.Fprintf(&b, "Timeouts:      Idle EXEC    Idle Session   Modem Answer  Session   Dispatch\n               %-12s never          none          none      not set\n", idle)
	b.WriteString("Session limit is not set.\n")
	if s != nil && !s.activated.IsZero() {
		fmt.Fprintf(&b, "Time since activation: %s\n", terminalDuration(now.Sub(s.activated)))
	}
	fmt.Fprintf(&b, "Editing is %s.\nHistory is %s, history size is %d.\n", enabled(state.Editing), enabled(state.History), state.HistorySize)
	fmt.Fprintf(&b, "Allowed input transports are %s.\nAllowed output transports are none.\nPreferred transport is none.\n", transports)
	b.WriteString("No output characters are padded\nNo special data dispatching characters\n")
	return b.String()
}

func showLine(words []string, sessions map[int32]*loginSession, current *loginSession, uses [config.VTYCount + 1]uint64, now time.Time) string {
	if len(words) > 2 {
		n, _ := strconv.Atoi(words[len(words)-1])
		if words[2] == "vty" {
			n++
		}
		return terminalDisplay(lineSession(sessions, n), n, current.cli.Running, now)
	}
	var b strings.Builder
	b.WriteString("   Tty Typ     Tx/Rx    A Modem  Roty AccO AccI   Uses   Noise  Overruns   Int\n")
	for n := 0; n <= config.VTYCount; n++ {
		kind := "VTY"
		rate := ""
		marker := " "
		if n == 0 {
			kind = "CTY"
		}
		if s := lineSession(sessions, n); s != nil {
			if s == current {
				marker = "*"
			}
			if n == 0 && s.terminal.TX != 0 {
				rate = fmt.Sprintf("%d/%d", s.terminal.TX, s.terminal.RX)
			}
		}
		fmt.Fprintf(&b, "%s%5d %s %11s  -    -     -    -    - %6d       -       -/-     -\n", marker, n, kind, rate, uses[n])
	}
	return b.String()
}
