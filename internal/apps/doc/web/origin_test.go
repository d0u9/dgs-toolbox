package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossOriginMutationsAreRefused(t *testing.T) {
	for _, endpoint := range []string{"/api/init", "/ui/files/folder", "/ui/files/rename"} {
		for _, contentType := range []string{"text/plain", "application/json"} {
			t.Run(endpoint+"/"+contentType, func(t *testing.T) {
				root := t.TempDir()
				source := filepath.Join(root, "original.txt")
				must(t, os.WriteFile(source, []byte("original"), 0600))
				body, _ := json.Marshal(map[string]any{"parent": root, "path": source, "name": "unexpected", "layout": "in-place", "outlines": 1})
				r := httptest.NewRequest("POST", "http://127.0.0.1:8767"+endpoint, strings.NewReader(string(body)))
				r.Header.Set("Origin", "https://untrusted.example")
				r.Header.Set("Content-Type", contentType)
				w := httptest.NewRecorder()
				Handler(Settings{Root: root}).ServeHTTP(w, r)
				if w.Code != http.StatusForbidden {
					t.Fatalf("status %d", w.Code)
				}
				entries, err := os.ReadDir(root)
				must(t, err)
				if len(entries) != 1 || entries[0].Name() != "original.txt" {
					t.Fatal("refused request changed the filesystem")
				}
			})
		}
	}
}

func TestSameOriginFolderCreationWorks(t *testing.T) {
	root := t.TempDir()
	body, _ := json.Marshal(map[string]string{"parent": root, "name": "new-folder"})
	r := httptest.NewRequest("POST", "http://127.0.0.1:8767/ui/files/folder", strings.NewReader(string(body)))
	r.Header.Set("Origin", "http://127.0.0.1:8767")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	Handler(Settings{Root: root}).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "new-folder")); err != nil {
		t.Fatal(err)
	}
}
