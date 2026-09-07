package gpx

import (
	"dgs-toolbox/internal/apps/placeholder"
	"dgs-toolbox/internal/tui"
)

// New returns the GPX app definition. It is a component consumed by the shell
// and never starts a nested Bubble Tea program.
func New() tui.App {
	return tui.App{
		ID:          "gpx",
		Name:        "GPX",
		Description: "GPX tools",
		Commands: []tui.Command{
			{
				ID:          "import",
				Name:        "Import",
				Description: "GPX import placeholder. No files are changed.",
				New: func() tui.CommandModel {
					return placeholder.New("GPX Import")
				},
			},
			{
				ID:          "inspect",
				Name:        "Inspect",
				Description: "GPX inspect placeholder. No files are changed.",
				New: func() tui.CommandModel {
					return placeholder.New("GPX Inspect")
				},
			},
		},
	}
}
