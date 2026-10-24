// SPDX-License-Identifier: Apache-2.0
// Adapted from moby/buildkit v0.34.1 cmd/buildctl; see UPSTREAM.json.
package buildclient

import (
	"context"
	"encoding/json"
	"fmt"
	"fractalcr/internal/workloadcatalog"
	"io"
	"os"

	_ "github.com/moby/buildkit/client/connhelper/npipe"
	bccommon "github.com/moby/buildkit/cmd/buildctl/common"
	"github.com/moby/buildkit/util/apicaps"
	"github.com/moby/buildkit/util/appdefaults"
	_ "github.com/moby/buildkit/util/grpcutil/encoding/proto"
	"github.com/moby/buildkit/util/stack"
	"github.com/moby/buildkit/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"go.opentelemetry.io/otel"
)

func init() {
	apicaps.ExportedProduct = "buildkit"

	stack.SetVersionInfo(version.Version, version.Revision)

	// do not log tracing errors to stdio
	otel.SetErrorHandler(skipErrors{})
}

func Run(args []string) error {
	cli.VersionPrinter = func(c *cli.Command) {
		fmt.Println(c.Name, version.Package, c.Version, version.Revision)
	}
	app := &cli.Command{}
	app.Name = "request container-build"
	app.Usage = "build utility"
	app.Version = version.Version
	app.EnableShellCompletion = false

	defaultAddress := os.Getenv("BUILDKIT_HOST")
	if defaultAddress == "" {
		defaultAddress = appdefaults.Address
	}

	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output in logs",
		},
		&cli.StringFlag{
			Name:  "addr",
			Usage: "buildkitd address",
			Value: defaultAddress,
		},
		// Add format flag to control log formatter
		&cli.StringFlag{
			Name:  "log-format",
			Usage: "log formatter: json or text",
			Value: "text",
		},
		&cli.StringFlag{
			Name:  "tlsservername",
			Usage: "buildkitd server name for certificate validation",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "tlscacert",
			Usage: "CA certificate for validation",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "tlscert",
			Usage: "client certificate",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "tlskey",
			Usage: "client key",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "tlsdir",
			Usage: "directory containing CA certificate, client certificate, and client key. Supported file names are (ca.pem, cert.pem, key.pem) or (ca.crt, tls.crt, tls.key)",
			Value: "",
		},
		&cli.IntFlag{
			Name:  "timeout",
			Usage: "timeout backend connection after value seconds",
			Value: 5,
		},
		&cli.BoolFlag{
			Name:  "wait",
			Usage: "block RPCs until the connection becomes available",
		},
	}

	app.Commands = []*cli.Command{
		diskUsageCommand,
		pruneCommand,
		pruneHistoriesCommand,
		buildCommand,
		debugCommand,
		dialStdioCommand,
	}
	for _, cmd := range debugCommand.Commands {
		if cmd.Name == "dump-metadata" {
			for _, flag := range cmd.Flags {
				if f, ok := flag.(*cli.StringFlag); ok && f.Name == "root" {
					f.Value = "/datastore/data0/buildkit"
				}
			}
		}
	}
	disableSliceFlagSeparator(app)
	if len(args) == 1 && args[0] == "catalog" {
		// Initialize the framework's real help/version flags and help command
		// graph without executing a build or connecting to a daemon.
		app.Writer, app.ErrWriter = io.Discard, io.Discard
		if err := app.Run(context.Background(), []string{app.Name, "--help"}); err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(commandCatalog(app))
	}
	// The management service owns help and syntax diagnostics. A client parse
	// failure must return an error instead of printing a second, Unix CLI.
	var nativeErrors func(*cli.Command)
	nativeErrors = func(cmd *cli.Command) {
		cmd.HideHelp = true
		cmd.OnUsageError = func(_ context.Context, _ *cli.Command, err error, _ bool) error { return err }
		cmd.ExitErrHandler = func(_ context.Context, _ *cli.Command, _ error) {}
		for _, child := range cmd.Commands {
			nativeErrors(child)
		}
	}
	nativeErrors(app)

	var debugEnabled bool

	app.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		debugEnabled = cmd.Bool("debug")
		// Use Format flag to control log formatter
		logFormat := cmd.String("log-format")
		switch logFormat {
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{})
		case "text", "":
			logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
		default:
			return ctx, errors.Errorf("unsupported log type %q", logFormat)
		}
		if debugEnabled {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return ctx, nil
	}

	if err := bccommon.AttachAppContext(app); err != nil {
		return err
	}

	resolved, err := workloadcatalog.PathArguments(commandCatalog(app), args[1:])
	if err != nil {
		return err
	}
	return app.Run(context.Background(), append([]string{args[0]}, resolved...))
}

func commandCatalog(cmd *cli.Command) workloadcatalog.Command {
	return commandCatalogFlags(cmd, nil)
}

func commandCatalogFlags(cmd *cli.Command, inherited []cli.Flag) workloadcatalog.Command {
	result := workloadcatalog.Command{Name: cmd.Name, Aliases: cmd.Aliases, Description: cmd.Usage, Usage: cmd.ArgsUsage, Runnable: cmd.Action != nil}
	// These pinned debug actions validate their operand inside Action instead
	// of declaring an ArgsUsage/arity to urfave. Describe that existing contract.
	switch cmd.Name {
	case "get":
		result.MinArgs, result.Usage = 1, "<DIGEST>"
	case "logs", "ctl":
		result.MinArgs, result.Usage = 1, "<BUILD-REF>"
	}
	seen := map[string]bool{}
	for _, flag := range append(append([]cli.Flag{}, cmd.Flags...), inherited...) {
		if len(flag.Names()) == 0 || seen[flag.Names()[0]] {
			continue
		}
		seen[flag.Names()[0]] = true
		if visible, ok := flag.(cli.VisibleFlag); ok && !visible.IsVisible() {
			continue
		}
		doc, ok := flag.(cli.DocGenerationFlag)
		if !ok || len(flag.Names()) == 0 {
			continue
		}
		f := workloadcatalog.Flag{Name: flag.Names()[0], Description: doc.GetUsage(), Type: doc.TypeName(), Boolean: !doc.TakesValue()}
		for _, alias := range flag.Names()[1:] {
			if len(alias) == 1 {
				f.Short = alias
				break
			}
		}
		result.Flags = append(result.Flags, f)
	}
	var persistent []cli.Flag
	for _, flag := range cmd.Flags {
		if local, ok := flag.(cli.LocalFlag); ok && !local.IsLocal() {
			persistent = append(persistent, flag)
		}
	}
	persistent = append(persistent, inherited...)
	for _, child := range cmd.Commands {
		if !child.Hidden {
			result.Children = append(result.Children, commandCatalogFlags(child, persistent))
		}
	}
	return result
}

type skipErrors struct{}

func (skipErrors) Handle(err error) {}

func commandAction(fn func(*cli.Command) error) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		return fn(cmd)
	}
}

func disableSliceFlagSeparator(cmd *cli.Command) {
	cmd.DisableSliceFlagSeparator = true
	for _, subcmd := range cmd.Commands {
		disableSliceFlagSeparator(subcmd)
	}
}
