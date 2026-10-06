package web

import (
	"dgs-toolbox/internal/doc/cases"
	"dgs-toolbox/internal/doc/tree"
	"net/http"
)

// definitionHandler shares routes while the domain packages retain operations
// particular to each kind (inheritance, shared-rule references and Case close).
func (s server) definitionHandler(w http.ResponseWriter, r *http.Request) {
	var handler http.HandlerFunc
	switch r.PathValue("kind") {
	case "templates":
		switch {
		case r.Method == http.MethodGet:
			handler = s.templateList
		case r.PathValue("action") == "trash":
			handler = s.templateDelete
		default:
			handler = s.templateSave
		}
	case "outlines":
		switch {
		case r.Method == http.MethodGet:
			handler = s.outlineList
		case r.PathValue("action") == "trash":
			handler = s.outlineDelete
		default:
			handler = s.outlineSave
		}
	case "rules":
		switch {
		case r.Method == http.MethodGet:
			handler = s.outlineList
		case r.PathValue("action") == "trash":
			handler = s.ruleDelete
		default:
			handler = s.ruleSave
		}
	case "snapshots":
		switch {
		case r.Method == http.MethodGet:
			handler = s.snapshotList
		case r.PathValue("action") == "trash":
			handler = s.snapshotDelete
		default:
			handler = s.snapshotSave
		}
	case "cases":
		switch {
		case r.Method == http.MethodGet:
			handler = s.caseList
		case r.PathValue("action") == "trash":
			handler = s.caseTrash
		default:
			handler = s.caseChange
		}
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no definition kind " + r.PathValue("kind")})
		return
	}
	handler(w, r)
}

func (s server) caseTrash(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	to, err := cases.ForStore(s.documents).Trash(request.Name, s.now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"trash": to})
}

type Metadata = tree.Metadata

func (s server) setMetadata(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item   string `json:"item"`
		Digest string `json:"digest"`
		Metadata
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := s.documents.SetMetadata(request.Item, request.Digest, request.Metadata, s.now())
	answer(w, item, err)
}
