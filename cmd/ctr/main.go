package main

import (
	"flag"
	"fmt"
	"os"

	"fractalcr/internal/cli"
)

func main() {
	state := flag.String("state", "./state", "startup configuration directory")
	batch := flag.Bool("batch", false, "read commands from stdin, fail on first error")
	flag.Parse()
	s, err := cli.New(*state)
	if err == nil {
		err = s.Loop(os.Stdin, os.Stdout, *batch)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "%", err)
		os.Exit(1)
	}
}
