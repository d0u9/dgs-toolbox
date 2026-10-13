package web

import (
	"dgs-toolbox/internal/doc/itemcache"
	"dgs-toolbox/internal/doc/tree"
	"net/http"
	"sync"
)

func (s server) requireTree() error {
	if s.treeError != nil {
		return s.treeError
	}
	return s.documents.Require()
}
func (s server) templates() ([]tree.Template, error) {
	if err := s.requireTree(); err != nil {
		return nil, err
	}
	return s.documents.LoadTemplates()
}

// treeReady keeps local source browsing available without manufacturing a
// Store over an invalid tree. Every tree operation sees the original Open error.
func (s server) treeReady(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.treeError != nil {
			local := r.URL.Path == "/api/init" || r.URL.Path == "/api/state" || r.URL.Path == "/api/source" || r.URL.Path == "/api/source/file" || r.URL.Path == "/api/reveal" || r.URL.Path == "/api/ahead"
			if (r.URL.Path == "/api/pages" || r.URL.Path == "/api/page" || r.URL.Path == "/api/text") && r.URL.Query().Get("item") == "" {
				local = true
			}
			if !local {
				writeJSON(w, http.StatusConflict, map[string]string{"error": s.treeError.Error()})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// apiWhenReady retains source browsing before init. If the marker is later
// created or corrected, Open supplies the Store for subsequent requests.
// A successfully opened Store is retained, never replaced by an unchecked one.
func (s server) apiWhenReady(cacheDir string) http.Handler {
	var opening sync.Mutex
	current := s.treeReady(s.followWrites(s.api()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opening.Lock()
		if s.documents == nil {
			if store, err := tree.Open(s.root); err == nil {
				s.documents, s.treeError = store, nil
				s.items = itemcache.New(store, cacheDir)
				store.UseForChecks(s.items.Items)
				current = s.followWrites(s.api())
			}
		}
		handler := current
		opening.Unlock()
		handler.ServeHTTP(w, r)
	})
}

func (s server) initialize(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Layout   string `json:"layout"`
		Outlines int    `json:"outlines"`
	}
	if !decode(w, r, &request) {
		return
	}
	if request.Outlines == 0 {
		request.Outlines = 1
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	if err := tree.InitWithOptions(s.root, s.now(), request.Layout, request.Outlines); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
