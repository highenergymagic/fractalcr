package cli

import (
	"fmt"
	"regexp"
	"strings"
)

type Filter struct {
	Mode    string `json:"mode"`
	Pattern string `json:"pattern"`
}

func (f *Filter) Redirect() bool {
	return f != nil && (f.Mode == "redirect" || f.Mode == "append" || f.Mode == "tee" || f.Mode == "tee-append")
}

func SplitFilter(words []string) ([]string, *Filter, error) {
	start := 0
	if len(words) > 0 && words[0] == "do" {
		start = 1
	}
	if len(words) <= start || (words[start] != "show" && words[start] != "dir") {
		return words, nil, nil
	}
	for i := start + 1; i < len(words); i++ {
		if words[i] == "|" {
			if i+2 >= len(words) {
				return nil, nil, fmt.Errorf("filter requires a regular expression")
			}
			f := &Filter{words[i+1], strings.Join(words[i+2:], " ")}
			if f.Mode == "tee" && words[i+2] == "/append" {
				if len(words) != i+4 {
					return nil, nil, fmt.Errorf("output destination required")
				}
				f.Mode = "tee-append"
				f.Pattern = words[i+3]
			}
			if f.Redirect() {
				if f.Mode != "tee-append" && len(words) != i+3 {
					return nil, nil, fmt.Errorf("one output destination required")
				}
				return words[:i], f, nil
			}
			if f.Mode == "section" && len(words) > i+3 && (words[i+2] == "include" || words[i+2] == "exclude") {
				f.Mode = "section-" + words[i+2]
				f.Pattern = strings.Join(words[i+3:], " ")
			}
			switch f.Mode {
			case "include", "exclude", "begin", "count", "section", "section-include", "section-exclude":
			default:
				return nil, nil, fmt.Errorf("unknown output filter")
			}
			if len(f.Pattern) > 512 {
				return nil, nil, fmt.Errorf("filter expression exceeds 512 bytes")
			}
			if _, err := regexp.CompilePOSIX(f.Pattern); err != nil {
				return nil, nil, fmt.Errorf("invalid filter expression: %w", err)
			}
			return words[:i], f, nil
		}
	}
	return words, nil, nil
}
func (f *Filter) Apply(text string) string {
	if f == nil {
		return text
	}
	re := regexp.MustCompilePOSIX(f.Pattern)
	var out []string
	begun := false
	count := 0
	section := false
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		match := re.MatchString(line)
		if match {
			count++
		}
		begun = begun || match
		if strings.HasPrefix(f.Mode, "section") {
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				section = false
			}
			section = section || match
			if section != (f.Mode == "section-exclude") {
				out = append(out, line)
			}
			continue
		}
		if (f.Mode == "include" && match) || (f.Mode == "exclude" && !match) || (f.Mode == "begin" && begun) {
			out = append(out, line)
		}
	}
	if f.Mode == "count" {
		return fmt.Sprintf("Number of lines which match regexp = %d", count)
	}
	return strings.Join(out, "\n")
}
