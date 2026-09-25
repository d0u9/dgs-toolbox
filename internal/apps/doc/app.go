package doc

import (
	"fmt"
	"strconv"

	docweb "dgs-toolbox/internal/apps/doc/web"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Doc app definition: `dgs doc`, which starts the local page,
// and `dgs doc init`.
func New() tui.App {
	return tui.App{
		ID:          "doc",
		Name:        "Doc",
		Description: "Important documents: identity, licences, records",
		Direct:      true,
		Commands:    []tui.Command{command()},
		Actions: []tui.Action{{
			ID:            "init",
			Usage:         "[<dir>]",
			Description:   "Make a folder a doc tree, by writing its marker and an example Template.",
			MaxArgs:       1,
			RunWithConfig: initAction,
		}},
	}
}

func command() tui.Command {
	build := func(global config.Config) tui.CommandModel {
		return NewModel(docweb.SettingsFrom(global))
	}
	return tui.Command{
		ID:            "doc",
		Name:          "Doc",
		Description:   "Start the local page for the document tree.",
		New:           func() tui.CommandModel { return build(config.Default()) },
		NewWithConfig: build,
		Flags: []tui.Flag{{
			Name:  "root",
			Usage: "the tree to open (default: doc.root, else the working directory)",
			Apply: func(global *config.Config, value string) error {
				global.Doc.Root = value
				return nil
			},
		}, {
			Name:  "port",
			Usage: fmt.Sprintf("port for the local page (default %d)", config.DefaultDocWebPort),
			Apply: func(global *config.Config, value string) error {
				port, err := strconv.Atoi(value)
				if err != nil || port < 1 || port > 65535 {
					return fmt.Errorf("%q is not a port between 1 and 65535", value)
				}
				global.Doc.Web.Port = port
				return nil
			},
		}},
	}
}
