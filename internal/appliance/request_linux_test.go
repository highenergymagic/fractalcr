package appliance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManagementFrameAcceptsEscapedEditorLine(t *testing.T) {
	for _, value := range []string{strings.Repeat("\\", 2047), strings.Repeat("\x01", 2047)} {
		want := managementRequest{Command: value, Transport: "ssh", User: "operator", Verifier: strings.Repeat("x", 128)}
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readManagementRequest(strings.NewReader(string(data) + "\n"))
		if err != nil || got.Command != want.Command || got.Verifier != want.Verifier {
			t.Fatalf("escaped editor frame (%d bytes): %v", len(data), err)
		}
	}
	if _, err := readManagementRequest(strings.NewReader(strings.Repeat("x", 16*1024) + "\n")); err == nil {
		t.Fatal("accepted oversized frame")
	}
}
