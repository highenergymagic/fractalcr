package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"fractalcr/internal/workloadcatalog"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Generated from the actual linked client command objects on the build host.
// No workload process or external help utility is invoked by completion.
//
//go:embed workload-catalog.json
var workloadCatalogJSON []byte
var workloadRoot workloadcatalog.Command

//go:embed buildkit-catalog.json
var buildkitCatalogJSON []byte
var buildkitRoot workloadcatalog.Command

func init() {
	if err := json.Unmarshal(workloadCatalogJSON, &workloadRoot); err != nil {
		panic(err)
	}
	if err := json.Unmarshal(buildkitCatalogJSON, &buildkitRoot); err != nil {
		panic(err)
	}
	execCommands = append(execCommands, Command{"request container-engine", "Container Engine operations", false})
	execCommands = append(execCommands, Command{"request container-build", "BuildKit operations", false}, Command{"request buildkit", "BuildKit operations", false}, Command{"request buildkit <ARG...>", "BuildKit operations", false})
	for _, name := range []string{"compose", "buildx", "swarm", "service", "stack", "node", "secret", "registry", "context"} {
		for _, child := range workloadRoot.Children {
			if child.Name == name {
				execCommands = append(execCommands, Command{"request " + name, "Manage " + name, false}, Command{"request " + name + " <ARG...>", "Manage " + name, false})
			}
		}
	}
}

// workloadInput identifies the linked client family before grammar resolution.
func workloadInput(words []string) (string, []string, bool) {
	w := words
	if len(w) > 0 && w[0] == "do" {
		w = w[1:]
	}
	if len(w) < 2 || w[0] != "request" {
		return "", nil, false
	}
	if w[1] == "container-build" || w[1] == "buildkit" {
		return "build", w[2:], true
	}
	if w[1] == "container-engine" {
		return "engine", w[2:], true
	}
	for _, name := range []string{"compose", "buildx", "swarm", "service", "stack", "node", "secret", "registry", "context"} {
		if w[1] == name {
			return "engine", append([]string{name}, w[2:]...), true
		}
	}
	return "", nil, false
}

// WorkloadArguments is the only translation boundary to the linked client.
// Resolved appliance words remain keyword/value grammar until dispatch.
func WorkloadArguments(words []string) (string, []string, bool) {
	service, args, ok := workloadInput(words)
	if !ok {
		return "", nil, false
	}
	p, _, _, kind := walkWorkload(service, args)
	if kind != "" || p.value != nil || p.literal {
		return "", nil, false
	}
	if p.terminal {
		return "help", nil, true
	}
	return service, p.native, true
}

type workloadPosition struct {
	command   *workloadcatalog.Command
	path      []string
	value     *workloadcatalog.Flag
	literal   bool
	rest      bool
	terminal  bool
	version   bool
	arguments int
	native    []string
}

func workloadGroupAction(cmd *workloadcatalog.Command) bool {
	if !cmd.Runnable {
		return false
	}
	group := false
	for _, child := range cmd.Children {
		if child.Name != "help" {
			group = true
			break
		}
	}
	return !group || cmd.MinArgs > 0 || cmd.MaxArgs != nil && *cmd.MaxArgs > 0
}

func fieldKeyword(cmd *workloadcatalog.Command, f workloadcatalog.Flag) string {
	name := f.Name
	for _, child := range cmd.Children {
		if child.Name == name {
			return "client-" + name
		}
	}
	if name == "argument" || name == "arguments" {
		return "client-" + name
	}
	return name
}

func workloadKeyword(p workloadPosition, word string) (string, *workloadcatalog.Flag, *workloadcatalog.Command, string) {
	type choice struct {
		name  string
		flag  *workloadcatalog.Flag
		child *workloadcatalog.Command
	}
	choices := []choice{{name: "argument"}, {name: "arguments"}, {name: "help"}}
	if p.arguments == 0 {
		for i := range p.command.Children {
			c := &p.command.Children[i]
			if !c.Hidden && c.Name != "help" {
				choices = append(choices, choice{name: c.Name, child: c})
			}
		}
	}
	for i := range p.command.Flags {
		f := &p.command.Flags[i]
		if !f.Hidden && f.Name != "help" {
			choices = append(choices, choice{name: fieldKeyword(p.command, *f), flag: f})
		}
	}
	var match *choice
	for i := range choices {
		if strings.EqualFold(choices[i].name, word) {
			v := choices[i]
			return v.name, v.flag, v.child, ""
		}
	}
	for i := range choices {
		if strings.HasPrefix(choices[i].name, strings.ToLower(word)) {
			if match != nil {
				return "", nil, nil, "ambiguous"
			}
			match = &choices[i]
		}
	}
	if match == nil {
		return "", nil, nil, "invalid"
	}
	return match.name, match.flag, match.child, ""
}

