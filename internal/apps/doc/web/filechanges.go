package web

import (
	"dgs-toolbox/internal/doc/check"
	"net/http"
)

// checkFiles is an explicit page action; the CLI check remains read-only.
func (s server) checkFiles(w http.ResponseWriter, r *http.Request) {
	s.writing.Lock()
	defer s.writing.Unlock()

	report, err := check.Tree(r.Context(), s.documents, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// Keep write failures in the report: prior successful sidecar writes stand
	// and the normal POST middleware must refresh their cache too.
	var problems []check.Problem
	var recorded []string
	for _, p := range report.Problems {
		if p.Kind == check.Changed && !p.Locked && !p.Original && p.Item != "" {
			_, err := s.documents.RecordFileChange(r.Context(), p.Item, p.Digest, p.Path, p.Actual, s.now())
			if err == nil {
				recorded = append(recorded, p.Item)
				continue
			}
			p.Detail = err.Error()
		}
		problems = append(problems, p)
	}
	report.Problems = problems
	writeJSON(w, http.StatusOK, struct {
		Report   check.Report `json:"report"`
		Recorded []string     `json:"recorded"`
	}{report, recorded})
}

func (s server) confirmFilePath(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item     string `json:"item"`
		Digest   string `json:"digest"`
		Previous string `json:"previous"`
		Path     string `json:"path"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := s.documents.ConfirmFilePath(r.Context(), request.Item, request.Digest, request.Previous, request.Path, s.now())
	answer(w, item, err)
}
