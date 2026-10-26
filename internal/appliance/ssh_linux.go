package appliance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"fractalcr/internal/config"
	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

var sshHostKeyPath = "/datastore/data0/config/ssh_host_ed25519_key"

func readHostKey(path string) ([]byte, error) {
	// Open first, then validate that same descriptor. A pre-read Lstat can race
	// replacement; ReadFile can block forever on an attacker-supplied FIFO.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !ok || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || owner.Uid != uint32(os.Geteuid()) || owner.Nlink != 1 || st.Size() > 16384 {
		return nil, errors.New("SSH host key must be an owned, private regular file below 16 KiB")
	}
	b, err := io.ReadAll(io.LimitReader(f, 16385))
	if len(b) > 16384 {
		return nil, errors.New("SSH host key exceeds size limit")
	}
	return b, err
}

func sshHostKey() (ssh.Signer, error) {
	path := sshHostKeyPath
	if err := checkDatastore(); err != nil {
		return nil, err
	}
	b, err := readHostKey(path)
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
	slots := [config.VTYCount]bool{}
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
		if e != nil || !c.AAA.Enabled || !c.AAA.SSHAvailable() || !exists || !config.CheckSecret(u.Secret, string(password)) {
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
		return &ssh.Permissions{Extensions: map[string]string{"user": meta.User(), "verifier": u.AuthVerifier(), "privilege": strconv.Itoa(u.Privilege)}}, nil
	}
	settings.PublicKeyCallback = func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		c, err := loadRunning()
		u, exists := c.AAA.Users[meta.User()]
		if err == nil && c.AAA.Enabled && c.AAA.SSHAvailable() && exists {
			for _, value := range u.SSHKeys {
				normalized, e := config.NormalizeSSHKey(value)
				if e == nil && normalized == strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) {
					return &ssh.Permissions{Extensions: map[string]string{"user": meta.User(), "verifier": u.AuthVerifier(), "privilege": strconv.Itoa(u.Privilege)}}, nil
				}
			}
		}
		return nil, errors.New("authentication failed")
	}
	settings.AddHostKey(signer)
	var listener net.Listener
	var address string
	var boundInterface string
	defer func() {
		if listener != nil {
			listener.Close()
		}
	}()
	for {
		c, e := loadRunning()
		desired := ""
		desiredInterface := ""
		if e == nil && c.AAA.Enabled && c.AAA.SSHAvailable() {
			port := c.AAA.SSHPort
			if port == 0 {
				port = 22
			}
			for name, n := range c.AllInterfaces() {
				if !n.IsManagement() || n.Shutdown {
					continue
				}
				iface, err := interfaceByName(name)
				if err != nil {
					continue
				}
				addresses, err := iface.Addrs()
				if err != nil {
					continue
				}
				for _, a := range addresses {
					ip, _, err := net.ParseCIDR(a.String())
					if err == nil && ip.To4() != nil && ip.IsGlobalUnicast() {
						desired = net.JoinHostPort(ip.String(), strconv.Itoa(port))
						desiredInterface = iface.Name
						break
					}
				}
			}
		}
		// Revocation must also close sessions occupied by a streaming native
		// client, whose console cannot issue its ordinary authentication tick.
		mu.Lock()
		for conn, principal := range active {
			if principal != nil {
				u, exists := c.AAA.Users[principal["user"]]
				line, lineErr := strconv.Atoi(principal["vty"])
				if e != nil || lineErr != nil || line < 0 || line >= config.VTYCount || !c.AAA.Line(line).SSH || !c.AAA.Enabled || !c.AAA.SSHAvailable() || !exists || u.AuthVerifier() != principal["verifier"] || strconv.Itoa(u.Privilege) != principal["privilege"] {
					conn.Close()
				}
			}
		}
		mu.Unlock()
		if desired != address || desiredInterface != boundInterface {
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
			boundInterface = desiredInterface
			if address != "" {
				lc := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
					var bindErr error
					err := raw.Control(func(fd uintptr) {
						bindErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, boundInterface)
					})
					if err != nil {
						return err
					}
					return bindErr
				}}
				listener, e = lc.Listen(context.Background(), "tcp4", address)
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
		// Accept may have blocked while the administrator changed line policy.
		// Allocate against the configuration at connection time, not the snapshot
		// used to maintain the listener before Accept.
		policy, loadErr := loadRunning()
		if loadErr != nil || !policy.AAA.Enabled {
			conn.Close()
			continue
		}
		mu.Lock()
		vty := -1
		for i := range slots {
			if !slots[i] && policy.AAA.Line(i).SSH {
				vty = i
				slots[i] = true
				break
			}
		}
		if vty < 0 {
			mu.Unlock()
			conn.Close()
			continue
		}
		active[conn] = nil
		mu.Unlock()
		go func(conn net.Conn, vty int) {
			defer func() { conn.Close(); mu.Lock(); delete(active, conn); slots[vty] = false; mu.Unlock() }()
			serveSSH(conn, settings, vty, func(principal map[string]string) { mu.Lock(); active[conn] = principal; mu.Unlock() })
		}(conn, vty)
	}
}
func serveSSH(conn net.Conn, settings *ssh.ServerConfig, vty int, register func(map[string]string)) {
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	server, channels, requests, err := ssh.NewServerConn(conn, settings)
	if err != nil {
		return
	}
	defer server.Close()
	server.Permissions.Extensions["vty"] = strconv.Itoa(vty)
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
		handleSSHSession(conn, channel, requests, server.Permissions.Extensions["user"], server.Permissions.Extensions["verifier"], vty)
		return
	}
}
func handleSSHSession(conn net.Conn, channel ssh.Channel, requests <-chan *ssh.Request, user, verifier string, vty int) {
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
			cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "TERM=vt100", "FCR_SSH_USER=" + user, "FCR_SSH_VERIFIER=" + verifier, "FCR_SSH_VTY=" + strconv.Itoa(vty), "FCR_SSH_PEER=" + conn.RemoteAddr().String()}
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
			drainSSHOutput(conn, done, 5*time.Second)
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			return
		default:
			request.Reply(false, nil)
		}
	}
}

// A client can stop updating its SSH channel window. Once the console exits,
// give pending output a bounded drain period, then close the transport so its
// blocked channel writer cannot retain the VTY indefinitely.
func drainSSHOutput(conn net.Conn, done <-chan struct{}, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		conn.Close()
	}
}
