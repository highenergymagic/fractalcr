package config

import (
	"crypto/md5"
	"crypto/subtle"
	"errors"
	"strings"
)

// IOS type 5 imports use the standard $1$ MD5-crypt representation. This
// verifier is compatibility-only: new plaintext secrets still use type 9.
func md5SecretSalt(encoded string) (string, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "" || parts[1] != "1" || len(parts[2]) < 1 || len(parts[2]) > 8 || len(parts[3]) != 22 {
		return "", errors.New("invalid type 5 secret encoding")
	}
	for _, c := range parts[2] + parts[3] {
		if !strings.ContainsRune(iosAlphabet, c) {
			return "", errors.New("invalid type 5 secret encoding")
		}
	}
	if !strings.ContainsRune(iosAlphabet[:4], rune(parts[3][21])) {
		return "", errors.New("invalid type 5 secret digest")
	}
	return parts[2], nil
}

func checkMD5Secret(encoded, password string) bool {
	salt, err := md5SecretSalt(encoded)
	if err != nil || len(password) > 256 || strings.ContainsAny(password, "\x00\r\n") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(encoded), []byte(md5Secret(password, salt))) == 1
}

func md5Secret(password, salt string) string {
	p := []byte(password)
	alternate := md5.Sum([]byte(password + salt + password))
	h := md5.New()
	h.Write([]byte(password + "$1$" + salt))
	for remaining := len(p); remaining > 0; remaining -= 16 {
		h.Write(alternate[:min(remaining, 16)])
	}
	for bits := len(p); bits > 0; bits >>= 1 {
		if bits&1 != 0 {
			h.Write([]byte{0})
		} else {
			h.Write(p[:1])
		}
	}
	digest := h.Sum(nil)
	for round := range 1000 {
		h.Reset()
		first, last := digest, p
		if round&1 != 0 {
			first, last = p, digest
		}
		h.Write(first)
		if round%3 != 0 {
			h.Write([]byte(salt))
		}
		if round%7 != 0 {
			h.Write(p)
		}
		h.Write(last)
		digest = h.Sum(digest[:0])
	}
	var output strings.Builder
	output.WriteString("$1$" + salt + "$")
	for _, group := range [][4]int{{0, 6, 12, 4}, {1, 7, 13, 4}, {2, 8, 14, 4}, {3, 9, 15, 4}, {4, 10, 5, 4}, {11, 11, 11, 2}} {
		value := uint32(digest[group[0]])<<16 | uint32(digest[group[1]])<<8 | uint32(digest[group[2]])
		if group[3] == 2 {
			value = uint32(digest[11])
		}
		for range group[3] {
			output.WriteByte(iosAlphabet[value&63])
			value >>= 6
		}
	}
	return output.String()
}
