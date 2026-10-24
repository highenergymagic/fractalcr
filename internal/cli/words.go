package cli

import (
	"fmt"
	"strings"
)

// Words recognizes quoted arguments without expansion, substitution or a shell.
func Words(line string) ([]string, error) {
	words, _, err := lex(line)
	return words, err
}

func (s *Session) lexLine(line string) ([]string, []int, error) {
	if s.Mode == "interface" {
		if words, spans, ok, err := descriptionWords(line, true); ok {
			return words, spans, err
		}
	}
	return lex(line)
}

func lex(line string) ([]string, []int, error) {
	if words, spans, ok, err := descriptionWords(line, false); ok {
		return words, spans, err
	}
	var words []string
	var spans []int
	var b strings.Builder
	var quote rune
	active, escape := false, false
	for offset, c := range line {
		if !active && c != ' ' && c != '\t' {
			spans = append(spans, offset)
		}
		if escape {
			b.WriteRune(c)
			escape = false
			active = true
			continue
		}
		if c == '\\' {
			escape = true
			active = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				b.WriteRune(c)
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			active = true
			continue
		}
		if c == ' ' || c == '\t' {
			if active {
				words = append(words, b.String())
				b.Reset()
				active = false
			}
			continue
		}
		if c < 32 || c == 127 {
			return nil, nil, fmt.Errorf("control character in command")
		}
		b.WriteRune(c)
		active = true
	}
	if escape || quote != 0 {
		return nil, nil, fmt.Errorf("unterminated quote or escape")
	}
	if active {
		words = append(words, b.String())
	}
	return words, spans, nil
}

// Join retains argument boundaries when a canonical command crosses the local
// transport. In particular, empty arguments and literal quotes survive.
func Join(words []string) string {
	if len(words) > 1 && words[0] == "description" {
		return "description " + strings.Join(words[1:], " ")
	}
	parts := make([]string, len(words))
	for i, word := range words {
		if word != "" && !strings.ContainsAny(word, " \t\\\"'") {
			parts[i] = word
			continue
		}
		parts[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(word) + `"`
	}
	return strings.Join(parts, " ")
}

// IOS descriptions are the remaining literal text, not shell-style arguments.
// Keep quotes, backslashes and repeated spaces through the typed management
// transport and configuration display/replay.
func descriptionWords(line string, abbreviations bool) ([]string, []int, bool, error) {
	start := len(line) - len(strings.TrimLeft(line, " \t"))
	end := start
	for end < len(line) && line[end] != ' ' && line[end] != '\t' {
		end++
	}
	keyword := strings.ToLower(line[start:end])
	if keyword == "" || (keyword != "description" && (!abbreviations || !strings.HasPrefix("description", keyword))) {
		return nil, nil, false, nil
	}
	for _, c := range line {
		if (c < 32 && c != '\t') || c == 127 {
			return nil, nil, true, fmt.Errorf("control character in command")
		}
	}
	text := end
	for text < len(line) && (line[text] == ' ' || line[text] == '\t') {
		text++
	}
	if text == len(line) {
		return []string{line[start:end]}, []int{start}, true, nil
	}
	return []string{line[start:end], line[text:]}, []int{start, text}, true, nil
}
