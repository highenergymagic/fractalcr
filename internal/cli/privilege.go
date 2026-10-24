package cli

import (
	"errors"
	"fractalcr/internal/config"
	"fractalcr/internal/workloadcatalog"
	"slices"
	"strconv"
	"strings"
)

var privilegeModes = []string{"exec", "configure", "interface", "line", "container", "network", "storage-datastore", "engine", "volume"}

func init() {
	for _, mode := range privilegeModes {
		for _, prefix := range []string{"privilege ", "no privilege "} {
			for _, all := range []string{"", "all "} {
				for _, action := range []string{"level <0-15> ", "reset "} {
					configCommands = append(configCommands, Command{prefix + mode + " " + all + action + "<COMMAND...>", "Set command privilege level", false})
				}
			}
		}
	}
}

func privilegeMode(mode string) string {
	switch mode {
	case "":
		return "exec"
	case "config":
		return "configure"
	case "line-console", "line-vty":
		return "line"
	}
	return mode
}

func privilegeSession(mode string) *Session {
	s := &Session{}
	switch mode {
	case "exec":
	case "configure":
		s.Mode = "config"
	case "line":
		s.Mode = "line-vty"
	default:
		s.Mode = mode
	}
	return s
}

func privilegeHelpSession(mode string) *Session {
	s := privilegeSession(mode)
	s.privilegeTarget = true
	return s
}

// Compute default levels from complete commands. Parameters and continuation
// keywords inherit their command's level; a more privileged child stays gated.
func defaultNodeLevels(n *node, mode string, path []string) int {
	level := 15
	if n.terminal {
		level = defaultCommandLevel(mode, path)
	}
	for _, c := range n.children {
		v := defaultNodeLevels(c, mode, append(append([]string(nil), path...), c.word))
		if v < level {
			level = v
		}
	}
	for i, w := range path {
		if w == "|" && i+1 < len(path) && (path[i+1] == "redirect" || path[i+1] == "append" || path[i+1] == "tee") {
			level = 15
		}
	}
	n.defaultLevel = level
	return level
}

func (s *Session) policyGrammar() *node {
	root := s.rawGrammar()
	if s.privilegeTarget {
		addWorkloadPolicyNodes(root, s.Mode)
	}
	return s.applyPrivilegeLevels(root)
}

func (s *Session) applyPrivilegeLevels(root *node) *node {
	defaultNodeLevels(root, s.Mode, nil)
	rules := map[string]int{}
	for _, r := range s.Running.AAA.CommandPrivileges {
		rules[r.Mode+"\x00"+r.Command] = r.Level
	}
	var apply func(*node, []string, int, int)
	apply = func(n *node, path []string, parentDefault, parentLevel int) {
		n.level = n.defaultLevel
		if len(path) > 1 && parameter(n) && n.defaultLevel <= parentDefault {
			n.level = parentLevel
		}
		// Read-only output modifiers share the display command's level. File
		// destinations retain a separate level and the file-access gate.
		for i, word := range path {
			if word == "|" && (i == len(path)-1 || (path[i+1] != "redirect" && path[i+1] != "append" && path[i+1] != "tee")) {
				n.level = parentLevel
				break
			}
		}
		mode, command := privilegeMode(s.Mode), path
		if len(path) > 0 && path[0] == "do" {
			mode, command = "exec", path[1:]
		}
		if level, ok := rules[mode+"\x00"+strings.Join(command, " ")]; ok {
			n.level = level
		}
		for _, c := range n.children {
			apply(c, append(append([]string(nil), path...), c.word), n.defaultLevel, n.level)
		}
	}
	apply(root, nil, 0, 0)
	return root
}

func prunePrivileges(n *node, level int) {
	children := n.children[:0]
	for _, c := range n.children {
		if c.level <= level {
			prunePrivileges(c, level)
			children = append(children, c)
		}
	}
	n.children = children
}

