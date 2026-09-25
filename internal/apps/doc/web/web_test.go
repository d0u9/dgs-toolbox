package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func tree(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, "jane"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "jane", "licence.pdf"), []byte("%PDF-1.4 x"), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "notes.txt"), []byte("n"), 0o644))
	outside := filepath.Join(t.TempDir(), "secret.pdf")
	must(t, os.WriteFile(outside, []byte("%PDF secret"), 0o644))
	return root, outside
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func get(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestFilesListsPDFs(t *testing.T) {
	root, _ := tree(t)
	rec := get(Handler(Settings{Root: root}), "/api/files")
	var files []fileJSON
	must(t, json.Unmarshal(rec.Body.Bytes(), &files))
	if len(files) != 1 || files[0].Path != "jane/licence.pdf" {
		t.Fatalf("files = %+v", files)
	}
}

func TestFileServesOnlyListedPDFs(t *testing.T) {
	root, outside := tree(t)
	h := Handler(Settings{Root: root})
	rec := get(h, "/api/file?path=jane/licence.pdf")
	if rec.Code != http.StatusOK || rec.Body.String() != "%PDF-1.4 x" || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("listed PDF: %d %q", rec.Code, rec.Body.String())
	}
	rel, _ := filepath.Rel(root, outside)
	for _, p := range []string{"notes.txt", rel, outside, "../x.pdf", ""} {
		if rec := get(h, "/api/file?path="+url.QueryEscape(p)); rec.Code != http.StatusNotFound {
			t.Errorf("%q: got %d, want 404", p, rec.Code)
		}
	}
}

func TestPageIsServed(t *testing.T) {
	root, _ := tree(t)
	h := Handler(Settings{Root: root})
	for _, p := range []string{"/", "/app.js", "/app.css", "/ui/tokens.css"} {
		if rec := get(h, p); rec.Code != http.StatusOK {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}
