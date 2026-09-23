package box

import (
	"fmt"
	"strconv"

	boxweb "dgs-toolbox/internal/apps/box/web"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Box app definition: two commands and four actions. Import
// and view are the interactive pair — a workspace is what they are for — and
// they are separate because their risk differs. Import copies, verifies and
// publishes bytes; view writes sidecars and never a scan. One entry point for
// both would force the careful ceremony onto the part meant to be used
// casually. The four actions write nothing into the Box and ask nothing once
// they start, so they report to stdout instead of opening a workspace.
func New() tui.App {
	return tui.App{
		ID:          "box",
		Name:        "Box",
		Description: "Scanned paper: take it in, look through it",
		Commands: []tui.Command{
			command("import", "Import", "Take new scans from the inbox into the Box.", Intake),
			command("view", "View", "Look through the Box and correct what is in it.", Browse),
		},
		Actions: []tui.Action{{
			ID:            "init",
			Usage:         "[<dir>]",
			Description:   "Make a folder a Box, by writing its marker.",
			MaxArgs:       1,
			RunWithConfig: initAction,
		}, {
			ID:            "index",
			Usage:         "[<dir>]",
			Description:   "Rebuild the discardable local cache from the sidecars.",
			MaxArgs:       1,
			RunWithConfig: indexAction,
		}, {
			ID:          "verify",
			Usage:       "[<dir>]",
			Description: "Read every byte and check it against its recorded digest.",
			MaxArgs:     1,
			Flags: []tui.ActionFlag{
				{Name: "quiet", Shorthand: "q", Bool: true, Usage: "report only the result, not every file as it is read"},
			},
			RunWithConfig: verifyAction,
		}, {
			ID:            "dedupe",
			Usage:         "[<dir>]",
			Description:   "Group the scans that are the same piece of paper.",
			MaxArgs:       1,
			RunWithConfig: dedupeAction,
		}},
	}
}

// command builds one box command.
func command(id, name, description string, work Page) tui.Command {
	build := func(global config.Config) tui.CommandModel {
		return NewModel(boxweb.SettingsFrom(global), work)
	}
	return tui.Command{
		ID:            id,
		Name:          name,
		Description:   description,
		New:           func() tui.CommandModel { return build(config.Default()) },
		NewWithConfig: build,
		Flags:         flags(work),
	}
}

func flags(work Page) []tui.Flag {
	all := []tui.Flag{
		{
			Name:  "root",
			Usage: "the Box to work in (default: box.root)",
			Apply: func(global *config.Config, value string) error {
				global.Box.Root = value
				return nil
			},
		},
		{
			Name:  "port",
			Usage: fmt.Sprintf("port for the local pages (default %d)", config.DefaultBoxWebPort),
			Apply: func(global *config.Config, value string) error {
				port, err := strconv.Atoi(value)
				if err != nil || port < 1 || port > 65535 {
					return fmt.Errorf("%q is not a port between 1 and 65535", value)
				}
				global.Box.Web.Port = port
				return nil
			},
		},
	}
	if work != Intake {
		return all
	}
	// Only import reads an inbox, and a second source is given here rather
	// than written into the configuration file.
	return append(all, tui.Flag{
		Name:  "inbox",
		Usage: "folder to take new scans from (default: box.inbox)",
		Apply: func(global *config.Config, value string) error {
			global.Box.Inbox = value
			return nil
		},
	})
}
