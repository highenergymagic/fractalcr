package cli

import (
	"fmt"
	"sort"
	"strings"
)

// This grammar is the authority for parsing, word completion and contextual
// help. Descriptions belong to a token, never to a recursively printed command.
type Command struct {
	Syntax, Description string
	Local               bool
}

var execCommands = []Command{
	{"enable", "Enter privileged EXEC", false},
	{"disable", "Return to user EXEC", false},
	{"exit", "Close the EXEC session", false},
	{"logout", "Close the EXEC session", false},
	{"show privilege", "Current privilege level", false},
	{"show users", "Authenticated sessions", false},
	{"request container-engine <ARG...>", "Docker Engine, Compose, Buildx and Swarm native arguments", false},
	{"request container-build <ARG...>", "BuildKit client native arguments", false},
	{"terminal length <0-512>", "Set page length; zero disables pagination", true},
	{"terminal width <40-512>", "Set terminal display width", true},
	{"terminal history size <0-256>", "Set command history size", true},
	{"show version", "Software and platform version", true},
	{"show history", "Recent commands in this terminal session", true},
	{"show terminal", "Terminal display and history settings", true},
	{"show platform", "Platform status", true},
	{"show platform software service", "Supervised service status", true},
	{"show logging", "System event log", true},
	{"show logging <SERVICE>", "Service or kernel log", true},
	{"request platform software service <SERVICE> start", "Start a service", true},
	{"request platform software service <SERVICE> stop", "Stop a service", true},
	{"request platform software service <SERVICE> restart", "Restart a service", true},
	{"request platform diagnostics containers", "Run container diagnostics", true},
	{"request platform debug service-container enter", "Enter the platform debug shell", true},
	{"request platform debug service-container enable", "Enable the platform debug service", false},
	{"request platform debug service-container disable", "Disable and remove the debug container", false},
	{"reload", "Restart the appliance", true},
	{"configure terminal", "Enter configuration mode", false},
	{"show running-config", "Running configuration", false},
	{"show candidate-config", "Staged configuration", false},
	{"show startup-config", "Saved startup configuration", false},
	{"show configuration diff", "Changes between candidate and running configuration", false},
	{"show interfaces", "Interface addresses and state", false},
	{"show ip interface brief", "Interface address summary", false},
	{"show ip route", "IPv4 routing table", false},
	{"show containers", "Container inventory", false},
	{"show container <NAME> detail", "Container configuration and status", false},
	{"show container <NAME> logs", "Recent container logs", false},
	{"show images", "Container images", false},
	{"show volumes", "Container volumes", false},
	{"show storage", "Datastore status", false},
	{"show storage disks", "Available disks", false},
	{"show storage datastore <NAME>", "Datastore usage", false},
	{"show container-isolation", "Container runtime policy", false},
	{"show container-isolation capabilities", "Runtime and hardware capabilities", false},
	{"show container-isolation detail", "Kata runtime profile", false},
	{"show capabilities", "Implemented platform capabilities", false},
	{"show system resources", "CPU, memory and filesystem resources", false},
	{"copy running-config startup-config", "Save running configuration", false},
	{"copy startup-config candidate-config", "Load startup configuration into the candidate", false},
	{"write memory", "Save running configuration", false},
	{"request image pull <IMAGE>", "Pull a container image", false},
	{"request container <NAME> create", "Create a configured container", false},
	{"request container <NAME> start", "Start a container", false},
	{"request container <NAME> stop", "Stop a container", false},
	{"request container <NAME> restart", "Restart a container", false},
	{"request container <NAME> remove", "Remove a container", false},
	{"request volume <NAME> create", "Create a named volume", false},
	{"request volume <NAME> remove", "Remove a named volume", false},
	{"request storage datastore <NAME> format confirm <NAME>", "Initialize explicitly selected unused disks", false},
	{"request system container-isolation install runtime kata-containers", "Select the bundled runtime in candidate configuration", false},
	{"request system container-isolation kata-containers vm-factory create", "VM factory availability", false},
	{"ping <IPv4>", "Send IPv4 echo requests", false},
	{"help", "Description of the interactive help system", false},
}

