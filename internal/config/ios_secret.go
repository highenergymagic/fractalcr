package config

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const iosAlphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var iosBase64 = base64.NewEncoding(iosAlphabet).WithPadding(base64.NoPadding).Strict()

func iosSecretParts(encoded string) (kind, salt string, digest []byte, err error) {
	p := strings.Split(encoded, "$")
	if len(p) != 4 || p[0] != "" || (p[1] != "8" && p[1] != "9") || len(p[2]) != 14 || len(p[3]) != 43 {
		return "", "", nil, errors.New("invalid IOS secret encoding")
	}
	for _, c := range p[2] {
		if !strings.ContainsRune(iosAlphabet, c) {
			return "", "", nil, errors.New("invalid IOS salt")
		}
	}
	digest, err = iosBase64.DecodeString(p[3])
	if err != nil || len(digest) != 32 {
		return "", "", nil, errors.New("invalid IOS secret digest")
	}
	return p[1], p[2], digest, nil
}

func iosDerive(kind, password, salt string) ([]byte, error) {
	switch kind {
	case "8":
		return pbkdf2.Key(sha256.New, password, []byte(salt), 20000, 32)
	case "9":
		return scrypt.Key([]byte(password), []byte(salt), 16384, 1, 1, 32)
	}
	return nil, errors.New("unsupported IOS secret type")
}

func checkIOSSecret(encoded, password string) bool {
	kind, salt, want, err := iosSecretParts(encoded)
	if err != nil || len(password) > 256 {
		return false
	}
	got, err := iosDerive(kind, password, salt)
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func (a AAA) HashIOSSecret(kind, password string) (string, error) {
	if len(password) < 1 || len(password) > 256 || strings.ContainsAny(password, "\r\n\x00") {
		return "", errors.New("password must contain 1..256 characters without control line delimiters")
	}
	if len(password) < a.PasswordMinLength {
		return "", fmt.Errorf("password is shorter than the configured minimum of %d characters", a.PasswordMinLength)
	}
	random := make([]byte, 14)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i := range random {
		random[i] = iosAlphabet[random[i]&63]
	}
	salt := string(random)
	digest, err := iosDerive(kind, password, salt)
	if err != nil {
		return "", err
	}
	return "$" + kind + "$" + salt + "$" + iosBase64.EncodeToString(digest), nil
}

func SecretCLI(encoded string) string {
	if strings.HasPrefix(encoded, "$8$") {
		return "8 " + encoded
	}
	if strings.HasPrefix(encoded, "$9$") {
		return "9 " + encoded
	}
	return "pbkdf2 " + encoded
}

// Type 7 is accepted only as an import representation and immediately replaced
// with a salted verifier; it never needs to be retained or decrypted at login.
func DecodeIOSPassword7(encoded string) (string, error) {
	const key = "dsfd;kfoA,.iyewrkldJKDHSUBsgvca69834ncxv9873254k;fg87"
	if len(encoded) < 4 || len(encoded) > 514 || len(encoded)%2 != 0 {
		return "", errors.New("invalid type 7 password")
	}
	seed, err := strconv.Atoi(encoded[:2])
	if err != nil || seed < 0 || seed > 15 {
		return "", errors.New("invalid type 7 seed")
	}
	data, err := hex.DecodeString(encoded[2:])
	if err != nil {
		return "", errors.New("invalid type 7 password")
	}
	for i := range data {
		data[i] ^= key[(seed+i)%len(key)]
	}
	return string(data), nil
}
