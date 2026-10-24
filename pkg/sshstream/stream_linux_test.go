package sshstream

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fractalcr/internal/nativeguard"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func fixtureKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func TestSSHStreamWithoutExecutables(t *testing.T) {
	if os.Getenv("FCR_SSH_STREAM_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSSHStreamWithoutExecutables$")
		cmd.Env = append(os.Environ(), "FCR_SSH_STREAM_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("SSH API: %v: %s", err, output)
		}
		return
	}
	if err := nativeguard.DenyExec(); err != nil {
		t.Fatal(err)
	}
	hostKey := fixtureKey(t)
	_, identityKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ssh.NewSignerFromKey(identityKey)
	if err != nil {
		t.Fatal(err)
	}
	var wrongIdentities atomic.Int32
	server := &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if meta.User() != "fixture" || !bytes.Equal(key.Marshal(), identity.PublicKey().Marshal()) {
			wrongIdentities.Add(1)
			return nil, errors.New("wrong identity")
		}
		return nil, nil
	}}
	server.AddHostKey(hostKey)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var streams, other atomic.Int32
	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				connection, channels, requests, err := ssh.NewServerConn(raw, server)
				if err != nil {
					raw.Close()
					return
				}
				defer connection.Close()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					var request struct {
						Socket, Reserved string
						Port             uint32
					}
					if incoming.ChannelType() != "direct-streamlocal@openssh.com" || ssh.Unmarshal(incoming.ExtraData(), &request) != nil || request.Socket != "/run/service.sock" {
						other.Add(1)
						incoming.Reject(ssh.Prohibited, "service channels only")
						continue
					}
					streams.Add(1)
					channel, requests, err := incoming.Accept()
					if err != nil {
						continue
					}
					go ssh.DiscardRequests(requests)
					go func() { defer channel.Close(); io.Copy(channel, channel) }()
				}
			}()
		}
	}()
	cfg := &ssh.ClientConfig{User: "fixture", Auth: []ssh.AuthMethod{ssh.PublicKeys(identity)}, HostKeyCallback: ssh.FixedHostKey(hostKey.PublicKey())}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	releases := 0
	stream, err := dial(ctx, listener.Addr().String(), "/run/service.sock", cfg, func() { releases++ })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Write([]byte("native")); err != nil {
		t.Fatal(err)
	}
	value := make([]byte, 6)
	if _, err = io.ReadFull(stream, value); err != nil {
		t.Fatal(err)
	}
	if string(value) != "native" {
		t.Fatalf("unexpected data: %q", value)
	}
	if err = stream.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadAll(stream); err != nil {
		t.Fatal(err)
	}
	stream.Close()
	stream.Close()
	if releases != 1 || streams.Load() != 1 || other.Load() != 0 {
		t.Fatalf("releases=%d stream=%d other=%d", releases, streams.Load(), other.Load())
	}
	cfg.HostKeyCallback = ssh.FixedHostKey(fixtureKey(t).PublicKey())
	if _, err = dial(ctx, listener.Addr().String(), "/run/service.sock", cfg, nil); err == nil {
		t.Fatal("untrusted host accepted")
	}
	if streams.Load() != 1 {
		t.Fatal("untrusted host received service request")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	if err = os.Mkdir(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	encoded, err := ssh.MarshalPrivateKey(identityKey, "")
	if err != nil {
		t.Fatal(err)
	}
	identityFile := filepath.Join(home, ".ssh/id_ed25519")
	if err = os.WriteFile(identityFile, pem.EncodeToMemory(encoded), 0600); err != nil {
		t.Fatal(err)
	}
	trustFile := filepath.Join(home, ".ssh/known_hosts")
	trust := knownhosts.Line([]string{knownhosts.Normalize(listener.Addr().String())}, hostKey.PublicKey()) + "\n"
	if err = os.WriteFile(trustFile, []byte(trust), 0600); err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	endpoint := Endpoint{Host: host, Port: port, User: "fixture", Socket: "/run/service.sock"}
	settings := fixtureSettings{"IdentitiesOnly": "yes", "IdentityFile": "~/.ssh/id_ed25519"}
	stream, err = dialConfigured(ctx, endpoint, settings)
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	if streams.Load() != 2 {
		t.Fatal("configured native SSH did not open the service")
	}
	if err = os.Chmod(identityFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = dialConfigured(ctx, endpoint, settings); err == nil {
		t.Fatal("publicly readable identity accepted")
	}
	if err = os.Chmod(identityFile, 0600); err != nil {
		t.Fatal(err)
	}
	ring := agent.NewKeyring()
	_, otherIdentity, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = ring.Add(agent.AddedKey{PrivateKey: otherIdentity}); err != nil {
		t.Fatal(err)
	}
	if err = ring.Add(agent.AddedKey{PrivateKey: identityKey}); err != nil {
		t.Fatal(err)
	}
	agentSocket := filepath.Join(home, "agent")
	agentListener, err := net.Listen("unix", agentSocket)
	if err != nil {
		t.Fatal(err)
	}
	defer agentListener.Close()
	go func() {
		for {
			conn, err := agentListener.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); agent.ServeAgent(ring, conn) }()
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", agentSocket)
	if err = os.WriteFile(identityFile, ssh.MarshalAuthorizedKey(identity.PublicKey()), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(identityFile, 0644); err != nil {
		t.Fatal(err)
	}
	stream, err = dialConfigured(ctx, endpoint, settings)
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	if streams.Load() != 3 || wrongIdentities.Load() != 0 {
		t.Fatal("IdentitiesOnly did not restrict agent keys")
	}
	trust = knownhosts.Line([]string{knownhosts.Normalize(listener.Addr().String())}, fixtureKey(t).PublicKey()) + "\n"
	if err = os.WriteFile(trustFile, []byte(trust), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = dialConfigured(ctx, endpoint, settings); err == nil {
		t.Fatal("known-host mismatch accepted")
	}
	settings["ProxyCommand"] = "forbidden-helper"
	if _, err = dialConfigured(ctx, endpoint, settings); err == nil {
		t.Fatal("executable proxy accepted")
	}
}

type fixtureSettings map[string]string

func (f fixtureSettings) GetStrict(_ string, key string) (string, error) { return f[key], nil }
func (f fixtureSettings) GetAllStrict(_ string, key string) ([]string, error) {
	if value := f[key]; value != "" {
		return []string{value}, nil
	}
	return nil, nil
}

func TestCancelledHandshakeReleasesResources(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			io.Copy(io.Discard, conn)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	released := 0
	_, err = dial(ctx, listener.Addr().String(), "/run/service.sock", &ssh.ClientConfig{User: "fixture", HostKeyCallback: ssh.FixedHostKey(fixtureKey(t).PublicKey())}, func() { released++ })
	if err == nil || released != 1 {
		t.Fatal("handshake did not release resources", err, released)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handshake connection leaked")
	}
}
