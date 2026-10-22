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

func lex(line string) ([]string, []int, error) {
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
