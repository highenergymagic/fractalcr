package appliance

import (
	"net"
	"testing"
	"time"
)

func TestSSHOutputDrainClosesStalledTransport(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Write([]byte("pending output"))
	}()
	drainSSHOutput(server, done, 10*time.Millisecond)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stalled output writer retained the closed session")
	}
}

func TestSSHOutputDrainPreservesCompletedTransport(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	done := make(chan struct{})
	close(done)
	drainSSHOutput(server, done, time.Second)
	if err := server.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("completed output closed the transport before exit status: %v", err)
	}
}
