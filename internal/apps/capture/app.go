package capture

import (
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the direct Capture application. Scan is the initial session;
// it is modeled as an internal leaf so `dgs capture` needs no subcommand.
func New() tui.App {
	return tui.App{
		ID:          "capture",
		Name:        "Capture",
		Description: "Organize captured data",
		Direct:      true,
		Reports: []tui.Report{{
			Flag:        "recipes",
			Description: "list the recipes this binary offers and what each one needs",
			Run:         writeRecipeReport,
		}, {
			Flag:        "export-recipes",
			Description: "write the recipes this version ships into the recipe directory",
			Run:         writeStarterRecipes,
		}, {
			Flag:        "actions",
			Description: "list the actions a recipe may name, and what each one does",
			Run:         writeActionReport,
		}},
		Commands: []tui.Command{{
			ID:          "scan",
			Name:        "Scan",
			Description: "Open the Capture workspace",
			New: func() tui.CommandModel {
				return newSession()
			},
			NewWithConfig: func(global config.Config) tui.CommandModel {
				root, indexFile := global.CaptureScanSettings()
				return newSessionWithSettings(root, indexFile, loadRecipes(global).Set, obsidianSettings(global))
			},
		}},
	}
}
