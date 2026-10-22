package appliance

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fractalcr/internal/cli"
	"fractalcr/internal/config"
)

type reply struct {
	Output      string      `json:"output"`
	Prompt      string      `json:"prompt"`
	Error       string      `json:"error,omitempty"`
	Completion  *string     `json:"completion,omitempty"`
	Local       string      `json:"local,omitempty"`
	ErrorColumn *int        `json:"error_column,omitempty"`
	Argv        []string    `json:"argv,omitempty"`
	Challenge   bool        `json:"challenge,omitempty"`
	Secret      bool        `json:"secret,omitempty"`
	Close       bool        `json:"close,omitempty"`
	Filter      *cli.Filter `json:"filter,omitempty"`
}

func Serve() error {
	path := "/run/fractal-management.sock"
	os.Remove(path)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	sessions := map[int32]*loginSession{}
	load := func() *loginSession {
		c, e := loadRunning()
		if e != nil {
			c = config.Default()
		}
		return &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), StateDir: "/datastore/data0/config", Apply: Apply, Platform: Operations, Save: Save, EnforcePrivilege: true}, stage: "username", last: time.Now()}
	}
	// Malformed configuration fails closed; ROMMON 0x2142 selects explicit
	// physical-console recovery without discarding the saved configuration.
	recoveryErr := recoverTransaction()
	if fd, e := strconv.Atoi(os.Getenv("FCR_READY_FD")); e == nil {
		f := os.NewFile(uintptr(fd), "ready")
		f.Write([]byte{'R'})
		f.Close()
		os.Unsetenv("FCR_READY_FD")
	}
	for {
		conn, e := listener.AcceptUnix()
		if e != nil {
			return e
		}
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		raw, _ := conn.SyscallConn()
		authorized := false
		var peerPID int32
		if raw != nil {
			raw.Control(func(fd uintptr) {
				u, e := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
				authorized = e == nil && u.Uid == 0
				if authorized {
					peerPID = u.Pid
				}
			})
		}
		if !authorized {
			conn.Close()
			continue
		}
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 4096), 4096)
		if !scanner.Scan() {
			conn.Close()
			continue
		}
		var request managementRequest
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			conn.Close()
			continue
		}
		login := sessions[peerPID]
		newSession := login == nil
		if login == nil || request.Command == "__connect" {
			for pid, old := range sessions {
				if syscall.Kill(int(pid), 0) != nil {
					if old.authenticated && old.cli.Running.AAA.Enabled {
						_ = audit(old.user, old.transport, "disconnect", "")
					}
					delete(sessions, pid)
				}
			}
			if len(sessions) >= 32 {
				conn.Close()
				continue
			}
			login = load()
			sessions[peerPID] = login
		}
		session := login.cli
		current, loadErr := loadRunning()
		if loadErr != nil {
			json.NewEncoder(conn).Encode(reply{Error: "Configuration unavailable; authentication fails closed.", Close: true})
			conn.Close()
			continue
		}
		if newSession && request.Command != "__connect" && (!current.AAA.Enabled || request.Transport == "ssh") {
			connect := request
			connect.Command = "__connect"
			response, _ := login.authenticate(connect, current, time.Now())
			if !login.authenticated {
				json.NewEncoder(conn).Encode(response)
				conn.Close()
				continue
			}
		}
		if response, handled := login.authenticate(request, current, time.Now()); handled {
			json.NewEncoder(conn).Encode(response)
			conn.Close()
			continue
		}
		line := strings.TrimSpace(request.Command)
		var output string
		var err error
		var completion *string
		var localCommand string
		var argv []string
		var filter *cli.Filter
		if recoveryErr != nil {
			recoveryErr = recoverTransaction()
		}
		if recoveryErr != nil {
			err = fmt.Errorf("interrupted configuration transaction needs recovery: %w", recoveryErr)
		} else if strings.HasPrefix(request.Command, "__complete ") {
			completed, matches := session.Complete(strings.TrimPrefix(request.Command, "__complete "))
			completion = &completed
			output = matches
		} else if strings.HasPrefix(request.Command, "__help ") {
			output = session.ContextHelp(strings.TrimPrefix(request.Command, "__help "))
		} else {
			var words []string
			var local bool
			if line != "__connect" && line != "__end" {
				words, local, err = session.Resolve(request.Command)
				if err == nil {
					words, filter, err = cli.SplitFilter(words)
				}
				if err == nil {
					line = cli.Join(words)
					if len(words) > 0 && words[0] == "do" {
						line = cli.Join(words[1:])
					}
				}
			}
			if err == nil && !session.Allowed(words) {
				err = fmt.Errorf("Authorization failed.")
			}
			if err == nil && current.AAA.Enabled && line != "__end" {
				err = audit(login.user, login.transport, "command", line)
			}
			if err == nil && (line == "logout" || (line == "exit" && session.Mode == "")) {
				if current.AAA.Enabled {
					_ = audit(login.user, login.transport, "logout", "")
				}
				login.logout()
				response := login.challenge()
				response.Close = login.transport == "ssh"
				if !current.AAA.Enabled {
					response, _ = login.authenticate(managementRequest{Command: "__connect"}, current, time.Now())
				}
				json.NewEncoder(conn).Encode(response)
				conn.Close()
				continue
			}
			if err == nil && line == "enable" {
				response := reply{Prompt: session.Prompt()}
				if session.Privilege != 15 {
					if current.AAA.EnableSecret == "" {
						response.Error = "No enable secret configured."
					} else {
						login.stage = "enable"
						response = login.challenge()
					}
				}
				json.NewEncoder(conn).Encode(response)
				conn.Close()
				continue
			}
			native := words
			if len(native) > 0 && native[0] == "do" {
				native = native[1:]
			}
			if err == nil && len(native) >= 3 && native[0] == "request" && (native[1] == "container-engine" || native[1] == "container-build") {
				program := "/usr/bin/docker"
				if native[1] == "container-build" {
					program = "/usr/bin/buildctl"
				}
				argv = append([]string{program}, native[2:]...)
			} else if local && err == nil {
				localCommand = line
			} else if err == nil {
				switch line {
				case "__end":
					session.Mode, session.Object = "", ""
				case "__connect":
					session.Mode = ""
					session.Object = ""
					session.Candidate = session.Running.Clone()
				case "help":
					output = cli.Help
				case "show users":
					for _, user := range sessions {
						if user.authenticated {
							output += fmt.Sprintf("%-20s %-8s privilege %d\n", user.user, user.transport, user.cli.Privilege)
						}
					}
				case "show configuration diff":
					output = configDiff(session.Running, session.Candidate)
				case "copy startup-config candidate-config":
					if session.Mode == "" {
						err = fmt.Errorf("enter configuration mode first")
					} else {
						var candidate config.Config
						candidate, err = config.Load(startupPath)
						if err == nil {
							session.Candidate = candidate
							session.Mode = "config"
							session.Object = ""
						}
					}
				default:
					if line == "config" || line == "configure terminal" || line == "show running-config" {
						if c, e := loadRunning(); e == nil {
							session.Running = c
						}
					}
					if strings.HasPrefix(line, "request storage ") {
						output, err = Operations(strings.Fields(line), session.Candidate)
					} else {
						output, err = session.Execute(cli.Join(words))
					}
				}
			}
		}
		response := reply{Output: output, Prompt: session.Prompt(), Completion: completion, Local: localCommand, Argv: argv, Filter: filter}
		if err != nil {
			response.Error = err.Error()
			var syntax *cli.SyntaxError
			if errors.As(err, &syntax) && syntax.Kind == "invalid" {
				response.ErrorColumn = &syntax.Column
			}
		}
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		json.NewEncoder(conn).Encode(response)
		conn.Close()
	}
}
