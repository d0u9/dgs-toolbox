package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/tree"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// exportRequest is what to export: the Outlines named, or every Outline
// with rules and a folder when All is set.
type exportRequest struct {
	Outlines []string `json:"outlines"`
	// Folders are the folders chosen for Outlines this time, by name. An
	// Outline without one goes to its own folder.
	Folders map[string]string `json:"folders"`
	All     bool              `json:"all"`
}

type exportPlanJSON struct {
	Jobs []export.JobPlan `json:"jobs"`
	// Problems stop the whole run: an Outline asked for that the tree
	// lacks, or that has no rule or no folder.
	Problems []string `json:"problems"`
	// Ready is set when every Job may be written, so the run may start.
	Ready bool `json:"ready"`
}

// plan is what the request would write, checked as a whole. It is computed
// afresh for the export itself, so what is written is what the state is
// then, never a plan the page was shown earlier.
func (s server) plan(ctx context.Context, request exportRequest) (exportPlanJSON, []tree.Item, int, error) {
	out := exportPlanJSON{Jobs: []export.JobPlan{}, Problems: []string{}}
	if !request.All && len(request.Outlines) == 0 {
		return out, nil, http.StatusBadRequest, errors.New("name the Outlines to export")
	}
	outlines, err := outline.Load(s.root)
	if err != nil {
		return out, nil, http.StatusConflict, err
	}
	names := request.Outlines
	if request.All {
		names = nil
	}
	jobs, problems := export.Jobs(outlines, request.Folders, home(), names)
	out.Problems = append(out.Problems, problems...)
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
	// HistoryError says the files were written but the Items' history of
	// them was not.
	HistoryError string `json:"history_error,omitempty"`
}

// exportRun plans again and writes only when the whole run is ready: one
// Outline with a problem means no Outline is written.
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
		result, err := s.applyExport(r, j, items)
		results = append(results, result)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": j.Path + ": " + err.Error(), "results": results})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// applyExport runs one job and records in each Item's history the PDFs it
// published, also when the job stops early. The files are in place whether or
// not the history can be written, so a failure to record is reported in the
// result, not as a failed export.
func (s server) applyExport(r *http.Request, j export.JobPlan, items []tree.Item) (exportResultJSON, error) {
	var published []tree.Exported
	result, err := export.Apply(context.WithoutCancel(r.Context()), s.root, j.Path, j.Views, j.Plan, items, nil,
		func(a export.Action) {
			published = append(published, tree.Exported{Item: a.Item, Digest: a.Digest, Target: j.Path, View: a.View})
		})
	out := exportResultJSON{Name: j.Name, Path: j.Path, Result: result}
	if recordErr := tree.RecordExports(s.root, published, s.now()); recordErr != nil {
		out.HistoryError = recordErr.Error()
	}
	return out, err
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
