package cli

import (
	"encoding/json"
	"fmt"

	"dgs-toolbox/internal/buildinfo"
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
		Long:          rootHelp,
		Version:       buildinfo.String(),
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
				written, skipped, err := config.ExportDefault(destination)
				for _, path := range written {
					fmt.Fprintf(command.OutOrStdout(), "Wrote %s\n", path)
				}
				for _, path := range skipped {
					fmt.Fprintf(command.OutOrStdout(), "Kept %s, which exists\n", path)
				}
				return err
			}
			return run(tui.Launch{ConfigPath: configPath})
		},
	}
	root.Flags().BoolVar(&exportConfig, "export-config", false, "write a default config.json for every command into a directory")
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "", "read configuration from this directory")

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
			Long:  leaf.Help,
			Args:  cobra.NoArgs,
		}
		if command.Long == "" {
			command.Long = app.Help
		}
		chosen := addReports(command, app.Reports)
		given := addFlags(command, leaf.Flags)
		command.RunE = func(cmd *cobra.Command, _ []string) error {
			if report, ok := chosen(); ok {
				return runReport(cmd, report, *configPath, app.ID)
			}
			return run(tui.Launch{App: app.ID, Command: leaf.ID, ConfigPath: *configPath, Flags: given(cmd)})
		}
		addActions(command, app.ID, app.Actions, configPath)
		return command
	}
	command := &cobra.Command{
		Use:   app.ID,
		Short: app.Description,
		Long:  app.Help,
		Args:  cobra.NoArgs,
	}
	chosen := addReports(command, app.Reports)
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		if report, ok := chosen(); ok {
			return runReport(cmd, report, *configPath, app.ID)
		}
		if len(app.Commands) == 0 {
			// Nothing to open: an app of Actions alone lists them.
			return cmd.Help()
		}
		return run(tui.Launch{App: app.ID, ConfigPath: *configPath})
	}

	for _, leaf := range app.Commands {
		leaf := leaf
		sub := &cobra.Command{
			Use:   leaf.ID,
			Short: leaf.Description,
			Long:  leaf.Help,
			Args:  cobra.NoArgs,
		}
		given := addFlags(sub, leaf.Flags)
		sub.RunE = func(cmd *cobra.Command, _ []string) error {
			return run(tui.Launch{App: app.ID, Command: leaf.ID, ConfigPath: *configPath, Flags: given(cmd)})
		}
		command.AddCommand(sub)
	}
	addActions(command, app.ID, app.Actions, configPath)
	return command
}

// addActions registers an app's shell-only subcommands.
func addActions(command *cobra.Command, part string, actions []tui.Action, configPath *string) {
	for _, action := range actions {
		action := action
		sub := &cobra.Command{
			Use:   action.ID + " " + action.Usage,
			Short: action.Description,
			Long:  action.Help,
			Args:  actionArgs(action),
			RunE: func(cmd *cobra.Command, args []string) error {
				flags := make(map[string]string, len(action.Flags))
				for _, flag := range action.Flags {
					if flag.Repeatable {
						values, err := cmd.Flags().GetStringArray(flag.Name)
						if err != nil {
							return err
						}
						encoded, err := json.Marshal(values)
						if err != nil {
							return err
						}
						flags[flag.Name] = string(encoded)
					} else {
						flags[flag.Name] = cmd.Flags().Lookup(flag.Name).Value.String()
					}
				}
				if action.RunWithConfig != nil {
					global, err := loadConfig(*configPath, part)
					if err != nil {
						return err
					}
					return action.RunWithConfig(cmd.InOrStdin(), cmd.OutOrStdout(), args, flags, global)
				}
				return action.Run(cmd.InOrStdin(), cmd.OutOrStdout(), args, flags)
			},
		}
		for _, flag := range action.Flags {
			if flag.Repeatable {
				sub.Flags().StringArrayP(flag.Name, flag.Shorthand, nil, flag.Usage)
			} else if flag.Bool {
				sub.Flags().BoolP(flag.Name, flag.Shorthand, flag.Default == "true", flag.Usage)
			} else {
				sub.Flags().StringP(flag.Name, flag.Shorthand, flag.Default, flag.Usage)
			}
		}
		command.AddCommand(sub)
	}
}

// actionArgs is how many positional arguments an action takes: exactly
// Args; at least MinArgs when it accepts a variable number, since a selector
// is one or more terms; or between the two when it also has a ceiling, since
// a directory that defaults to a configured one is none or one.
func actionArgs(action tui.Action) cobra.PositionalArgs {
	switch {
	case action.MaxArgs > 0:
		return cobra.RangeArgs(action.MinArgs, action.MaxArgs)
	case action.MinArgs > 0:
		return cobra.MinimumNArgs(action.MinArgs)
	}
	return cobra.ExactArgs(action.Args)
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
func runReport(command *cobra.Command, report tui.Report, configPath, app string) error {
	global, err := loadConfig(configPath, app)
	if err != nil {
		return err
	}
	return report.Run(command.OutOrStdout(), global)
}

// loadConfig loads the configuration and refuses when the file of part, the
// command about to run, was refused.
func loadConfig(path, part string) (config.Config, error) {
	var global config.Config
	var err error
	if path != "" {
		global, err = config.LoadDir(path)
	} else {
		global, err = config.Load()
	}
	if err != nil {
		return global, err
	}
	return global, global.PartErr(part)
}

const rootHelp = `A workflow toolbox with terminal workspaces, local web pages and CLI actions.

Examples:
  dgs                         Open the command picker.
  dgs photo                   Open the Photo picker.
  dgs photo import            Open a workspace directly.
  dgs box verify /path/to/box  Run a CLI action, print results, and return.
  dgs photo encode --help     Read help without starting a workspace.
  dgs --export-config /path/to/config-directory

--export-config writes a default <part>/config.json for each app. With no
argument it uses the working directory; existing files are kept.
Configuration directory selection: --config / -c, then DGS_TOOLBOX_CONFIG,
then $XDG_CONFIG_HOME/dgs-toolbox (or ~/.config/dgs-toolbox).
Each app reads its own <part>/config.json. Command flags override configuration
for this process only. Omitted flags retain configured values.

In terminal forms, Tab / Shift+Tab move focus and Enter edits or selects.
Arrow keys and h j k l navigate; printable keys belong to an active editor.
Esc backs out of an interaction; leaving a workspace and quitting are confirmed.
Ctrl+C requests a safe-default exit confirmation; Ctrl+Z suspends the process.
CLI actions are listed below but do not appear in the interactive picker.`
