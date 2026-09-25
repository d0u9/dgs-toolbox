package web

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

type targetJSON struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// View is the View last exported there, from the Target's manifest.
	View  string `json:"view,omitempty"`
	Error string `json:"error,omitempty"`
}

func (s server) targetList(w http.ResponseWriter, _ *http.Request) {
	out := []targetJSON{}
	for name, path := range s.targets {
		t := targetJSON{Name: name, Path: path}
		if err := export.CheckTarget(s.root, path); err != nil {
			t.Error = err.Error()
		} else if m, err := export.ReadManifest(path); err != nil {
			t.Error = err.Error()
		} else {
			t.View = m.View
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

type exportRequest struct {
	View   string `json:"view"`
	Target string `json:"target"`
}

type exportPlanJSON struct {
	export.Plan
	View   view.Plan `json:"view"`
	Target string    `json:"target"`
}

// plan is what writing a saved View to a named Target would do. It is
// computed afresh for the export itself, so what is written is what the
// state is then, never a plan the page was shown earlier.
func (s server) plan(ctx context.Context, request exportRequest) (exportPlanJSON, []tree.Item, int, error) {
	out := exportPlanJSON{}
	path, ok := s.targets[request.Target]
	if !ok {
		return out, nil, http.StatusBadRequest, errors.New("no Target named " + request.Target + " in doc.targets")
	}
	out.Target = path
	if err := export.CheckTarget(s.root, path); err != nil {
		return out, nil, http.StatusConflict, err
	}
	views, err := view.Load(s.root)
	if err != nil {
		return out, nil, http.StatusConflict, err
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
	items, err := tree.LoadItems(s.root)
	if err != nil {
		return out, nil, http.StatusConflict, err
	}
	if out.View, err = view.Build(*v, items); err != nil {
		return out, nil, http.StatusBadRequest, err
	}
	if out.Plan, err = export.Compute(ctx, path, out.View.Files); err != nil {
		return out, nil, http.StatusConflict, err
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
	if !out.View.Complete() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "the View is not complete: fill in missing keys and resolve clashes first"})
		return
	}
	// The export finishes even if the page goes away: stopping midway only
	// leaves more to do next time.
	result, err := export.Apply(context.WithoutCancel(r.Context()), s.root, out.Target, request.View, out.Plan, items, nil)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "result": result})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
