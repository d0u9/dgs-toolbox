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
					return newModel("")
				},
				NewWithConfig: func(global config.Config) tui.CommandModel {
					return newModel(global.ConfRoot())
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
				},
			},
		},
	}
}
