package appliance

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func readManagementRequest(r io.Reader) (managementRequest, error) {
	var request managementRequest
	scanner := bufio.NewScanner(r)
	// The editor accepts 2047 bytes. JSON escaping can expand each byte to
	// six characters, in addition to the session's authentication fields.
	scanner.Buffer(make([]byte, 4096), 16*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return request, err
		}
		return request, io.EOF
	}
	err := json.Unmarshal(scanner.Bytes(), &request)
	return request, err
}

func Serve() error {
	path := "/run/fractal-management.sock"
	os.Remove(path)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	reloads, err := newReloadManager("/run/fractal-reload.json", func() error { _, err := control(4, ""); return err })
	if err != nil {
		return err
	}
	go reloads.run()
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	sessions := map[int32]*loginSession{}
	load := func() *loginSession {
		c, e := loadRunning()
		if e != nil {
			c = config.Default()
		}
		s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), StateDir: "/datastore/data0/config", Apply: Apply, InterfaceName: canonicalInterface, InterfaceNames: interfaceNames, FileSystems: filesystemNames, Save: Save, EnforcePrivilege: true}, stage: "username", last: time.Now()}
		s.cli.Platform = s.operations
		return s
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
		request, decodeErr := readManagementRequest(conn)
		if decodeErr != nil {
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
		respond := func(response reply) {
			login.commandCompleted(request.Command, time.Now())
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			json.NewEncoder(conn).Encode(response)
		}
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
				respond(response)
				conn.Close()
				continue
			}
		}
		if response, handled := login.authenticate(request, current, time.Now()); handled {
			if request.Command == "__connect" && login.authenticated && login.transport == "console" && !current.AAA.Enabled && !recoveryBoot() {
				if _, e := os.Stat(startupPath); os.IsNotExist(e) {
					login.setup = newSetup(current)
					response = login.setup.prompt()
				}
			} else if login.setup != nil && login.authenticated && request.Command == "__tick" {
				response = login.setup.prompt()
				response.Output = ""
			}
			if login.authenticated && len(login.pendingFile) > 0 && request.Command == "__tick" {
				response = reply{Prompt: "Delete " + deleteURL(login.pendingFile) + "? [confirm]", Challenge: true}
			}
			if login.authenticated && login.pendingReload != nil && request.Command == "__tick" {
				response = login.pendingReload.prompt()
			}
			if login.authenticated && login.pendingCopy != nil && request.Command == "__tick" {
				response = login.pendingCopy.prompt()
			}
			respond(response)
			conn.Close()
			continue
		}
		line := strings.TrimSpace(request.Command)
		if login.pendingCopy != nil {
			respond(login.answerCopy(line))
			conn.Close()
			continue
		}
		if login.pendingReload != nil {
			respond(login.answerReload(line, reloads, time.Now()))
			conn.Close()
			continue
		}
		if len(login.pendingFile) > 0 {
			pending := login.pendingFile
			login.pendingFile = nil
			response := reply{Prompt: session.Prompt()}
			if session.Privilege != 15 {
				response.Error = "Authorization failed."
			} else if line == "" || strings.EqualFold(line, "y") || strings.EqualFold(line, "yes") {
				out, err := fileOperation(pending)
				response.Output = out
				if err != nil {
					response.Error = err.Error()
				}
			} else {
				response.Output = "Delete cancelled."
			}
			respond(response)
			conn.Close()
			continue
		}
		if login.setup != nil || line == "setup" {
			response := reply{Prompt: session.Prompt()}
			if recoveryErr != nil {
				response.Error = "Configuration recovery must complete before setup."
			} else if login.transport != "console" || session.Privilege != 15 {
				response.Error = "Setup requires privileged physical-console access."
			} else {
				if login.setup == nil {
					login.setup = newSetup(current)
					response = login.setup.prompt()
				} else {
					response = login.setup.answer(request.Command, session)
					if !response.Challenge {
						completed := login.setup.stage == "complete"
						login.setup = nil
						if completed {
							login.logout()
							challenge := login.challenge()
							challenge.Output = response.Output
							response = challenge
						}
					}
				}
			}
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			respond(response)
			conn.Close()
			continue
		}
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
			output, err = session.QueryHelp(strings.TrimPrefix(request.Command, "__help "))
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
			if err == nil {
				fwords := words
				if len(fwords) > 0 && fwords[0] == "do" {
					fwords = fwords[1:]
				}
				fwords = login.fileWords(fwords)
				force := false
				for _, word := range fwords {
					if word == "/force" {
						force = true
					}
				}
				if len(fwords) >= 2 && fwords[0] == "delete" && !force {
					login.pendingFile = append([]string(nil), fwords...)
					respond(reply{Prompt: "Delete " + deleteURL(fwords) + "? [confirm]", Challenge: true})
					conn.Close()
					continue
				}
				if len(fwords) > 0 && fwords[0] == "reload" {
					respond(login.beginReload(fwords, reloads, time.Now()))
					conn.Close()
					continue
				}
				if len(fwords) == 3 && fwords[0] == "copy" && fwords[2] != "candidate-config" {
					respond(login.beginCopy(fwords))
					conn.Close()
					continue
				}
				if strings.Join(fwords, " ") == "show reload" {
					out := reloads.status(time.Now())
					if filter != nil {
						out = filter.Apply(out)
					}
					respond(reply{Prompt: session.Prompt(), Output: out})
					conn.Close()
					continue
				}
			}
			if err == nil && (line == "logout" || (line == "exit" && session.Mode == "")) {
				if current.AAA.Enabled {
					_ = audit(login.user, login.transport, "logout", "")
				}
				login.logout()
				response := login.challenge()
				response.Close = login.transport == "ssh" || !current.AAA.Enabled
				respond(response)
				conn.Close()
				continue
			}
			if level, enable := execEnableLevel(words); err == nil && enable {
				response := login.enable(level, current)
				respond(response)
				conn.Close()
				continue
			}
			native := words
			if len(native) > 0 && native[0] == "do" {
				native = native[1:]
			}
			if service, args, ok := cli.WorkloadArguments(native); err == nil && ok {
				if service == "help" {
					output = session.ContextHelp(cli.Join(native[:len(native)-1]) + " ")
				} else {
					argv = append([]string{"/usr/libexec/fractal-workload", service}, args...)
				}
			} else if local && err == nil {
				localCommand = line
			} else if err == nil {
				switch line {
				case "__end":
					if session.Mode != "" {
						output, err = session.Execute("end")
					}
				case "__connect":
					session.Mode = ""
					session.Object = ""
					session.Candidate = session.Running.Clone()
				case "help":
					output = cli.Help
				case "show users":
					output = sessionUsers(sessions, login, time.Now())
				case "show configuration diff":
					output = configDiff(session.Running, session.Candidate)
				case "copy startup-config candidate-config":
					if !session.Transactional {
						err = fmt.Errorf("enter configure candidate to stage startup configuration")
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
		respond(response)
		conn.Close()
	}
}
