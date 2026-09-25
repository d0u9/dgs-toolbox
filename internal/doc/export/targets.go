package export

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// Job is one Target to export into, with the Views that go there.
type Job struct {
	// Name is the Target's name, or empty for a folder chosen by hand.
	Name  string
	Path  string
	Views []view.View
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

// Jobs groups views by the Target each names. names picks Targets; none
// means every Target a View names. A View naming a Target that is not
// configured, and a Target asked for that no View names, are problems.
func Jobs(views []view.View, targets map[string]string, names []string) ([]Job, []string) {
	var problems []string
	byTarget := map[string][]view.View{}
	for _, v := range views {
		if v.Target == "" {
			continue
		}
		if _, ok := targets[v.Target]; !ok {
			problems = append(problems, fmt.Sprintf("the View %s names the Target %s, which doc.targets does not have", v.Name, v.Target))
			continue
		}
		byTarget[v.Target] = append(byTarget[v.Target], v)
	}
	if len(names) == 0 {
		for name := range byTarget {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	var jobs []Job
	for _, name := range names {
		path, ok := targets[name]
		switch {
		case !ok:
			problems = append(problems, "no Target named "+name+" in doc.targets")
		case len(byTarget[name]) == 0:
			problems = append(problems, "no View names the Target "+name)
		default:
			jobs = append(jobs, Job{Name: name, Path: path, Views: byTarget[name]})
		}
	}
	return jobs, problems
}

// PlanJobs plans every Job against the same Items, so a run of several is
// checked as a whole before anything is written: each Target against the
// tree, the Targets against each other, each Target's Views against each
// other, and every wanted path against what is on disk.
func PlanJobs(ctx context.Context, root string, jobs []Job, items []tree.Item) ([]JobPlan, error) {
	out := make([]JobPlan, len(jobs))
	for i, j := range jobs {
		p := JobPlan{Name: j.Name, Path: j.Path, Problems: []string{}}
		for _, v := range j.Views {
			p.Views = append(p.Views, v.Name)
		}
		if err := CheckTarget(root, j.Path); err != nil {
			p.Problems = append(p.Problems, err.Error())
		}
		for k, other := range jobs {
			if k != i && (within(j.Path, other.Path) || within(other.Path, j.Path)) {
				p.Problems = append(p.Problems, fmt.Sprintf("the folder %s overlaps %s's, %s", j.Path, label(other), other.Path))
			}
		}
		combined, err := view.Combine(j.Views, items)
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
		return "the Target " + j.Name
	}
	return filepath.Base(j.Path)
}
