package cli

import (
	"errors"
	"fmt"
	"fractalcr/internal/config"
	"strconv"
	"strings"
)

func (s *Session) Allowed(words []string) bool {
	if !s.EnforcePrivilege || s.Privilege == 15 {
		return true
	}
	if len(words) > 0 && words[0] == "do" {
		words = words[1:]
	}
	if len(words) == 0 {
		return true
	}
	command := strings.Join(words, " ")
	switch command {
	case "enable", "disable", "exit", "logout", "help":
		return true
	}
	if s.Privilege < 1 {
		return false
	}
	if words[0] == "terminal" {
		return true
	}
	// Configuration, logs, workload inspection and native clients may contain
	// credentials or grant host control. Only explicit read-only views are public.
	for _, prefix := range []string{"show version", "show privilege", "show users", "show terminal", "show history", "show interfaces", "show ip interface brief", "show ip route", "show platform", "show system resources", "show capabilities"} {
		if command == prefix {
			return true
		}
	}
	return false
}
func (s *Session) aaaCommand(w []string) (bool, string, error) {
	line := strings.Join(w, " ")
	switch line {
	case "show privilege":
		return true, fmt.Sprintf("Current privilege level is %d", s.Privilege), nil
	case "disable":
		if s.Privilege > 1 {
			s.Privilege = 1
		}
		s.Mode = ""
		s.Object = ""
		return true, "", nil
	case "aaa new-model", "aaa authentication login default local", "aaa authorization exec default local", "aaa accounting exec default start-stop local", "aaa accounting commands 15 default start-stop local":
		s.Candidate.AAA.Enabled = true
		return true, "", nil
	case "no aaa new-model":
		s.Candidate.AAA.Enabled = false
		s.Candidate.AAA.SSH = false
		return true, "", nil
	case "ip ssh version 2":
		s.Candidate.AAA.SSH = true
		return true, "", nil
	case "no ip ssh":
		s.Candidate.AAA.SSH = false
		return true, "", nil
	case "line console 0":
		s.Mode = "line-console"
		return true, "", nil
	case "line vty 0 15":
		s.Mode = "line-vty"
		return true, "", nil
	case "login local":
		s.Candidate.AAA.Enabled = true
		return true, "", nil
	case "transport input ssh":
		s.Candidate.AAA.SSH = true
		return true, "", nil
	case "transport input none":
		s.Candidate.AAA.SSH = false
		return true, "", nil
	}
	if len(w) == 3 && w[0] == "no" && w[1] == "username" {
		delete(s.Candidate.AAA.Users, w[2])
		return true, "", nil
	}
	if len(w) >= 6 && w[0] == "username" && w[2] == "privilege" && w[4] == "secret" {
		n, e := strconv.Atoi(w[3])
		if e != nil || n < 0 || n > 15 || !config.Name.MatchString(w[1]) {
			return true, "", errors.New("invalid username or privilege")
		}
		secret := w[5:]
		if len(secret) > 1 && secret[0] == "0" {
			secret = secret[1:]
		}
		h, e := config.HashSecret(strings.Join(secret, " "))
		if e != nil {
			return true, "", e
		}
		if s.Candidate.AAA.Users == nil {
			s.Candidate.AAA.Users = map[string]config.User{}
		}
		s.Candidate.AAA.Users[w[1]] = config.User{Secret: h, Privilege: n}
		return true, "", nil
	}
	if len(w) >= 3 && w[0] == "enable" && w[1] == "secret" {
		secret := w[2:]
		if len(secret) > 1 && secret[0] == "0" {
			secret = secret[1:]
		}
		h, e := config.HashSecret(strings.Join(secret, " "))
		if e == nil {
			s.Candidate.AAA.EnableSecret = h
		}
		return true, "", e
	}
	if len(w) == 3 && w[0] == "exec-timeout" {
		m, e := strconv.Atoi(w[1])
		sec, f := strconv.Atoi(w[2])
		if e != nil || f != nil || m < 0 || m > 1440 || sec < 0 || sec > 59 || m*60+sec > 86400 {
			return true, "", errors.New("invalid exec-timeout")
		}
		if s.Mode == "line-console" {
			s.Candidate.AAA.ConsoleTimeout = m*60 + sec
		} else {
			s.Candidate.AAA.VTYTimeout = m*60 + sec
		}
		return true, "", nil
	}
	return false, "", nil
}
