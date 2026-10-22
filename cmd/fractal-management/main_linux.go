package main

import (
	"fmt"
	"fractalcr/internal/appliance"
	"os"
)

func main() {
	var err error
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
	case "serve":
		err = appliance.Serve()
	case "ssh-service":
		err = appliance.SSHService()
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
