package doc

import (
	"context"
	"fmt"
	"io"
	"os"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/check"
)

// verifyAction checks a tree against its sidecars and changes nothing. It
// fails when it finds anything, so a script can tell.
func verifyAction(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	var root string
	if len(args) > 0 {
		root = args[0]
	} else {
		chosen, err := global.DocTreeNamed(flags["tree"])
		if err != nil {
			return err
		}
		root = chosen.Root
	}
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = wd
	}
	var progress check.Progress
	if flags["quiet"] != "true" {
		progress = func(path string, done, total int) {
			fmt.Fprintf(out, "reading  %d/%d  %s\n", done+1, total, path)
		}
	}
	report, err := check.Tree(context.Background(), root, progress)
	if err != nil {
		return err
	}
	if progress != nil && report.Checked > 0 {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%d Items, %d PDFs read (%d bytes).\n", report.Items, report.Checked, report.Bytes)
	if len(report.Problems) == 0 {
		fmt.Fprintln(out, "Every PDF is where its sidecar says and still matches its digest.")
		return nil
	}
	fmt.Fprintln(out, "Nothing was changed.")
	for _, p := range report.Problems {
		fmt.Fprintf(out, "  %-12s  %s  %s\n", p.Kind, p.Path, p.Detail)
	}
	return fmt.Errorf("%d problem(s) found", len(report.Problems))
}
