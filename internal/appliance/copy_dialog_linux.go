package appliance

import (
	"os"
	"path"
	"strings"
)

type copyDialog struct {
	source, destination, suggested string
	stage                          string
}

func (d *copyDialog) prompt() reply {
	text := "Destination filename [" + d.suggested + "]? "
	if d.stage == "source" {
		text = "Source filename []? "
		if !strings.HasSuffix(d.source, ":") {
			text = "Source filename [" + d.source + "]? "
		}
	}
	if d.stage == "overwrite" {
		text = "Destination file exists, overwrite? [confirm]"
	}
	return reply{Prompt: text, Challenge: true}
}

func (s *loginSession) beginCopy(w []string) reply {
	d := &copyDialog{source: w[1], destination: w[2], stage: "destination"}
	if strings.HasSuffix(d.source, ":") || s.cli.Running.FilePrompt == "noisy" {
		d.stage = "source"
	}
	d.defaultName()
	s.pendingCopy = d
	if d.stage == "destination" && s.cli.Running.FilePrompt == "quiet" && d.suggested != "" {
		return s.answerCopy("")
	}
	return d.prompt()
}

func (d *copyDialog) defaultName() {
	d.suggested = path.Base(d.destination)
	if prefix, filename, ok := strings.Cut(d.destination, ":"); ok {
		_ = prefix
		d.suggested = strings.TrimPrefix(filename, "/")
		if filename == "" {
			_, source, ok := strings.Cut(d.source, ":")
			if !ok {
				source = d.source
			}
			d.suggested = path.Base(source)
			if d.suggested == "." {
				d.suggested = ""
			}
		}
	}
}

func (s *loginSession) answerCopy(answer string) reply {
	r := reply{Prompt: s.cli.Prompt()}
	d := s.pendingCopy
	if !s.cli.AllowedEXEC([]string{"copy", d.source, d.destination}) {
		s.pendingCopy = nil
		r.Error = "Authorization failed."
		return r
	}
	if d.stage == "source" {
		if answer == "" {
			if strings.HasSuffix(d.source, ":") {
				return d.prompt()
			}
		} else if strings.Contains(answer, ":") {
			d.source = answer
		} else if strings.HasSuffix(d.source, ":") {
			d.source += answer
		} else {
			d.source = s.fileURL(answer)
		}
		d.stage = "destination"
		d.defaultName()
		if s.cli.Running.FilePrompt == "quiet" && d.suggested != "" {
			return s.answerCopy("")
		}
		return d.prompt()
	}
	overwrite := d.stage == "overwrite"
	if overwrite && answer != "" && !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		s.pendingCopy = nil
		return r
	}
	if !overwrite {
		if answer == "" {
			answer = d.suggested
		}
		if answer == "" {
			return d.prompt()
		}
		if strings.Contains(answer, ":") || d.destination == "startup-config" {
			d.destination = answer
		} else if prefix, _, ok := strings.Cut(d.destination, ":"); ok {
			d.destination = prefix + ":" + answer
		} else {
			d.destination = answer
		}
		if d.source == "running-config" && (d.destination == "startup-config" || d.destination == "nvram:startup-config" || d.destination == "nvram:startup-config.json") {
			s.pendingCopy = nil
			if s.cli.Transactional && string(s.cli.Candidate.JSON()) != string(s.cli.Running.JSON()) {
				r.Error = "uncommitted transaction; commit before saving"
				return r
			}
			if !s.cli.AllowedEXEC([]string{"copy", d.source, d.destination}) {
				r.Error = "Authorization failed."
				return r
			}
			if s.cli.Save == nil {
				r.Error = "Configuration storage is unavailable"
				return r
			}
			if err := s.cli.Save(s.cli.Running); err != nil {
				r.Error = err.Error()
			} else {
				r.Output = "Building configuration...\n[OK]"
			}
			return r
		}
		if configurationURL(d.destination) != "" {
			s.pendingCopy = nil
			return s.copyConfiguration(d.source, d.destination)
		}
		v, name, err := resolveFile(d.destination)
		if err != nil {
			s.pendingCopy = nil
			r.Error = err.Error()
			return r
		}
		if protectedFile(v, name) {
			s.pendingCopy = nil
			r.Error = "Destination is a protected system object"
			return r
		}
		root, err := os.OpenRoot(v.Root)
		if err != nil {
			s.pendingCopy = nil
			r.Error = fileError(d.destination, err).Error()
			return r
		}
		st, err := root.Lstat(name)
		root.Close()
		if err == nil {
			if !st.Mode().IsRegular() {
				s.pendingCopy = nil
				r.Error = "Destination must be a regular file"
				return r
			}
			if s.cli.Running.FilePrompt != "quiet" {
				d.stage = "overwrite"
				return d.prompt()
			}
			overwrite = true
		}
		if err != nil && !os.IsNotExist(err) {
			s.pendingCopy = nil
			r.Error = fileError(d.destination, err).Error()
			return r
		}
	}
	s.pendingCopy = nil
	if !s.cli.AllowedEXEC([]string{"copy", d.source, d.destination}) {
		r.Error = "Authorization failed."
		return r
	}
	if d.source == d.destination {
		r.Error = "Source and destination are the same file"
		return r
	}
	var err error
	r.Output, err = copyFileOverwrite([]string{"copy", d.source, d.destination}, s.cli.Running, overwrite)
	if err != nil {
		r.Error = err.Error()
	}
	return r
}
