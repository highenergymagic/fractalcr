package appliance

import (
	"errors"
	"testing"
)

func TestDebugCleanupOwnershipAndFailure(t *testing.T) {
	owned := debugContainer{ID: "abc123", Names: []string{"/" + debugName}, Labels: map[string]string{"com.fractalcr.debug": "true"}}
	foreign := debugContainer{ID: "def456", Names: []string{"/" + debugName}}
	other := debugContainer{ID: "other", Names: []string{"/" + debugName + "-other"}}
	for _, tc := range []struct {
		name                  string
		entries               []debugContainer
		listErr, removeErr    error
		wantRemove, wantError bool
	}{
		{name: "absent"},
		{name: "owned", entries: []debugContainer{owned}, wantRemove: true},
		{name: "foreign", entries: []debugContainer{foreign}, wantError: true},
		{name: "exact name", entries: []debugContainer{other}},
		{name: "ambiguous", entries: []debugContainer{owned, owned}, wantError: true},
		{name: "daemon unavailable", listErr: errors.New("unavailable"), wantError: true},
		{name: "removal failed", entries: []debugContainer{owned}, removeErr: errors.New("failed"), wantRemove: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			list, remove := debugContainers, deleteDebugContainer
			defer func() { debugContainers, deleteDebugContainer = list, remove }()
			removed := false
			debugContainers = func() ([]debugContainer, error) { return tc.entries, tc.listErr }
			deleteDebugContainer = func(id string) error {
				removed = true
				if id != "abc123" {
					t.Fatal("removal must use inspected container ID")
				}
				return tc.removeErr
			}
			if err := removeDebug(); (err != nil) != tc.wantError {
				t.Fatalf("error=%v, want error %v", err, tc.wantError)
			}
			if removed != tc.wantRemove {
				t.Fatalf("removed=%v, want %v", removed, tc.wantRemove)
			}
		})
	}
}
