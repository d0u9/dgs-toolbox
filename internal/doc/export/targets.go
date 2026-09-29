package export

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/snapshot"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// Job is one folder to export into, with the rules that go there and the
// files its Snapshots fixed.
type Job struct {
	// Name is the Outline's name, or empty for a folder chosen by hand.
	Name  string
	Path  string
	Views []view.View
	// Files are the Snapshots' files, each at its path and marked with its
	// Snapshot, which owns it in the manifest; Snapshots names them, and
	// Lost are their files the tree no longer has.
	Files     []view.File
	Snapshots []string
	Lost      []string
}

// JobPlan is what exporting a Job would do, and what stops it.
type JobPlan struct {
	Name  string   `json:"name"`
	Path  string   `json:"path"`
	Views []string `json:"views"`
	// Combined is the Views planned together: each one's missing keys and
	// clashes, and the clashes between them.
	Combined view.Combined `json:"combined"`
	Plan     Plan          `json:"plan"`
	// Problems are what stops the Job besides missing keys, clashes and
	// blocked files: a Target that overlaps the tree or another Target, or a
	// manifest that cannot be read.
	Problems []string `json:"problems"`
}

// Ready reports whether the Job may be written: nothing missing, no clash,
// no blocked file and no other problem.
func (j JobPlan) Ready() bool {
	return j.Combined.Complete() && len(j.Plan.Blocked) == 0 && len(j.Problems) == 0
}

// Source is an Outline, as Jobs chooses among them.
type Source struct {
	Name      string
	Folder    string
	Rules     []view.View
	Files     []view.File
	Snapshots []string
	Lost      []string
}

// FromOutlines is each Outline as a Source, its Snapshots' files as items
// have them. A Snapshot the tree lacks is one of its Lost.
func FromOutlines(outlines []outline.Outline, snapshots []snapshot.Snapshot, items []tree.Item) []Source {
	out := make([]Source, 0, len(outlines))
	for _, o := range outlines {
		src := Source{Name: o.Name, Folder: o.Folder, Rules: o.Rules}
		for _, m := range o.Snapshots {
			src.Snapshots = append(src.Snapshots, m.Name)
		}
		files, lost, err := outline.Mounted(o, snapshots, items)
		if err != nil {
			src.Lost = append(src.Lost, err.Error())
		}
		src.Files = files
		for _, l := range lost {
			src.Lost = append(src.Lost, fmt.Sprintf("%s (Item %s): %s", l.Path, l.Item, l.Why))
		}
		out = append(out, src)
	}
	return out
}

// Jobs is one Job per Outline named, or, when none is, per one with
// something to export and a folder. Each goes to the folder chosen for it
// this time, else its own folder with ~ made home. A name the tree lacks,
// an Outline with no rule and no Snapshot, or no folder, is a problem.
func Jobs(sources []Source, chosen map[string]string, home string, names []string) ([]Job, []string) {
	var problems []string
	folder := func(o Source) string {
		if f := chosen[o.Name]; f != "" {
			return filepath.Clean(f)
		}
		if o.Folder != "" {
			return filepath.Clean(outline.Expand(o.Folder, home))
		}
		return ""
	}
	empty := func(o Source) bool { return len(o.Rules) == 0 && len(o.Snapshots) == 0 }
	byName := map[string]Source{}
	for _, o := range sources {
		byName[o.Name] = o
	}
	all := len(names) == 0
	if all {
		for _, o := range sources {
			if !empty(o) && folder(o) != "" {
				names = append(names, o.Name)
			}
		}
		sort.Strings(names)
	}
	var jobs []Job
	for _, name := range names {
		o, ok := byName[name]
		switch {
		case !ok:
			problems = append(problems, "no Outline named "+name)
		case empty(o):
			problems = append(problems, "the Outline "+name+" has no rule and no Snapshot")
		case folder(o) == "":
			problems = append(problems, "choose a folder for "+name+": it has none of its own")
		default:
			jobs = append(jobs, Job{Name: name, Path: folder(o), Views: o.Rules, Files: o.Files, Snapshots: o.Snapshots, Lost: o.Lost})
		}
	}
	return jobs, problems
}

// PlanJobs plans every Job against the same Items, so a run of several is
// checked as a whole before anything is written: each Target against the
// tree, the Targets against each other, each Target's Views against each
// other, and every wanted path against what is on disk.
func PlanJobs(ctx context.Context, root string, jobs []Job, items []tree.Item) ([]JobPlan, error) {
	templates, err := tree.LoadTemplates(root)
	if err != nil {
		return nil, err
	}
	names := view.TypesOf(templates)
	out := make([]JobPlan, len(jobs))
	for i, j := range jobs {
		p := JobPlan{Name: j.Name, Path: j.Path, Problems: []string{}}
		for _, v := range j.Views {
			p.Views = append(p.Views, v.Name)
		}
		p.Views = append(p.Views, j.Snapshots...)
		p.Problems = append(p.Problems, j.Lost...)
		if err := CheckTarget(root, j.Path); err != nil {
			p.Problems = append(p.Problems, err.Error())
		}
		for k, other := range jobs {
			if k != i && (within(j.Path, other.Path) || within(other.Path, j.Path)) {
				p.Problems = append(p.Problems, fmt.Sprintf("the folder %s overlaps %s's, %s", j.Path, label(other), other.Path))
			}
		}
		combined, err := view.CombineWith(j.Views, j.Files, items, names)
		if err != nil {
			return nil, err
		}
		p.Combined = combined
		if len(p.Problems) == 0 {
			plan, err := Compute(ctx, j.Path, p.Views, combined.Files)
			if err != nil {
				if ctx.Err() != nil {
					return nil, err
				}
				p.Problems = append(p.Problems, err.Error())
			}
			p.Plan = plan
		} else {
			p.Plan = Plan{Add: []Action{}, Replace: []Action{}, Remove: []Action{}, Keep: []Action{}, Left: []Action{}, Blocked: []Action{}}
		}
		out[i] = p
	}
	return out, nil
}

func label(j Job) string {
	if j.Name != "" {
		return "the Outline " + j.Name
	}
	return filepath.Base(j.Path)
}

// Other is another tree kept on the same machine.
type Other struct {
	Name string
	Root string
}

// AgainstOthers adds to each plan a folder inside another tree, or holding
// one: an export there would write into that tree.
func AgainstOthers(plans []JobPlan, others []Other) {
	for i := range plans {
		p := &plans[i]
		for _, o := range others {
			if within(o.Root, p.Path) || within(p.Path, o.Root) {
				p.Problems = append(p.Problems, fmt.Sprintf("the folder %s overlaps the tree %s, %s", p.Path, o.Name, o.Root))
			}
		}
	}
}
