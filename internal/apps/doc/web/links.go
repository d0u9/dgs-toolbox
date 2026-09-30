package web

import (
	"fmt"
	"net/http"

	"dgs-toolbox/internal/doc/anchor"
	"dgs-toolbox/internal/doc/tree"
)

// links answers, for one link field of a form, the Items it may point to and
// those its within suggests: the page offers the first and fills in the
// one suggestion when the field is empty.
func (s server) links(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Type   string            `json:"type"`
		Key    string            `json:"key"`
		Self   string            `json:"self"`
		Fields map[string]string `json:"fields"`
	}
	if !decode(w, r, &request) {
		return
	}
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	items, err := tree.LoadItems(s.root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var field *tree.Field
	for _, t := range templates {
		if t.Type != request.Type {
			continue
		}
		for i := range t.Fields {
			if t.Fields[i].Key == request.Key {
				field = &t.Fields[i]
			}
		}
	}
	if field == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("no field %s in the Template %s", request.Key, request.Type)})
		return
	}
	ids := func(list []tree.Item) []string {
		out := []string{}
		for _, it := range list {
			out = append(out, it.ID)
		}
		return out
	}
	offered := anchor.Offered(*field, request.Self, request.Fields, items, tree.AnchorTypes(templates))
	writeJSON(w, http.StatusOK, map[string][]string{
		"offered":   ids(offered),
		"suggested": ids(anchor.Within(*field, request.Fields, offered)),
	})
}
