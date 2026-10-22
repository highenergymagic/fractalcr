package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fractalcr/internal/config"
)

type Session struct {
	Running          config.Config
	Candidate        config.Config
	StateDir         string
	Mode             string
	Object           string
	Apply            func(config.Config, config.Config) error
	Platform         func([]string, config.Config) (string, error)
	Save             func(config.Config) error
	Recovery         bool
	EnforcePrivilege bool
	Privilege        int
	Username         string
	ConfigBase       string
}

func New(state string) (*Session, error) {
	c, err := config.Load(filepath.Join(state, "startup-config.json"))
	if os.IsNotExist(err) {
		c, err = config.Default(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("startup-config: %w", err)
	}
	return &Session{Running: c, Candidate: c.Clone(), StateDir: state}, nil
}

func (s *Session) Prompt() string {
	suffix := ""
	if s.Mode != "" {
		suffix = "(config"
		if s.Mode != "config" {
			suffix += "-" + s.Mode
		}
		suffix += ")"
	}
	mark := "# "
	if s.EnforcePrivilege && s.Privilege < 15 && s.Mode == "" {
		mark = "> "
	}
	return s.Running.Hostname + suffix + mark
}

func (s *Session) Loop(in io.Reader, out io.Writer, batch bool) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for {
		if !batch {
			fmt.Fprint(out, s.Prompt())
		}
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		if line == "quit" {
			return nil
		}
		result, err := s.Execute(line)
		if err != nil {
			if batch {
				return fmt.Errorf("%s: %w", line, err)
			}
			fmt.Fprintln(out, "% "+err.Error())
		} else if result != "" {
			fmt.Fprintln(out, result)
		}
	}
}

