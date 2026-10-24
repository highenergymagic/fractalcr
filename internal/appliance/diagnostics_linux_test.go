package appliance

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestDiagnosticCleanupLostResponse(t *testing.T) {
	calls := 0
	err := removeDiagnosticContainer(context.Background(), "owned-id", time.Millisecond, 0, func(ctx context.Context, id string) error {
		if id != "owned-id" {
			t.Fatal("changed cleanup target")
		}
		calls++
		if calls == 1 {
			<-ctx.Done()
			return ctx.Err()
		}
		return &engineStatusError{Status: http.StatusNotFound, Message: "Object not found"}
	})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestDiagnosticCleanupBoundsAndRejection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := removeDiagnosticContainer(ctx, "id", time.Second, 0, func(c context.Context, _ string) error { <-c.Done(); return c.Err() })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	calls := 0
	err = removeDiagnosticContainer(context.Background(), "id", time.Second, 0, func(context.Context, string) error {
		calls++
		return &engineStatusError{Status: http.StatusForbidden, Message: "Forbidden"}
	})
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestDiagnosticCleanupRemovalInProgress(t *testing.T) {
	calls := 0
	err := removeDiagnosticContainer(context.Background(), "id", time.Second, 0, func(context.Context, string) error {
		calls++
		if calls == 1 {
			return &engineStatusError{Status: http.StatusConflict, Message: "removal in progress"}
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
