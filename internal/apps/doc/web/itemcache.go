package web

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
)

// A POST names in its path what it may change in the tree, so the Item
// cache follows it: readOnly ones change nothing; oneItem ones change the
// Item their body's "item" names; notItems ones change files beside the
// Items, which only moves the change mark. Any other changes Items the
// cache cannot name and reads them all again.
var (
	readOnly = map[string]bool{
		"/api/links": true, "/api/items/reload": true, "/api/ahead": true, "/api/reveal": true, "/api/read-all": true,
		"/api/outlines/group": true, "/api/snapshots/names": true,
		"/api/export/plan": true, "/api/merge/plan": true, "/api/cases/export/plan": true,
	}
	oneItem = map[string]bool{
		"/api/revisions": true, "/api/head": true, "/api/revisions/delete": true, "/api/fields": true,
		"/api/change-type": true, "/api/notes": true, "/api/tags": true, "/api/revision-tags": true,
		"/api/frequent": true, "/api/retired": true, "/api/shared": true, "/api/items/delete": true,
	}
	notItems = map[string]bool{
		"/api/templates": true, "/api/templates/delete": true,
		"/api/outlines": true, "/api/outlines/delete": true,
		"/api/rules": true, "/api/rules/delete": true, "/api/rules/layout": true, "/api/rules/if": true,
		"/api/snapshots/take": true, "/api/snapshots": true, "/api/snapshots/delete": true,
		"/api/cases/change": true,
	}
)

// followWrites brings the Item cache up to date after a POST that wrote,
// before the page hears it succeeded, so the next question sees the write.
func (s server) followWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || readOnly[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		answer := httptest.NewRecorder()
		next.ServeHTTP(answer, r)
		if answer.Code < 300 {
			var named struct {
				Item string `json:"item"`
			}
			switch {
			case notItems[r.URL.Path]:
				err = s.items.Marked()
			case oneItem[r.URL.Path] && json.Unmarshal(body, &named) == nil && named.Item != "":
				err = s.items.Wrote(named.Item)
			default:
				err = s.items.Wrote()
			}
			// The write stands; a cache that could not follow it reads the
			// tree again at the next question.
			if err != nil {
				log.Printf("doc: item cache after %s: %v", r.URL.Path, err)
			}
		}
		for k, v := range answer.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(answer.Code)
		w.Write(answer.Body.Bytes())
	})
}

// reloadItems reads every sidecar again, for one edited by hand.
func (s server) reloadItems(w http.ResponseWriter, _ *http.Request) {
	if err := s.items.Reload(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
