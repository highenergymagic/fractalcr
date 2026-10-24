package config

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CommandPrivilege names a parser node, not an executable command or shell
// fragment. All includes descendants and associated configuration submodes;
// explicit node assignments override group inheritance.
type CommandPrivilege struct {
	Mode     string   `json:"mode"`
	Command  string   `json:"command,omitempty"`
	Commands []string `json:"commands,omitempty"`
	Level    int      `json:"level"`
	All      bool     `json:"all,omitempty"`
}

func validateCommandPrivileges(rules []CommandPrivilege) error {
	if len(rules) > 16384 {
		return errors.New("too many command privilege entries")
	}
	seen := map[string]bool{}
	count := 0
	for _, r := range rules {
		switch r.Mode {
		case "exec", "configure", "interface", "line", "container", "network", "storage-datastore", "engine", "volume":
		default:
			return errors.New("invalid command privilege mode")
		}
		commands := []string{r.Command}
		if len(r.Commands) > 0 {
			if r.Command != "" || r.All {
				return errors.New("invalid grouped command privilege entry")
			}
			commands = r.Commands
		}
		count += len(commands)
		if count > 16384 || r.Level < 0 || r.Level > 15 {
			return errors.New("invalid command privilege entry")
		}
		for _, command := range commands {
			key := fmt.Sprintf("%s\x00%s\x00%t", r.Mode, command, r.All)
			if seen[key] || len(command) == 0 || len(command) > 4096 || strings.ContainsAny(command, "\x00\r\n\t") || strings.Join(strings.Fields(command), " ") != command {
				return errors.New("invalid command privilege entry")
			}
			seen[key] = true
		}
	}
	return nil
}

func (a AAA) OrderedPrivileges() []CommandPrivilege {
	var rules []CommandPrivilege
	for _, rule := range a.CommandPrivileges {
		if len(rule.Commands) == 0 {
			rules = append(rules, rule)
		} else {
			for _, command := range rule.Commands {
				rules = append(rules, CommandPrivilege{Mode: rule.Mode, Command: command, Level: rule.Level})
			}
		}
	}
	// Groups establish inheritance first. Explicit descendants precede their
	// parents because replaying a command also assigns its ancestor keywords.
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].All != rules[j].All {
			return rules[i].All
		}
		if rules[i].Mode != rules[j].Mode {
			return rules[i].Mode < rules[j].Mode
		}
		if x, y := strings.Count(rules[i].Command, " "), strings.Count(rules[j].Command, " "); x != y {
			if rules[i].All {
				return x < y
			}
			return x > y
		}
		return rules[i].Command < rules[j].Command
	})
	return rules
}

// Node exceptions share mode and level in storage. This preserves the bounded
// configuration size when a partial reset expands an all-policy, without
// storing thousands of repeated object keys or changing effective authority.
func CompactPrivileges(rules []CommandPrivilege) []CommandPrivilege {
	groups := map[string]CommandPrivilege{}
	var result []CommandPrivilege
	for _, rule := range (AAA{CommandPrivileges: rules}).OrderedPrivileges() {
		if rule.All {
			result = append(result, rule)
			continue
		}
		key := rule.Mode + "\x00" + strconv.Itoa(rule.Level)
		group := groups[key]
		group.Mode, group.Level = rule.Mode, rule.Level
		group.Commands = append(group.Commands, rule.Command)
		groups[key] = group
	}
	for _, key := range Keys(groups) {
		group := groups[key]
		sort.Strings(group.Commands)
		if len(group.Commands) == 1 {
			group.Command, group.Commands = group.Commands[0], nil
		}
		result = append(result, group)
	}
	return result
}

func (a AAA) PrivilegeCLI() []string {
	var out []string
	for _, r := range a.OrderedPrivileges() {
		all := ""
		if r.All {
			all = " all"
		}
		out = append(out, fmt.Sprintf("privilege %s%s level %d %s", r.Mode, all, r.Level, r.Command))
	}
	return out
}

func (a AAA) FileAccessLevel() int {
	if a.FilePrivilege == nil {
		return 15
	}
	return *a.FilePrivilege
}