func walkWorkload(service string, args []string) (workloadPosition, []string, int, string) {
	root := &workloadRoot
	if service == "build" {
		root = &buildkitRoot
	}
	p := workloadPosition{command: root}
	out := append([]string{}, args...)
	var operands []string
	addOperand := func(word string) bool {
		p.arguments++
		operands = append(operands, word)
		return p.command.MaxArgs == nil || p.arguments <= *p.command.MaxArgs
	}
	for i, word := range args {
		if p.terminal {
			return p, nil, i, "invalid"
		}
		if p.value != nil {
			value := word
			if !validWorkloadValue(*p.value, word) {
				return p, nil, i, "invalid"
			}
			if p.value.Boolean {
				switch strings.ToLower(word) {
				case "enable":
					value = "true"
					out[i] = "enable"
				case "disable":
					value = "false"
					out[i] = "disable"
				default:
					return p, nil, i, "invalid"
				}
			}
			if p.value.Name == "version" && value == "true" {
				p.version = true
			}
			p.native = append(p.native, "--"+p.value.Name+"="+value)
			p.value = nil
			continue
		}
		if p.literal || p.rest {
			p.literal = false
			if !addOperand(word) {
				return p, nil, i, "invalid"
			}
			continue
		}
		if strings.HasPrefix(word, "-") {
			return p, nil, i, "invalid"
		}
		key, flag, child, kind := workloadKeyword(p, word)
		if kind == "ambiguous" {
			return p, nil, i, kind
		}
		if kind == "" {
			out[i] = key
			if flag != nil {
				p.value = flag
				continue
			}
			if child != nil {
				p.native = append(p.native, child.Name)
				p.path = append(p.path, child.Name)
				p.command = child
				continue
			}
			switch key {
			case "help":
				p.terminal = true
			case "argument":
				p.literal = true
			case "arguments":
				p.rest = true
				p.literal = true
			}
			continue
		}
		if len(p.command.Children) > 0 && !workloadGroupAction(p.command) {
			return p, nil, i, "invalid"
		}
		if !p.command.Runnable || !addOperand(word) {
			return p, nil, i, "invalid"
		}
	}
	if len(operands) > 0 {
		p.native = append(p.native, "--")
		p.native = append(p.native, operands...)
	}
	return p, out, -1, ""
}

func canonicalWorkload(words []string, line string, spans []int) ([]string, error) {
	service, args, ok := workloadInput(words)
	if !ok {
		return words, nil
	}
	p, out, index, kind := walkWorkload(service, args)
	offset := len(words) - len(args)
	if offset < 0 {
		offset = 0
	}
	if kind != "" {
		column := len(line)
		if offset+index >= 0 && offset+index < len(spans) {
			column = spans[offset+index]
		}
		return nil, &SyntaxError{kind, column, line}
	}
	if p.value != nil || p.literal {
		return nil, &SyntaxError{"incomplete", len(line), line}
	}
	if len(p.command.Children) > 0 && !workloadGroupAction(p.command) && !p.terminal && !p.version {
		return nil, &SyntaxError{"incomplete", len(line), line}
	}
	if !p.terminal && !p.version && p.arguments < p.command.MinArgs {
		return nil, &SyntaxError{"incomplete", len(line), line}
	}
	// Aliases insert their family into the frontend argv, but retain the
	// appliance prefix in the resolved command.
	if offset == 1 || (len(words) > 0 && words[0] == "do" && offset == 2) {
		out = out[1:]
		offset++
	}
	return append(append([]string{}, words[:offset]...), out...), nil
}

