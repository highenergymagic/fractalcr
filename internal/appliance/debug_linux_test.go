package appliance

import (
	"errors"
	"strings"
	"testing"
)

func TestDebugCleanupOwnershipAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name, listing         string
		listErr, removeErr    error
		wantRemove, wantError bool
	}{
		{name: "absent"},
		{name: "owned", listing: "abc123 true", wantRemove: true},
		{name: "foreign", listing: "abc123 false", wantError: true},
		{name: "unlabelled", listing: "abc123", wantError: true},
		{name: "ambiguous", listing: "abc123 true\ndef456 true", wantError: true},
		{name: "daemon unavailable", listErr: errors.New("unavailable"), wantError: true},
		{name: "removal failed", listing: "abc123 true", removeErr: errors.New("failed"), wantRemove: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := debugRun
			defer func() { debugRun = original }()
			removed := false
			debugRun = func(args ...string) (string, error) {
				switch args[0] {
				case "ps":
					if !strings.Contains(strings.Join(args, " "), "name=^/fractal-platform-debug$") {
						t.Fatal("container name is not matched exactly")
					}
					return tc.listing, tc.listErr
				case "rm":
					removed = true
					if strings.Join(args, " ") != "rm -f abc123" {
						t.Fatal("removal must use the inspected container ID")
					}
					return "", tc.removeErr
				default:
					t.Fatalf("unexpected Docker operation: %v", args)
					return "", nil
				}
			}
			if err := removeDebug(); (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error %v", err, tc.wantError)
			}
			if removed != tc.wantRemove {
				t.Fatalf("removed = %v, want %v", removed, tc.wantRemove)
			}
		})
	}
}
