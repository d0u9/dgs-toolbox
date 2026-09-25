package web

import (
	"net/http"
	"os"

	"dgs-toolbox/internal/doc/tree"
)

type templateEntry struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Data        string `json:"data"`
	Items       int    `json:"items"`
}

// templateList answers every Template as its file is written, and how many
// Items use it.
func (s server) templateList(w http.ResponseWriter, r *http.Request) {
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	items, err := tree.LoadItems(s.root)
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
		data, err := os.ReadFile(tree.TemplatePath(s.root, t.Type))
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, templateEntry{Type: t.Type, Description: t.Description, Kind: string(t.Kind), Data: string(data), Items: used[t.Type]})
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
	t, err := tree.SaveTemplate(s.root, request.Previous, []byte(request.Data), s.now())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s server) templateDelete(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Type string `json:"type"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	to, err := tree.TrashTemplate(s.root, request.Type, s.now())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"trash": to})
}
