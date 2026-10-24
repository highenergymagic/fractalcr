package cli

import (
	"errors"
	"fmt"
	"fractalcr/internal/config"
	"slices"
	"strconv"
	"strings"
)

func defaultCommandLevel(mode string, words []string) int {
	if len(words) > 0 && words[0] == "do" {
		words = words[1:]
		mode = ""
	}
	if len(words) == 0 {
		return 0
	}
	if mode != "" {
		if len(words) == 1 && (words[0] == "exit" || words[0] == "end" || words[0] == "help") {
			return 0
		}
		return 15
	}
	command := strings.Join(words, " ")
	switch words[0] {
	case "enable", "disable", "exit", "logout", "help":
		return 0
	}
	// IOS exposes these calendar transfers in user EXEC. Setting an arbitrary
	// clock value remains privileged; keep the exceptions exact.
	if command == "clock read-calendar" || command == "clock update-calendar" {
		return 1
	}
	if len(words) >= 2 && words[0] == "show" && words[1] == "interfaces" {
		return 1
	}
	if len(words) == 5 && words[0] == "show" && words[1] == "ip" && words[2] == "interface" && words[4] == "brief" {
		return 1
	}
	if words[0] == "terminal" || (len(words) >= 2 && words[0] == "show" && (words[1] == "users" || words[1] == "line")) {
		return 1
	}
	if words[0] == "dir" || words[0] == "cd" || command == "pwd" || command == "show file systems" || command == "show reload" || command == "show clock detail" || command == "show calendar" {
		return 1
	}
	// Configuration, logs, workload inspection and native clients may contain
	// credentials or grant host control. Only explicit read-only views are public.
	for _, prefix := range []string{"show version", "show privilege", "show users", "show terminal", "show history", "show interfaces", "show ip interface brief", "show ip route", "show platform", "show system resources", "show clock", "show ntp status", "show ntp associations"} {
		if command == prefix {
			return 1
		}
	}
	return 15
}
func (s *Session) aaaCommand(w []string) (bool, string, error) {
	if len(w) > 0 && (w[0] == "enable" || w[0] == "username") {
		for _, i := range []int{1, 2, 4} {
			if i >= len(w) || w[i] != "algorithm-type" || (w[0] == "enable" && i != 1) || (w[0] == "username" && (i == 1 || (i == 4 && w[2] != "privilege"))) {
				continue
			}
			if i+3 >= len(w) || w[i+2] != "secret" {
				return true, "", errors.New("incomplete secret algorithm command")
			}
			kind := "9"
			if w[i+1] == "sha256" {
				kind = "8"
			} else if w[i+1] != "scrypt" {
				return true, "", errors.New("unsupported secret algorithm")
			}
			h, err := s.Candidate.AAA.HashIOSSecret(kind, strings.Join(w[i+3:], " "))
			if err != nil {
				return true, "", err
			}
			next := append(append([]string{}, w[:i]...), "secret", kind, h)
			return s.aaaCommand(next)
		}
	}
	if len(w) > 0 && w[0] == "disable" {
		level := 1
		if s.Privilege < level {
			level = s.Privilege
		}
		if len(w) == 2 {
			level, _ = strconv.Atoi(w[1])
		}
		if level > s.Privilege {
			return true, "", fmt.Errorf("New privilege level must be less than current privilege level")
		}
		s.Privilege = level
		s.Mode, s.Object = "", ""
		return true, "", nil
	}
	line := strings.Join(w, " ")
	switch line {
	case "no exec-timeout", "default exec-timeout":
		timeout := 0
		if line == "default exec-timeout" {
			timeout = 600
		}
		if s.Mode == "line-console" {
			s.Candidate.AAA.ConsoleTimeout = timeout
		} else {
			s.editVTY(func(line *config.VTYLine) { line.Timeout = timeout })
		}
		return true, "", nil
	case "no enable secret":
		s.Candidate.AAA.EnableSecret = ""
		return true, "", nil
	case "show privilege":
		return true, fmt.Sprintf("Current privilege level is %d", s.Privilege), nil
	case "aaa new-model", "aaa authentication login default local", "aaa authorization exec default local", "aaa accounting exec default start-stop local", "aaa accounting commands 15 default start-stop local":
		s.Candidate.AAA.Enabled = true
		return true, "", nil
	case "no aaa new-model":
		s.Candidate.AAA.Enabled = false
		s.Candidate.AAA.SSH = false
		for n, line := range s.Candidate.AAA.VTY {
			line.SSH = false
			s.Candidate.AAA.VTY[n] = line
		}
		return true, "", nil
	case "ip ssh version 2":
		s.Candidate.AAA.SSH = true
		return true, "", nil
	case "no ip ssh":
		s.Candidate.AAA.SSH = false
		for n, line := range s.Candidate.AAA.VTY {
			line.SSH = false
			s.Candidate.AAA.VTY[n] = line
		}
		return true, "", nil
	case "line console 0":
		s.Mode = "line-console"
		s.Object = "0"
		return true, "", nil
	case "login local":
		s.Candidate.AAA.Enabled = true
		return true, "", nil
	case "transport input ssh":
		s.editVTY(func(line *config.VTYLine) { line.SSH = true })
		return true, "", nil
	case "transport input none":
		s.editVTY(func(line *config.VTYLine) { line.SSH = false })
		return true, "", nil
	case "no security passwords min-length":
		s.Candidate.AAA.PasswordMinLength = 0
		return true, "", nil
	}
	if len(w) == 4 && w[0] == "security" && w[1] == "passwords" {
		s.Candidate.AAA.PasswordMinLength, _ = strconv.Atoi(w[3])
		return true, "", nil
	}
	if (len(w) == 3 || len(w) == 4) && w[0] == "line" && w[1] == "vty" {
		first, _ := strconv.Atoi(w[2])
		last := first
		if len(w) == 4 {
			last, _ = strconv.Atoi(w[3])
		}
		if last < first {
			return true, "", errors.New("last line number must be greater than or equal to the first")
		}
		s.Mode, s.Object = "line-vty", fmt.Sprintf("%d %d", first, last)
		return true, "", nil
	}
	if len(w) >= 6 && w[0] == "username" && w[4] == "ssh-key" {
		key, err := config.NormalizeSSHKey(strings.Join(w[5:], " "))
		if err != nil {
			return true, "", err
		}
		if s.Candidate.AAA.Users == nil {
			s.Candidate.AAA.Users = map[string]config.User{}
		}
		u := s.Candidate.AAA.Users[w[1]]
		u.Privilege, _ = strconv.Atoi(w[3])
		if !slices.Contains(u.SSHKeys, key) {
			u.SSHKeys = append(u.SSHKeys, key)
		}
		s.Candidate.AAA.Users[w[1]] = u
		return true, "", nil
	}
	if len(w) >= 5 && w[0] == "no" && w[1] == "username" && w[3] == "ssh-key" {
		key, err := config.NormalizeSSHKey(strings.Join(w[4:], " "))
		if err != nil {
			return true, "", err
		}
		u, ok := s.Candidate.AAA.Users[w[2]]
		if !ok {
			return true, "", errors.New("user does not exist")
		}
		u.SSHKeys = slices.DeleteFunc(u.SSHKeys, func(v string) bool { return v == key })
		s.Candidate.AAA.Users[w[2]] = u
		return true, "", nil
	}
	if len(w) == 3 && w[0] == "no" && w[1] == "username" {
		delete(s.Candidate.AAA.Users, w[2])
		return true, "", nil
	}
	if len(w) >= 4 && w[0] == "username" {
		if !config.Name.MatchString(w[1]) {
			return true, "", errors.New("invalid username")
		}
		u, exists := s.Candidate.AAA.Users[w[1]]
		if !exists {
			u.Privilege = 1
		}
		i := 2
		if w[i] == "privilege" {
			n, e := strconv.Atoi(w[i+1])
			if e != nil || n < 0 || n > 15 {
				return true, "", errors.New("invalid privilege (0..15)")
			}
			u.Privilege = n
			i += 2
		}
		if i < len(w) {
			if w[i] == "secret" && i+1 < len(w) && w[i+1] == "7" {
				return true, "", errors.New("type 7 is a password encoding, not a secret encoding")
			}
			h, e := s.hashCLISecret(w[i+1:])
			if e != nil {
				return true, "", e
			}
			u.Secret = h
		} else if !exists {
			return true, "", errors.New("new users require a password or secret")
		}
		if s.Candidate.AAA.Users == nil {
			s.Candidate.AAA.Users = map[string]config.User{}
		}
		s.Candidate.AAA.Users[w[1]] = u
		return true, "", nil
	}
	if len(w) >= 3 && w[0] == "enable" && w[1] == "secret" {
		level, start := 15, 2
		if w[2] == "level" {
			level, _ = strconv.Atoi(w[3])
			start = 4
		}
		if start < len(w) && w[start] == "7" {
			return true, "", errors.New("type 7 is a password encoding, not a secret encoding")
		}
		h, e := s.hashCLISecret(w[start:])
		if e == nil {
			if level == 15 {
				s.Candidate.AAA.EnableSecret = h
			} else {
				if s.Candidate.AAA.EnableSecrets == nil {
					s.Candidate.AAA.EnableSecrets = map[int]string{}
				}
				s.Candidate.AAA.EnableSecrets[level] = h
			}
		}
		return true, "", e
	}
	if len(w) == 5 && w[0] == "no" && w[1] == "enable" && w[2] == "secret" && w[3] == "level" {
		level, _ := strconv.Atoi(w[4])
		if level == 15 {
			s.Candidate.AAA.EnableSecret = ""
		} else {
			delete(s.Candidate.AAA.EnableSecrets, level)
		}
		return true, "", nil
	}
	if (len(w) == 2 || len(w) == 3) && w[0] == "exec-timeout" {
		m, e := strconv.Atoi(w[1])
		sec := 0
		var f error
		if len(w) == 3 {
			sec, f = strconv.Atoi(w[2])
		}
		if e != nil || f != nil || m < 0 || m > 1440 || sec < 0 || sec > 59 || m*60+sec > 86400 {
			return true, "", errors.New("invalid exec-timeout")
		}
		if s.Mode == "line-console" {
			s.Candidate.AAA.ConsoleTimeout = m*60 + sec
		} else {
			s.editVTY(func(line *config.VTYLine) { line.Timeout = m*60 + sec })
		}
		return true, "", nil
	}
	return false, "", nil
}

