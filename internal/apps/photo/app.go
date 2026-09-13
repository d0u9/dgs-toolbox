package photo

import (
	"dgs-toolbox/internal/apps/placeholder"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Photo app definition. It contains no Bubble Tea program of
// its own, so the same app can be embedded in the toolbox or launched alone.
func New() tui.App {
	return tui.App{
		ID:          "photo",
		Name:        "Photo",
		Description: "Photo tools",
		Commands: []tui.Command{
			{
				ID:          "import",
				Name:        "Import",
				Description: "Photo import placeholder. No files are changed.",
				New: func() tui.CommandModel {
					return newImportModel()
				},
				NewWithConfig: func(global config.Config) tui.CommandModel {
					source, destination := global.PhotoImportPaths()
					return newImportModelWithSettings(global.PhotoImportStateFile(), source, destination)
				},
			},
			{
				ID:          "encode",
				Name:        "Encode",
				Description: "Photo encode placeholder. No files are changed.",
				New: func() tui.CommandModel {
					return placeholder.New("Photo Encode")
				},
			},
		},
		Actions: []tui.Action{
			{
				ID:          "organize",
				Usage:       "<folder>",
				Description: "Move photos in a folder (not subfolders) into YYYYMMDD capture-date folders.",
				Args:        1,
				Run:         organizeFolder,
			},
		},
	}
}
