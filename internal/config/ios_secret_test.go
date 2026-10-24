package config

import (
	"strings"
	"testing"
)

func TestType5IndependentVectors(t *testing.T) {
	// Generated independently with OpenSSL passwd -1 on the Linux test host.
	for _, test := range []struct{ password, encoded string }{
		{"password", "$1$salt$qJH7.N4xYta3aEG/dfqo/0"},
		{"cisco", "$1$abcd$2hDGYdE0UrwK2pqoiF6BB1"},
		{"synthetic-password", "$1$12345678$ucEINfSoBuudVOjQhEiYE/"},
	} {
		if err := ValidateSecret(test.encoded); err != nil {
			t.Fatal(err)
		}
		if !CheckSecret(test.encoded, test.password) || CheckSecret(test.encoded, "wrong") || CheckSecret(test.encoded, strings.Repeat("x", 257)) {
			t.Fatal(test.encoded)
		}
		if got := SecretCLI(test.encoded); got != "5 "+test.encoded {
			t.Fatal(got)
		}
	}
	for _, bad := range []string{"$1$salt$short", "$1$123456789$qJH7.N4xYta3aEG/dfqo/0", "$1$salt$qJH7.N4xYta3aEG/dfqo/z", "$1$salt$qJH7.N4xYta3aEG/dfqo/0$", "$1$salt\n$qJH7.N4xYta3aEG/dfqo/0"} {
		if ValidateSecret(bad) == nil || CheckSecret(bad, "password") {
			t.Fatal("malformed type 5 accepted", bad)
		}
	}
}

// Independent published vectors: hashcat v6.2.6 module_09200.c/module_09300.c.
func TestIOSSecretPublishedVectors(t *testing.T) {
	for _, encoded := range []string{"$8$84486783037343$pYNyVrtyMalQrZLxRi7ZLQS1Fl.jkYCgASUi5P8JNb2", "$9$87023684531115$phio0TBQwaO7KZ8toQFyGFyDvyOzidaypRWN0uKX0hU"} {
		if err := ValidateSecret(encoded); err != nil {
			t.Fatal(err)
		}
		if !CheckSecret(encoded, "hashcat") || CheckSecret(encoded, "wrong") {
			t.Fatal("independent verifier failed", encoded)
		}
		if ValidateSecret(encoded+"!") == nil {
			t.Fatal("accepted malformed digest")
		}
	}
	for _, kind := range []string{"8", "9"} {
		a, err := (AAA{}).HashIOSSecret(kind, "synthetic-password")
		if err != nil {
			t.Fatal(err)
		}
		b, err := (AAA{}).HashIOSSecret(kind, "synthetic-password")
		if err != nil {
			t.Fatal(err)
		}
		if a == b || !CheckSecret(a, "synthetic-password") {
			t.Fatal("salt generation")
		}
	}
	p, err := DecodeIOSPassword7("070C285F4D06")
	if err != nil || p != "cisco" {
		t.Fatalf("type 7: %q %v", p, err)
	}
}
