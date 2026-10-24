package cli

import (
	"fractalcr/internal/config"
	"strings"
	"testing"
)

func TestIOSSecretImportAndAlgorithms(t *testing.T) {
	s := session(t)
	execute(t, s, "conf t")
	for _, test := range []struct{ cmd, password, prefix string }{
		{"username five secret 5 $1$abcd$2hDGYdE0UrwK2pqoiF6BB1", "cisco", "$1$"},
		{"username eight secret 8 $8$84486783037343$pYNyVrtyMalQrZLxRi7ZLQS1Fl.jkYCgASUi5P8JNb2", "hashcat", "$8$"},
		{"username nine privilege 15 secret 9 $9$87023684531115$phio0TBQwaO7KZ8toQFyGFyDvyOzidaypRWN0uKX0hU", "hashcat", "$9$"},
		{"username seven password 7 070C285F4D06", "cisco", "$9$"},
		{"username generated algorithm-type sha256 secret testing algorithm-type literal", "testing algorithm-type literal", "$8$"},
		{"username privileged privilege 5 algorithm-type scrypt secret test word", "test word", "$9$"},
	} {
		execute(t, s, test.cmd)
		name := strings.Fields(test.cmd)[1]
		secret := s.Running.AAA.Users[name].Secret
		if !strings.HasPrefix(secret, test.prefix) || !config.CheckSecret(secret, test.password) {
			t.Fatal(test.cmd)
		}
	}
	execute(t, s, "enable algorithm-type sha256 secret test-enable")
	if !strings.HasPrefix(s.Running.AAA.EnableSecret, "$8$") || !config.CheckSecret(s.Running.AAA.EnableSecret, "test-enable") {
		t.Fatal("enable algorithm")
	}
	execute(t, s, "enable secret level 7 5 $1$salt$qJH7.N4xYta3aEG/dfqo/0", "end")
	loaded, err := config.Decode(s.Running.JSON())
	if err != nil || !config.CheckSecret(loaded.AAA.EnableSecrets[7], "password") || !strings.Contains(loaded.CLI(), "username five privilege 1 secret 5 $1$") {
		t.Fatal("type 5 did not survive configuration persistence", err)
	}
	execute(t, s, "conf t")
	for _, bad := range []string{
		"username bad secret 5 $9$87023684531115$phio0TBQwaO7KZ8toQFyGFyDvyOzidaypRWN0uKX0hU",
		"username bad password 5 $1$salt$qJH7.N4xYta3aEG/dfqo/0",
		"username bad password 8 $8$84486783037343$pYNyVrtyMalQrZLxRi7ZLQS1Fl.jkYCgASUi5P8JNb2",
		"username bad secret 5 garbage",
	} {
		if _, err := s.Execute(bad); err == nil {
			t.Fatal("invalid encoding accepted", bad)
		}
	}
	execute(t, s, "username fresh secret new-password")
	if !strings.HasPrefix(s.Running.AAA.Users["fresh"].Secret, "$9$") {
		t.Fatal("legacy import changed new-secret algorithm")
	}
}
