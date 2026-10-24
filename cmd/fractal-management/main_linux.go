package main

import (
	"fmt"
	"fractalcr/internal/appliance"
	"fractalcr/internal/nativeguard"
	"os"
)

func main() {
	var err error
	// Only these three service entry points own child-process lifecycles.
	// Management operations, configuration, networking, recovery and diagnostics
	// use native APIs and cannot execute host programs, including accidentally
	// introduced executable helpers in linked dependencies.
	if len(os.Args) > 1 && !ownsChildLifecycle(os.Args[1]) {
		if err = nativeguard.DenyExec(); err != nil {
			fmt.Fprintln(os.Stderr, "Native service policy unavailable")
			os.Exit(1)
		}
	}
	if len(os.Args) == 3 && os.Args[1] == "recover" {
		if appliance.Recovery(os.Args[2], os.Stdout) != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "diagnostics":
		err = appliance.Diagnostics()
	case "serve":
		err = appliance.Serve()
	case "ssh-service":
		err = appliance.SSHService()
	case "network-service":
		err = appliance.NetworkService()
	case "time-service":
		err = appliance.TimeService()
	case "configuration-service":
		err = appliance.ConfigurationService()
	case "buildkit-service":
		err = appliance.BuildkitService()
	case "containerd-service":
		err = appliance.ContainerdService()
	case "prepare":
		err = appliance.Prepare()
	case "debug-service":
		err = appliance.DebugService()
	default:
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func ownsChildLifecycle(mode string) bool {
	return mode == "ssh-service" || mode == "time-service" || mode == "buildkit-service" || mode == "containerd-service"
}
