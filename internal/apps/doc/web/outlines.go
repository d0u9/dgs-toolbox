package web

import (
	"net/http"

	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// outlineList answers every Outline as its file is written.
func (s server) outlineList(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"outlines": []outline.Entry{}, "example": outline.Example}
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
	var request struct {
		Data string `json:"data"`
	}
	if !decode(w, r, &request) {
		return
	}
	o, err := outline.Parse([]byte(request.Data))
	if err != nil {
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
		Previous string `json:"previous"`
		Data     string `json:"data"`
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
	o, err := outline.Save(s.root, request.Previous, []byte(request.Data))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, o)
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
