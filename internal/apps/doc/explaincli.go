package doc

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/snapshot"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// explainAction prints how each Outline, or those named, places one Item's
// PDFs, or why it does not: every condition asked, the branch taken, and
// what each part of the layout wrote. It changes nothing.
func explainAction(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	named, err := global.DocTreeNamed(flags["tree"])
	if err != nil {
		return err
	}
	root := named.Root
	if root == "" {
		if root, err = os.Getwd(); err != nil {
			return err
		}
	}
	items, err := tree.LoadItems(root)
	if err != nil {
		return err
	}
	it, err := findItem(items, args[0])
	if err != nil {
		return err
	}
	outlines, err := outline.Load(root)
	if err != nil {
		return err
	}
	snapshots, err := snapshot.Load(root)
	if err != nil {
		return err
	}
	templates, err := tree.LoadTemplates(root)
	if err != nil {
		return err
	}
	names := view.TypesOf(templates)
	fmt.Fprintf(out, "%s  %s  %s\n", it.ID, it.Type, it.Fields["name"])
	shown := 0
	for _, o := range outlines {
		if len(args) > 1 && !slices.Contains(args[1:], o.Name) {
			continue
		}
		shown++
		xs, err := outline.Explain(o, snapshots, items, names, it.ID)
		if err != nil {
			return fmt.Errorf("outline %s: %w", o.Name, err)
		}
		fmt.Fprintf(out, "\nOutline %s\n", o.Name)
		if len(xs) == 0 {
			fmt.Fprintln(out, "  no rule or Snapshot")
		}
		for _, x := range xs {
			fmt.Fprintln(out)
			for _, line := range strings.Split(strings.TrimRight(x.Text(), "\n"), "\n") {
				fmt.Fprintf(out, "  %s\n", line)
			}
		}
	}
	if shown == 0 {
		return fmt.Errorf("no Outline named %s", strings.Join(args[1:], ", "))
	}
	return nil
}

// findItem is the Item whose ID is id, or the one whose ID starts with
// it.
func findItem(items []tree.Item, id string) (tree.Item, error) {
	id = strings.ToUpper(strings.TrimSpace(id))
	var found []tree.Item
	for _, it := range items {
		if it.ID == id {
			return it, nil
		}
		if id != "" && strings.HasPrefix(it.ID, id) {
			found = append(found, it)
		}
	}
	switch len(found) {
	case 0:
		return tree.Item{}, fmt.Errorf("no Item %s", id)
	case 1:
		return found[0], nil
	}
	return tree.Item{}, fmt.Errorf("%d Items start with %s: give more of the ID", len(found), id)
}
