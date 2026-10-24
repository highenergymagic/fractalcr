package cli

import (
	"fmt"
	"fractalcr/internal/config"
)

func init() {
	configCommands = append(configCommands, Command{"volume <NAME>", "Configure a named volume", false}, Command{"no volume <NAME>", "Remove a volume definition", false})
	modeCommands["volume"] = []string{"driver <DRIVER>", "no driver", "option <KEY> <VALUE>", "no option <KEY>", "label <KEY> <VALUE>", "no label <KEY>"}
	execCommands = append(execCommands, Command{"show volume <NAME> detail", "Volume details", false})
}

func (s *Session) volumeConfiguration(words []string) (bool, error) {
	w := words
	remove := w[0] == "no"
	if remove {
		w = w[1:]
	}
	if s.Mode != "container" && len(w) == 2 && w[0] == "volume" {
		if !config.Name.MatchString(w[1]) {
			return true, fmt.Errorf("invalid volume name")
		}
		if remove {
			delete(s.Candidate.Volumes, w[1])
			if s.Mode == "volume" && s.Object == w[1] {
				s.Mode, s.Object = "config", ""
			}
		} else {
			if s.Candidate.Volumes == nil {
				s.Candidate.Volumes = map[string]config.VolumeDefinition{}
			}
			if _, ok := s.Candidate.Volumes[w[1]]; !ok {
				s.Candidate.Volumes[w[1]] = config.VolumeDefinition{Driver: "local"}
			}
			s.Mode, s.Object = "volume", w[1]
		}
		return true, nil
	}
	if s.Mode != "volume" {
		return false, nil
	}
	v := s.Candidate.Volumes[s.Object]
	switch w[0] {
	case "driver":
		if remove {
			v.Driver = ""
		} else {
			v.Driver = w[1]
		}
	case "option", "label":
		m := &v.Options
		if w[0] == "label" {
			m = &v.Labels
		}
		if remove {
			delete(*m, w[1])
		} else {
			if *m == nil {
				*m = map[string]string{}
			}
			(*m)[w[1]] = w[2]
		}
	default:
		return true, fmt.Errorf("invalid volume setting")
	}
	s.Candidate.Volumes[s.Object] = v
	return true, nil
}
