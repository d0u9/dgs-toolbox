package doc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	docweb "dgs-toolbox/internal/apps/doc/web"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/target"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// exportAction exports the Views into the Targets they name: the Targets
// given, or every Target a View names. The whole run is planned and checked
// first; any conflict, missing key or problem means nothing is written.
func exportAction(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	named, err := global.DocTreeNamed(flags["tree"])
	if err != nil {
		return err
	}
	root := named.Root
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
	targets, err := target.Load(root)
	if err != nil {
		return err
	}
	chosen, err := parseTo(flags["to"])
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	jobs, problems := export.Jobs(views, target.Folders(targets, chosen, home), args)
	plans, err := export.PlanJobs(context.Background(), root, jobs, items)
	if err != nil {
		return err
	}
	export.AgainstOthers(plans, docweb.Others(docweb.SettingsFrom(global).Trees, root))
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
		return errors.New("no View names a Target: add target: <name> to a View, and the Target to targets.yaml")
	case !ready:
		fmt.Fprintln(out, "Nothing was written.")
		return errors.New("the export has conflicts or missing keys")
	case flags["dry-run"] == "true":
		fmt.Fprintln(out, "Dry run: nothing was written.")
		return nil
	}
	for _, j := range plans {
		var published []tree.Exported
		result, err := export.Apply(context.Background(), root, j.Path, j.Views, j.Plan, items, nil,
			func(a export.Action) {
				published = append(published, tree.Exported{Item: a.Item, Digest: a.Digest, Target: j.Path, View: a.View})
			})
		if recordErr := tree.RecordExports(root, published, time.Now()); recordErr != nil {
			fmt.Fprintf(out, "%s: the files are written, but their history was not recorded: %v\n", j.Name, recordErr)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", j.Name, err)
		}
		fmt.Fprintf(out, "%s: %d written, %d removed, %d unchanged.\n", j.Name, result.Written, result.Removed, result.Kept)
	}
	return nil
}

// parseTo reads --to: name=folder pairs, comma separated, each an absolute
// folder for that Target this time.
func parseTo(value string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(value) == "" {
		return out, nil
	}
	for _, pair := range strings.Split(value, ",") {
		name, folder, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || name == "" || !filepath.IsAbs(folder) {
			return nil, fmt.Errorf("--to %q: write <target>=<absolute folder>, comma separated", pair)
		}
		out[name] = folder
	}
	return out, nil
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
