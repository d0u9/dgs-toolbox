package doc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// exportAction exports the Views into the Targets they name: the Targets
// given, or every Target a View names. The whole run is planned and checked
// first; any conflict, missing key or problem means nothing is written.
func exportAction(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	root := global.DocRoot()
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = wd
	}
	views, err := view.Load(root)
	if err != nil {
		return err
	}
	items, err := tree.LoadItems(root)
	if err != nil {
		return err
	}
	jobs, problems := export.Jobs(views, global.Doc.Targets, args)
	plans, err := export.PlanJobs(context.Background(), root, jobs, items)
	if err != nil {
		return err
	}
	ready := len(problems) == 0 && len(plans) > 0
	for _, p := range problems {
		fmt.Fprintf(out, "problem  %s\n", p)
	}
	for _, j := range plans {
		ready = ready && j.Ready()
		describe(out, j)
	}
	switch {
	case len(plans) == 0 && len(problems) == 0:
		return errors.New("no View names a Target: add target: <name> to a View")
	case !ready:
		fmt.Fprintln(out, "Nothing was written.")
		return errors.New("the export has conflicts or missing keys")
	case flags["dry-run"] == "true":
		fmt.Fprintln(out, "Dry run: nothing was written.")
		return nil
	}
	for _, j := range plans {
		result, err := export.Apply(context.Background(), root, j.Path, j.Views, j.Plan, items, nil)
		if err != nil {
			return fmt.Errorf("%s: %w", j.Name, err)
		}
		fmt.Fprintf(out, "%s: %d written, %d removed, %d unchanged.\n", j.Name, result.Written, result.Removed, result.Kept)
	}
	return nil
}

// describe prints one Target's plan: what changes, then what stops it.
func describe(out io.Writer, j export.JobPlan) {
	fmt.Fprintf(out, "%s  %s  (%d View(s): %v)\n", j.Name, j.Path, len(j.Views), j.Views)
	fmt.Fprintf(out, "  %d to add, %d to replace, %d to remove, %d unchanged\n",
		len(j.Plan.Add), len(j.Plan.Replace), len(j.Plan.Remove), len(j.Plan.Keep))
	for _, p := range j.Problems {
		fmt.Fprintf(out, "  problem   %s\n", p)
	}
	names := make([]string, 0, len(j.Combined.Plans))
	for name := range j.Combined.Plans {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := j.Combined.Plans[name]
		for _, m := range p.Missing {
			fmt.Fprintf(out, "  missing   %s: Item %s revision %d lacks %v\n", name, m.Item, m.Revision, m.Keys)
		}
		for _, c := range p.Clashes {
			fmt.Fprintf(out, "  clash     %s: %s wanted by %d PDFs\n", name, c.Path, len(c.Files))
		}
	}
	for _, c := range j.Combined.Clashes {
		var by []string
		for _, f := range c.Files {
			by = append(by, f.View+":"+f.Path)
		}
		fmt.Fprintf(out, "  clash     %s wanted by %v\n", c.Path, by)
	}
	for _, a := range j.Plan.Blocked {
		fmt.Fprintf(out, "  blocked   %s: %s\n", a.Path, a.Reason)
	}
	for _, a := range j.Plan.Left {
		fmt.Fprintf(out, "  left      %s: %s\n", a.Path, a.Reason)
	}
}
