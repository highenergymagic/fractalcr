package appliance

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"fractalcr/internal/config"
	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

func sshHostKey() (ssh.Signer, error) {
	path := "/datastore/data0/config/ssh_host_ed25519_key"
	if err := checkDatastore(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		_, key, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return nil, e
		}
		der, e := x509.MarshalPKCS8PrivateKey(key)
		if e != nil {
			return nil, e
		}
		b = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		if e = config.AtomicWrite(path, b, 0600); e != nil {
			return nil, e
		}
	} else if err != nil {
		return nil, err
	}
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return nil, errors.New("SSH host key must be a private regular file")
	}
	return ssh.ParsePrivateKey(b)
}

// SSHService exposes only an appliance PTY. SSH exec, SFTP, environment,
// forwarding and arbitrary Unix shells are never dispatched.
func SSHService() error {
	signer, err := sshHostKey()
	if err != nil {
		return err
	}
	if fd, e := strconv.Atoi(os.Getenv("FCR_READY_FD")); e == nil {
		f := os.NewFile(uintptr(fd), "ready")
		f.Write([]byte{'R'})
		f.Close()
		os.Unsetenv("FCR_READY_FD")
	}
	var mu sync.Mutex
	failures := map[string]loginFailure{}
	active := map[net.Conn]map[string]string{}
	slots := make(chan struct{}, 16)
	settings := &ssh.ServerConfig{MaxAuthTries: 3, ServerVersion: "SSH-2.0-FractalCR"}
	settings.PasswordCallback = func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		address, _, _ := net.SplitHostPort(meta.RemoteAddr().String())
		now := time.Now()
		mu.Lock()
		f := failures[address]
		mu.Unlock()
		if now.Before(f.until) {
			return nil, errors.New("authentication failed")
		}
		c, e := loadRunning()
		u, exists := c.AAA.Users[meta.User()]
		if e != nil || !c.AAA.Enabled || !c.AAA.SSH || !exists || !config.CheckSecret(u.Secret, string(password)) {
			if c.AAA.Enabled {
				_ = audit(meta.User(), "ssh", "authentication-failed", "")
			}
			mu.Lock()
			f = failures[address]
			f.count++
			if f.count >= 3 {
				f.until = now.Add(30 * time.Second)
				f.count = 0
			}
			if len(failures) > 4096 {
				for k, v := range failures {
					if !now.Before(v.until) {
						delete(failures, k)
					}
				}
			}
			failures[address] = f
			mu.Unlock()
			return nil, errors.New("authentication failed")
		}
		mu.Lock()
		delete(failures, address)
		mu.Unlock()
		return &ssh.Permissions{Extensions: map[string]string{"user": meta.User(), "verifier": u.Secret, "privilege": strconv.Itoa(u.Privilege)}}, nil
	}
	settings.AddHostKey(signer)
	var listener net.Listener
	var address string
	defer func() {
		if listener != nil {
			listener.Close()
		}
	}()
	for {
		c, e := loadRunning()
		desired := ""
		if e == nil && c.AAA.Enabled && c.AAA.SSH {
			port := c.AAA.SSHPort
			if port == 0 {
				port = 22
			}
			desired = fmt.Sprintf(":%d", port)
		}
		// Revocation must also close sessions occupied by a streaming native
		// client, whose console cannot issue its ordinary authentication tick.
		mu.Lock()
		for conn, principal := range active {
			if principal != nil {
				u, exists := c.AAA.Users[principal["user"]]
				if e != nil || !c.AAA.Enabled || !c.AAA.SSH || !exists || u.Secret != principal["verifier"] || strconv.Itoa(u.Privilege) != principal["privilege"] {
					conn.Close()
				}
			}
		}
		mu.Unlock()
		if desired != address {
			if listener != nil {
				listener.Close()
				listener = nil
			}
			mu.Lock()
			for conn := range active {
				conn.Close()
			}
			mu.Unlock()
			address = desired
			if address != "" {
				listener, e = net.Listen("tcp", address)
				if e != nil {
					return e
				}
			}
		}
		if listener == nil {
			time.Sleep(time.Second)
			continue
		}
		listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
		conn, e := listener.Accept()
		if e != nil {
			if n, ok := e.(net.Error); ok && n.Timeout() {
				continue
			}
			return e
		}
		select {
		case slots <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		mu.Lock()
		active[conn] = nil
		mu.Unlock()
		go func() {
			defer func() { conn.Close(); mu.Lock(); delete(active, conn); mu.Unlock(); <-slots }()
			serveSSH(conn, settings, func(principal map[string]string) { mu.Lock(); active[conn] = principal; mu.Unlock() })
		}()
	}
}
func serveSSH(conn net.Conn, settings *ssh.ServerConfig, register func(map[string]string)) {
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	server, channels, requests, err := ssh.NewServerConn(conn, settings)
	if err != nil {
		return
	}
	defer server.Close()
	register(server.Permissions.Extensions)
	defer audit(server.User(), "ssh", "disconnect", "")
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	go ssh.DiscardRequests(requests)
	for next := range channels {
		if next.ChannelType() != "session" {
			next.Reject(ssh.Prohibited, "Only appliance sessions are available")
			continue
		}
		channel, requests, e := next.Accept()
		if e != nil {
			return
		}
		// One PTY per connection; closing the console also closes this connection.
		handleSSHSession(conn, channel, requests, server.Permissions.Extensions["user"], server.Permissions.Extensions["verifier"])
		return
	}
}
func handleSSHSession(conn net.Conn, channel ssh.Channel, requests <-chan *ssh.Request, user, verifier string) {
	defer channel.Close()
	size := pty.Winsize{Rows: 24, Cols: 80}
	gotPTY := false
	for request := range requests {
		switch request.Type {
		case "pty-req":
			var p struct {
				Term                      string
				Cols, Rows, Width, Height uint32
				Modes                     string
			}
			ok := !gotPTY && ssh.Unmarshal(request.Payload, &p) == nil && p.Cols >= 40 && p.Cols <= 512 && p.Rows >= 1 && p.Rows <= 512
			if ok {
				size.Rows = uint16(p.Rows)
				size.Cols = uint16(p.Cols)
				gotPTY = true
			}
			request.Reply(ok, nil)
		case "shell":
			if !gotPTY || len(request.Payload) != 0 {
				request.Reply(false, nil)
				continue
			}
			cmd := exec.Command("/usr/libexec/fractal-platform", "console")
			cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
			cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "TERM=vt100", "FCR_SSH_USER=" + user, "FCR_SSH_VERIFIER=" + verifier}
			terminal, err := pty.StartWithSize(cmd, &size)
			if err != nil {
				request.Reply(false, nil)
				return
			}
			request.Reply(true, nil)
			conn.SetDeadline(time.Time{})
			done := make(chan struct{})
			go func() { io.Copy(terminal, channel); cmd.Process.Kill() }()
			go func() { io.Copy(channel, terminal); close(done) }()
			go func() {
				for r := range requests {
					if r.Type == "window-change" {
						var w struct{ Cols, Rows, Width, Height uint32 }
						ok := ssh.Unmarshal(r.Payload, &w) == nil && w.Cols >= 40 && w.Cols <= 512 && w.Rows > 0 && w.Rows <= 512
						if ok {
							pty.Setsize(terminal, &pty.Winsize{Rows: uint16(w.Rows), Cols: uint16(w.Cols)})
						}
						r.Reply(ok, nil)
					} else {
						r.Reply(false, nil)
					}
				}
			}()
			cmd.Wait()
			terminal.Close()
			<-done
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			return
		default:
			request.Reply(false, nil)
		}
	}
}