func (s *Session) Execute(line string) (string, error) {
	w, _, err := s.Resolve(line)
	if err != nil {
		return "", err
	}
	base, filter, err := SplitFilter(w)
	if err != nil {
		return "", err
	}
	if filter != nil {
		result, e := s.Execute(Join(base))
		if e != nil {
			return "", e
		}
		return filter.Apply(result), nil
	}
	line = strings.Join(w, " ")
	if len(w) == 0 {
		return "", nil
	}
	if !s.Allowed(w) {
		return "", errors.New("Authorization failed.")
	}
	if handled, output, err := s.aaaCommand(w); handled {
		return output, err
	}
	if w[0] == "do" {
		if s.Mode == "" {
			return "", errors.New("do is only available in configuration mode")
		}
		return s.operational(w[1:])
	}
	switch line {
	case "help":
		return Help, nil
	case "config", "configure terminal":
		if s.Mode == "" {
			s.Candidate = s.Running.Clone()
			s.ConfigBase = string(s.Running.JSON())
		}
		s.Mode, s.Object = "config", ""
		return "", nil
	case "end":
		s.Mode, s.Object = "", ""
		return "", nil
	case "exit":
		if s.Mode != "config" && s.Mode != "" {
			s.Mode, s.Object = "config", ""
		} else {
			s.Mode = ""
		}
		return "", nil
	case "abort":
		s.Candidate = s.Running.Clone()
		s.Mode, s.Object = "", ""
		return "Candidate discarded.", nil
	case "commit":
		if s.Mode == "" {
			return "", errors.New("enter configuration mode first")
		}
		if s.ConfigBase != "" && s.ConfigBase != string(s.Running.JSON()) {
			return "", errors.New("running configuration changed in another session; abort and re-enter configuration mode")
		}
		if err := s.Candidate.Validate(); err != nil {
			return "", err
		}
		if s.Apply != nil {
			if err := s.Apply(s.Running, s.Candidate); err != nil {
				return "", err
			}
		}
		s.Running = s.Candidate.Clone()
		s.ConfigBase = string(s.Running.JSON())
		return "Running configuration committed. Use copy running-config startup-config to persist.", nil
	}
	if w[0] == "show" || w[0] == "copy" || w[0] == "request" || w[0] == "write" || w[0] == "ping" {
		return s.operational(w)
	}
	if s.Mode == "" {
		return "", errors.New("enter config to change configuration; ? lists commands")
	}
	if len(w) == 2 && w[0] == "interface" {
		if !config.Interface.MatchString(w[1]) {
			return "", errors.New("invalid interface")
		}
		if s.Candidate.Network == nil {
			s.Candidate.Network = &config.Network{}
		}
		if s.Candidate.Network.Interface != "" && s.Candidate.Network.Interface != w[1] {
			return "", errors.New("one management interface is supported; remove it with no interface NAME first")
		}
		s.Candidate.Network.Interface = w[1]
		s.Mode, s.Object = "interface", w[1]
		return "", nil
	}
	if len(w) == 3 && w[0] == "no" {
		switch w[1] {
		case "container":
			delete(s.Candidate.Containers, w[2])
			if s.Mode == "container" && s.Object == w[2] {
				s.Mode = "config"
				s.Object = ""
			}
			return "", nil
		case "interface":
			if s.Candidate.Network != nil && s.Candidate.Network.Interface == w[2] {
				s.Candidate.Network = nil
			}
			if s.Mode == "interface" && s.Object == w[2] {
				s.Mode = "config"
				s.Object = ""
			}
			return "", nil
		}
	}
	if s.Mode == "interface" {
		if s.Candidate.Network == nil {
			return "", errors.New("interface no longer exists; re-enter configuration mode")
		}
		if len(w) == 3 && w[0] == "ip" {
			switch w[1] {
			case "address":
				s.Candidate.Network.Address = w[2]
			case "gateway":
				s.Candidate.Network.Gateway = w[2]
			case "name-server":
				s.Candidate.Network.DNS = w[2]
			default:
				return "", errors.New("expected ip address, ip gateway, or ip name-server")
			}
			return "", nil
		}
		if line == "no ip gateway" {
			s.Candidate.Network.Gateway = ""
			return "", nil
		}
		if line == "no ip name-server" {
			s.Candidate.Network.DNS = ""
			return "", nil
		}
	}
	if len(w) == 2 && w[0] == "hostname" {
		if !config.Name.MatchString(w[1]) {
			return "", errors.New("invalid hostname")
		}
		s.Candidate.Hostname = w[1]
		return "", nil
	}
	if len(w) == 5 && strings.Join(w[:4], " ") == "storage datastore name" {
		// Kept below as exact four-token syntax; no permissive prefix matching.
		return "", errors.New("syntax: storage datastore name NAME")
	}
	if len(w) == 4 && strings.Join(w[:3], " ") == "storage datastore name" {
		name := w[3]
		if !config.Name.MatchString(name) {
			return "", errors.New("invalid datastore name")
		}
		if _, ok := s.Candidate.Datastores[name]; !ok {
			s.Candidate.Datastores[name] = config.Datastore{Filesystem: "btrfs", Preset: "single", Disks: []string{}}
		}
		s.Mode, s.Object = "storage-datastore", name
		return "", nil
	}
	if len(w) == 2 && w[0] == "container" {
		if !config.Name.MatchString(w[1]) {
			return "", errors.New("invalid container name")
		}
		if _, ok := s.Candidate.Containers[w[1]]; !ok {
			s.Candidate.Containers[w[1]] = config.Container{Isolation: "inherit"}
		}
		s.Mode, s.Object = "container", w[1]
		return "", nil
	}
	if len(w) >= 3 && w[0] == "system" && w[1] == "container-isolation" {
		return s.isolation(w[2:])
	}
	if s.Mode == "storage-datastore" {
		return s.datastore(w)
	}
	if s.Mode == "container" {
		return s.container(w)
	}
	return "", errors.New("unknown command; ? lists supported commands")
}

