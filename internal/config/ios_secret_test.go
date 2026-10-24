package config

import "testing"

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
