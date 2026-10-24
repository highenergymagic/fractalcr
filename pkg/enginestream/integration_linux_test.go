package enginestream

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"fractalcr/internal/nativeguard"
	"github.com/moby/moby/client"
)

// The fixture is an explicitly created disposable workload container. The
// host remains under the no-exec policy throughout the protocol exchange.
func TestEngineStreamWithoutExecutables(t *testing.T) {
	container := os.Getenv("FCR_ENGINE_STREAM_CONTAINER")
	if container == "" {
		t.Skip("Engine integration fixture not configured")
	}
	if os.Getenv("FCR_ENGINE_STREAM_CHILD") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestEngineStreamWithoutExecutables$")
		command.Env = append(os.Environ(), "FCR_ENGINE_STREAM_CHILD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("native stream: %v: %s", err, output)
		}
		return
	}
	if err := nativeguard.DenyExec(); err != nil {
		t.Fatal(err)
	}
	api, err := client.New(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := Dial(ctx, api, container, []string{"/usr/bin/cat"}, nil, func() { api.Close() })
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	stream.SetDeadline(time.Now().Add(10 * time.Second))
	payload := []byte("native container stream\n")
	if _, err = stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	received := make([]byte, len(payload))
	if _, err = io.ReadFull(stream, received); err != nil {
		t.Fatal(err)
	}
	if string(received) != string(payload) {
		t.Fatalf("unexpected stream: %q", received)
	}
	if err = stream.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadAll(stream); err != nil {
		t.Fatal(err)
	}
}
