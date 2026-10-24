package appliance

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRecoveryUsesLocalGrammarWithoutServices(t *testing.T) {
	for _, test := range []struct{ action, input, local, contains string }{
		{"tick", "", "", ""},
		{"", "sh ver", "show version", ""},
		{"", "req plat soft serv management rest", "request platform software service management restart", ""},
		{"help", "sh", "", "show"},
		{"help", "show ", "", "version"},
	} {
		var b bytes.Buffer
		if err := Recovery(test.action, test.input, &b); err != nil {
			t.Fatal(err)
		}
		var r reply
		if err := json.Unmarshal(b.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		if r.Error != "" || r.Local != test.local || !strings.Contains(r.Output, test.contains) {
			t.Fatalf("%q: %+v", test.input, r)
		}
		if strings.Contains(r.Output, "running-config") {
			t.Fatal("recovery advertised unavailable configuration")
		}
	}
}
