package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"fractalcr/internal/config"
	"golang.org/x/crypto/ssh"
	"strings"
	"testing"
)

func TestSSHKeyConfigurationRoundTrip(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	parsed, _ := ssh.NewPublicKey(pub)
	key := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(parsed)))
	c := config.Default()
	s := Session{Running: c, Candidate: c.Clone()}
	for _, command := range []string{"conf t", "username admin privilege 15 ssh-key " + key, "aaa new-model", "ip ssh version 2", "end"} {
		if _, err := s.Execute(command); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	if err := s.Running.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Running.CLI(), "username admin privilege 15 ssh-key "+key) {
		t.Fatal(s.Running.CLI())
	}
	if strings.Contains(s.Running.CLI(), "secret pbkdf2") {
		t.Fatal("invented a password for a key-only account")
	}
	c = config.Default()
	copy := Session{Running: c, Candidate: c.Clone()}
	_, _ = copy.Execute("conf t")
	for _, line := range strings.Split(s.Running.CLI(), "\n") {
		if line == "" || line == "!" {
			continue
		}
		if _, err := copy.Execute(line); err != nil {
			t.Fatalf("replay %q: %v", line, err)
		}
	}
	if copy.Running.AAA.Users["admin"].AuthVerifier() != s.Running.AAA.Users["admin"].AuthVerifier() {
		t.Fatal("key lost on replay")
	}
}
