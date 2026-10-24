package cli

import (
	"fractalcr/internal/config"
	"strings"
)

func privilegeKey(rule config.CommandPrivilege) string {
	key := rule.Mode + "\x00" + rule.Command
	if rule.All {
		key += "\x00all"
	}
	return key
}

// A group is stored as the command the administrator configured. Expansion is
// confined to this parser's nodes; arbitrary values and native client options
// are never interpreted as commands or policy paths.
func privilegeGroup(rule config.CommandPrivilege) map[string]config.CommandPrivilege {
	entries := map[string]config.CommandPrivilege{}
	path, n, err := privilegePath(rule.Mode, strings.Fields(rule.Command))
	if err != nil || len(path) == 0 {
		return entries
	}
	var descend func(string, *node, []string)
	descend = func(mode string, n *node, path []string) {
		if len(path) > 0 {
			r := config.CommandPrivilege{Mode: mode, Command: strings.Join(path, " "), Level: rule.Level}
			entries[privilegeKey(r)] = r
		}
		for _, child := range n.children {
			if child.word != "do" {
				descend(mode, child, append(append([]string(nil), path...), child.word))
			}
		}
	}
	descend(rule.Mode, n, path)
	if rule.Mode == "configure" {
		for _, entry := range []struct{ prefix, mode string }{{"interface", "interface"}, {"line", "line"}, {"container", "container"}, {"network", "network"}, {"storage datastore name", "storage-datastore"}, {"system container-engine", "engine"}, {"volume", "volume"}} {
			p := strings.Join(path, " ")
			if entry.prefix == p || strings.HasPrefix(entry.prefix, p+" ") || strings.HasPrefix(p, entry.prefix+" ") {
				descend(entry.mode, privilegeSession(entry.mode).rawGrammar(), nil)
			}
		}
	}
	return entries
}

func effectivePrivileges(aaa config.AAA) map[string]int {
	rules := map[string]int{}
	for _, rule := range aaa.OrderedPrivileges() {
		if rule.All {
			for key, member := range privilegeGroup(rule) {
				rules[key] = member.Level
			}
		} else {
			rules[privilegeKey(rule)] = rule.Level
		}
	}
	return rules
}

func removeCoveredPrivilegeGroups(entries map[string]config.CommandPrivilege, targets map[string]config.CommandPrivilege) {
	for key, rule := range entries {
		if !rule.All {
			continue
		}
		members := privilegeGroup(rule)
		covered := len(members) != 0
		for member := range members {
			if _, ok := targets[member]; !ok {
				covered = false
				break
			}
		}
		if covered {
			delete(entries, key)
		}
	}
}

// Resetting only part of an all-policy must not resurrect the removed node
// through its ancestor group. Materialize intersecting groups before removing
// those nodes. Explicit exceptions always win over a group's inherited level.
func materializePrivilegeGroups(entries map[string]config.CommandPrivilege, targets map[string]config.CommandPrivilege) {
	groups := config.AAA{}
	before := config.AAA{}
	for _, rule := range entries {
		before.CommandPrivileges = append(before.CommandPrivileges, rule)
		if rule.All {
			groups.CommandPrivileges = append(groups.CommandPrivileges, rule)
		}
	}
	levels := effectivePrivileges(before)
	expanded := map[string]config.CommandPrivilege{}
	for _, rule := range groups.OrderedPrivileges() {
		members := privilegeGroup(rule)
		intersects := false
		for key := range members {
			if _, ok := targets[key]; ok {
				intersects = true
				break
			}
		}
		if !intersects {
			continue
		}
		delete(entries, privilegeKey(rule))
		for key, member := range members {
			expanded[key] = member
		}
	}
	retained := config.AAA{}
	for _, rule := range entries {
		if rule.All {
			retained.CommandPrivileges = append(retained.CommandPrivileges, rule)
		}
	}
	retainedLevels := effectivePrivileges(retained)
	for key, member := range expanded {
		member.Level = levels[key]
		if level, covered := retainedLevels[key]; covered && level == member.Level {
			continue
		}
		if _, exists := entries[key]; !exists {
			entries[key] = member
		}
	}
}

func defaultPrivilegeLevel(mode, command string) int {
	s := privilegeSession(mode)
	root := s.rawGrammar()
	addWorkloadPolicyNodes(root, s.Mode)
	s.applyPrivilegeLevels(root)
	n := root
	for _, word := range strings.Fields(command) {
		var next *node
		for _, child := range n.children {
			if child.word == word {
				next = child
				break
			}
		}
		if next == nil {
			return 15
		}
		n = next
	}
	return n.level
}

// A remaining child assignment would reassign a reset ancestor when replayed.
// Keep that ancestor at its default explicitly, so saving and replaying CLI
// configuration cannot silently broaden the operator's authority.
func retainDefaultPrivilegeParents(entries map[string]config.CommandPrivilege) {
	aaa := config.AAA{}
	for _, rule := range entries {
		aaa.CommandPrivileges = append(aaa.CommandPrivileges, rule)
	}
	levels := effectivePrivileges(aaa)
	for key := range levels {
		mode, command, _ := strings.Cut(key, "\x00")
		words := strings.Fields(command)
		for i := 1; i < len(words); i++ {
			parent := strings.Join(words[:i], " ")
			rule := config.CommandPrivilege{Mode: mode, Command: parent}
			parentKey := privilegeKey(rule)
			if _, exists := levels[parentKey]; exists {
				continue
			}
			rule.Level = defaultPrivilegeLevel(mode, parent)
			entries[parentKey], levels[parentKey] = rule, rule.Level
		}
	}
}