func (s *Session) datastore(w []string) (string, error) {
	ds := s.Candidate.Datastores[s.Object]
	switch {
	case strings.Join(w, " ") == "file-system format btrfs":
		ds.Filesystem = "btrfs"
	case len(w) == 2 && w[0] == "preset" && (w[1] == "single" || w[1] == "raid1"):
		ds.Preset = w[1]
	case len(w) == 2 && w[0] == "disk":
		if !config.Disk.MatchString(w[1]) {
			return "", errors.New("invalid whole-disk identifier")
		}
		ds.Disks = []string{w[1]}
	case len(w) == 3 && w[0] == "disk" && w[1] == "add":
		if !config.Disk.MatchString(w[2]) {
			return "", errors.New("invalid whole-disk identifier")
		}
		for _, d := range ds.Disks {
			if d == w[2] {
				return "", errors.New("disk already configured")
			}
		}
		ds.Disks = append(ds.Disks, w[2])
	default:
		return "", errors.New("expected file-system format btrfs, preset single|raid1, disk NAME, or disk add NAME")
	}
	s.Candidate.Datastores[s.Object] = ds
	return "", nil
}

func (s *Session) container(w []string) (string, error) {
	c := s.Candidate.Containers[s.Object]
	switch strings.Join(w, " ") {
	case "read-only":
		c.ReadOnly = true
	case "no read-only":
		c.ReadOnly = false
	case "security no-new-privileges":
		c.NoNewPrivileges = true
	case "no security no-new-privileges":
		c.NoNewPrivileges = false
	case "capability drop all":
		c.DropAllCapabilities = true
	case "no capability drop all":
		c.DropAllCapabilities = false
	case "no volume":
		c.Volumes = nil
	case "no container-isolation":
		c.Isolation = "none"
	case "container-isolation inherit":
		c.Isolation = "inherit"
	case "container-isolation kata-containers enable":
		c.Isolation = "kata"
	default:
		if len(w) >= 3 && len(w) <= 4 && w[0] == "volume" {
			if len(w) == 4 && w[3] != "read-only" {
				return "", errors.New("expected volume NAME TARGET [read-only]")
			}
			c.Volumes = append(c.Volumes, config.Volume{Name: w[1], Target: w[2], ReadOnly: len(w) == 4})
		} else if len(w) == 2 && w[0] == "memory" {
			m, err := memorySize(w[1], 4)
			if err != nil {
				return "", err
			}
			c.MemoryMiB = m
		} else if len(w) == 2 && w[0] == "pids-limit" {
			n, err := strconv.Atoi(w[1])
			if err != nil || n < 1 || n > 1048576 {
				return "", errors.New("PID limit must be 1..1048576")
			}
			c.Pids = n
		} else if len(w) == 2 && w[0] == "image" {
			c.Image = w[1]
		} else if len(w) >= 2 && w[0] == "command" {
			c.Command = append([]string{}, w[1:]...)
		} else if len(w) == 3 && w[0] == "environment" {
			if c.Environment == nil {
				c.Environment = map[string]string{}
			}
			c.Environment[w[1]] = w[2]
		} else if len(w) == 2 && w[0] == "publish" {
			c.Publish = append(c.Publish, w[1])
		} else if len(w) == 2 && w[0] == "restart-policy" {
			c.Restart = w[1]
		} else if len(w) == 3 && w[0] == "no" && w[1] == "environment" {
			delete(c.Environment, w[2])
		} else if strings.Join(w, " ") == "no publish" {
			c.Publish = nil
		} else if strings.Join(w, " ") == "no command" {
			c.Command = nil
		} else {
			return "", errors.New("expected image REF, no container-isolation, container-isolation inherit, or container-isolation kata-containers enable")
		}
	}
	s.Candidate.Containers[s.Object] = c
	return "", nil
}

func (s *Session) isolation(w []string) (string, error) {
	i := s.Candidate.Isolation
	switch strings.Join(w, " ") {
	case "enable":
		i.Enabled = true
	case "disable":
		i.Enabled = false
	case "install runtime kata-containers":
		i.Installed = true
	case "kata-containers default-runtime clh":
		i.Runtime = "clh"
	case "kata-containers apply-to all-containers":
		i.ApplyTo = "all-containers"
	case "kata-containers apply-to explicit":
		i.ApplyTo = "explicit"
	case "kata-containers runtime clh vm-factory enable":
		return "", errors.New("VM factory is not implemented for this clh integration; configuration unchanged")
	case "kata-containers runtime clh vm-factory disable":
		i.VMFactory = false
	default:
		if len(w) == 5 && strings.Join(w[:4], " ") == "kata-containers runtime clh default-memory" {
			v, err := memory(w[4])
			if err != nil {
				return "", err
			}
			i.MemoryMiB = v
		} else if len(w) == 3 && w[0] == "kata-containers" && w[1] == "base-os" {
			if w[2] != "appliance" && w[2] != "Microsoft/CBL-Mariner" {
				return "", errors.New("guest not bundled; base-os is a build-time Kata guest artifact, not an arbitrary OCI image")
			}
			i.BaseOS = w[2]
		} else {
			return "", errors.New("unsupported isolation command; ? lists supported commands")
		}
	}
	s.Candidate.Isolation = i
	return "", nil
}