func (s *Session) Allowed(words []string) bool {
	if !s.EnforcePrivilege {
		return true
	}
	if s.Privilege < 0 || s.Privilege > 15 {
		return false
	}
	if len(words) == 0 {
		return true
	}
	if fileAccessCommand(words) && s.Privilege < s.Running.AAA.FileAccessLevel() {
		return false
	}
	if _, _, ok := workloadInput(words); ok {
		return s.workloadPolicyAllows(words)
	}
	n := s.policyGrammar()
	for _, word := range words {
		next, kind := n.match(word)
		if kind != "" {
			if s.Mode != "" && s.Mode != "config" {
				parent := *s
				parent.Mode = "config"
				return parent.Allowed(words)
			}
			return false
		}
		if next.level > s.Privilege {
			return false
		}
		n = next
	}
	return n.terminal
}

// Dialogs and detached operations always authorize against EXEC and current
// running policy, even when they were entered with `do` from a submode.
func (s *Session) AllowedEXEC(words []string) bool {
	exec := *s
	exec.Mode, exec.EnforcePrivilege = "", true
	return exec.Allowed(words)
}

type privilegeRequest struct {
	mode         string
	all, reset   bool
	level, start int
}

func parsePrivilege(w []string) (privilegeRequest, bool) {
	r := privilegeRequest{}
	i := 0
	if len(w) > 0 && w[0] == "no" {
		r.reset = true
		i++
	}
	if len(w) <= i || w[i] != "privilege" {
		return r, false
	}
	i++
	if len(w) <= i || !slices.Contains(privilegeModes, w[i]) {
		return r, false
	}
	r.mode = w[i]
	i++
	if len(w) > i && w[i] == "all" {
		r.all = true
		i++
	}
	if len(w) <= i {
		return r, false
	}
	if w[i] == "reset" {
		r.reset = true
		i++
	} else if w[i] == "level" && len(w) > i+1 {
		level, err := strconv.Atoi(w[i+1])
		if err != nil || level < 0 || level > 15 {
			return r, false
		}
		r.level = level
		i += 2
	} else {
		return r, false
	}
	r.start = i
	return r, true
}

func privilegePath(mode string, words []string) ([]string, *node, error) {
	n := privilegeSession(mode).rawGrammar()
	addWorkloadPolicyNodes(n, privilegeSession(mode).Mode)
	var path []string
	for index, word := range words {
		if n.rest {
			return nil, nil, errors.New("Privilege target extends beyond the command parser")
		}
		var next *node
		for _, c := range n.children {
			if word == c.word {
				next = c
				break
			}
		}
		if next == nil {
			var kind string
			next, kind = n.match(word)
			if kind != "" {
				return nil, nil, &SyntaxError{Kind: kind, Input: strings.Join(words, " "), Column: index}
			}
			if parameter(next) {
				return nil, nil, errors.New("Privilege target must name a command keyword or parser argument")
			}
		}
		if next.word == "do" {
			return nil, nil, errors.New("Use privilege exec for EXEC commands")
		}
		path = append(path, next.word)
		n = next
	}
	return path, n, nil
}

// Workload operands and options must not choose an authorization rule. The
// linked client's typed parser supplies the actual operation path, including
// subcommands after global options; aliases keep their own appliance prefix.
func (s *Session) workloadPolicyAllows(words []string) bool {
	if !s.EnforcePrivilege {
		return true
	}
	service, args, ok := workloadInput(words)
	if !ok {
		return false
	}
	p, _, _, kind := walkWorkload(service, args)
	if kind != "" {
		return false
	}
	w := words
	if w[0] == "do" {
		w = w[1:]
	}
	path := append([]string(nil), w[:2]...)
	parts := p.path
	if w[1] != "container-engine" && w[1] != "container-build" && w[1] != "buildkit" && len(parts) > 0 {
		parts = parts[1:]
	}
	path = append(path, parts...)
	exec := *s
	exec.Mode = ""
	root := exec.rawGrammar()
	addWorkloadPolicyNodes(root, "")
	n := exec.applyPrivilegeLevels(root)
	for _, word := range path {
		next, kind := n.match(word)
		if kind != "" || next.level > s.Privilege {
			return false
		}
		n = next
	}
	return true
}

