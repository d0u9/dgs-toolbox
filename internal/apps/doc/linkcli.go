package doc

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/anchor"
	"dgs-toolbox/internal/doc/tree"
)

// linkAction fills empty link fields whose within finds exactly one Item:
// a bill's tenancy, by the bill's date. It lists what it would write, and
// writes only with --apply. Where several Items fit it lists them and
// writes nothing.
func linkAction(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
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
	templates, err := tree.LoadTemplates(root)
	if err != nil {
		return err
	}
	items, err := tree.LoadItems(root)
	if err != nil {
		return err
	}
	byID := map[string]tree.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	name := func(id string) string {
		it := byID[id]
		fields := it.FieldsAt(it.Current())
		var parts []string
		for _, k := range []string{"name", "address", "provider", "issuer", "subject", "service", "period", "date", "start", "end"} {
			if v := fields[k]; v != "" {
				parts = append(parts, v)
			}
		}
		return it.Type + " " + id + " (" + strings.Join(parts, ", ") + ")"
	}
	proposals, unsure := anchor.Proposals(templates, items)
	for _, p := range proposals {
		fmt.Fprintf(out, "link      %s\n          %s: %s\n", name(p.Item), p.Key, name(strings.Split(p.Link, "@")[0]))
	}
	for _, u := range unsure {
		fitting := make([]string, len(u.Fitting))
		for i, id := range u.Fitting {
			fitting[i] = name(id)
		}
		fmt.Fprintf(out, "choose    %s\n          %s: one of %s\n", name(u.Item), u.Key, strings.Join(fitting, "; "))
	}
	if len(proposals) == 0 {
		fmt.Fprintf(out, "No link to fill: %d left to choose on the page.\n", len(unsure))
		return nil
	}
	if flags["apply"] != "true" {
		fmt.Fprintf(out, "%d links to write, %d to choose on the page. Nothing was changed: run again with --apply to write them.\n", len(proposals), len(unsure))
		return nil
	}
	byType := map[string]tree.Template{}
	for _, t := range templates {
		byType[t.Type] = t
	}
	for i, p := range proposals {
		it := byID[p.Item]
		fields := it.FieldsAt(it.Current())
		fields[p.Key] = p.Link
		if _, err := tree.SetFields(root, it.ID, it.Current(), byType[it.Type], fields, time.Now()); err != nil {
			return fmt.Errorf("%s: %w (%d of %d written)", it.ID, err, i, len(proposals))
		}
	}
	fmt.Fprintf(out, "%d links written, %d to choose on the page.\n", len(proposals), len(unsure))
	return nil
}
