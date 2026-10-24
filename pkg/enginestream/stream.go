// Package enginestream opens container streams through the Engine API.
package enginestream

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

type API interface {
	ExecCreate(context.Context, string, client.ExecCreateOptions) (client.ExecCreateResult, error)
	ExecAttach(context.Context, string, client.ExecAttachOptions) (client.ExecAttachResult, error)
}

// Dial starts the protocol endpoint inside the selected container. No host
// command is started. The returned connection owns the HTTP hijack and client.
func Dial(ctx context.Context, api API, container string, command []string, stderr io.Writer, release func()) (net.Conn, error) {
	fail := func(err error) (net.Conn, error) {
		if release != nil {
			release()
		}
		return nil, err
	}
	created, err := api.ExecCreate(ctx, container, client.ExecCreateOptions{Cmd: command, AttachStdin: true, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return fail(err)
	}
	if created.ID == "" {
		return fail(fmt.Errorf("container stream identifier missing"))
	}
	attached, err := api.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return fail(err)
	}
	reader, writer := io.Pipe()
	stream := &connection{Conn: attached.Conn, reader: reader, release: release}
	if stderr == nil {
		stderr = io.Discard
	}
	go func() {
		// Reader includes bytes already buffered while upgrading HTTP. Reading
		// Conn directly would silently lose the first protocol frame.
		_, err := stdcopy.StdCopy(writer, stderr, attached.Reader)
		writer.CloseWithError(err)
	}()
	return stream, nil
}

type connection struct {
	net.Conn
	reader  *io.PipeReader
	release func()
	once    sync.Once
	err     error
}

func (c *connection) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *connection) Close() error {
	c.once.Do(func() {
		c.reader.Close()
		c.err = c.Conn.Close()
		if c.release != nil {
			c.release()
		}
	})
	return c.err
}
func (c *connection) CloseWrite() error {
	if half, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return half.CloseWrite()
	}
	return fmt.Errorf("container connection does not support write shutdown")
}
