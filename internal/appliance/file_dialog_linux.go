package appliance

import "strings"

type fileDialog struct {
	words   []string
	operand int
	confirm bool
	force   bool
}

func (d *fileDialog) prompt() reply {
	if d.confirm {
		return reply{Prompt: "Delete " + deleteURL(d.words) + "? [confirm]", Challenge: true}
	}
	label := "Filename"
	switch d.words[0] {
	case "mkdir", "rmdir":
		label = "Directory name"
	case "rename", "move":
		label = "Source filename"
		if d.operand == 2 {
			label = "Destination filename"
		}
	}
	return reply{Prompt: label + " [" + d.words[d.operand] + "]? ", Challenge: true}
}

func (s *loginSession) beginFile(words []string) reply {
	d := &fileDialog{words: append([]string(nil), words...)}
	for i, word := range words[1:] {
		if word == "/force" {
			d.force = true
		}
		if !strings.HasPrefix(word, "/") && d.operand == 0 {
			d.operand = i + 1
		}
	}
	s.pendingFile = d
	if s.cli.Running.FilePrompt == "noisy" && !d.force {
		return d.prompt()
	}
	return s.confirmFile()
}

func (s *loginSession) confirmFile() reply {
	d := s.pendingFile
	if d.words[0] == "delete" && !d.force && s.cli.Running.FilePrompt != "quiet" {
		d.confirm = true
		return d.prompt()
	}
	return s.performFile()
}

func (s *loginSession) performFile() reply {
	d := s.pendingFile
	s.pendingFile = nil
	r := reply{Prompt: s.cli.Prompt()}
	if !s.cli.AllowedEXEC(d.words) {
		r.Error = "Authorization failed."
		return r
	}
	var err error
	r.Output, err = fileOperation(d.words)
	if err != nil {
		r.Error = err.Error()
	}
	return r
}

func (s *loginSession) answerFile(answer string) reply {
	d := s.pendingFile
	if !s.cli.AllowedEXEC(d.words) {
		s.pendingFile = nil
		return reply{Prompt: s.cli.Prompt(), Error: "Authorization failed."}
	}
	if d.confirm {
		if answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
			return s.performFile()
		}
		s.pendingFile = nil
		return reply{Prompt: s.cli.Prompt()}
	}
	if answer != "" {
		d.words[d.operand] = s.fileURL(answer)
	}
	if (d.words[0] == "rename" || d.words[0] == "move") && d.operand == 1 {
		d.operand = 2
		return d.prompt()
	}
	return s.confirmFile()
}
