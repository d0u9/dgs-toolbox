package doc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	before, _ := tree.LoadTemplates(root)
	if err := tree.Init(root, time.Now()); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote  %s\n", filepath.Join(root, tree.MarkerName))
	if len(before) == 0 {
		fmt.Fprintf(out, "wrote  %s\n", filepath.Join(root, tree.TemplatesDir, "id_card.yaml"))
	}
	fmt.Fprintf(out, "\n%s is a doc tree now. Nothing else in it was touched.\nImport PDFs with `dgs doc`.\n", root)
	return nil
}
