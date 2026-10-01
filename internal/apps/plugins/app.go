// Package plugins is `dgs plugins`: the plugins dgs carries for other
// applications, which it installs, updates and removes, and reports on.
// See docs/apps/plugins.md.
package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"dgs-toolbox/internal/buildinfo"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/plugins"
	"dgs-toolbox/internal/plugins/bundled"
	"dgs-toolbox/internal/plugins/obsidian"
	"dgs-toolbox/internal/tui"
)

var (
	vaultFlag = tui.ActionFlag{Name: "vault", Repeatable: true, Usage: "an Obsidian vault to work on, instead of plugins.obsidian.vaults; may be given more than once"}
	forceFlag = tui.ActionFlag{Name: "force", Bool: true, Usage: "replace or remove files changed since dgs installed them"}
	fromFlag  = tui.ActionFlag{Name: "from", Usage: "a folder dgs plugins export wrote: start from its files instead of the ones dgs carries"}
)

// New returns the Plugins app definition. It has no TUI: every part of it is
// an Action.
func New() tui.App {
	return tui.App{
		ID:          "plugins",
		Name:        "Plugins",
		Description: "Plugins dgs carries for other applications: status, install, update, uninstall, bootstrap, export",
		Actions: []tui.Action{{
			ID:          "status",
			Usage:       "[<plugin>...]",
			Description: "Show every plugin dgs carries in every place it goes: installed or not, which version, changed since or not, switched on or not. Changes nothing.",
			MaxArgs:     64,
			Flags:       []tui.ActionFlag{vaultFlag},
			RunWithConfig: func(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
				return run(out, args, flags, global, "status")
			},
		}, {
			ID:          "install",
			Usage:       "[<plugin>...]",
			Description: "Install plugins, every one dgs carries when none is named, in every place they go. Leaves files changed by hand alone unless --force.",
			MaxArgs:     64,
			Flags:       []tui.ActionFlag{vaultFlag, forceFlag},
			RunWithConfig: func(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
				return run(out, args, flags, global, "install")
			},
		}, {
			ID:          "update",
			Usage:       "[<plugin>...]",
			Description: "Replace installed plugins an older dgs wrote with what this one carries. Installs nothing new.",
			MaxArgs:     64,
			Flags:       []tui.ActionFlag{vaultFlag, forceFlag},
			RunWithConfig: func(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
				return run(out, args, flags, global, "update")
			},
		}, {
			ID:          "uninstall",
			Usage:       "<plugin>...",
			Description: "Remove the files dgs installed for a plugin. The application's own settings for it stay.",
			MinArgs:     1,
			MaxArgs:     64,
			Flags:       []tui.ActionFlag{vaultFlag, forceFlag},
			RunWithConfig: func(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
				return run(out, args, flags, global, "uninstall")
			},
		}, {
			ID:          "bootstrap",
			Description: "Set up a vault: install everything dgs carries, write the starting files it lacks, and list what is left to do by hand. Overwrites nothing.",
			Flags:       []tui.ActionFlag{vaultFlag, fromFlag},
			RunWithConfig: func(_ io.Reader, out io.Writer, _ []string, flags map[string]string, global config.Config) error {
				return bootstrap(out, flags, global)
			},
		}, {
			ID:          "export",
			Usage:       "<dir>",
			Description: "Copy a vault's starting files, as they are now, into a folder, for someone else's bootstrap --from.",
			MinArgs:     1,
			MaxArgs:     1,
			Flags:       []tui.ActionFlag{vaultFlag, forceFlag},
			RunWithConfig: func(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
				return export(out, args[0], flags, global)
			},
		}},
	}
}

