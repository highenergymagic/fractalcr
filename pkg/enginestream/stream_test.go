package enginestream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

type fixtureAPI struct {
	attached             client.ExecAttachResult
	createErr, attachErr error
	options              client.ExecCreateOptions
}

func (f *fixtureAPI) ExecCreate(_ context.Context, _ string, o client.ExecCreateOptions) (client.ExecCreateResult, error) {
	f.options = o
	return client.ExecCreateResult{ID: "fixture"}, f.createErr
}
func (f *fixtureAPI) ExecAttach(_ context.Context, _ string, _ client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return f.attached, f.attachErr
}

func TestBufferedDuplexStream(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	var frames bytes.Buffer
	writeFrame(&frames, stdcopy.Stdout, []byte("buffered"))
	stderr := &bytes.Buffer{}
	reader := bufio.NewReader(io.MultiReader(bytes.NewReader(frames.Bytes()), local))
	if _, err := reader.Peek(1); err != nil {
		t.Fatal(err)
	}
	fixture := &fixtureAPI{attached: client.ExecAttachResult{HijackedResponse: client.HijackedResponse{Conn: local, Reader: reader}}}
	released := 0
	stream, err := Dial(context.Background(), fixture, "fixture", []string{"endpoint"}, stderr, func() { released++ })
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	stream.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan error, 1)
	go func() {
		input := make([]byte, 4)
		_, err := io.ReadFull(remote, input)
		if err == nil && string(input) != "ping" {
			err = errors.New("wrong input")
		}
		if err == nil {
			_, err = writeFrame(remote, stdcopy.Stderr, []byte("diagnostic"))
		}
		if err == nil {
			_, err = writeFrame(remote, stdcopy.Stdout, []byte("reply"))
		}
		remote.Close()
		done <- err
	}()
	if _, err = stream.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "bufferedreply" || stderr.String() != "diagnostic" {
		t.Fatalf("streams: %q %q", output, stderr.String())
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if !fixture.options.AttachStdin || !fixture.options.AttachStdout || !fixture.options.AttachStderr || fixture.options.TTY {
		t.Fatal("invalid stream attachment")
	}
	stream.Close()
	stream.Close()
	if released != 1 {
		t.Fatalf("release called %d times", released)
	}
}

func TestFailedDialReleasesClient(t *testing.T) {
	for _, stage := range []string{"create", "attach"} {
		t.Run(stage, func(t *testing.T) {
			fixture := &fixtureAPI{}
			if stage == "create" {
				fixture.createErr = errors.New(stage)
			} else {
				fixture.attachErr = errors.New(stage)
			}
			released := 0
			_, err := Dial(context.Background(), fixture, "fixture", nil, nil, func() { released++ })
			if err == nil || !strings.Contains(err.Error(), stage) || released != 1 {
				t.Fatalf("error=%v release=%d", err, released)
			}
		})
	}
}

func TestCloseUnblocksRead(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	fixture := &fixtureAPI{attached: client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(local, "")}}
	stream, err := Dial(context.Background(), fixture, "fixture", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := stream.Read(make([]byte, 1)); done <- err }()
	stream.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("read remained blocked")
	}
}

func writeFrame(w io.Writer, stream stdcopy.StdType, payload []byte) (int, error) {
	header := make([]byte, 8)
	header[0] = byte(stream)
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return 0, err
	}
	return w.Write(payload)
}
