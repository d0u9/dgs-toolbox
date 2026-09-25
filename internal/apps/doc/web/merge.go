package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/merge"
	"dgs-toolbox/internal/doc/tree"
)

type mergeRequest struct {
	Dir     string            `json:"dir"`
	Choices map[string]string `json:"choices"`
}

type mergePlanJSON struct {
	merge.Plan
	Dir string `json:"dir"`
	// From is what Dir is: "tree" (a sub-tree) or "target" (an export).
	From string `json:"from"`
}

// source reads Dir as a tree when it has the marker, else as a Target when
// it has an export manifest.
func (s server) mergeSource(dir string) (merge.Source, string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return merge.Source{}, "", err
	}
	root, _ := filepath.Abs(s.root)
	if abs == root {
		return merge.Source{}, "", errors.New("that is this tree")
	}
	if _, err := os.Stat(filepath.Join(abs, tree.MarkerName)); err == nil {
		src, err := merge.FromTree(abs)
		return src, "tree", err
	}
	if _, err := os.Stat(filepath.Join(abs, export.ManifestName)); err == nil {
		src, err := merge.FromTarget(abs, s.now())
		return src, "target", err
	}
	return merge.Source{}, "", errors.New(abs + " is neither a doc tree nor an export Target")
}

func (s server) mergePlan(w http.ResponseWriter, r *http.Request) {
	var request mergeRequest
	if !decode(w, r, &request) {
		return
	}
	src, from, err := s.mergeSource(request.Dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	plan, err := merge.Compute(s.root, src)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, mergePlanJSON{Plan: plan, Dir: request.Dir, From: from})
}

// mergeRun plans again from what both sides hold now, and applies that with
// the choices made; a conflict the page did not show has no choice, and the
// merge is refused.
func (s server) mergeRun(w http.ResponseWriter, r *http.Request) {
	var request mergeRequest
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	src, _, err := s.mergeSource(request.Dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	plan, err := merge.Compute(s.root, src)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	result, err := merge.Apply(context.WithoutCancel(r.Context()), s.root, src, plan, request.Choices)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "result": result})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
