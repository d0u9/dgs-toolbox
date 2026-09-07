package cli

import (
	"dgs-toolbox/internal/tui"

	"github.com/spf13/cobra"
)

// NewRootCommand builds the dgs command hierarchy from the same app registry
// rendered by the TUI. The runner is injected so routing can be tested without
// opening a terminal.
func NewRootCommand(apps []tui.App, run tui.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:           "dgs",
		Short:         "A small toolbox for photo and GPX workflows",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(tui.Launch{})
		},
	}

	for _, app := range apps {
		root.AddCommand(newAppCommand(app, run))
	}
	return root
}

func newAppCommand(app tui.App, run tui.Runner) *cobra.Command {
	command := &cobra.Command{
		Use:   app.ID,
		Short: app.Description,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(tui.Launch{App: app.ID})
		},
	}

	for _, leaf := range app.Commands {
		leaf := leaf
		command.AddCommand(&cobra.Command{
			Use:   leaf.ID,
			Short: leaf.Description,
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return run(tui.Launch{App: app.ID, Command: leaf.ID})
			},
		})
	}
	return command
}
