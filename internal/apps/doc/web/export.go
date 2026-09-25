package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/target"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

type targetJSON struct {
	target.Target
	// Default is the Target's own folder on this machine, ~ made home.
	Default string `json:"default"`
	// Views are the Views that name this Target.
	Views []string `json:"views"`
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// targetList is the tree's Targets, each with the Views naming it.
func (s server) targetList(w http.ResponseWriter, _ *http.Request) {
	out := struct {
		Targets []targetJSON `json:"targets"`
		Error   string       `json:"error,omitempty"`
	}{Targets: []targetJSON{}}
	targets, err := target.Load(s.root)
	if err != nil {
		out.Error = err.Error()
	}
	views, _ := view.Load(s.root)
	for _, t := range targets {
		j := targetJSON{Target: t, Views: []string{}}
		if t.Folder != "" {
			j.Default = target.Expand(t.Folder, home())
		}
		for _, v := range views {
			if v.Target == t.Name {
				j.Views = append(j.Views, v.Name)
			}
		}
		out.Targets = append(out.Targets, j)
	}
	writeJSON(w, http.StatusOK, out)
}

// targetSave replaces the tree's Targets. A Target a View still names is
// not removed.
func (s server) targetSave(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Targets []target.Target `json:"targets"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	kept := map[string]bool{}
	for _, t := range request.Targets {
		kept[t.Name] = true
	}
	views, _ := view.Load(s.root)
	for _, v := range views {
		if v.Target != "" && !kept[v.Target] {
			if old, _ := target.Load(s.root); containsTarget(old, v.Target) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "the View " + v.Name + " exports to " + v.Target + ": point it elsewhere first"})
				return
			}
		}
	}
	if err := target.Save(s.root, request.Targets); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.targetList(w, r)
}

func containsTarget(targets []target.Target, name string) bool {
	for _, t := range targets {
		if t.Name == name {
			return true
		}
	}
	return false
}

// exportRequest is what to export: the Targets named (every Target a View
// names when All is set), or one View into a folder chosen by hand.
type exportRequest struct {
	Targets []string `json:"targets"`
	// Folders are the folders chosen for Targets this time, by name. A
	// Target without one goes to its own folder.
	Folders map[string]string `json:"folders"`
	All     bool              `json:"all"`
	View    string            `json:"view"`
	Folder  string            `json:"folder"`
}

type exportPlanJSON struct {
	Jobs []export.JobPlan `json:"jobs"`
	// Problems stop the whole run: a View naming an unknown Target, a
	// Target asked for that no View names.
	Problems []string `json:"problems"`
	// Ready is set when every Job may be written, so the run may start.
	Ready bool `json:"ready"`
}

// plan is what the request would write, checked as a whole. It is computed
// afresh for the export itself, so what is written is what the state is
// then, never a plan the page was shown earlier.
func (s server) plan(ctx context.Context, request exportRequest) (exportPlanJSON, []tree.Item, int, error) {
	out := exportPlanJSON{Jobs: []export.JobPlan{}, Problems: []string{}}
	views, err := view.Load(s.root)
	if err != nil {
		return out, nil, http.StatusConflict, err
	}
	var jobs []export.Job
	switch {
	case request.View != "":
		if !filepath.IsAbs(request.Folder) {
			return out, nil, http.StatusBadRequest, errors.New("the folder to export to must be an absolute path")
		}
		var v *view.View
		for i := range views {
			if views[i].Name == request.View {
				v = &views[i]
			}
		}
		if v == nil {
			return out, nil, http.StatusBadRequest, errors.New("no View named " + request.View)
		}
		jobs = []export.Job{{Path: filepath.Clean(request.Folder), Views: []view.View{*v}}}
	case request.All || len(request.Targets) > 0:
		targets, err := target.Load(s.root)
		if err != nil {
			return out, nil, http.StatusConflict, err
		}
		var problems []string
		jobs, problems = export.Jobs(views, target.Folders(targets, request.Folders, home()), request.Targets)
		out.Problems = append(out.Problems, problems...)
	default:
		return out, nil, http.StatusBadRequest, errors.New("name a View and a folder, or Targets")
	}
	items, err := tree.LoadItems(s.root)
	if err != nil {
		return out, nil, http.StatusConflict, err
	}
	if out.Jobs, err = export.PlanJobs(ctx, s.root, jobs, items); err != nil {
		return out, nil, http.StatusConflict, err
	}
	export.AgainstOthers(out.Jobs, Others(s.trees, s.root))
	out.Ready = len(out.Problems) == 0 && len(out.Jobs) > 0
	for _, j := range out.Jobs {
		out.Ready = out.Ready && j.Ready()
	}
	return out, items, http.StatusOK, nil
}

// exportPlan is the dry run: everything an export would do, writing nothing.
func (s server) exportPlan(w http.ResponseWriter, r *http.Request) {
	var request exportRequest
	if !decode(w, r, &request) {
		return
	}
	out, _, status, err := s.plan(r.Context(), request)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type exportResultJSON struct {
	Name   string        `json:"name"`
	Path   string        `json:"path"`
	Result export.Result `json:"result"`
}

// exportRun plans again and writes only when the whole run is ready: one
// Target with a problem means no Target is written.
func (s server) exportRun(w http.ResponseWriter, r *http.Request) {
	var request exportRequest
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	out, items, status, err := s.plan(r.Context(), request)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if !out.Ready {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the export has conflicts or missing keys: nothing was written", "plan": out})
		return
	}
	// The export finishes even if the page goes away: stopping midway only
	// leaves more to do next time.
	results := []exportResultJSON{}
	for _, j := range out.Jobs {
		result, err := export.Apply(context.WithoutCancel(r.Context()), s.root, j.Path, j.Views, j.Plan, items, nil)
		results = append(results, exportResultJSON{Name: j.Name, Path: j.Path, Result: result})
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": j.Path + ": " + err.Error(), "results": results})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// Others is every tree but the one at root.
func Others(trees []Tree, root string) []export.Other {
	var out []export.Other
	for _, t := range trees {
		if filepath.Clean(t.Root) == filepath.Clean(root) {
			continue
		}
		out = append(out, export.Other{Name: t.Name, Root: t.Root})
	}
	return out
}
