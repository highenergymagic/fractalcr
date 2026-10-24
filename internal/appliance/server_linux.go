package appliance

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	Output      string         `json:"output"`
	Prompt      string         `json:"prompt"`
	Error       string         `json:"error,omitempty"`
	Completion  *string        `json:"completion,omitempty"`
	Local       string         `json:"local,omitempty"`
	ErrorColumn *int           `json:"error_column,omitempty"`
	Argv        []string       `json:"argv,omitempty"`
	Challenge   bool           `json:"challenge,omitempty"`
	Secret      bool           `json:"secret,omitempty"`
	Close       bool           `json:"close,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	Filter      *cli.Filter    `json:"filter,omitempty"`
	Notify      string         `json:"notify,omitempty"`
	Privilege   *int           `json:"privilege,omitempty"`
	Terminal    *terminalState `json:"terminal_settings,omitempty"`
}

func readManagementRequest(r io.Reader) (managementRequest, error) {
	var request managementRequest
	scanner := bufio.NewScanner(r)
	// The editor accepts 2047 bytes. JSON escaping can expand each byte to
	// six characters, in addition to the session's authentication fields.
	scanner.Buffer(make([]byte, 4096), 7*1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return request, err
		}
		return request, io.EOF
	}
	err := json.Unmarshal(scanner.Bytes(), &request)
	if err == nil {
		err = request.validate()
	}
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
	lineUses, counterErr := loadLineCounters("/run/fractal-line-counters.json")
	if counterErr != nil {
		log.Print("Line usage counters could not be restored")
	}
	load := func() *loginSession {
		c, e := loadRunning()
		if e != nil {
			c = config.Default()
		}
		s := &loginSession{cli: &cli.Session{Running: c, Candidate: c.Clone(), StateDir: "/datastore/data0/config", Apply: Apply, InterfaceName: canonicalInterface, InterfaceNames: interfaceNames, FileSystems: filesystemNames, Save: Save, EnforcePrivilege: true}, stage: "username", last: time.Now()}
		s.cli.Platform = s.operations
		s.cli.HardwareCalendar = hardwareCalendarPresent
		s.terminal = defaultTerminal()
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
		if newSession && (request.Action == "stream-check" || request.Action == "stream-end") {
			json.NewEncoder(conn).Encode(reply{Error: "Authorization failed."})
			conn.Close()
			continue
		}
		if login == nil || request.Action == "connect" {
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
			login.noticeSequence, _ = reloads.notifications(^uint64(0))
			sessions[peerPID] = login
		}
		session := login.cli
		if request.Terminal != nil {
			login.terminal = *request.Terminal
		}
		wasAuthenticated := login.authenticated
		respond := func(response reply) {
			if login.authenticated {
				response.Terminal = login.syncLineTerminal()
				level := session.Privilege
				response.Privilege = &level
			}
			if !wasAuthenticated && login.authenticated {
				if err := lineUses.record(terminalLine(login)); err != nil {
					log.Print("Line usage counters could not be saved")
				}
			}
			if login.authenticated && !response.Challenge {
				login.noticeSequence, response.Notify = reloads.notifications(login.noticeSequence)
			}
			login.commandCompleted(request, time.Now())
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			json.NewEncoder(conn).Encode(response)
		}
		current, loadErr := loadRunning()
		if loadErr != nil {
			json.NewEncoder(conn).Encode(reply{Error: "Configuration unavailable; authentication fails closed.", Close: true})
			conn.Close()
			continue
		}
		if newSession && request.Action != "connect" && (!current.AAA.Enabled || request.Transport == "ssh") {
			connect := request
			connect.Action = "connect"
			connect.Command = ""
			response, _ := login.authenticate(connect, current, time.Now())
			if !login.authenticated {
				respond(response)
				conn.Close()
				continue
			}
		}
		if response, handled := login.authenticate(request, current, time.Now()); handled {
			if request.Action == "connect" && login.authenticated && login.transport == "console" && !current.AAA.Enabled && !recoveryBoot() {
				if _, e := os.Stat(startupPath); os.IsNotExist(e) {
					login.setup = newSetup(current)
					response = login.setup.prompt()
				}
			} else if login.setup != nil && login.authenticated && request.Action == "tick" {
				response = login.setup.prompt()
				response.Output = ""
			}
			if login.authenticated && login.pendingFile != nil && request.Action == "tick" {
				response = login.pendingFile.prompt()
			}
			if login.authenticated && login.pendingReload != nil && request.Action == "tick" {
				response = login.pendingReload.prompt()
			}
			if login.authenticated && login.pendingCopy != nil && request.Action == "tick" {
				response = login.pendingCopy.prompt()
			}
			respond(response)
			conn.Close()
			continue
		}
		if request.Action == "end" {
			response := login.cancelDialog()
			if session.Mode != "" {
				response.Output, e = session.Execute("end")
				if e != nil {
					response.Error = e.Error()
				}
				response.Prompt = session.Prompt()
			}
			respond(response)
			conn.Close()
			continue
		}
		line := strings.TrimSpace(request.Command)
		if request.Action == "output" {
			response := reply{Prompt: session.Prompt()}
			f := login.pendingOutput
			login.pendingOutput = nil
			if f == nil {
				response.Error = "No output redirection is pending."
			} else {
				var err error
				response.Output, err = login.writeOutput(f, request.Command)
				if err != nil {
					response.Error = err.Error()
				}
			}
			login.outputCommand = nil
			respond(response)
			conn.Close()
			continue
		}
		if request.Action == "" {
			login.pendingOutput = nil
			login.outputCommand = nil
		}
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
		if login.pendingFile != nil {
			respond(login.answerFile(line))
			conn.Close()
			continue
		}
		if login.setup != nil || line == "setup" {
			response := reply{Prompt: session.Prompt()}
			if recoveryErr != nil {
				response.Error = "Configuration recovery must complete before setup."
			} else if login.transport != "console" || !session.AllowedEXEC([]string{"setup"}) {
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
		} else if request.Action == "complete" {
			completed, matches := session.Complete(request.Command)
			completion = &completed
			output = matches
		} else if request.Action == "help" {
			output, err = session.QueryHelp(request.Command)
		} else {
			var words []string
			var local bool
			{
				words, local, err = session.Resolve(request.Command)
				if err == nil && !session.Allowed(words) {
					err = fmt.Errorf("Authorization failed.")
				}
				if err == nil {
					login.outputCommand = append([]string(nil), words...)
					if len(login.outputCommand) > 0 && login.outputCommand[0] == "do" {
						login.outputCommand = login.outputCommand[1:]
					}
				}
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
			if err == nil && current.AAA.Enabled {
				err = audit(login.user, login.transport, "command", line)
			}
			if err == nil {
				fwords := words
				if len(fwords) > 0 && fwords[0] == "do" {
					fwords = fwords[1:]
				}
				fwords = login.fileWords(fwords)
				if len(fwords) >= 2 {
					switch fwords[0] {
					case "delete", "mkdir", "rmdir", "rename", "move":
						respond(login.beginFile(fwords))
						conn.Close()
						continue
					}
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
			} else if err == nil && len(native) >= 2 && native[0] == "show" && native[1] == "line" {
				output = showLine(native, sessions, login, lineUses.Uses, time.Now())
			} else if err == nil && len(native) == 2 && native[0] == "show" && native[1] == "terminal" {
				output = terminalDisplay(login, terminalLine(login), current, time.Now())
			} else if local && err == nil {
				localCommand = line
			} else if err == nil {
				switch line {
				case "show reload":
					output = reloads.status(time.Now(), current.Clock)
				case "help":
					output = cli.Help
				case "show users", "show users all", "show users wide", "show users all wide":
					output = sessionUsersOptions(sessions, login, time.Now(), strings.Contains(line, " all"), strings.HasSuffix(line, " wide"))
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
		if err == nil && filter.Redirect() {
			if localCommand != "" {
				login.pendingOutput = filter
			} else {
				output, err = login.writeOutput(filter, strings.TrimSuffix(output, "\n")+"\n")
				filter = nil
			}
		}
		response := reply{Output: output, Prompt: session.Prompt(), Completion: completion, Local: localCommand, Argv: argv, Filter: filter}
		if err == nil && (len(argv) > 0 || localCommand == "request platform debug service-container enter") {
			login.streamCommand = append([]string(nil), login.outputCommand...)
		}
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
