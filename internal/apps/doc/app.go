package doc

import (
	"fmt"
	"strconv"

	docweb "dgs-toolbox/internal/apps/doc/web"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Doc app definition: `dgs doc`, which starts the local page,
// `dgs doc init`, `dgs doc verify`, `dgs doc export`, `dgs doc link` and
// `dgs doc explain`.
func New() tui.App {
	return tui.App{
		ID:          "doc",
		Name:        "Doc",
		Description: "Important documents: identity, licences, records",
		Direct:      true,
		Commands:    []tui.Command{command()},
		Actions: []tui.Action{{
			ID:            "init",
			Help:          actionHelp["init"],
			Usage:         "[<dir>]",
			Description:   "Make a folder a doc tree, by writing its marker and an example Template.",
			MaxArgs:       1,
			RunWithConfig: initAction,
		}, {
			ID:          "verify",
			Help:        actionHelp["verify"],
			Usage:       "[<dir>]",
			Description: "Check every PDF against its sidecar and digest. Changes nothing.",
			MaxArgs:     1,
			Flags: []tui.ActionFlag{
				{Name: "tree", Shorthand: "t", Usage: "the tree in doc.trees to use, when there are several"},
				{Name: "quiet", Shorthand: "q", Bool: true, Usage: "report only the result, not every file as it is read"},
			},
			RunWithConfig: verifyAction,
		}, {
			ID:          "export",
			Help:        actionHelp["export"],
			Usage:       "[<outline>...]",
			Description: "Export Outlines into their folders: those given, or every one with a folder. Checks everything first; a conflict writes nothing.",
			MaxArgs:     64,
			Flags: []tui.ActionFlag{
				{Name: "tree", Shorthand: "t", Usage: "the tree in doc.trees to use, when there are several"},
				{Name: "to", Usage: "a folder for an Outline this time, instead of its own: <outline>=<folder>, comma separated"},
				{Name: "dry-run", Shorthand: "n", Bool: true, Usage: "plan and check, and write nothing"},
			},
			RunWithConfig: exportAction,
		}, {
			ID:          "link",
			Help:        actionHelp["link"],
			Usage:       "[<dir>]",
			Description: "Fill each empty link field whose within finds exactly one Item, such as a bill's tenancy by its date. Lists them; writes only with --apply.",
			MaxArgs:     1,
			Flags: []tui.ActionFlag{
				{Name: "tree", Shorthand: "t", Usage: "the tree in doc.trees to use, when there are several"},
				{Name: "apply", Bool: true, Usage: "write the links listed"},
			},
			RunWithConfig: linkAction,
		}, {
			ID:          "explain",
			Help:        actionHelp["explain"],
			Usage:       "<item-id> [<outline>...]",
			Description: "Show how each Outline, or those given, places an Item's PDFs, or why it does not: each condition, branch and key. Changes nothing.",
			MinArgs:     1,
			MaxArgs:     64,
			Flags: []tui.ActionFlag{
				{Name: "tree", Shorthand: "t", Usage: "the tree in doc.trees to use, when there are several"},
			},
			RunWithConfig: explainAction,
		}},
	}
}

func command() tui.Command {
	build := func(global config.Config) tui.CommandModel {
		return NewModel(docweb.SettingsFrom(global))
	}
	return tui.Command{
		ID:            "doc",
		Help:          docHelp,
		Name:          "Doc",
		Description:   "Start the local page for the document tree.",
		New:           func() tui.CommandModel { return build(config.Default()) },
		NewWithConfig: build,
		Flags: []tui.Flag{{
			Name:  "root",
			Usage: "the tree to open (default: the trees in doc/config.json, else the working directory)",
			Apply: func(global *config.Config, value string) error {
				global.Doc.Trees = map[string]string{"": value}
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
				global.Doc.Web = config.SetWeb(global.Doc.Web, config.DocPagesServer, func(w *config.Web) { w.Port = port })
				return nil
			},
		}},
	}
}