var configCommands = []Command{
	{"aaa new-model", "Enable local authentication, authorization and accounting", false},
	{"no aaa new-model", "Disable local AAA and SSH", false},
	{"aaa authentication login default local", "Authenticate using local users", false},
	{"aaa authorization exec default local", "Authorize local EXEC privilege", false},
	{"aaa accounting exec default start-stop local", "Record session events locally", false},
	{"aaa accounting commands 15 default start-stop local", "Record administrative command events locally", false},
	{"username <NAME> privilege <0-15> secret <SECRET...>", "Set a local user and hashed secret", false},
	{"no username <NAME>", "Remove a local user", false},
	{"enable secret <SECRET...>", "Set the privileged EXEC secret", false},
	{"ip ssh version 2", "Enable SSH version 2", false},
	{"no ip ssh", "Disable SSH access", false},
	{"line console 0", "Configure console login and timeout", false},
	{"line vty 0 15", "Configure SSH sessions", false},
	{"hostname <NAME>", "Set the appliance hostname", false},
	{"interface <INTERFACE>", "Configure the management interface", false},
	{"no interface <INTERFACE>", "Remove interface configuration", false},
	{"container <NAME>", "Configure a container", false},
	{"no container <NAME>", "Remove a container definition", false},
	{"storage datastore name <NAME>", "Configure a datastore", false},
	{"system container-isolation install runtime kata-containers", "Select the bundled Kata runtime", false},
	{"system container-isolation enable", "Enable container isolation", false},
	{"system container-isolation disable", "Disable container isolation", false},
	{"system container-isolation kata-containers default-runtime clh", "Select Cloud Hypervisor", false},
	{"system container-isolation kata-containers runtime clh default-memory <MEMORY>", "Set guest memory", false},
	{"system container-isolation kata-containers runtime clh vm-factory enable", "VM factory availability", false},
	{"system container-isolation kata-containers runtime clh vm-factory disable", "Disable VM factory", false},
	{"system container-isolation kata-containers apply-to all-containers", "Use Kata by default", false},
	{"system container-isolation kata-containers apply-to explicit", "Use Kata only when selected", false},
	{"system container-isolation kata-containers base-os <GUEST>", "Select a bundled guest OS", false},
	{"commit", "Apply candidate configuration", false},
	{"abort", "Discard candidate configuration and return to EXEC", false},
	{"end", "Return to privileged EXEC", false},
	{"exit", "Leave the current configuration mode", false},
	{"help", "Description of the interactive help system", false},
}

var modeCommands = map[string][]string{
	"line-console":      {"login local", "exec-timeout <MINUTES> <SECONDS>"},
	"line-vty":          {"login local", "exec-timeout <MINUTES> <SECONDS>", "transport input ssh", "transport input none"},
	"interface":         {"ip address <IPv4/PREFIX>", "ip gateway <IPv4>", "ip name-server <IPv4>", "no ip gateway", "no ip name-server"},
	"storage-datastore": {"file-system format btrfs", "preset single", "preset raid1", "disk <DISK>", "disk add <DISK>"},
	"container": {"image <IMAGE>", "command <ARG...>", "environment <VARIABLE> <VALUE>", "publish <PORT-MAPPING>",
		"restart-policy no", "restart-policy always", "restart-policy unless-stopped", "restart-policy on-failure",
		"volume <NAME> <PATH>", "volume <NAME> <PATH> read-only", "memory <MEMORY>", "pids-limit <1-1048576>",
		"read-only", "security no-new-privileges", "capability drop all", "container-isolation inherit",
		"container-isolation kata-containers enable", "no container-isolation", "no volume", "no read-only",
		"no security no-new-privileges", "no capability drop all", "no command", "no environment <VARIABLE>", "no publish"},
}

