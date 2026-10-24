package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"fractalcr/internal/config"
)

// ParseConfiguration builds a candidate entirely in memory. Authorization uses
// the caller's original running policy throughout; a file cannot grant itself
// permission to execute its subsequent lines. No platform/EXEC action is run.
func (s *Session) ParseConfiguration(data []byte, base config.Config) (config.Config, error) {
	if len(data) > config.MaxConfigBytes {
		return config.Config{}, fmt.Errorf("Configuration exceeds storage limit")
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		legacy, err := config.Decode(data)
		if err != nil {
			return config.Config{}, fmt.Errorf("Invalid configuration data: %w", err)
		}
		data = []byte(legacy.CLI())
	}
	parser := *s
	parser.Running = s.Running.Clone()
	parser.Candidate = base.Clone()
	parser.Mode, parser.Object, parser.Transactional = "config", "", true
	parser.Apply, parser.Save = nil, nil
	parser.Platform = func([]string, config.Config) (string, error) {
		return "", fmt.Errorf("Operational commands are not allowed in a configuration file")
	}
	ended := false
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 4096), config.MaxConfigBytes+1)
	lineNumber := 0
	for scan.Scan() {
		lineNumber++
		line := strings.TrimLeft(scan.Text(), " \t")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "!") {
			continue
		}
		fail := func(err error) (config.Config, error) {
			return config.Config{}, fmt.Errorf("Invalid configuration at line %d: %w", lineNumber, err)
		}
		if ended {
			return fail(fmt.Errorf("Commands follow end of configuration"))
		}
		words, local, _, err := parser.resolveMode(line)
		if err != nil {
			return fail(err)
		}
		if len(words) == 0 {
			continue
		}
		if len(words) == 1 && words[0] == "end" {
			ended = true
			continue
		}
		if local {
			return fail(fmt.Errorf("Operational commands are not allowed in a configuration file"))
		}
		switch words[0] {
		case "do", "commit", "abort", "configure", "config", "help", "show", "request", "copy", "write", "reload", "logout":
			return fail(fmt.Errorf("Operational commands are not allowed in a configuration file"))
		case "exit":
			if parser.Mode == "config" {
				return fail(fmt.Errorf("No configuration submode to exit"))
			}
		}
		if _, err = parser.Execute(line); err != nil {
			return fail(err)
		}
	}
	if err := scan.Err(); err != nil {
		return config.Config{}, fmt.Errorf("Cannot read configuration")
	}
	if err := parser.Candidate.Validate(); err != nil {
		return config.Config{}, err
	}
	if len(parser.Candidate.JSON()) > config.MaxConfigBytes {
		return config.Config{}, fmt.Errorf("Configuration exceeds storage limit")
	}
	return parser.Candidate.Clone(), nil
}
