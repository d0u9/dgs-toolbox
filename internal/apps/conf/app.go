package conf

import (
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
	"path/filepath"
)

// New returns the Conf app definition: the inspect page, the export action
// and the two reports.
func New() tui.App {
	return tui.App{
		ID:          "conf",
		Name:        "Conf",
		Description: "Configuration generation",
		// One command, so `dgs conf` opens it rather than a picker holding a
		// single entry. Exporting is `dgs conf export` on the command line
		// and x inside the page; see rhumb docs/export.md.
		Direct: true,
		Reports: []tui.Report{{
			Flag:        "check",
			Description: "report every problem in the inventory, the manifests and the secrets store, without opening the TUI",
			Run:         writeCheckReport,
		}, {
			Flag:        "targets",
			Description: "list every target the generator root holds, grouped by node",
			Run:         writeTargetReport,
		}},
		Actions: []tui.Action{{
			ID:          "init",
			Help:        actionHelp["init"],
			Usage:       "[<dir>]",
			Description: "Write the scaffold a generator root starts from, overwriting nothing.",
			MaxArgs:     1,
			Flags: []tui.ActionFlag{
				{Name: "secrets", Usage: "the secrets root to scaffold too (default: conf.secrets)"},
				{Name: "gitignore", Bool: true, Usage: "write a .gitignore into the secrets root as well"},
			},
			RunWithConfig: initAction,
		}, {
			ID:          "export",
			Help:        actionHelp["export"],
			Usage:       "<selector>...",
			Description: "Render the targets a selector matches and write them to a folder, a zip or deploy bundles.",
			MinArgs:     1,
			Flags: []tui.ActionFlag{
				{Name: "to", Usage: "write the bundle into this directory, or - for stdout (default: conf.export.dir)"},
				{Name: "format", Usage: "with --to -, write every file as one yaml document instead of one file's bytes"},
				{Name: "zip", Usage: "write the bundle into this .zip instead of a directory"},
				{Name: "bundle", Bool: true, Usage: "build a deploy bundle per instance with a manifest, as rhumb deploy build does"},
				{Name: "download", Usage: "with --bundle, when a release is fetched: build, into the bundle, or install, on the machine (default: each node's)"},
				{Name: "services", Usage: "with --bundle, directory of deploy service definitions to prefer (default: conf.services)"},
				{Name: "install-root", Usage: "with --bundle, where a Linux host bundle with no deploy.dir is installed, as <root>/<service> (default: conf.install_root, else /srv/rhumb)"},
				{Name: "label-prefix", Usage: "with --bundle, prefix of the systemd unit and launchd label, <prefix>.<node>.<instance> (default: conf.label_prefix, else rhumb)"},
				{Name: "tool", Usage: "with --bundle, tool name for bundle comments and ownership markers (default: conf.tool, else " + config.DefaultConfTool + ")"},
				{Name: "overwrite", Bool: true, Usage: "replace files the destination already holds"},
				{Name: "yes", Shorthand: "y", Bool: true, Usage: "write without asking; everything written is plaintext"},
			},
			RunWithConfig: exportAction,
		}, {
			ID:          "bundle",
			Help:        actionHelp["bundle"],
			Usage:       "<export-dir>",
			Description: "Build the deploy bundle for one exported instance, as rhumb deploy build does.",
			MinArgs:     1,
			MaxArgs:     1,
			Flags: []tui.ActionFlag{
				{Name: "to", Usage: "bundle directory to write"},
				{Name: "platform", Usage: "target os/arch (default: the manifest's, else this machine's)"},
				{Name: "bin", Usage: "bundle this program instead of what the source gives"},
				{Name: "download", Usage: "when a release is fetched: build, into the bundle, or install, on the machine (default: the node's, else install)"},
				{Name: "services", Usage: "directory of deploy service definitions to prefer (default: conf.services)"},
				{Name: "overwrite", Bool: true, Usage: "rebuild a bundle of the same instance built under another label prefix (not a macOS one)"},
				{Name: "install-root", Usage: "where a Linux host bundle with no deploy.dir is installed, as <root>/<service> (default: conf.install_root, else /srv/rhumb)"},
				{Name: "label-prefix", Usage: "prefix of the systemd unit and launchd label, <prefix>.<node>.<instance> (default: conf.label_prefix, else rhumb)"},
				{Name: "tool", Usage: "tool name for bundle comments and ownership markers (default: conf.tool, else " + config.DefaultConfTool + ")"},
			},
			RunWithConfig: bundleAction,
		}, {
			ID:          "bundle-gc",
			Help:        actionHelp["bundle-gc"],
			Description: "List what installed bundles left behind when deleted without ctl uninstall.",
			MaxArgs:     0,
			Flags: []tui.ActionFlag{
				{Name: "yes", Shorthand: "y", Bool: true, Usage: "remove what is listed"},
				{Name: "label-prefix", Usage: "label prefix the bundles were built with (default: conf.label_prefix, else rhumb)"},
				{Name: "tool", Usage: "tool name the bundles were built with (default: conf.tool, else " + config.DefaultConfTool + ")"},
			},
			RunWithConfig: bundleGCAction,
		}, {
			ID:          "secret",
			Help:        actionHelp["secret"],
			Usage:       "sync",
			Description: "Generate the credentials the inventory implies and are not on disk yet.",
			MinArgs:     1,
			MaxArgs:     1,
			Flags: []tui.ActionFlag{
				{Name: "yes", Shorthand: "y", Bool: true, Usage: "generate without asking"},
			},
			RunWithConfig: secretAction,
		}, {
			ID:            "reservations",
			Help:          actionHelp["reservations"],
			Usage:         "<network>",
			Description:   "Print the address reservations a network's router should hold, in address order.",
			MinArgs:       1,
			MaxArgs:       1,
			RunWithConfig: reservationsAction,
		}},
		Commands: []tui.Command{
			{
				ID:          "inspect",
				Help:        inspectHelp,
				Name:        "Inspect",
				Description: "Look up a node, an instance, a user or a secret in the inventory.",
				New: func() tui.CommandModel {
					return newInspectModel("", "")
				},
				NewWithConfig: func(global config.Config) tui.CommandModel {
					m := newInspectModel(global.ConfRoot(), global.ConfSecrets())
					m.exportDir = global.ConfExportDir()
					m.bundleBase = configBundleOptions(global)
					if m.migration != nil {
						m.migration.planDir = filepath.Join(global.AppDir("conf"), "migrations")
						m.migration.outputDir = m.migration.planDir
						m.migration.refreshPlans()
					}
					return m
				},
				Flags: []tui.Flag{
					{
						Name:  "root",
						Usage: "generator root, one subdirectory per service (default: conf.root)",
						Apply: func(global *config.Config, value string) error {
							global.Conf.Root = value
							return nil
						},
					},
					{
						Name:  "secrets",
						Usage: "directory a manifest's secrets file is named relative to (default: conf.secrets)",
						Apply: func(global *config.Config, value string) error {
							global.Conf.Secrets = value
							return nil
						},
					},
					{
						Name:  "services",
						Usage: "directory of deploy service definitions a bundle prefers (default: conf.services)",
						Apply: func(global *config.Config, value string) error {
							global.Conf.Services = value
							return nil
						},
					},
				},
			},
		},
	}
}
