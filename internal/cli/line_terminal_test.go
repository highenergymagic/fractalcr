package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestLineTerminalPersistenceAndDefaults(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "line console 0", "no history", "history size 35", "no editing", "length 0")
	want := config.LineTerminal{Length: 0, HistorySize: 35, History: false, Editing: false}
	if got := s.Running.AAA.Terminal(0); got != want {
		t.Fatalf("size reenabled history: %+v", got)
	}
	execute(t, s, "exit", "line vty 0 4", "history size 27", "length 12", "exit", "line vty 2", "no history", "end", "write memory")
	reloaded, err := New(s.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Running.AAA.Terminal(0) != want {
		t.Fatal("console settings lost")
	}
	for i := 0; i < config.VTYCount; i++ {
		got := reloaded.Running.AAA.Terminal(i + 1)
		want := config.DefaultLineTerminal()
		if i < 5 {
			want.HistorySize = 27
			want.Length = 12
		}
		if i == 2 {
			want.History = false
		}
		if got != want {
			t.Fatalf("line %d: %+v want %+v", i, got, want)
		}
	}
	text := reloaded.Running.CLI()
	for _, needle := range []string{"line con 0\n", " length 0\n", " history size 35\n no history\n", "line vty 0 1\n", "line vty 2\n", "line vty 3 4\n"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in %s", needle, text)
		}
	}
	if strings.Contains(text, "exec-timeout 10 0") {
		t.Fatal("default timeout should not be emitted")
	}
	// Replaying displayed policy preserves disabled history independently of size.
	r := session(t)
	execute(t, r, "conf t")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "!" {
			continue
		}
		if _, err := r.Execute(line); err != nil {
			t.Fatalf("replay %q: %v", line, err)
		}
	}
	for n := 0; n <= config.VTYCount; n++ {
		if r.Running.AAA.Terminal(n) != reloaded.Running.AAA.Terminal(n) {
			t.Fatalf("replay line %d", n)
		}
	}
	execute(t, s, "conf t", "line console 0", "default history", "no history size", "default editing", "no length")
	if s.Running.AAA.ConsoleTerminal != nil {
		t.Fatal("defaults not canonicalized")
	}
	before := string(s.Running.JSON())
	for _, line := range []string{"history size 257", "length 513", "length -1", "editing extra"} {
		if _, err := s.Execute(line); err == nil {
			t.Fatalf("accepted %q", line)
		}
		if string(s.Running.JSON()) != before {
			t.Fatal("invalid command changed policy")
		}
	}
}

func TestLineTerminalDelegatedDisplay(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t", "line con 0", "history size 35", "exit", "line vty 0", "history size 27", "exit",
		"file privilege 5", "privilege exec level 5 show running-config", "privilege configure all level 5 line console", "end")
	s.EnforcePrivilege, s.Privilege = true, 5
	out, err := s.Execute("show running-config")
	if err != nil || !strings.Contains(out, "line con 0\n history size 35") || strings.Contains(out, "history size 27") {
		t.Fatalf("canonical console spelling changed visibility: %s %v", out, err)
	}
}