// Type 0 describes input, never the on-disk representation. Do not silently
// treat an imported Cisco ciphertext as the password a user must log in with.
func (s *Session) hashCLISecret(words []string) (string, error) {
	if len(words) > 0 && (words[0] == "8" || words[0] == "9") {
		if len(words) != 2 || !strings.HasPrefix(words[1], "$"+words[0]+"$") {
			return "", errors.New("invalid encoded secret")
		}
		if err := config.ValidateSecret(words[1]); err != nil {
			return "", err
		}
		return words[1], nil
	}
	if len(words) > 0 && words[0] == "7" {
		if len(words) != 2 {
			return "", errors.New("expected a type 7 password")
		}
		password, err := config.DecodeIOSPassword7(words[1])
		if err != nil {
			return "", err
		}
		return s.Candidate.AAA.HashIOSSecret("9", password)
	}
	if len(words) > 0 && words[0] == "pbkdf2" {
		if len(words) != 2 {
			return "", errors.New("expected a PBKDF2 secret verifier")
		}
		if err := config.ValidateSecret(words[1]); err != nil {
			return "", err
		}
		return words[1], nil
	}
	if len(words) > 0 && words[0] == "0" {
		words = words[1:]
	} else if len(words) > 0 && len(words[0]) == 1 && words[0][0] >= '1' && words[0][0] <= '9' {
		return "", errors.New("unsupported password encoding; use 0 followed by cleartext")
	}
	return s.Candidate.AAA.HashPassword(strings.Join(words, " "))
}

func (s *Session) editVTY(edit func(*config.VTYLine)) {
	var first, last int
	if _, err := fmt.Sscanf(s.Object, "%d %d", &first, &last); err != nil {
		return
	}
	if s.Candidate.AAA.VTY == nil {
		s.Candidate.AAA.VTY = map[int]config.VTYLine{}
	}
	for i := first; i <= last; i++ {
		line := s.Candidate.AAA.Line(i)
		edit(&line)
		s.Candidate.AAA.VTY[i] = line
	}
}
