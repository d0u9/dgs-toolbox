package main

import (
	"fmt"
	"os"

	"dgs-toolbox/internal/apps"
	"dgs-toolbox/internal/cli"
	"dgs-toolbox/internal/tui"
)

func main() {
	registry := apps.All()
	run := func(launch tui.Launch) error {
		return tui.Run(registry, launch)
	}
	if err := cli.NewRootCommand(registry, run).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
