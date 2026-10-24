package cli

import (
	"fmt"
	"fractalcr/internal/config"
	"strings"
)

func (s *Session) interfaceConfigurationDisplay(words []string) (string, error) {
	numbered := words[len(words)-1] == "linenum"
	if numbered {
		words = words[:len(words)-1]
	}
	name := strings.Join(words, "")
	if s.InterfaceName != nil {
		var err error
		name, err = s.InterfaceName(name)
		if err != nil {
			return "", err
		}
	} else {
		matched := ""
		for key := range s.Running.AllInterfaces() {
			cut, full := strings.IndexAny(name, "0123456789"), strings.IndexAny(key, "0123456789")
			if strings.EqualFold(name, key) || (cut > 0 && full > 0 && strings.HasPrefix(strings.ToLower(key[:full]), strings.ToLower(name[:cut])) && key[full:] == name[cut:]) {
				if matched != "" {
					return "", fmt.Errorf("Ambiguous interface name")
				}
				matched = key
			}
		}
		if matched == "" {
			return "", fmt.Errorf("Invalid interface type and number")
		}
		name = matched
	}
	_, exists := s.Running.AllInterfaces()[name]
	if !exists && s.InterfaceNames != nil {
		for _, candidate := range s.InterfaceNames() {
			if candidate == name {
				exists = true
				break
			}
		}
	}
	if !exists {
		return "", fmt.Errorf("Invalid interface type and number")
	}
	var b strings.Builder
	b.WriteString("!\n")
	active, found := false, false
	for _, line := range strings.Split(s.configurationText(s.Running), "\n") {
		if line == "interface "+name {
			active, found = true, true
			b.WriteString(line + "\n")
			continue
		}
		if active {
			if line == " exit" || !strings.HasPrefix(line, " ") {
				break
			}
			b.WriteString(line + "\n")
		}
	}
	// An existing unconfigured port has no non-default settings. A restricted
	// session must not regain a hidden interface through this narrower view.
	view := *s
	view.Mode = "config"
	if !found && view.Allowed([]string{"interface", name}) {
		b.WriteString("interface " + name + "\n")
	}
	b.WriteString("end\n")
	return runningConfigurationDisplay(b.String(), numbered), nil
}

func runningConfigurationDisplay(text string, numbered bool) string {
	header := fmt.Sprintf("Building configuration...\n\nCurrent configuration : %d bytes\n", len(text))
	if !numbered {
		return header + text
	}
	var b strings.Builder
	b.WriteString(header)
	for index, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		fmt.Fprintf(&b, " %d : %s\n", index+1, line)
	}
	return b.String()
}

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
		if len(words) > 1 && (words[1] == "console" || words[1] == "con") {
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
	if len(words) > 1 && ((words[0] == "write" && (words[1] == "memory" || words[1] == "terminal")) || (words[0] == "show" && (words[1] == "startup-config" || words[1] == "running-config"))) {
		return true
	}
	for i, word := range words {
		if word == "|" && i+1 < len(words) {
			return words[i+1] == "redirect" || words[i+1] == "append" || words[i+1] == "tee"
		}
	}
	return false
}
