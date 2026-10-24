package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	InterfaceName    func(string) (string, error)
	InterfaceNames   func() []string
	FileSystems      func() []string
	Save             func(config.Config) error
	Recovery         bool
	EnforcePrivilege bool
	Privilege        int
	Username         string
	ConfigBase       string
	Transactional    bool
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
			mode := s.Mode
			if mode == "line-console" || mode == "line-vty" {
				mode = "line"
			}
			suffix += "-" + mode
		}
		suffix += ")"
	}
	mark := "# "
	if s.EnforcePrivilege && s.Privilege < 2 && s.Mode == "" {
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
	// Ordinary IOS configuration edits the running configuration one command at
	// a time. Keep a private copy only while validating/applying that command.
	// Explicit transactions remain available for changes with interdependencies.
	if !s.Transactional {
		s.Candidate = s.Running.Clone()
		s.ConfigBase = string(s.Running.JSON())
	}
	before, mode, object := s.Candidate.Clone(), s.Mode, s.Object
	output, err := s.execute(line)
	if err == nil && !s.Transactional && string(s.Candidate.JSON()) != string(s.Running.JSON()) {
		err = s.applyCandidate()
	}
	if err != nil {
		s.Candidate, s.Mode, s.Object = before, mode, object
		return "", err
	}
	return output, nil
}

func (s *Session) applyCandidate() error {
	if s.ConfigBase != "" && s.ConfigBase != string(s.Running.JSON()) {
		return errors.New("running configuration changed in another session; abort and re-enter configuration mode")
	}
	if err := s.Candidate.Validate(); err != nil {
		return err
	}
	if string(s.Candidate.JSON()) != string(s.Running.JSON()) && s.Apply != nil {
		if err := s.Apply(s.Running, s.Candidate); err != nil {
			return err
		}
	}
	s.Running = s.Candidate.Clone()
	s.ConfigBase = string(s.Running.JSON())
	return nil
}

