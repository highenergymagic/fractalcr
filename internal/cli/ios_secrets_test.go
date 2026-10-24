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
}
