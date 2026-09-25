package web

import (
	"net/http"
	"sort"

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

type viewsJSON struct {
	Views []view.View `json:"views"`
	// Keys is every key a layout can use with the tree's Items: the
	// Templates' fields and the keys every Item has.
	Keys  []string `json:"keys"`
	Error string   `json:"error,omitempty"`
}

// builtIn are the keys every Item has, and those derived from its fields.
var builtIn = []string{"type", "kind", "id", "revision", "ext", "year", "month", "date"}

func (s server) viewList(w http.ResponseWriter, _ *http.Request) {
	out := viewsJSON{Views: []view.View{}}
	seen := map[string]bool{}
	var fields []string
	if templates, err := tree.LoadTemplates(s.root); err == nil {
		for _, t := range templates {
			for _, f := range t.Fields {
				if !seen[f.Key] {
					seen[f.Key] = true
					fields = append(fields, f.Key)
				}
			}
		}
	}
	sort.Strings(fields)
	for _, k := range builtIn {
		if !seen[k] {
			out.Keys = append(out.Keys, k)
		}
	}
	out.Keys = append(fields, out.Keys...)
	views, err := view.Load(s.root)
	if err != nil {
		out.Error = err.Error()
	} else {
		out.Views = views
	}
	writeJSON(w, http.StatusOK, out)
}

// viewPlan computes where a View, saved or not, would put every PDF it
// selects. It writes nothing.
func (s server) viewPlan(w http.ResponseWriter, r *http.Request) {
	var v view.View
	if !decode(w, r, &v) {
		return
	}
	if v.Selection == "" {
		v.Selection = view.Head
	}
	items, err := tree.LoadItems(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	plan, err := view.Build(v, items)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// viewSave writes a View. Previous, when it names another View, is the name
// it had, and that file is removed once the new one is written.
func (s server) viewSave(w http.ResponseWriter, r *http.Request) {
	var request struct {
		View     view.View `json:"view"`
		Previous string    `json:"previous"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if err := view.Save(s.root, request.View); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if request.Previous != "" && request.Previous != request.View.Name {
		if err := view.Delete(s.root, request.Previous); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, request.View)
}

func (s server) viewDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := view.Delete(s.root, request.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}
