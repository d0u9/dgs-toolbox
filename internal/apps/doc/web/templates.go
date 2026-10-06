package web

import (
	"net/http"

	"dgs-toolbox/internal/doc/tree"
)

type templateEntry struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Data        string `json:"data"`
	Items       int    `json:"items"`
	// Lineage is the type, then each it extends, nearest first.
	Lineage  []string `json:"lineage"`
	Abstract bool     `json:"abstract,omitempty"`
}

// templateList answers every Template as its file is written, and how many
// Items use it.
func (s server) templateList(w http.ResponseWriter, r *http.Request) {
	templates, err := s.documents.LoadAllTemplates()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	items, err := s.items.Items()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	used := map[string]int{}
	for _, item := range items {
		used[item.Type]++
	}
	out := []templateEntry{}
	for _, t := range templates {
		data, err := s.documents.DefinitionBytes(tree.TemplatesDir, t.Type)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, templateEntry{Type: t.Type, Description: t.Description, Kind: string(t.Kind), Data: string(data), Items: used[t.Type], Lineage: t.Lineage, Abstract: t.Abstract})
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out, "example": tree.ExampleTemplate})
}

func (s server) templateSave(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Previous string `json:"previous"`
		Data     string `json:"data"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	t, err := s.documents.SaveTemplate(request.Previous, []byte(request.Data), s.now())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s server) templateDelete(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	to, err := s.documents.TrashTemplate(request.Name, s.now())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"trash": to})
}
