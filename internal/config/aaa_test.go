package config

import "testing"

func TestSecretsAndAAALockoutPrevention(t *testing.T) {
	a, e := HashSecret("a long test password")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := HashSecret("a long test password")
	if a == b || !CheckSecret(a, "a long test password") || CheckSecret(a, "wrong") {
		t.Fatal("secret derivation")
	}
	if CheckSecret("$fcr-pbkdf2-sha256$999999999999$bad$bad", "x") {
		t.Fatal("unbounded verifier accepted")
	}
	c := Default()
	c.AAA.Enabled = true
	if c.Validate() == nil {
		t.Fatal("AAA without administrator")
	}
	c.AAA.Users = map[string]User{"admin": {Secret: a, Privilege: 15}}
	if e = c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.AAA.Users["admin"] = User{Secret: a, Privilege: 1}
	if c.Validate() == nil {
		t.Fatal("removing last administrator accepted")
	}
}
