package appliance

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"fractalcr/internal/config"
)

func configurationURL(url string) string {
	switch strings.Replace(url, ":/", ":", 1) {
	case "running-config", "system:running-config":
		return "running-config"
	case "startup-config", "nvram:startup-config", "nvram:startup-config.json":
		return "startup-config"
	}
	return ""
}

func configurationExport(url string, running config.Config) ([]byte, bool, error) {
	kind := configurationURL(url)
	if kind == "" {
		return nil, false, nil
	}
	c := running
	if kind == "startup-config" {
		var err error
		c, err = config.Load(startupPath)
		if err != nil {
			return nil, true, fmt.Errorf("Cannot read startup configuration")
		}
	}
	return []byte(c.CLI()), true, nil
}

func configurationInput(url string, running config.Config) ([]byte, error) {
	if data, ok, err := configurationExport(url, running); ok {
		return data, err
	}
	v, name, err := resolveFile(url)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(v.Root)
	if err != nil {
		return nil, fileError(url, err)
	}
	defer root.Close()
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fileError(url, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fileError(url, err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("Source must be a regular file")
	}
	if st.Size() > config.MaxConfigBytes {
		return nil, fmt.Errorf("Configuration exceeds storage limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, config.MaxConfigBytes+1))
	if err != nil {
		return nil, fileError(url, err)
	}
	if len(data) > config.MaxConfigBytes {
		return nil, fmt.Errorf("Configuration exceeds storage limit")
	}
	return data, nil
}

func (s *loginSession) copyConfiguration(source, destination string) reply {
	r := reply{Prompt: s.cli.Prompt()}
	if !s.cli.AllowedEXEC([]string{"copy", source, destination}) {
		r.Error = "Authorization failed."
		return r
	}
	if s.cli.Transactional && string(s.cli.Candidate.JSON()) != string(s.cli.Running.JSON()) {
		r.Error = "uncommitted transaction; commit before copying configuration"
		return r
	}
	kind := configurationURL(destination)
	if configurationURL(source) == kind {
		r.Error = "Source and destination are the same file"
		return r
	}
	data, err := configurationInput(source, s.cli.Running)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	base := config.Default()
	if kind == "running-config" {
		base = s.cli.Running
	}
	next, err := s.cli.ParseConfiguration(data, base)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	if kind == "startup-config" {
		if s.cli.Save == nil {
			r.Error = "Configuration storage is unavailable"
			return r
		}
		if err = s.cli.Save(next); err != nil {
			r.Error = err.Error()
			return r
		}
		r.Output = "Building configuration...\n[OK]"
	} else {
		if s.cli.Apply == nil {
			r.Error = "Configuration application is unavailable"
			return r
		}
		if err = s.cli.Apply(s.cli.Running, next); err != nil {
			r.Error = err.Error()
			return r
		}
		s.cli.Running, s.cli.Candidate = next, next.Clone()
		s.cli.ConfigBase = string(next.JSON())
		r.Prompt = s.cli.Prompt()
		r.Output = fmt.Sprintf("%d bytes copied", len(data))
	}
	return r
}
