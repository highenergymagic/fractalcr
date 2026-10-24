package cli

import (
	"fractalcr/internal/config"
	"strings"
)

// The configuration model renders CLI commands; the same parser policy used
// to edit those commands controls their visibility. This never executes a
// command or reads a host utility's output. Unknown rendered commands fail
// closed at restricted levels.
func (s *Session) configurationText(c config.Config) string {
	text := c.CLI()
	if !s.EnforcePrivilege || s.Privilege == 15 {
		return text
	}
	roots := map[string]*node{}
	allowed := func(mode string, words []string) bool {
		n := roots[mode]
		if n == nil {
			view := *s
			view.Mode = mode
			n = view.policyGrammar()
			roots[mode] = n
		}
		for _, word := range words {
			next, kind := n.match(word)
			if kind != "" || next.level > s.Privilege {
				return false
			}
			n = next
		}
		return n.terminal
	}
	mode, visible := "config", true
	var output []string
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		command := strings.TrimSpace(line)
		if command == "" || command == "!" {
			if visible {
				output = append(output, line)
			}
			continue
		}
		if command == "end" {
			output = append(output, line)
			continue
		}
		if strings.HasPrefix(line, " ") {
			if command == "exit" {
				if visible {
					output = append(output, line)
				}
				mode, visible = "config", true
				continue
			}
			if visible {
				if words, err := Words(command); err == nil && allowed(mode, words) {
					output = append(output, line)
				}
			}
			continue
		}
		words, err := Words(command)
		visible = err == nil && allowed("config", words)
		if visible {
			output = append(output, line)
		}
		mode = configurationMode(words)
		if mode == "config" {
			visible = true
		}
	}
	return strings.Join(output, "\n") + "\n"
}

func configurationMode(words []string) string {
	if len(words) == 0 {
		return "config"
	}
	switch words[0] {
	case "interface", "container", "network", "volume":
		return words[0]
	case "line":
		if len(words) > 1 && words[1] == "console" {
			return "line-console"
		}
		return "line-vty"
	case "storage":
		if len(words) > 2 && words[1] == "datastore" {
			return "storage-datastore"
		}
	case "system":
		if len(words) == 2 && words[1] == "container-engine" {
			return "engine"
		}
	}
	return "config"
}

// Directory metadata remains available in user EXEC. Reading or modifying
// file contents requires both the command's level and the file access level.
func fileAccessCommand(words []string) bool {
	if len(words) > 0 && words[0] == "do" {
		words = words[1:]
	}
	if len(words) == 0 {
		return false
	}
	switch words[0] {
	case "copy", "delete", "move", "rename", "mkdir", "rmdir", "more":
		return true
	}
	if len(words) > 1 && ((words[0] == "write" && words[1] == "memory") || (words[0] == "show" && words[1] == "startup-config")) {
		return true
	}
	for i, word := range words {
		if word == "|" && i+1 < len(words) {
			return words[i+1] == "redirect" || words[i+1] == "append" || words[i+1] == "tee"
		}
	}
	return false
}