var tokenHelp = map[string]string{
	"|": "Filter command output", "include": "Include matching lines", "exclude": "Exclude matching lines", "begin": "Begin at the first matching line", "count": "Count matching lines", "<REGEX...>": "POSIX extended regular expression",
	"show": "Display system information", "request": "Perform an operational action", "platform": "Platform services and diagnostics",
	"software": "Platform software", "service": "Supervised services", "debug": "Platform debugging", "diagnostics": "Platform diagnostic tests",
	"ip": "IPv4 information or configuration", "storage": "Datastore and disk management", "datastore": "Persistent datastore",
	"container": "Container configuration and operations", "container-isolation": "Container isolation policy", "system": "System configuration",
	"kata-containers": "Kata Containers runtime", "runtime": "Hypervisor settings", "clh": "Cloud Hypervisor", "vm-factory": "VM factory settings",
	"configuration": "Configuration comparison", "copy": "Copy configuration", "write": "Write persistent configuration", "configure": "Enter configuration mode",
	"no": "Remove a setting or restore its default", "do": "Run an EXEC command", "file-system": "Filesystem settings", "disk": "Whole-disk selection",
	"preset": "Storage redundancy profile", "volume": "Persistent named volume", "environment": "Container environment variable",
	"image": "Container image", "command": "Container command and arguments", "security": "Container security settings", "capability": "Linux capabilities",
	"<NAME>": "Object name (case sensitive)", "<SERVICE>": "Service name or kernel", "<IMAGE>": "Image name, tag or digest",
	"<INTERFACE>": "Network interface name", "<DISK>": "Whole-disk device name", "<MEMORY>": "Memory size, for example 256M",
	"<IPv4>": "IPv4 address", "<IPv4/PREFIX>": "IPv4 address and prefix length", "<VARIABLE>": "Environment variable name",
	"<VALUE>": "Value; quote spaces", "<ARG...>": "Arguments; quotes preserve spaces", "<PATH>": "Absolute path",
	"<PORT-MAPPING>": "HOST:CONTAINER[/tcp|udp]", "<1-1048576>": "Integer from 1 to 1048576", "<GUEST>": "Bundled guest OS name",
}

type node struct {
	word, description     string
	children              []*node
	terminal, local, rest bool
}

func (n *node) add(c Command) {
	if (strings.HasPrefix(c.Syntax, "show ") || strings.HasPrefix(c.Syntax, "do show ")) && !strings.Contains(c.Syntax, " | ") {
		for _, kind := range []string{"include", "exclude", "begin", "count"} {
			n.add(Command{c.Syntax + " | " + kind + " <REGEX...>", "Filter output using a regular expression", c.Local})
		}
	}
	for _, word := range strings.Fields(c.Syntax) {
		var next *node
		for _, child := range n.children {
			if child.word == word {
				next = child
				break
			}
		}
		if next == nil {
			next = &node{word: word, description: tokenHelp[word], rest: strings.HasSuffix(word, "...>")}
			n.children = append(n.children, next)
		}
		n = next
	}
	n.terminal, n.local = true, c.Local
	if c.Description != "" {
		n.description = c.Description
	}
}

func (s *Session) grammar() *node {
	root := &node{}
	if s.Mode == "" {
		for _, c := range execCommands {
			if !s.Allowed(strings.Fields(c.Syntax)) {
				continue
			}
			if s.Recovery && !c.Local {
				continue
			}
			root.add(c)
		}
		if s.Platform == nil && !s.Recovery {
			root.add(Command{"request system render <PATH>", "Export configuration for offline use", false})
		}
	} else {
		for _, c := range configCommands {
			root.add(c)
		}
		for _, pattern := range modeCommands[s.Mode] {
			root.add(Command{pattern, "", false})
		}
		for _, c := range execCommands {
			if !strings.HasPrefix(c.Syntax, "configure ") {
				c.Syntax = "do " + c.Syntax
				root.add(c)
			}
		}
	}
	return root
}

type SyntaxError struct {
	Kind   string
	Column int
	Input  string
}

func (e *SyntaxError) Error() string {
	switch e.Kind {
	case "incomplete":
		return "Incomplete command."
	case "ambiguous":
		return fmt.Sprintf("Ambiguous command: %q", e.Input)
	default:
		return "Invalid input detected at '^' marker."
	}
}

