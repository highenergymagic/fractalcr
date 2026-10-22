package config

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Secrets are salted PBKDF2-SHA256 verifiers, never reversible IOS type 7.
const secretRounds = 600000

type User struct {
	Secret    string `json:"secret"`
	Privilege int    `json:"privilege"`
}
type AAA struct {
	Enabled        bool            `json:"enabled"`
	Users          map[string]User `json:"users,omitempty"`
	EnableSecret   string          `json:"enable_secret,omitempty"`
	SSH            bool            `json:"ssh"`
	SSHPort        int             `json:"ssh_port,omitempty"`
	ConsoleTimeout int             `json:"console_timeout_seconds"`
	VTYTimeout     int             `json:"vty_timeout_seconds"`
}

func HashSecret(password string) (string, error) {
	if len(password) < 12 || len(password) > 256 || strings.ContainsAny(password, "\x00\r\n") {
		return "", errors.New("secret must contain 12..256 characters without control line delimiters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, secretRounds, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$fcr-pbkdf2-sha256$600000$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}
func secretParts(encoded string) ([]byte, []byte, error) {
	p := strings.Split(encoded, "$")
	if len(p) != 5 || p[1] != "fcr-pbkdf2-sha256" || p[2] != "600000" {
		return nil, nil, errors.New("unsupported secret encoding")
	}
	salt, e := base64.RawStdEncoding.DecodeString(p[3])
	key, f := base64.RawStdEncoding.DecodeString(p[4])
	if e != nil || f != nil || len(salt) != 16 || len(key) != 32 {
		return nil, nil, errors.New("invalid secret encoding")
	}
	return salt, key, nil
}
func CheckSecret(encoded, password string) bool {
	salt, want, err := secretParts(encoded)
	if err != nil || len(password) > 256 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, secretRounds, 32)
	return err == nil && subtle.ConstantTimeCompare(want, got) == 1
}
func (a AAA) Validate() error {
	if len(a.Users) > 64 {
		return errors.New("at most 64 local users")
	}
	admin := false
	for name, u := range a.Users {
		if !Name.MatchString(name) || u.Privilege < 0 || u.Privilege > 15 {
			return errors.New("invalid local username or privilege (0..15)")
		}
		if _, _, err := secretParts(u.Secret); err != nil {
			return fmt.Errorf("user %s: %w", name, err)
		}
		admin = admin || u.Privilege == 15
	}
	if a.Enabled && !admin {
		return errors.New("AAA requires at least one privilege 15 local user")
	}
	if a.SSH && !a.Enabled {
		return errors.New("SSH requires local AAA")
	}
	if a.SSHPort < 0 || a.SSHPort > 65535 {
		return errors.New("invalid SSH port")
	}
	if a.ConsoleTimeout < 0 || a.ConsoleTimeout > 86400 || a.VTYTimeout < 0 || a.VTYTimeout > 86400 {
		return errors.New("session timeout must be 0..86400 seconds")
	}
	if a.EnableSecret != "" {
		_, _, err := secretParts(a.EnableSecret)
		return err
	}
	return nil
}
