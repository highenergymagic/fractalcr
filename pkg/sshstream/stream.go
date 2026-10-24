// Package sshstream connects directly to service sockets using SSH channels.
// It never requests a remote shell, executes a remote command or starts ssh.
package sshstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sshconfig "github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Endpoint struct{ Host, Port, User, Socket string }

// Dial uses native key/agent authentication and mandatory known-host checking.
// A remote sshd must permit direct-streamlocal forwarding to the service socket.
func Dial(ctx context.Context, endpoint Endpoint) (net.Conn, error) {
	return dialConfigured(ctx, endpoint, sshconfig.DefaultUserSettings)
}

type settingsReader interface {
	GetStrict(string, string) (string, error)
	GetAllStrict(string, string) ([]string, error)
}

func dialConfigured(ctx context.Context, endpoint Endpoint, configuration settingsReader) (net.Conn, error) {
	if endpoint.Host == "" || !filepath.IsAbs(endpoint.Socket) || strings.ContainsRune(endpoint.Socket, 0) {
		return nil, fmt.Errorf("invalid SSH service endpoint")
	}
	settings := map[string]string{}
	for _, key := range []string{"HostName", "Port", "User", "ProxyCommand", "ProxyJump", "LocalCommand", "IdentityAgent", "IdentitiesOnly", "UserKnownHostsFile", "GlobalKnownHostsFile", "HostKeyAlias"} {
		value, err := configuration.GetStrict(endpoint.Host, key)
		if err != nil {
			return nil, err
		}
		settings[key] = value
	}
	for _, key := range []string{"ProxyCommand", "ProxyJump", "LocalCommand"} {
		if value := settings[key]; value != "" && value != "none" {
			return nil, fmt.Errorf("SSH %s requires a native transport implementation", key)
		}
	}
	alias := endpoint.Host
	if value := settings["HostName"]; value != "" {
		endpoint.Host = strings.ReplaceAll(value, "%h", alias)
	}
	if endpoint.Port == "" {
		endpoint.Port = settings["Port"]
	}
	if endpoint.Port == "" {
		endpoint.Port = "22"
	}
	port, err := strconv.Atoi(endpoint.Port)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SSH port")
	}
	if endpoint.User == "" {
		endpoint.User = settings["User"]
	}
	if endpoint.User == "" {
		current, err := user.Current()
		if err != nil {
			return nil, err
		}
		endpoint.User = current.Username
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	expand := func(value string) (string, error) {
		if strings.HasPrefix(value, "~/") {
			value = filepath.Join(home, value[2:])
		}
		value = strings.NewReplacer("%%", "\x00", "%d", home, "%h", endpoint.Host, "%n", alias, "%p", endpoint.Port, "%r", endpoint.User).Replace(value)
		if strings.Contains(value, "%") || strings.HasPrefix(value, "~") {
			return "", fmt.Errorf("unsupported SSH path expansion")
		}
		return strings.ReplaceAll(value, "\x00", "%"), nil
	}
	var hosts []string
	for _, key := range []string{"UserKnownHostsFile", "GlobalKnownHostsFile"} {
		value := settings[key]
		if value == "" && key == "UserKnownHostsFile" {
			value = "~/.ssh/known_hosts"
		}
		if value == "none" {
			continue
		}
		for _, path := range strings.Fields(value) {
			path, err = expand(path)
			if err != nil {
				return nil, err
			}
			if _, err = os.Stat(path); err == nil {
				hosts = append(hosts, path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("SSH known-host trust is not configured")
	}
	callback, err := knownhosts.New(hosts...)
	if err != nil {
		return nil, err
	}
	hostKeyName := endpoint.Host
	if value := settings["HostKeyAlias"]; value != "" {
		hostKeyName = value
	}
	checkHost := func(_ string, remote net.Addr, key ssh.PublicKey) error {
		return callback(net.JoinHostPort(hostKeyName, endpoint.Port), remote, key)
	}
	identities, err := configuration.GetAllStrict(alias, "IdentityFile")
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 || len(identities) == 1 && identities[0] == "~/.ssh/identity" {
		identities = append(identities, "~/.ssh/id_ed25519", "~/.ssh/id_ecdsa", "~/.ssh/id_rsa")
	}
	var signers []ssh.Signer
	allowedKeys := map[string]bool{}
	var encrypted bool
	for _, path := range identities {
		if path == "none" {
			continue
		}
		path, err = expand(path)
		if err != nil {
			return nil, err
		}
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			file, err = os.Open(path + ".pub")
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return nil, err
		}
		if !info.Mode().IsRegular() {
			file.Close()
			return nil, fmt.Errorf("SSH identity must be a regular file")
		}
		data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
		file.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > 1024*1024 {
			return nil, fmt.Errorf("SSH identity file exceeds size limit")
		}
		if key, _, _, _, err := ssh.ParseAuthorizedKey(data); err == nil {
			allowedKeys[string(key.Marshal())] = true
			continue
		}
		if info.Mode().Perm()&0077 != 0 {
			return nil, fmt.Errorf("SSH identity file must be private to its owner")
		}
		signer, err := ssh.ParsePrivateKey(data)
		if missing, ok := err.(*ssh.PassphraseMissingError); ok {
			encrypted = true
			if missing.PublicKey != nil {
				allowedKeys[string(missing.PublicKey.Marshal())] = true
			} else {
				public, err := os.ReadFile(path + ".pub")
				if err == nil {
					if key, _, _, _, err := ssh.ParseAuthorizedKey(public); err == nil {
						allowedKeys[string(key.Marshal())] = true
					}
				}
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("invalid SSH identity file")
		}
		signers = append(signers, signer)
		allowedKeys[string(signer.PublicKey().Marshal())] = true
	}
	var agentConn net.Conn
	release := func() {
		if agentConn != nil {
			agentConn.Close()
		}
	}
	auth := []ssh.AuthMethod{}
	if len(signers) > 0 {
		auth = append(auth, ssh.PublicKeys(signers...))
	}
	{
		path := settings["IdentityAgent"]
		if path == "" || path == "SSH_AUTH_SOCK" {
			path = os.Getenv("SSH_AUTH_SOCK")
		}
		if path != "" && path != "none" {
			path, err = expand(path)
			if err != nil {
				return nil, err
			}
			agentConn, err = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", path)
			if err != nil {
				if len(auth) == 0 {
					return nil, fmt.Errorf("SSH authentication agent is unavailable")
				}
			} else {
				agentConn.SetDeadline(time.Now().Add(30 * time.Second))
				agentClient := agent.NewClient(agentConn)
				auth = append(auth, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
					candidates, err := agentClient.Signers()
					if err != nil {
						return nil, err
					}
					if settings["IdentitiesOnly"] != "yes" {
						return candidates, nil
					}
					selected := []ssh.Signer{}
					for _, signer := range candidates {
						if allowedKeys[string(signer.PublicKey().Marshal())] {
							selected = append(selected, signer)
						}
					}
					return selected, nil
				}))
			}
		}
	}
	if len(auth) == 0 {
		release()
		if encrypted {
			return nil, fmt.Errorf("encrypted SSH identity requires an authentication agent")
		}
		return nil, fmt.Errorf("SSH identity is not configured")
	}
	cfg := &ssh.ClientConfig{User: endpoint.User, Auth: auth, HostKeyCallback: checkHost, Timeout: 30 * time.Second}
	stopAgent := context.AfterFunc(ctx, release)
	defer stopAgent()
	return dial(ctx, net.JoinHostPort(endpoint.Host, endpoint.Port), endpoint.Socket, cfg, release)
}

func dial(ctx context.Context, address, socket string, config *ssh.ClientConfig, release func()) (net.Conn, error) {
	if release == nil {
		release = func() {}
	}
	pending := true
	defer func() {
		if pending {
			release()
		}
	}()
	dialctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(dialctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(dialctx, func() { raw.Close() })
	defer stop()
	transport, channels, requests, err := ssh.NewClientConn(raw, address, config)
	if err != nil {
		raw.Close()
		return nil, err
	}
	client := ssh.NewClient(transport, channels, requests)
	stream, err := client.DialContext(dialctx, "unix", socket)
	if err != nil {
		client.Close()
		return nil, err
	}
	if !stop() {
		stream.Close()
		client.Close()
		return nil, dialctx.Err()
	}
	pending = false
	return &connection{Conn: stream, client: client, release: release}, nil
}

type connection struct {
	net.Conn
	client  *ssh.Client
	release func()
	once    sync.Once
	err     error
}

func (c *connection) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close(); c.client.Close(); c.release() })
	return c.err
}
func (c *connection) CloseWrite() error {
	if half, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return half.CloseWrite()
	}
	return fmt.Errorf("SSH stream has no write shutdown")
}
