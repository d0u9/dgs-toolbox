package photo

import (
	"dgs-toolbox/internal/apps/photo/postprocess"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Photo app definition. It contains no Bubble Tea program of
// its own, so the same app can be embedded in the toolbox or launched alone.
func New() tui.App {
	return tui.App{
		ID:          "photo",
		Help:        appHelp,
		Name:        "Photo",
		Description: "Photo tools",
		Commands: []tui.Command{
			{
				ID:          "import",
				Help:        importHelp,
				Name:        "Import",
				Description: "Import photos with independently verified publication.",
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
				Description: "Encode photos as JPEG with verified publication.",
				Help:        encodeHelp,
				New: func() tui.CommandModel {
					return newEncodeModel(config.Default().Photo.Encode)
				},
				NewWithConfig: func(global config.Config) tui.CommandModel { return newEncodeModel(global.Photo.Encode) },
			},
		},
		Actions: []tui.Action{
			{
				ID:          "organize",
				Help:        actionHelp["organize"],
				Usage:       "<folder>",
				Description: "Move photos in a folder into capture-date folders (YYYYMMDD by default).",
				Args:        1,
				Flags: []tui.ActionFlag{
					{Name: "yes", Shorthand: "y", Bool: true, Usage: "move into date folders that already exist without asking"},
					{Name: "dry-run", Shorthand: "n", Bool: true, Usage: "show where each photo would go without moving anything"},
					{Name: "recursive", Shorthand: "r", Bool: true, Usage: "also organize photos in subfolders (hidden folders are skipped)"},
					{Name: "format", Default: postprocess.DefaultLayout, Usage: "date folder format from YYYY, MM, DD, - _ . and / (e.g. YYYY/MM/DD)"},
					{Name: "dest", Usage: "create date folders under this directory instead of <folder>"},
				},
				Run: organizeFolder,
			},
		},
	}
}
