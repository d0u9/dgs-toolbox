package conf

import (
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Conf app definition: the inspect page, its actions and the
// two reports.
func New() tui.App {
	return tui.App{
		ID:          "conf",
		Name:        "Conf",
		Description: "Configuration generation",
		// One command, so `dgs conf` opens it rather than a picker holding a
		// single entry.
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
