package conf

import (
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Conf app definition: dgs conf export.
func New() tui.App {
	return tui.App{
		ID:          "conf",
		Name:        "Conf",
		Description: "Configuration generation",
		Commands: []tui.Command{
			{
				ID:          "export",
				Name:        "Export",
				Description: "Render service configuration from templates and export it.",
				New: func() tui.CommandModel {
					return newModel("", "")
				},
				NewWithConfig: func(global config.Config) tui.CommandModel {
					return newModel(global.ConfRoot(), global.ConfSecrets())
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
