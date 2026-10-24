package workloadcatalog

import (
	"regexp"
	"strings"
)

var optionReference = regexp.MustCompile(`(^|[\s\x60'"(,])--([a-zA-Z][a-zA-Z0-9-]*)`)

// NativeWording adapts client-authored help and errors, not workload output.
// Container output and supplied values must never pass through this function.
func NativeWording(text string) string {
	text = strings.ReplaceAll(text, "[OPTIONS]", "")
	text = strings.ReplaceAll(text, "[flags]", "")
	return optionReference.ReplaceAllString(text, "${1}${2}")
}
