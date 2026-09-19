package conf

import (
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
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
		// and x inside the page; see docs/apps/conf/export.md.
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
			Usage:       "<selector>...",
			Description: "Render the targets a selector matches and write them to a folder or a zip.",
			MinArgs:     1,
			Flags: []tui.ActionFlag{
				{Name: "to", Usage: "write the bundle into this directory, or - for stdout (default: conf.export.dir)"},
				{Name: "format", Usage: "with --to -, write every file as one yaml document instead of one file's bytes"},
				{Name: "zip", Usage: "write the bundle into this .zip instead of a directory"},
				{Name: "overwrite", Bool: true, Usage: "replace files the destination already holds"},
				{Name: "yes", Shorthand: "y", Bool: true, Usage: "write without asking; everything written is plaintext"},
			},
			RunWithConfig: exportAction,
		}},
		Commands: []tui.Command{
			{
				ID:          "inspect",
				Name:        "Inspect",
				Description: "Look up a node, an instance, a user or a secret in the inventory.",
				New: func() tui.CommandModel {
					return newInspectModel("", "")
				},
				NewWithConfig: func(global config.Config) tui.CommandModel {
					return newInspectModel(global.ConfRoot(), global.ConfSecrets())
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
				},
			},
		},
	}
}
