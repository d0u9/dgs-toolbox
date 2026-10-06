package doc

import (
	"fmt"
	"io"
	"os"

	"time"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/tree"
)

// initAction makes a folder a doc tree: the marker, and the example Template
// when there is none. Nothing else in the folder is touched.
func initAction(_ io.Reader, out io.Writer, args []string, _ map[string]string, global config.Config) error {
	root := global.DocRoot()
	if len(args) > 0 {
		root = args[0]
	}
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = wd
	}
	if err := tree.Init(root, time.Now(), func(path string) { fmt.Fprintf(out, "wrote  %s\n", path) }); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%s is a doc tree now. Nothing else in it was touched.\nImport PDFs with `dgs doc`.\n", root)
	return nil
}
