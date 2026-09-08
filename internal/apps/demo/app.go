package demo

import "dgs-toolbox/internal/tui"

// New returns the root-level interactive component gallery.
func New() tui.App {
	return tui.App{
		ID:          "demo",
		Name:        "Component Demo",
		Description: "Interactive gallery of reusable TUI components",
		Direct:      true,
		Commands: []tui.Command{{
			ID:          "demo",
			Name:        "Component Demo",
			Description: "Explore reusable TUI components",
			New: func() tui.CommandModel {
				return newModel()
			},
		}},
	}
}