func run(out io.Writer, args []string, flags map[string]string, global config.Config, action string) error {
	chosen, err := choose(bundled.All(pathsOf(global)), args)
	if err != nil {
		return err
	}
	vaults, err := vaultsOf(flags, global)
	if err != nil {
		return err
	}
	force := flags["force"] == "true"
	statuses, err := statusesOf(chosen, vaults, global)
	if err != nil {
		return err
	}

	var failures []error
	for index, status := range statuses {
		var err error
		switch action {
		case "install":
			statuses[index], err = plugins.Install(status.Target, status.Plugin, buildinfo.String(), force, time.Now())
		case "update":
			if status.State == plugins.Outdated || (force && status.State == plugins.Modified) {
				statuses[index], err = plugins.Install(status.Target, status.Plugin, buildinfo.String(), force, time.Now())
			}
		case "uninstall":
			statuses[index], err = plugins.Uninstall(status.Target, status.Plugin, force)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	write(out, statuses)
	return errors.Join(failures...)
}

// pathsOf is where in a vault the Obsidian files go.
func pathsOf(global config.Config) bundled.Paths {
	obsidian := global.Plugins.Obsidian
	return bundled.Paths{
		Public: obsidian.Folders.Public, QuickAdd: obsidian.Folders.QuickAdd,
		Templates: obsidian.Folders.Templates, Timelines: obsidian.Timelines,
	}
}

// vaultsOf is the vaults named with --vault, else the configured ones.
func vaultsOf(flags map[string]string, global config.Config) ([]string, error) {
	var vaults []string
	if raw := flags["vault"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &vaults); err != nil {
			return nil, err
		}
	}
	if len(vaults) == 0 {
		vaults = global.Plugins.Obsidian.Vaults
	}
	if len(vaults) == 0 {
		return nil, errors.New("no Obsidian vault: name one with --vault, or list them in obsidian.vaults in plugins/config.json")
	}
	return vaults, nil
}

// choose is the plugins named, by "host/id" or by id alone when only one host
// has a plugin of that id; every plugin when none is named.
func choose(all []plugins.Plugin, names []string) ([]plugins.Plugin, error) {
	if len(names) == 0 {
		return all, nil
	}
	var chosen []plugins.Plugin
	for _, name := range names {
		var matches []plugins.Plugin
		for _, plugin := range all {
			if plugin.Name() == name || plugin.ID == name {
				matches = append(matches, plugin)
			}
		}
		switch len(matches) {
		case 0:
			return nil, fmt.Errorf("dgs carries no plugin %q; dgs plugins status lists them", name)
		case 1:
			chosen = append(chosen, matches[0])
		default:
			return nil, fmt.Errorf("%q is a plugin of more than one application; name it as <host>/%s", name, name)
		}
	}
	return chosen, nil
}

// statusesOf inspects each plugin in every place its host has.
func statusesOf(chosen []plugins.Plugin, vaults []string, global config.Config) ([]plugins.Status, error) {
	targets := map[string][]plugins.Target{}
	var statuses []plugins.Status
	for _, plugin := range chosen {
		found, ok := targets[plugin.Host]
		if !ok {
			var err error
			if found, err = targetsOf(plugin.Host, vaults, global); err != nil {
				return nil, err
			}
			targets[plugin.Host] = found
		}
		for _, target := range found {
			if target.Kind != plugin.KindOf() {
				continue
			}
			status, err := plugins.Inspect(target, plugin)
			if err != nil {
				return nil, err
			}
			statuses = append(statuses, status)
		}
	}
	return statuses, nil
}

// targetsOf is every place a host keeps plugins. Each host is one case here.
func targetsOf(host string, vaults []string, global config.Config) ([]plugins.Target, error) {
	switch host {
	case obsidian.Host:
		var targets []plugins.Target
		for _, vault := range vaults {
			found, err := obsidian.Targets(vault)
			if err != nil {
				return nil, err
			}
			targets = append(targets, found...)
		}
		return targets, nil
	}
	return nil, fmt.Errorf("dgs does not know where %s keeps plugins", host)
}

func write(out io.Writer, statuses []plugins.Status) {
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "PLUGIN\tPLACE\tSTATE\tINSTALLED\tCARRIED\tON")
	var notes []string
	for _, status := range statuses {
		installed, carried, on := "-", status.Bundled, "-"
		if status.Installed != nil {
			installed = status.Installed.Version
		}
		if carried == "" {
			carried = "not built in"
			notes = append(notes, status.BundleErr.Error())
		}
		if status.Enabled != nil {
			on = map[bool]string{true: "yes", false: "no"}[*status.Enabled]
		}
		state := string(status.State)
		if status.State == plugins.Modified {
			state += " (" + strings.Join(status.Changed, ", ") + ")"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", status.Plugin.Name(), status.Target.Place, state, installed, carried, on)
	}
	table.Flush()
	for _, note := range unique(notes) {
		fmt.Fprintln(out, "note:", note)
	}
}

func unique(values []string) []string {
	seen := map[string]bool{}
	var kept []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			kept = append(kept, value)
		}
	}
	return kept
}
