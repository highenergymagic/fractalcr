package cli

import (
	"fractalcr/internal/config"
	"strconv"
)

func (s *Session) lineTerminalCommand(w []string) bool {
	if len(w) == 0 {
		return false
	}
	if s.Mode != "line-console" && s.Mode != "line-vty" {
		return false
	}
	reset := w[0] == "no" || w[0] == "default"
	defaults := w[0] == "default"
	if reset {
		w = w[1:]
	}
	if len(w) == 0 {
		return false
	}
	if w[0] != "length" && w[0] != "editing" && w[0] != "history" {
		return false
	}
	edit := func(target **config.LineTerminal) {
		t := (*target).Effective()
		switch w[0] {
		case "length":
			t.Length = 24
			if !reset {
				t.Length, _ = strconv.Atoi(w[1])
			}
		case "editing":
			t.Editing = !reset || defaults
		case "history":
			if len(w) > 1 && w[1] == "size" {
				t.HistorySize = 10
				if !reset {
					t.HistorySize, _ = strconv.Atoi(w[2])
				}
			} else {
				t.History = !reset || defaults
			}
		}
		if t == config.DefaultLineTerminal() {
			*target = nil
		} else {
			*target = &t
		}
	}
	if s.Mode == "line-console" {
		edit(&s.Candidate.AAA.ConsoleTerminal)
	} else {
		s.editVTY(func(line *config.VTYLine) { edit(&line.Terminal) })
	}
	return true
}