func (s *Session) execute(line string) (string, error) {
	w, _, mode, err := s.resolveMode(line)
	if err != nil {
		return "", err
	}
	if mode != s.Mode {
		s.Mode, s.Object = mode, ""
	}
	base, filter, err := SplitFilter(w)
	if err != nil {
		return "", err
	}
	if filter != nil {
		if filter.Redirect() {
			return "", errors.New("output redirection requires the appliance terminal")
		}
		result, e := s.execute(Join(base))
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
	if line == "no clock timezone" || line == "default clock timezone" {
		s.Candidate.Clock.Zone = ""
		s.Candidate.Clock.OffsetMinutes = 0
		return "", nil
	}
	if len(w) >= 4 && w[0] == "clock" && w[1] == "timezone" {
		minutes := "0"
		if len(w) == 5 {
			minutes = w[4]
		}
		c, err := config.ParseTimezone(w[2], w[3], minutes)
		if err == nil {
			c.Summer = s.Candidate.Clock.Summer
			s.Candidate.Clock = c
		}
		return "", err
	}
	if line == "no clock summer-time" {
		s.Candidate.Clock.Summer = nil
		return "", nil
	}
	if len(w) >= 4 && w[0] == "clock" && w[1] == "summer-time" {
		summer, err := config.ParseSummerTime(w[2], w[3], w[4:])
		if err == nil {
			s.Candidate.Clock.Summer = summer
		}
		return "", err
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
	case "config", "configure terminal", "configure candidate":
		if s.Mode == "" {
			s.Transactional = line == "configure candidate"
			s.Candidate = s.Running.Clone()
			s.ConfigBase = string(s.Running.JSON())
		}
		s.Mode, s.Object = "config", ""
		if line != "configure candidate" {
			return "Enter configuration commands, one per line.  End with CNTL/Z.", nil
		}
		return "", nil
	case "end":
		if s.Transactional && string(s.Candidate.JSON()) != string(s.Running.JSON()) {
			return "", errors.New("uncommitted transaction; commit or abort before leaving configuration mode")
		}
		s.Mode, s.Object = "", ""
		s.Transactional = false
		return "", nil
	case "exit":
		if s.Mode != "config" && s.Mode != "" {
			s.Mode, s.Object = "config", ""
		} else {
			if s.Transactional && string(s.Candidate.JSON()) != string(s.Running.JSON()) {
				return "", errors.New("uncommitted transaction; commit or abort before leaving configuration mode")
			}
			s.Mode = ""
			s.Transactional = false
		}
		return "", nil
	case "abort":
		transactional := s.Transactional
		s.Candidate = s.Running.Clone()
		s.Mode, s.Object = "", ""
		s.Transactional = false
		if transactional {
			return "Candidate discarded.", nil
		}
		return "Configuration mode ended; accepted commands remain in running-config.", nil
	case "commit":
		if s.Mode == "" {
			return "", errors.New("enter configuration mode first")
		}
		if err := s.applyCandidate(); err != nil {
			return "", err
		}
		return "Running configuration committed. Use copy running-config startup-config to persist.", nil
	}
	if w[0] == "show" || w[0] == "copy" || w[0] == "request" || w[0] == "write" || w[0] == "ping" || w[0] == "dir" || w[0] == "cd" || w[0] == "pwd" || w[0] == "delete" || w[0] == "move" || w[0] == "rename" || w[0] == "mkdir" || w[0] == "rmdir" || w[0] == "clock" || w[0] == "calendar" {
		return s.operational(w)
	}
	if s.Mode == "" {
		return "", errors.New("enter config to change configuration; ? lists commands")
	}
	if len(w) == 2 && w[0] == "interface" {
		if name, _, _, _ := config.VirtualInterface(w[1]); name != "" {
			w[1] = name
		}
		if s.InterfaceName != nil {
			name, err := s.InterfaceName(w[1])
			if err != nil {
				return "", err
			}
			w[1] = name
		}
		if !config.Interface.MatchString(w[1]) {
			return "", errors.New("invalid interface")
		}
		s.Candidate.EditInterface(w[1])
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
			if name, _, _, _ := config.VirtualInterface(w[2]); name != "" {
				w[2] = name
			}
			if s.InterfaceName != nil {
				name, err := s.InterfaceName(w[2])
				if err != nil {
					return "", err
				}
				w[2] = name
			}
			delete(s.Candidate.Interfaces, w[2])
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
	if handled, err := s.staticRoute(w); handled {
		return "", err
	}
	if handled, err := s.volumeConfiguration(w); handled {
		return "", err
	}
	if line == "system container-engine" {
		s.Mode, s.Object = "engine", ""
		return "", nil
	}
	if s.Mode == "engine" {
		return s.engine(w)
	}
	if s.Mode == "interface" {
		n := s.Candidate.AllInterfaces()[s.Object]
		if n == nil {
			return "", errors.New("interface no longer exists; re-enter configuration mode")
		}
		if handled, err := s.configureVirtualInterface(n, w); handled {
			return "", err
		}
		w, err = normalizeInterfaceAddress(w)
		if err != nil {
			return "", err
		}
		if configureInterfaceOptions(n, w) {
			return "", nil
		}
		if len(w) == 4 && w[0] == "no" && w[1] == "ip" && w[2] == "address" {
			if n.Address != w[3] {
				return "", errors.New("Invalid address")
			}
			n.Address = ""
			return "", nil
		}
		if len(w) == 3 && w[0] == "ip" {
			switch w[1] {
			case "address":
				if w[2] == "dhcp" {
					n.DHCP = true
					n.SecondaryAddresses = nil
					n.Address, n.Gateway, n.DNS = "", "", ""
				} else {
					n.Address = w[2]
					n.DHCP = false
				}
			case "gateway":
				n.Gateway = w[2]
			case "name-server":
				n.DNS = w[2]
			default:
				return "", errors.New("expected ip address, ip gateway, or ip name-server")
			}
			return "", nil
		}
		if line == "no ip gateway" {
			n.Gateway = ""
			return "", nil
		}
		if line == "no ip name-server" {
			n.DNS = ""
			return "", nil
		}
		if len(w) == 2 && w[0] == "role" {
			n.Role = w[1]
			return "", nil
		}
		if line == "shutdown" || line == "no shutdown" {
			n.Shutdown = line == "shutdown"
			return "", nil
		}
		if line == "no ip address" {
			n.Address = ""
			n.SecondaryAddresses = nil
			n.DHCP = false
			n.Gateway = ""
			n.DNS = ""
			return "", nil
		}
	}
	if len(w) == 3 && w[0] == "ntp" && w[1] == "server" {
		for _, v := range s.Candidate.NTP.Servers {
			if v == w[2] {
				return "", nil
			}
		}
		s.Candidate.NTP.Servers = append(s.Candidate.NTP.Servers, w[2])
		return "", nil
	}
	if len(w) == 4 && w[0] == "no" && w[1] == "ntp" && w[2] == "server" {
		for i, v := range s.Candidate.NTP.Servers {
			if v == w[3] {
				s.Candidate.NTP.Servers = append(s.Candidate.NTP.Servers[:i], s.Candidate.NTP.Servers[i+1:]...)
				break
			}
		}
		return "", nil
	}
	if len(w) == 2 && w[0] == "network" && s.Mode != "container" {
		if !config.Name.MatchString(w[1]) {
			return "", errors.New("invalid network name")
		}
		if s.Candidate.Networks == nil {
			s.Candidate.Networks = map[string]config.DockerNetwork{}
		}
		if _, ok := s.Candidate.Networks[w[1]]; !ok {
			s.Candidate.Networks[w[1]] = config.DockerNetwork{Driver: "bridge"}
		}
		s.Mode, s.Object = "network", w[1]
		return "", nil
	}
	if len(w) == 3 && w[0] == "no" && w[1] == "network" {
		delete(s.Candidate.Networks, w[2])
		if s.Mode == "network" && s.Object == w[2] {
			s.Mode, s.Object = "config", ""
		}
		return "", nil
	}
	if s.Mode == "network" {
		return s.network(w)
	}
	if len(w) == 2 && w[0] == "hostname" {
		if !config.Name.MatchString(w[1]) {
			return "", errors.New("invalid hostname")
		}
		s.Candidate.Hostname = w[1]
		return "", nil
	}
	if len(w) == 3 && w[0] == "file" && w[1] == "prompt" {
		s.Candidate.FilePrompt = w[2]
		if w[2] == "alert" {
			s.Candidate.FilePrompt = ""
		}
		return "", nil
	}
	if len(w) == 3 && (w[0] == "no" || w[0] == "default") && w[1] == "file" && w[2] == "prompt" {
		s.Candidate.FilePrompt = ""
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
	if handled, err := configureAttachments(&c, w); handled {
		if err == nil {
			s.Candidate.Containers[s.Object] = c
		}
		return "", err
	}
	if handled, err := configureContainerOptions(&c, w); handled {
		if err == nil {
			s.Candidate.Containers[s.Object] = c
		}
		return "", err
	}
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
	case "no network":
		c.Network = ""
	case "no container-isolation":
		c.Isolation = "none"
	case "container-isolation inherit":
		c.Isolation = "inherit"
	case "container-isolation kata-containers enable":
		c.Isolation = "kata"
	default:
		if len(w) == 2 && w[0] == "network" {
			c.Network = w[1]
		} else if len(w) >= 3 && len(w) <= 4 && w[0] == "volume" {
			if len(w) == 4 && w[3] != "read-only" {
				return "", errors.New("expected volume NAME TARGET [read-only]")
			}
			c.Volumes = slices.DeleteFunc(c.Volumes, func(v config.Volume) bool { return v.Target == w[2] })
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
			if !slices.Contains(c.Publish, w[1]) {
				c.Publish = append(c.Publish, w[1])
			}
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
		i.VMFactory = true
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
		text := s.Running.CLI()
		return fmt.Sprintf("Building configuration...\n\nCurrent configuration : %d bytes\n%s", len(text), text), nil
	case "show running-config json":
		return string(s.Running.JSON()), nil
	case "show candidate-config":
		return s.Candidate.CLI(), nil
	case "show startup-config", "show startup-config json":
		c, err := config.Load(filepath.Join(s.StateDir, "startup-config.json"))
		if err != nil {
			return "", err
		}
		if line == "show startup-config json" {
			return string(c.JSON()), nil
		}
		return c.CLI(), nil
	case "show container-isolation":
		var b strings.Builder
		fmt.Fprintf(&b, "enabled=%t default=%s runtime=%s memory=%dMiB\n", s.Running.Isolation.Enabled, s.Running.Isolation.ApplyTo, s.Running.Isolation.Runtime, s.Running.Isolation.MemoryMiB)
		for _, n := range config.Keys(s.Running.Containers) {
			fmt.Fprintf(&b, "%s override=%s effective=%s\n", n, s.Running.Containers[n].Isolation, s.Running.EffectiveRuntime(n))
		}
		return strings.TrimSpace(b.String()), nil
	case "copy running-config startup-config", "write memory":
		if s.Transactional && string(s.Candidate.JSON()) != string(s.Running.JSON()) {
			return "", errors.New("uncommitted transaction; commit before saving")
		}
		if s.Save != nil {
			if err := s.Save(s.Running); err != nil {
				return "", err
			}
			return "Building configuration...\n[OK]", nil
		}
		if err := config.AtomicWrite(filepath.Join(s.StateDir, "startup-config.json"), s.Running.JSON(), 0600); err != nil {
			return "", err
		}
		return "Building configuration...\n[OK]", nil
	case "request system container-isolation install runtime kata-containers":
		if s.Mode == "" {
			return "", errors.New("enter config; runtime selection is part of candidate configuration")
		}
		return s.isolation([]string{"install", "runtime", "kata-containers"})
	case "request system container-isolation kata-containers vm-factory create":
		if s.Platform != nil {
			return s.Platform(w, s.Running)
		}
		return "", errors.New("VM factory requires a live platform")
	case "request platform debug service-container enable", "request platform debug service-container enter", "request platform debug service-container disable":
		if s.Platform != nil {
			return s.Platform(w, s.Running)
		}
		return "", errors.New("debug service-container requires the Linux platform adapter; not available in the configuration CLI")
	case "show version":
		return "FractalCR offline configuration compiler\nFractal Microsystems - Copyright (c) 2025-2026\nNo live platform attached.", nil
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
