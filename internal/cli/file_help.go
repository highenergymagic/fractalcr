package cli

import (
	"slices"
	"strings"
)

func (s *Session) displayFileSystems(line string) []string {
	names := append([]string(nil), s.FileSystems()...)
	words := strings.Fields(line)
	if len(words) > 0 && words[0] == "do" {
		words = words[1:]
	}
	if len(words) > 0 && strings.HasPrefix("more", words[0]) && !slices.Contains(names, "system:") {
		names = append(names, "system:")
	}
	slices.Sort(names)
	return names
}
