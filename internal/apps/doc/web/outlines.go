package web

import (
	"net/http"

	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// outlineList answers every Outline.
func (s server) outlineList(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"outlines": []outline.Outline{}}
	entries, err := outline.Load(s.root)
	if err != nil {
		out["error"] = err.Error()
	} else {
		out["outlines"] = entries
	}
	writeJSON(w, http.StatusOK, out)
}

// outlineGroup places the PDFs an Outline, saved or not, selects in its
// folders. It writes nothing.
func (s server) outlineGroup(w http.ResponseWriter, r *http.Request) {
	var o outline.Outline
	if !decode(w, r, &o) {
		return
	}
	if err := o.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, err := tree.LoadItems(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	g, err := outline.Group(o, items, view.NamesOf(templates))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s server) outlineSave(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Outline  outline.Outline `json:"outline"`
		Previous string          `json:"previous"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	if err := outline.Save(s.root, request.Previous, request.Outline); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, request.Outline)
}

func (s server) outlineDelete(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	if err := outline.Delete(s.root, request.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}
