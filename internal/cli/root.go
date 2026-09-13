package cli

import (
	"fmt"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"

	"github.com/spf13/cobra"
)

// NewRootCommand builds the dgs command hierarchy from the same app registry
// rendered by the TUI. The runner is injected so routing can be tested without
// opening a terminal.
func NewRootCommand(apps []tui.App, run tui.Runner) *cobra.Command {
	var exportConfig bool
	var configPath string
	root := &cobra.Command{
		Use:           "dgs",
		Short:         "A small workflow toolbox",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args: func(command *cobra.Command, args []string) error {
			if exportConfig {
				return cobra.MaximumNArgs(1)(command, args)
			}
			return cobra.NoArgs(command, args)
		},
		RunE: func(command *cobra.Command, args []string) error {
			if exportConfig {
				destination := ""
				if len(args) == 1 {
					destination = args[0]
				}
				path, err := config.ExportDefault(destination)
				if err != nil {
					return err
				}
				fmt.Fprintf(command.OutOrStdout(), "Exported default config to %s\n", path)
				return nil
			}
			return run(tui.Launch{ConfigPath: configPath})
		},
	}
	root.Flags().BoolVar(&exportConfig, "export-config", false, "write the default global configuration")
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "", "read configuration from this file")

	for _, app := range apps {
		root.AddCommand(newAppCommand(app, run, &configPath))
	}
	return root
}

func newAppCommand(app tui.App, run tui.Runner, configPath *string) *cobra.Command {
	if app.Direct && len(app.Commands) == 1 {
		leaf := app.Commands[0]
		command := &cobra.Command{
			Use:   app.ID,
			Short: app.Description,
			Args:  cobra.NoArgs,
		}
		chosen := addReports(command, app.Reports)
		given := addFlags(command, leaf.Flags)
		command.RunE = func(cmd *cobra.Command, _ []string) error {
			if report, ok := chosen(); ok {
				return runReport(cmd, report, *configPath)
			}
			return run(tui.Launch{App: app.ID, Command: leaf.ID, ConfigPath: *configPath, Flags: given(cmd)})
		}
		return command
	}
	command := &cobra.Command{
		Use:   app.ID,
		Short: app.Description,
		Args:  cobra.NoArgs,
	}
	chosen := addReports(command, app.Reports)
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		if report, ok := chosen(); ok {
			return runReport(cmd, report, *configPath)
		}
		return run(tui.Launch{App: app.ID, ConfigPath: *configPath})
	}

	for _, leaf := range app.Commands {
		leaf := leaf
		sub := &cobra.Command{
			Use:   leaf.ID,
			Short: leaf.Description,
			Args:  cobra.NoArgs,
		}
		given := addFlags(sub, leaf.Flags)
		sub.RunE = func(cmd *cobra.Command, _ []string) error {
			return run(tui.Launch{App: app.ID, Command: leaf.ID, ConfigPath: *configPath, Flags: given(cmd)})
		}
		command.AddCommand(sub)
	}
	return command
}

// addFlags registers a command's flags and returns the ones given on the
// command line, so flags left out keep the configured values.
func addFlags(command *cobra.Command, flags []tui.Flag) func(*cobra.Command) map[string]string {
	values := make([]string, len(flags))
	for index, flag := range flags {
		command.Flags().StringVar(&values[index], flag.Name, "", flag.Usage)
	}
	return func(cmd *cobra.Command) map[string]string {
		var given map[string]string
		for index, flag := range flags {
			if cmd.Flags().Changed(flag.Name) {
				if given == nil {
					given = map[string]string{}
				}
				given[flag.Name] = values[index]
			}
		}
		return given
	}
}

// addReports turns an app's reports into flags on its command and returns the
// one the caller asked for. A report answers on stdout and never opens the TUI.
func addReports(command *cobra.Command, reports []tui.Report) func() (tui.Report, bool) {
	selected := make([]bool, len(reports))
	for index, report := range reports {
		command.Flags().BoolVar(&selected[index], report.Flag, false, report.Description)
	}
	return func() (tui.Report, bool) {
		for index, chosen := range selected {
			if chosen {
				return reports[index], true
			}
		}
		return tui.Report{}, false
	}
}

// runReport loads the configuration the report reads and writes it to stdout.
func runReport(command *cobra.Command, report tui.Report, configPath string) error {
	global, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	return report.Run(command.OutOrStdout(), global)
}

func loadConfig(path string) (config.Config, error) {
	if path != "" {
		return config.LoadPath(path)
	}
	return config.Load()
}