func memory(v string) (int, error) {
	return memorySize(v, 128)
}
func memorySize(v string, minimum int) (int, error) {
	mult := 1
	switch {
	case strings.HasSuffix(v, "MiB"):
		v = strings.TrimSuffix(v, "MiB")
	case strings.HasSuffix(v, "M"):
		v = strings.TrimSuffix(v, "M")
	case strings.HasSuffix(v, "GiB"):
		v, mult = strings.TrimSuffix(v, "GiB"), 1024
	case strings.HasSuffix(v, "G"):
		v, mult = strings.TrimSuffix(v, "G"), 1024
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 1048576/mult || n*mult < minimum {
		return 0, fmt.Errorf("memory must be %dM..1024G", minimum)
	}
	return n * mult, nil
}

func (s *Session) operational(w []string) (string, error) {
	line := strings.Join(w, " ")
	switch line {
	case "show running-config":
		return string(s.Running.JSON()), nil
	case "show candidate-config":
		return string(s.Candidate.JSON()), nil
	case "show startup-config":
		c, err := config.Load(filepath.Join(s.StateDir, "startup-config.json"))
		if err != nil {
			return "", err
		}
		return string(c.JSON()), nil
	case "show container-isolation":
		var b strings.Builder
		fmt.Fprintf(&b, "enabled=%t default=%s runtime=%s memory=%dMiB\n", s.Running.Isolation.Enabled, s.Running.Isolation.ApplyTo, s.Running.Isolation.Runtime, s.Running.Isolation.MemoryMiB)
		for _, n := range config.Keys(s.Running.Containers) {
			fmt.Fprintf(&b, "%s override=%s effective=%s\n", n, s.Running.Containers[n].Isolation, s.Running.EffectiveRuntime(n))
		}
		return strings.TrimSpace(b.String()), nil
	case "copy running-config startup-config", "write memory":
		if s.Save != nil {
			if err := s.Save(s.Running); err != nil {
				return "", err
			}
			return "Startup configuration saved.", nil
		}
		if err := config.AtomicWrite(filepath.Join(s.StateDir, "startup-config.json"), s.Running.JSON(), 0600); err != nil {
			return "", err
		}
		return "Startup configuration saved.", nil
	case "request system container-isolation install runtime kata-containers":
		if s.Mode == "" {
			return "", errors.New("enter config; runtime selection is part of candidate configuration")
		}
		return s.isolation([]string{"install", "runtime", "kata-containers"})
	case "request system container-isolation kata-containers vm-factory create":
		return "", errors.New("VM factory is unavailable in this clh integration")
	case "request platform debug service-container enable", "request platform debug service-container enter", "request platform debug service-container disable":
		if s.Platform != nil {
			return s.Platform(w, s.Running)
		}
		return "", errors.New("debug service-container requires the Linux platform adapter; not available in the configuration CLI")
	case "show version":
		return "FractalCR Software, Version 0.5 (development)\nOffline configuration session; no live platform attached.", nil
	}
	if len(w) == 4 && strings.Join(w[:3], " ") == "request system render" {
		if s.Platform != nil {
			return "", errors.New("render is available only in the offline configuration compiler")
		}
		if err := s.Running.Export(w[3]); err != nil {
			return "", err
		}
		return "Runtime configuration and container creation plan rendered to " + w[3], nil
	}
	if s.Platform != nil {
		return s.Platform(w, s.Running)
	}
	return "", errors.New("unknown operational command; ? lists supported commands")
}
