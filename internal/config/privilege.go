package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// CommandPrivilege names a parser node, not an executable command or shell
// fragment. Argument nodes apply to every value accepted by that node.
type CommandPrivilege struct {
	Mode    string `json:"mode"`
	Command string `json:"command"`
	Level   int    `json:"level"`
}

func validateCommandPrivileges(rules []CommandPrivilege) error {
	if len(rules) > 16384 {
		return errors.New("too many command privilege entries")
	}
	seen := map[string]bool{}
	for _, r := range rules {
		switch r.Mode {
		case "exec", "configure", "interface", "line", "container", "network", "storage-datastore":
		default:
			return errors.New("invalid command privilege mode")
		}
		key := r.Mode + "\x00" + r.Command
		if seen[key] || r.Level < 0 || r.Level > 15 || len(r.Command) == 0 || len(r.Command) > 4096 || strings.ContainsAny(r.Command, "\x00\r\n\t") || strings.Join(strings.Fields(r.Command), " ") != r.Command {
			return errors.New("invalid command privilege entry")
		}
		seen[key] = true
	}
	return nil
}

func (a AAA) PrivilegeCLI() []string {
	rules := append([]CommandPrivilege(nil), a.CommandPrivileges...)
	// Descendants first: replaying a command also sets its ancestors. Explicit
	// parent assignments must therefore be restored last.
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Mode != rules[j].Mode {
			return rules[i].Mode < rules[j].Mode
		}
		if x, y := strings.Count(rules[i].Command, " "), strings.Count(rules[j].Command, " "); x != y {
			return x > y
		}
		return rules[i].Command < rules[j].Command
	})
	var out []string
	for _, r := range rules {
		out = append(out, fmt.Sprintf("privilege %s level %d %s", r.Mode, r.Level, r.Command))
	}
	return out
}