func parameter(n *node) bool { return strings.HasPrefix(n.word, "<") }
func (n *node) match(word string) (*node, string) {
	var matches []*node
	var argument *node
	for _, child := range n.children {
		if parameter(child) {
			argument = child
			continue
		}
		if strings.EqualFold(word, child.word) {
			return child, ""
		}
		if strings.HasPrefix(child.word, strings.ToLower(word)) {
			matches = append(matches, child)
		}
	}
	if len(matches) == 1 {
		return matches[0], ""
	}
	if len(matches) > 1 {
		return nil, "ambiguous"
	}
	if argument != nil {
		return argument, ""
	}
	if n.rest {
		return n, ""
	}
	return nil, "invalid"
}

// Resolve canonicalizes only grammar keywords. User values retain their case,
// punctuation and argument boundaries; no command ever passes through a shell.
func (s *Session) Resolve(line string) ([]string, bool, error) {
	words, spans, err := lex(line)
	if err != nil {
		return nil, false, err
	}
	if len(words) == 0 {
		return nil, false, nil
	}
	// Retain the original appliance's exact shortcut without making "conf t"
	// ambiguous with a second command keyword.
	if len(words) == 1 && strings.EqualFold(words[0], "config") && s.Mode == "" {
		return []string{"configure", "terminal"}, false, nil
	}
	n := s.grammar()
	for i, word := range words {
		next, kind := n.match(word)
		if kind != "" {
			return nil, false, &SyntaxError{kind, spans[i], line}
		}
		if !parameter(next) {
			words[i] = next.word
		}
		n = next
	}
	if !n.terminal {
		return nil, false, &SyntaxError{"incomplete", len(line), line}
	}
	return words, n.local, nil
}

func (s *Session) helpNode(line string) (*node, string, int, bool) {
	words, spans, err := lex(line)
	if err != nil {
		return nil, "", 0, false
	}
	partial := ""
	start := len(line)
	full := len(line) == 0 || strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t")
	if !full && len(words) > 0 {
		partial = words[len(words)-1]
		start = spans[len(spans)-1]
		words = words[:len(words)-1]
	}
	n := s.grammar()
	for _, word := range words {
		next, kind := n.match(word)
		if kind != "" {
			return nil, "", 0, full
		}
		n = next
	}
	return n, partial, start, full
}

func (s *Session) ContextHelp(line string) string {
	n, partial, _, full := s.helpNode(line)
	if n == nil {
		return ""
	}
	items := append([]*node{}, n.children...)
	if n.rest {
		items = append(items, n)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].word < items[j].word })
	var out []string
	for _, child := range items {
		if !full {
			if !parameter(child) && strings.HasPrefix(child.word, strings.ToLower(partial)) {
				out = append(out, child.word)
			}
		} else {
			description := child.description
			if description == "" {
				description = "Configure " + strings.ReplaceAll(child.word, "-", " ")
			}
			out = append(out, fmt.Sprintf("  %-22s %s", child.word, description))
		}
	}
	if full && n.terminal {
		out = append(out, "  <cr>")
	}
	separator := "\n"
	if !full {
		separator = "  "
	}
	return strings.Join(out, separator)
}

func (s *Session) Complete(line string) (string, string) {
	n, partial, start, full := s.helpNode(line)
	if n == nil || full {
		return line, "\a"
	}
	var found string
	for _, child := range n.children {
		if !parameter(child) && strings.HasPrefix(child.word, strings.ToLower(partial)) {
			if found != "" {
				return line, "\a"
			}
			found = child.word
		}
	}
	if found == "" {
		return line, "\a"
	}
	return line[:start] + found + " ", ""
}

const Help = "Use ? at a word boundary for the next valid keywords or arguments.\nAppend ? to a partial word for matching keywords. The input line is preserved.\nTab completes a unique keyword. Keywords accept unambiguous abbreviations.\nUse Ctrl-V before a literal question mark."