func (s *Session) workloadHelp(line string) (string, bool) {
	words, _, err := lex(line)
	if err != nil {
		return "", false
	}
	full := line == "" || strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t")
	partial := ""
	if !full && len(words) > 0 {
		partial = words[len(words)-1]
		words = words[:len(words)-1]
	}
	if len(words) < 2 {
		return "", false
	}
	// Canonicalize only the appliance prefix using the normal privilege-filtered
	// grammar. Its variadic edge is replaced by the linked command catalog.
	n := s.grammar()
	canonical := []string{}
	used := 0
	for i, word := range words {
		next, kind := n.match(word)
		if kind != "" {
			return "", false
		}
		canonical = append(canonical, next.word)
		n = next
		if next.word == "request" {
			continue
		}
		if len(canonical) >= 2 && canonical[len(canonical)-2] == "request" {
			used = i + 1
			break
		}
	}
	if used == 0 {
		return "", false
	}
	canonical = append(canonical, words[used:]...)
	service, args, ok := workloadInput(canonical)
	if !ok {
		return "", false
	}
	p, _, _, kind := walkWorkload(service, args)
	if kind != "" {
		return "", true
	}
	if !s.workloadPolicyAllows(canonical) {
		return "", true
	}
	if p.terminal {
		if full {
			return "  <cr>", true
		}
		return "", true
	}
	if p.value != nil {
		if p.value.Boolean {
			items := []string{}
			for _, key := range []string{"disable", "enable"} {
				if full {
					items = append(items, fmt.Sprintf("  %-24s %s", key, "Set "+fieldKeyword(p.command, *p.value)))
				} else if strings.HasPrefix(key, strings.ToLower(partial)) {
					items = append(items, key)
				}
			}
			if full {
				return strings.Join(items, "\n"), true
			}
			return strings.Join(items, "  "), true
		}
		if full {
			return fmt.Sprintf("  <%s>  %s", p.value.Type, nativeDescription(p.command, p.value.Description)), true
		}
		return "", true
	}
	items := map[string]string{}
	if p.literal || p.rest {
		if full {
			items["<ARG>"] = "Literal operand; passed to the workload without interpretation"
			if !p.literal && p.arguments >= p.command.MinArgs {
				items["<cr>"] = ""
			}
		}
	} else {
		if p.arguments == 0 {
			for _, child := range p.command.Children {
				if !child.Hidden && child.Name != "help" {
					items[child.Name] = nativeDescription(p.command, child.Description)
				}
			}
		}
		for _, f := range p.command.Flags {
			if !f.Hidden && f.Name != "help" {
				items[fieldKeyword(p.command, f)] = nativeDescription(p.command, f.Description)
			}
		}
		items["help"] = "Describe this operation"
		if p.command.Runnable && (len(p.command.Children) == 0 || workloadGroupAction(p.command)) {
			if full && p.arguments >= p.command.MinArgs {
				items["<cr>"] = ""
			}
			if p.command.MaxArgs == nil || p.arguments < *p.command.MaxArgs {
				items["argument"] = "Supply one literal operand"
				items["arguments"] = "Supply remaining literal workload operands"
				if full {
					items["<ARG>"] = nativeDescription(p.command, p.command.Usage)
				}
			}
		}
	}
	keys := []string{}
	for key := range items {
		if !strings.HasPrefix(key, "<") && !s.workloadPolicyAllows(append(append([]string(nil), canonical...), key)) {
			continue
		}
		if full || strings.HasPrefix(strings.ToLower(key), strings.ToLower(partial)) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	lines := []string{}
	for _, key := range keys {
		if full {
			lines = append(lines, fmt.Sprintf("  %-24s %s", key, items[key]))
		} else {
			lines = append(lines, key)
		}
	}
	separator := "\n"
	if !full {
		separator = "  "
	}
	return strings.Join(lines, separator), true
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Validate primitive catalog types before dispatch so malformed values receive
// the same caret diagnostics as other appliance configuration commands.
func validWorkloadValue(f workloadcatalog.Flag, value string) bool {
	var err error
	switch f.Type {
	case "int", "int64":
		_, err = strconv.ParseInt(value, 10, 64)
	case "uint", "uint64", "uint32", "uint16":
		bits := 64
		if f.Type == "uint32" {
			bits = 32
		}
		if f.Type == "uint16" {
			bits = 16
		}
		_, err = strconv.ParseUint(value, 10, bits)
	case "float", "float64":
		var number float64
		number, err = strconv.ParseFloat(value, 64)
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return false
		}
	case "duration":
		_, err = time.ParseDuration(value)
	case "ip":
		return net.ParseIP(value) != nil
	}
	return err == nil
}

// Client help strings are data; descriptions must use appliance keywords too.
func nativeDescription(cmd *workloadcatalog.Command, text string) string {
	text = strings.ReplaceAll(text, "[OPTIONS]", "")
	text = strings.ReplaceAll(text, "[flags]", "")
	for _, f := range cmd.Flags {
		text = strings.ReplaceAll(text, "--"+f.Name, fieldKeyword(cmd, f))
		if f.Short != "" {
			text = strings.ReplaceAll(text, "-"+f.Short+" ", fieldKeyword(cmd, f)+" ")
		}
	}
	return oneLine(workloadcatalog.NativeWording(text))
}
