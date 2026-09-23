package box

import (
	"fmt"
	"strconv"

	boxweb "dgs-toolbox/internal/apps/box/web"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Box app definition: `dgs box` itself and four actions. The
// command starts the local pages; intake, browse and the exceptions all live
// there and link to each other, so the terminal has nothing to choose between.
// The four actions write nothing into the Box and ask nothing once they start,
// so they report to stdout instead of opening a workspace.
func New() tui.App {
	return tui.App{
		ID:          "box",
		Name:        "Box",
		Description: "Scanned paper: take it in, look through it",
		Direct:      true,
		Commands:    []tui.Command{command()},
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

// command builds the one box command.
func command() tui.Command {
	build := func(global config.Config) tui.CommandModel {
		return NewModel(boxweb.SettingsFrom(global))
	}
	return tui.Command{
		ID:            "box",
		Name:          "Box",
		Description:   "Start the local pages for the Box.",
		New:           func() tui.CommandModel { return build(config.Default()) },
		NewWithConfig: build,
		Flags:         flags(),
	}
}

func flags() []tui.Flag {
	return []tui.Flag{
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
		{
			// A second source is given here rather than written into the
			// configuration file.
			Name:  "inbox",
			Usage: "folder to take new scans from (default: box.inbox)",
			Apply: func(global *config.Config, value string) error {
				global.Box.Inbox = value
				return nil
			},
		},
	}
}
