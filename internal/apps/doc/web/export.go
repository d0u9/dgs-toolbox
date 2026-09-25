package web

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sort"

	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

type targetJSON struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Views are the Views that name this Target.
	Views []string `json:"views"`
	// Held are the Views whose files the Target's manifest records.
	Held  []string `json:"held"`
	Error string   `json:"error,omitempty"`
}

func (s server) targetList(w http.ResponseWriter, _ *http.Request) {
	views, _ := view.Load(s.root)
	out := []targetJSON{}
	for name, path := range s.targets {
		t := targetJSON{Name: name, Path: path, Views: []string{}, Held: []string{}}
		for _, v := range views {
			if v.Target == name {
				t.Views = append(t.Views, v.Name)
			}
		}
		if err := export.CheckTarget(s.root, path); err != nil {
			t.Error = err.Error()
		} else if m, err := export.ReadManifest(path); err != nil {
			t.Error = err.Error()
		} else {
			t.Held = append(t.Held, m.Views...)
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

// exportRequest is what to export: the Targets named (every Target a View
// names when All is set), or one View into a folder chosen by hand.
type exportRequest struct {
	Targets []string `json:"targets"`
	All     bool     `json:"all"`
	View    string   `json:"view"`
	Folder  string   `json:"folder"`
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
		var problems []string
		jobs, problems = export.Jobs(views, s.targets, request.Targets)
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

// Others is every tree but the one at root, with the Targets its Views
// name. A tree whose Views cannot be read names none.
func Others(trees []Tree, root string) []export.Other {
	var out []export.Other
	for _, t := range trees {
		if filepath.Clean(t.Root) == filepath.Clean(root) {
			continue
		}
		o := export.Other{Name: t.Name, Root: t.Root}
		views, _ := view.Load(t.Root)
		for _, v := range views {
			if v.Target != "" {
				o.Targets = append(o.Targets, v.Target)
			}
		}
		out = append(out, o)
	}
	return out
}
