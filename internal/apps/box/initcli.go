package box

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/maintain"
	"dgs-toolbox/internal/config"
)

// initAction makes a folder a Box.
//
// It is a CLI action rather than a command with a workspace because writing the
// marker is the whole of it: there is nothing to watch, nothing to choose and
// nothing to correct once it starts. A TUI around it would show an empty
// workspace, and would put a keystroke between a script and the only Box it can
// create — every other box command refuses to run until this one has.
func initAction(_ io.Reader, out io.Writer, args []string, _ map[string]string, global config.Config) error {
	root := global.BoxRoot()
	if len(args) > 0 {
		root = args[0]
	}
	if root == "" {
		return fmt.Errorf("no directory given and box.root is not configured")
	}

	marker := global.BoxMarker()
	if err := maintain.Init(root, marker, time.Now()); err != nil {
		return err
	}

	fmt.Fprintf(out, "wrote  %s\n", filepath.Join(root, marker))
	fmt.Fprintf(out, "wrote  %s\n", boxlog.PathFor(root))
	fmt.Fprintf(out, "\n%s is a Box now. Nothing else in it was touched.\nTake scans in with `dgs box`.\n", root)
	return nil
}
