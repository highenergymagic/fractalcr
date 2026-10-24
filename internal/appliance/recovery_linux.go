package appliance

import (
	"encoding/json"
	"fractalcr/internal/cli"
	"fractalcr/internal/config"
	"io"
)

// Recovery uses the same parser without a management socket, configuration
// transaction, datastore or Docker dependency.
func Recovery(action, line string, out io.Writer) error {
	if c, err := loadRunning(); err != nil || c.AAA.Enabled {
		return json.NewEncoder(out).Encode(reply{Error: "Management unavailable; authenticated recovery requires restoring the management service or ROMMON recovery.", Close: true})
	}
	if err := (managementRequest{Action: action, Command: line}).validate(); err != nil {
		return json.NewEncoder(out).Encode(reply{Error: err.Error(), Close: true})
	}
	s := &cli.Session{Running: config.Default(), Recovery: true}
	r := reply{Prompt: "ctr# "}
	switch {
	case action == "connect" || action == "end" || action == "tick" || action == "cancel":
	case action == "help":
		r.Output = s.ContextHelp(line)
	case action == "complete":
		value, output := s.Complete(line)
		r.Completion, r.Output = &value, output
	default:
		words, local, err := s.Resolve(line)
		if err != nil {
			r.Error = err.Error()
		} else if local {
			r.Local = cli.Join(words)
		} else if len(words) > 0 {
			r.Error = "Management service unavailable."
		}
	}
	return json.NewEncoder(out).Encode(r)
}