func addWorkloadPolicyNodes(root *node, mode string) {
	prefix := "request "
	if mode != "" {
		prefix = "do request "
	}
	var add func(string, *workloadcatalog.Command)
	add = func(path string, command *workloadcatalog.Command) {
		root.add(Command{path, command.Description, false})
		for i := range command.Children {
			child := &command.Children[i]
			if !child.Hidden && child.Name != "help" {
				add(path+" "+child.Name, child)
			}
		}
	}
	add(prefix+"container-engine", &workloadRoot)
	add(prefix+"container-build", &buildkitRoot)
	add(prefix+"buildkit", &buildkitRoot)
	for _, name := range []string{"compose", "buildx", "swarm", "service", "stack", "node", "secret", "registry", "context"} {
		for i := range workloadRoot.Children {
			if workloadRoot.Children[i].Name == name {
				add(prefix+name, &workloadRoot.Children[i])
			}
		}
	}
}

func (s *Session) resolvePrivilege(w []string, line string, spans []int) ([]string, error) {
	r, ok := parsePrivilege(w)
	if !ok {
		return w, nil
	}
	if len(w) == r.start {
		return nil, &SyntaxError{Kind: "incomplete"}
	}
	path, _, err := privilegePath(r.mode, w[r.start:])
	if err != nil {
		if syntax, ok := err.(*SyntaxError); ok {
			syntax.Input = line
			if position := r.start + syntax.Column; position < len(spans) {
				syntax.Column = spans[position]
			}
		}
		return nil, err
	}
	return append(w[:r.start:r.start], path...), nil
}

func (s *Session) privilegeCommand(w []string) (bool, error) {
	r, ok := parsePrivilege(w)
	if !ok {
		return false, nil
	}
	path, n, err := privilegePath(r.mode, w[r.start:])
	if err != nil {
		return true, err
	}
	entries := map[string]config.CommandPrivilege{}
	for _, rule := range s.Candidate.AAA.CommandPrivileges {
		entries[rule.Mode+"\x00"+rule.Command] = rule
	}
	set := func(mode string, path []string) {
		command := strings.Join(path, " ")
		key := mode + "\x00" + command
		if r.reset {
			delete(entries, key)
		} else {
			entries[key] = config.CommandPrivilege{Mode: mode, Command: command, Level: r.level}
		}
	}
	for i := 1; i <= len(path); i++ {
		set(r.mode, path[:i])
	}
	var descend func(string, *node, []string)
	descend = func(mode string, n *node, path []string) {
		if len(path) > 0 {
			set(mode, path)
		}
		for _, c := range n.children {
			if c.word != "do" {
				descend(mode, c, append(append([]string(nil), path...), c.word))
			}
		}
	}
	if r.all {
		descend(r.mode, n, path)
		if r.mode == "configure" {
			// The parser's configuration entry commands select these submodes.
			for _, entry := range []struct{ prefix, mode string }{{"interface", "interface"}, {"line", "line"}, {"container", "container"}, {"network", "network"}, {"storage datastore name", "storage-datastore"}, {"system container-engine", "engine"}, {"volume", "volume"}} {
				p := strings.Join(path, " ")
				if entry.prefix == p || strings.HasPrefix(entry.prefix, p+" ") || strings.HasPrefix(p, entry.prefix+" ") {
					descend(entry.mode, privilegeSession(entry.mode).rawGrammar(), nil)
				}
			}
		}
	}
	s.Candidate.AAA.CommandPrivileges = nil
	for _, key := range config.Keys(entries) {
		s.Candidate.AAA.CommandPrivileges = append(s.Candidate.AAA.CommandPrivileges, entries[key])
	}
	return true, nil
}

func (s *Session) privilegeHelpTarget(line string) (*Session, string, bool) {
	if s.Mode == "" {
		return nil, "", false
	}
	w, spans, err := lex(line)
	if err != nil {
		return nil, "", false
	}
	// Resolve the management prefix with the normal grammar, preserving partial
	// target text so '?' and TAB retain the caller's input buffer.
	n := s.grammar()
	for i, word := range w {
		next, kind := n.match(word)
		if kind != "" {
			return nil, "", false
		}
		if next.word == "<COMMAND...>" {
			r, ok := parsePrivilege(w[:i])
			if !ok {
				return nil, "", false
			}
			return privilegeHelpSession(r.mode), line[spans[i]:], true
		}
		if !parameter(next) {
			w[i] = next.word
		}
		n = next
	}
	if strings.HasSuffix(line, " ") {
		if r, ok := parsePrivilege(w); ok {
			return privilegeHelpSession(r.mode), "", true
		}
	}
	return nil, "", false
}
