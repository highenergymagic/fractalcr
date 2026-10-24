package config

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"strings"
)

// Retain the original PBKDF2 format for existing configurations. New passwords
// use IOS type 9; imported type 7 passwords are rehashed before storage.
const secretRounds = 600000

type User struct {
	Secret    string   `json:"secret"`
	Privilege int      `json:"privilege"`
	SSHKeys   []string `json:"ssh_keys,omitempty"`
}

// AuthVerifier revokes existing sessions when either credential set changes.
func (u User) AuthVerifier() string {
	if len(u.SSHKeys) == 0 {
		return u.Secret
	}
	h := sha256.Sum256([]byte(u.Secret + "\x00" + strings.Join(u.SSHKeys, "\x00")))
	return fmt.Sprintf("keys:%x", h)
}

func NormalizeSSHKey(value string) (string, error) {
	if len(value) > 16384 || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("invalid SSH public key")
	}
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(value))
	if err != nil || len(options) != 0 || len(rest) != 0 {
		return "", errors.New("expected one SSH public key without authorized_keys options")
	}
	switch key.Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoRSA, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
	default:
		return "", errors.New("unsupported SSH public key type")
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), nil
}

type AAA struct {
	Enabled           bool               `json:"enabled"`
	Users             map[string]User    `json:"users,omitempty"`
	EnableSecret      string             `json:"enable_secret,omitempty"`
	EnableSecrets     map[int]string     `json:"enable_secrets,omitempty"`
	CommandPrivileges []CommandPrivilege `json:"command_privileges,omitempty"`
	FilePrivilege     *int               `json:"file_privilege,omitempty"`
	SSH               bool               `json:"ssh"`
	SSHPort           int                `json:"ssh_port,omitempty"`
	ConsoleTimeout    int                `json:"console_timeout_seconds"`
	VTYTimeout        int                `json:"vty_timeout_seconds"`
	VTY               map[int]VTYLine    `json:"vty,omitempty"`
	PasswordMinLength int                `json:"password_min_length,omitempty"`
}

const VTYCount = 16

func (a AAA) EnableVerifier(level int) string {
	if level == 15 {
		return a.EnableSecret
	}
	return a.EnableSecrets[level]
}

type VTYLine struct {
	Timeout int  `json:"timeout_seconds"`
	SSH     bool `json:"ssh"`
}

// Older configurations supplied one policy for all VTYs. Unmodified lines
// inherit it, while a selected line/range can now be configured independently.
func (a AAA) Line(number int) VTYLine {
	if line, ok := a.VTY[number]; ok {
		return line
	}
	return VTYLine{Timeout: a.VTYTimeout, SSH: a.SSH}
}

func (a AAA) SSHAvailable() bool {
	for i := 0; i < VTYCount; i++ {
		if a.Line(i).SSH {
			return true
		}
	}
	return false
}

func (a AAA) HashPassword(password string) (string, error) {
	if len(password) < a.PasswordMinLength {
		return "", fmt.Errorf("password is shorter than the configured minimum of %d characters", a.PasswordMinLength)
	}
	return a.HashIOSSecret("9", password)
}

func HashSecret(password string) (string, error) {
	if len(password) < 1 || len(password) > 256 || strings.ContainsAny(password, "\x00\r\n") {
		return "", errors.New("password must contain 1..256 characters without control line delimiters")
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
	if strings.HasPrefix(encoded, "$1$") {
		return checkMD5Secret(encoded, password)
	}
	if strings.HasPrefix(encoded, "$8$") || strings.HasPrefix(encoded, "$9$") {
		return checkIOSSecret(encoded, password)
	}
	salt, want, err := secretParts(encoded)
	if err != nil || len(password) > 256 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, secretRounds, 32)
	return err == nil && subtle.ConstantTimeCompare(want, got) == 1
}
func (a AAA) Validate() error {
	if a.FilePrivilege != nil && (*a.FilePrivilege < 0 || *a.FilePrivilege > 15) {
		return errors.New("file privilege must be 0..15")
	}
	if err := validateCommandPrivileges(a.CommandPrivileges); err != nil {
		return err
	}
	if len(a.Users) > 64 {
		return errors.New("at most 64 local users")
	}
	admin := false
	for name, u := range a.Users {
		if !Name.MatchString(name) || u.Privilege < 0 || u.Privilege > 15 {
			return errors.New("invalid local username or privilege (0..15)")
		}
		if u.Secret != "" || len(u.SSHKeys) == 0 {
			if err := ValidateSecret(u.Secret); err != nil {
				return fmt.Errorf("user %s: %w", name, err)
			}
		}
		if len(u.SSHKeys) > 16 {
			return errors.New("at most 16 SSH keys per user")
		}
		for _, key := range u.SSHKeys {
			if _, err := NormalizeSSHKey(key); err != nil {
				return fmt.Errorf("user %s: %w", name, err)
			}
		}
		admin = admin || u.Privilege == 15
	}
	if a.Enabled && !admin {
		return errors.New("AAA requires at least one privilege 15 local user")
	}
	if a.SSHAvailable() && !a.Enabled {
		return errors.New("SSH requires local AAA")
	}
	if a.SSHPort < 0 || a.SSHPort > 65535 {
		return errors.New("invalid SSH port")
	}
	if a.ConsoleTimeout < 0 || a.ConsoleTimeout > 86400 || a.VTYTimeout < 0 || a.VTYTimeout > 86400 {
		return errors.New("session timeout must be 0..86400 seconds")
	}
	if a.PasswordMinLength < 0 || a.PasswordMinLength > 256 {
		return errors.New("invalid password minimum length (0..256)")
	}
	for number, line := range a.VTY {
		if number < 0 || number >= VTYCount || line.Timeout < 0 || line.Timeout > 86400 {
			return errors.New("invalid virtual terminal line or timeout")
		}
	}
	if a.EnableSecret != "" {
		err := ValidateSecret(a.EnableSecret)
		if err != nil {
			return err
		}
	}
	for level, secret := range a.EnableSecrets {
		if level < 1 || level >= 15 {
			return errors.New("enable secret level must be 1..14; level 15 uses the default secret")
		}
		if err := ValidateSecret(secret); err != nil {
			return err
		}
	}
	return nil
}
