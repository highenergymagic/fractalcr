// The streaming workload frontend links the clients' API implementations. It
// does not invoke docker, docker-compose, docker-buildx, buildctl or a shell.
package main

import (
	"encoding/json"
	"fmt"
	"fractalcr/internal/buildclient"
	"fractalcr/internal/nativeguard"
	"fractalcr/internal/workloadcatalog"
	_ "fractalcr/pkg/gitapi"
	buildx "github.com/docker/buildx/commands"
	_ "github.com/docker/buildx/driver/docker"
	_ "github.com/docker/buildx/driver/docker-container"
	_ "github.com/docker/buildx/driver/kubernetes"
	_ "github.com/docker/buildx/driver/remote"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/command/commands"
	cliflags "github.com/docker/cli/cli/flags"
	compose "github.com/docker/compose/v5/cmd/compose"
	engineclient "github.com/moby/moby/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"
)

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("Workload operation required")
	}
	if args[0] == "build" {
		return buildclient.Run(append([]string{"request container-build"}, args[1:]...))
	}
	if args[0] == "build-catalog" {
		return buildclient.Run([]string{"catalog"})
	}
	if args[0] != "engine" && args[0] != "catalog" {
		return fmt.Errorf("Invalid workload service")
	}
	cli, err := command.NewDockerCli(command.WithInputStream(os.Stdin), command.WithOutputStream(os.Stdout), command.WithErrorStream(os.Stderr))
	if err != nil {
		return err
	}
	root := engineRoot(cli)
	if args[0] == "catalog" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(catalog(root))
	}
	resolved, err := workloadcatalog.PathArguments(catalog(root), args[1:])
	if err != nil {
		return err
	}
	root.SetArgs(resolved)
	root.SetIn(os.Stdin)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	return root.Execute()
}

func engineRoot(cli *command.DockerCli) *cobra.Command {
	options := cliflags.NewClientOptions()
	root := &cobra.Command{Use: "request container-engine", SilenceUsage: true, SilenceErrors: true, TraverseChildren: true}
	root.CompletionOptions.DisableDefaultCmd = true
	options.InstallFlags(root.Flags())
	initialize := func(cmd *cobra.Command, args []string) error {
		options.SetDefaultOptions(root.Flags())
		return cli.Initialize(options, command.WithAPIClientOptions(engineclient.WithHTTPRequestHook(func(req *http.Request) error {
			if guestFilesystemTarget(req) == "" {
				return nil
			}
			return protectGuestFilesystem(req, cli.Client())
		})))
	}
	root.PersistentPreRunE = initialize
	commands.AddCommands(root, cli)
	bx := buildx.NewRootCmd("buildx", false, cli)
	previous := bx.PersistentPreRunE
	bx.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := previous(cmd, args); err != nil {
			return err
		}
		return initialize(cmd, args)
	}
	root.AddCommand(compose.RootCommand(cli, &compose.BackendOptions{}), bx)
	// Install linked build and bake implementations as root aliases. This also
	// works when global connection flags precede the command.
	installLinked := func(parent *cobra.Command, name string) {
		aliases := buildx.NewRootCmd("buildx", false, cli)
		for _, sub := range aliases.Commands() {
			if sub.Name() == name {
				sub.Flags().AddFlagSet(aliases.PersistentFlags())
				sub.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
					if err := aliases.PersistentPreRunE(cmd, args); err != nil {
						return err
					}
					return initialize(cmd, args)
				}
				for _, old := range parent.Commands() {
					if old.Name() == name {
						parent.RemoveCommand(old)
					}
				}
				aliases.RemoveCommand(sub)
				parent.AddCommand(sub)
				return
			}
		}
	}
	installLinked(root, "build")
	installLinked(root, "bake")
	for _, parent := range root.Commands() {
		if parent.Name() == "image" || parent.Name() == "builder" {
			installLinked(parent, "build")
		}
	}
	root.InitDefaultHelpCmd()
	return root
}

func catalog(cmd *cobra.Command) workloadcatalog.Command {
	cmd.InitDefaultHelpFlag()
	result := workloadcatalog.Command{Name: cmd.Name(), Aliases: cmd.Aliases, Description: cmd.Short, Usage: cmd.Use, Runnable: cmd.Runnable(), Hidden: cmd.Hidden}
	result.MinArgs, result.MaxArgs = argumentBounds(cmd)
	cmd.Flags().AddFlagSet(cmd.InheritedFlags())
	cmd.Flags().AddFlagSet(cmd.PersistentFlags())
	// pflag exposes no getter for SetInterspersed. Probe the real parser with
	// the help flag and a no-op setter, so catalog generation never changes a
	// flag value or executes a command. Preserve run/exec's opaque argv tail.
	if err := cmd.Flags().ParseAll([]string{"catalog-position", "--help"}, func(*pflag.Flag, string) error { return nil }); err != nil {
		panic(err)
	}
	result.Interspersed = cmd.Flags().NArg() == 1
	_ = cmd.Flags().ParseAll(nil, func(*pflag.Flag, string) error { return nil })
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		result.Flags = append(result.Flags, workloadcatalog.Flag{Name: flag.Name, Short: flag.Shorthand, Description: flag.Usage, Type: flag.Value.Type(), Boolean: flag.NoOptDefVal != "", Hidden: flag.Hidden || flag.Deprecated != ""})
	})
	for _, sub := range cmd.Commands() {
		result.Children = append(result.Children, catalog(sub))
	}
	return result
}

// Only probe the clients' known pure arity validators. Custom validators stay
// with their command implementation; catalog generation must not contact APIs.
func argumentBounds(cmd *cobra.Command) (int, *int) {
	if cmd.Args == nil {
		return 0, nil
	}
	name := runtime.FuncForPC(reflect.ValueOf(cmd.Args).Pointer()).Name()
	known := false
	for _, validator := range []string{".ExactArgs", ".RequiresMinArgs", ".RequiresMaxArgs", ".RequiresRangeArgs", ".MinimumNArgs", ".MaximumNArgs", ".RangeArgs", ".NoArgs"} {
		if strings.Contains(name, validator) {
			known = true
			break
		}
	}
	if !known {
		return 0, nil
	}
	minimum := -1
	for count := 0; count <= 16; count++ {
		args := make([]string, count)
		for i := range args {
			args[i] = "catalog-argument"
		}
		if cmd.Args(cmd, args) == nil {
			if minimum < 0 {
				minimum = count
			}
		} else if minimum >= 0 {
			maximum := count - 1
			return minimum, &maximum
		}
	}
	if minimum < 0 {
		return 0, nil
	}
	return minimum, nil
}
func main() {
	if err := nativeguard.DenyExec(); err != nil {
		fmt.Fprintln(os.Stderr, "% Native workload policy unavailable.")
		os.Exit(1)
	}
	if err := run(os.Args[1:]); err != nil {
		text := strings.NewReplacer("/datastore/data0/files/", "data0:", "/datastore/data0/projects/", "data0:", "/run/docker.sock", "container service", "/run/buildkit/buildkitd.sock", "build service").Replace(err.Error())
		fmt.Fprintln(os.Stderr, "% "+workloadcatalog.NativeWording(text))
		os.Exit(1)
	}
}
