// Package dockercontainer provides connhelper for docker-container://<container>
package dockercontainer

import (
	"context"
	"net"
	"net/url"

	"fractalcr/pkg/enginestream"
 "github.com/docker/cli/cli/command"
 cliflags "github.com/docker/cli/cli/flags"
 "os"
	"github.com/moby/buildkit/client/connhelper"
	"github.com/pkg/errors"
)

func init() {
	connhelper.Register("docker-container", Helper)
}

// Helper returns helper for connecting to a Docker container.
// Requires BuildKit v0.5.0 or later in the container.
func Helper(u *url.URL) (*connhelper.ConnectionHelper, error) {
	sp, err := SpecFromURL(u)
	if err != nil {
		return nil, err
	}
	return &connhelper.ConnectionHelper{
		ContextDialer: func(ctx context.Context, addr string) (net.Conn, error) {
            cli,err:=command.NewDockerCli()
            if err!=nil{return nil,err}
            options:=cliflags.NewClientOptions()
            options.Context=sp.Context
            if err=cli.Initialize(options);err!=nil{return nil,err}
            api:=cli.Client()
            return enginestream.Dial(ctx,api,sp.Container,[]string{"buildctl","dial-stdio"},os.Stderr,func(){api.Close()})
		},
	}, nil
}

// Spec
type Spec struct {
	Context   string
	Container string
}

// SpecFromURL creates Spec from URL.
// URL is like docker-container://<container>?context=<context>
// Only <container> part is mandatory.
func SpecFromURL(u *url.URL) (*Spec, error) {
	q := u.Query()
	sp := Spec{
		Context:   q.Get("context"),
		Container: u.Hostname(),
	}
	if sp.Container == "" {
		return nil, errors.New("url lacks container name")
	}
	return &sp, nil
}
