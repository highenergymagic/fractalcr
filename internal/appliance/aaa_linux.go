package appliance

import (
	"encoding/json"
	"fmt"
	"fractalcr/internal/cli"
	"fractalcr/internal/config"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type managementRequest struct {
	// Action is an editor/control event. Command is always literal user text.
	Action    string `json:"action,omitempty"`
	Command   string `json:"command"`
	User      string `json:"user,omitempty"`
	Transport string `json:"transport,omitempty"`
	Verifier  string `json:"verifier,omitempty"`
	VTY       int    `json:"vty,omitempty"`
	Peer      string `json:"peer,omitempty"`
}

func (r managementRequest) validate() error {
	switch r.Action {
	case "", "help", "complete":
		return nil
	case "connect", "tick", "cancel", "end":
		if r.Command == "" {
			return nil
		}
	}
	return fmt.Errorf("invalid management event")
}

type loginSession struct {
	pendingFile                      *fileDialog
	pendingReload                    *reloadDialog
	pendingCopy                      *copyDialog
	enableLevel                      int
	setup                            *setupDialog
	cli                              *cli.Session
	user, stage, transport, verifier string
	authenticated                    bool
	bootstrap                        bool
	vty                              int
	userPrivilege                    int
	peer                             string
	workingDirectory                 string
	last                             time.Time
}
type loginFailure struct {
	count int
	until time.Time
}

var loginFailures = map[string]loginFailure{}
var auditPath = "/datastore/data0/log/aaa.jsonl"

func audit(user, transport, event, command string) error {
	if err := checkDatastore(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(auditPath), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(auditPath+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if !privateAccountingFile(lock) {
		return fmt.Errorf("invalid accounting lock file")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if st, e := os.Lstat(auditPath); e == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("invalid accounting file")
		}
		if st.Size() > 4*1024*1024 {
			if err := os.Rename(auditPath, auditPath+".1"); err != nil {
				return err
			}
		}
	}
	f, err := os.OpenFile(auditPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if !privateAccountingFile(f) {
		return fmt.Errorf("invalid accounting file descriptor")
	}
	// Record command classes, never passwords, tokens, environment or native argv.
	words, _ := cli.Words(command)
	if len(words) > 2 {
		words = words[:2]
	}
	if len(words) > 1 && (words[0] == "command" || words[0] == "entrypoint" || words[0] == "description") {
		words = words[:1]
	}
	err = json.NewEncoder(f).Encode(map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "user": user, "transport": transport, "event": event, "command": strings.Join(words, " ")})
	if err != nil {
		return err
	}
	return f.Sync()
}

func privateAccountingFile(f *os.File) bool {
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return false
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Geteuid()) && owner.Nlink == 1
}

func (s *loginSession) challenge() reply {
	prompt := "Username: "
	secret := false
	if s.stage == "password" || s.stage == "enable" {
		prompt = "Password: "
		secret = true
	}
	return reply{Prompt: prompt, Challenge: true, Secret: secret}
}
func (s *loginSession) logout() {
	s.workingDirectory = ""
	s.pendingFile = nil
	s.pendingReload = nil
	s.pendingCopy = nil
	s.setup = nil
	s.authenticated = false
	s.bootstrap = false
	s.stage = "username"
	s.user = ""
	s.verifier = ""
	s.cli.Mode = ""
	s.cli.Object = ""
	s.cli.Privilege = 0
	s.cli.Username = ""
	s.cli.Transactional = false
	s.cli.Candidate = s.cli.Running.Clone()
}

// The request arrives on a root-only Unix socket. Only the SSH service can
// supply a verified VTY principal; SSH env/exec/subsystem requests are refused.
// Command execution is activity, not time spent idle at the prompt. Heartbeats
// must never renew the idle deadline, including while a native client streams.
func (s *loginSession) commandCompleted(r managementRequest, now time.Time) {
	if s.authenticated && r.Action != "tick" {
		s.last = now
	}
}

func (s *loginSession) authenticate(r managementRequest, c config.Config, now time.Time) (reply, bool) {
	s.cli.Running = c
	if !s.cli.Transactional {
		s.cli.Candidate = c.Clone()
	}
	if r.Action == "cancel" {
		if s.authenticated && s.stage == "enable" {
			s.stage = ""
			return reply{Prompt: s.cli.Prompt()}, true
		}
		if !s.authenticated {
			s.logout()
			return s.challenge(), true
		}
		return s.cancelDialog(), true
	}
	if r.Action == "connect" {
		s.logout()
		s.transport = "console"
		s.peer = r.Peer
		s.last = now
		if r.Transport == "ssh" {
			s.transport = "ssh"
			s.vty = r.VTY
			u, ok := c.AAA.Users[r.User]
			if !c.AAA.Enabled || r.VTY < 0 || r.VTY >= config.VTYCount || !c.AAA.Line(r.VTY).SSH || !ok || r.Verifier != u.AuthVerifier() {
				return reply{Close: true, Error: "SSH authorization failed."}, true
			}
			s.user = r.User
			s.verifier = u.AuthVerifier()
			s.userPrivilege = u.Privilege
			s.cli.Privilege = u.Privilege
			s.authenticated = true
		} else if !c.AAA.Enabled {
			s.user = "console"
			s.cli.Privilege = 15
			s.authenticated = true
			s.bootstrap = true
		}
		if s.authenticated {
			if c.AAA.Enabled {
				if e := audit(s.user, s.transport, "login", ""); e != nil {
					s.logout()
					return reply{Close: true, Error: "Accounting unavailable."}, true
				}
			}
			s.cli.Username = s.user
			return reply{Prompt: s.cli.Prompt()}, true
		}
		return s.challenge(), true
	}
	timeout := c.AAA.ConsoleTimeout
	if s.transport == "ssh" {
		timeout = c.AAA.Line(s.vty).Timeout
		if !c.AAA.Line(s.vty).SSH {
			s.logout()
			return reply{Close: true}, true
		}
	}
	if c.AAA.Enabled && s.authenticated && timeout > 0 && now.Sub(s.last) >= time.Duration(timeout)*time.Second {
		_ = audit(s.user, s.transport, "idle-timeout", "")
		s.logout()
		if s.transport == "ssh" {
			return reply{Close: true, Error: "Session timed out."}, true
		}
		return s.challenge(), true
	}
	// Enabling local login does not eject the physical console that configured
	// it. That bootstrap grant ends on logout/timeout; subsequent logins use AAA.
	if s.authenticated && c.AAA.Enabled && !s.bootstrap {
		u, ok := c.AAA.Users[s.user]
		if !ok || u.AuthVerifier() != s.verifier || u.Privilege != s.userPrivilege {
			_ = audit(s.user, s.transport, "authorization-revoked", "")
			s.logout()
			if s.transport == "ssh" {
				return reply{Close: true}, true
			}
			return s.challenge(), true
		}
	}
	if r.Action == "tick" {
		if s.stage == "enable" {
			return s.challenge(), true
		}
		if s.authenticated {
			return reply{Prompt: s.cli.Prompt()}, true
		}
		return s.challenge(), true
	}
	s.last = now
	if !s.authenticated || s.stage == "enable" {
		if r.Action != "" {
			return s.challenge(), true
		}
		if s.stage == "username" {
			if len(r.Command) > 63 {
				return s.challenge(), true
			}
			s.user = r.Command
			s.stage = "password"
			return s.challenge(), true
		}
		key := s.transport + ":" + s.user
		if _, known := c.AAA.Users[s.user]; !known {
			key = s.transport + ":<unknown>"
		}
		failure := loginFailures[key]
		if now.Before(failure.until) {
			r := s.challenge()
			r.Error = "Authentication temporarily locked. Try again later."
			return r, true
		}
		u, exists := c.AAA.Users[s.user]
		verifier := u.Secret
		if s.stage == "enable" {
			verifier = c.AAA.EnableSecret
			exists = verifier != ""
		}
		if !exists || !config.CheckSecret(verifier, r.Command) {
			failure.count++
			if failure.count >= 3 {
				failure.until = now.Add(30 * time.Second)
				failure.count = 0
			}
			loginFailures[key] = failure
			_ = audit(s.user, s.transport, "authentication-failed", "")
			s.stage = "username"
			if s.authenticated {
				s.stage = ""
				return reply{Prompt: s.cli.Prompt(), Error: "Access denied."}, true
			}
			answer := s.challenge()
			answer.Error = "Authentication failed."
			return answer, true
		}
		delete(loginFailures, key)
		event := "login"
		if s.stage == "enable" {
			s.cli.Privilege = s.enableLevel
			if s.enableLevel == 0 {
				s.cli.Privilege = 15
			}
			event = "enable"
		} else {
			s.cli.Privilege = u.Privilege
			s.verifier = u.AuthVerifier()
			s.userPrivilege = u.Privilege
		}
		if err := audit(s.user, s.transport, event, ""); err != nil {
			s.logout()
			return reply{Close: true, Error: "Accounting unavailable."}, true
		}
		s.authenticated = true
		s.stage = ""
		s.cli.Username = s.user
		return reply{Prompt: s.cli.Prompt()}, true
	}
	return reply{}, false
}
