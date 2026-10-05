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
					source, destinations := global.PhotoImportPaths()
					return newImportModelWithSettings(global.PhotoImportStateFile(), source, destinations...)
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
				ID: "import", Help: importHelp, Usage: "[<source>]", Args: 1,
				Description: "Import originals into independently verified destinations.",
				Flags: []tui.ActionFlag{
					{Name: "dest", Repeatable: true, Usage: "destination directory; repeat for each copy"},
					{Name: "move", Bool: true, Usage: "delete each source only after every destination verifies it"},
					{Name: "incremental", Bool: true, Default: "true", Usage: "find existing copies by size and SHA-256 in each destination"},
					{Name: "duplicates", Default: "skip", Usage: "name conflict policy: skip or keep-both"},
					{Name: "workers", Default: "1", Usage: "whole-file workers per destination (1–8)"},
					{Name: "ext", Repeatable: true, Usage: "include this extension (e.g. JPG); repeat to select several; default all regular files"},
					{Name: "organize", Bool: true, Usage: "organize imported JPG/RAW into YYYYMMDD folders"},
					{Name: "dry-run", Shorthand: "n", Bool: true, Usage: "preview selected files and destination inventories without writing or deleting"},
					{Name: "delete-state", Bool: true, Usage: "delete destination state after full success; retain unfinished batches"},
				},
				RunWithConfig: importDirectories,
			},
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
