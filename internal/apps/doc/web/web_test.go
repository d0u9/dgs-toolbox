package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// folder holds one loose PDF and a text file, and a PDF outside it.
func folder(t *testing.T, init bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	if init {
		must(t, tree.Init(root, time.Now()))
	}
	must(t, os.MkdirAll(filepath.Join(root, "jane"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "jane", "licence.pdf"), []byte("%PDF-1.4 x"), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "notes.txt"), []byte("n"), 0o644))
	outside := filepath.Join(t.TempDir(), "secret.pdf")
	must(t, os.WriteFile(outside, []byte("%PDF secret"), 0o644))
	return root, outside
}

func do(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func state(t *testing.T, h http.Handler) stateJSON {
	t.Helper()
	var s stateJSON
	must(t, json.Unmarshal(do(h, "GET", "/api/state", "").Body.Bytes(), &s))
	return s
}

func TestStateOfAPlainFolder(t *testing.T) {
	root, _ := folder(t, false)
	s := state(t, Handler(Settings{Root: root}))
	if s.Tree || len(s.Loose) != 1 || s.Loose[0].Path != "jane/licence.pdf" || s.Error != "" {
		t.Fatalf("state = %+v", s)
	}
}

func TestFileServesOnlyListedPDFs(t *testing.T) {
	root, outside := folder(t, false)
	h := Handler(Settings{Root: root})
	rec := do(h, "GET", "/api/file?path=jane/licence.pdf", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "%PDF-1.4 x" || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("listed PDF: %d %q", rec.Code, rec.Body.String())
	}
	rel, _ := filepath.Rel(root, outside)
	for _, p := range []string{"notes.txt", rel, outside, "../x.pdf", ""} {
		if rec := do(h, "GET", "/api/file?path="+url.QueryEscape(p), ""); rec.Code != http.StatusNotFound {
			t.Errorf("%q: got %d, want 404", p, rec.Code)
		}
	}
}

func TestImportThroughThePage(t *testing.T) {
	root, _ := folder(t, true)
	h := Handler(Settings{Root: root})
	if raw := do(h, "GET", "/api/state", "").Body.String(); strings.Contains(raw, "null") {
		t.Fatalf("empty tree sends null: %s", raw)
	}
	s := state(t, h)
	if !s.Tree || len(s.Templates) != 1 {
		t.Fatalf("state = %+v", s)
	}
	body := `{"path":"jane/licence.pdf","type":"id_card","fields":{"owner":"jane","country":"AU"}}`
	rec := do(h, "POST", "/api/import", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	s = state(t, h)
	if len(s.Items) != 1 || len(s.Loose) != 1 || s.Loose[0].Item != s.Items[0].ID {
		t.Fatalf("after import: %+v", s)
	}
	item := s.Items[0]
	if rec := do(h, "GET", "/api/revision?item="+item.ID+"&digest="+item.Head, ""); rec.Body.String() != "%PDF-1.4 x" {
		t.Fatalf("revision: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(h, "POST", "/api/import", body); rec.Code != http.StatusConflict {
		t.Fatalf("second import: %d", rec.Code)
	}
}

func TestImportRefusedOutsideATree(t *testing.T) {
	root, _ := folder(t, false)
	rec := do(Handler(Settings{Root: root}), "POST", "/api/import", `{"path":"jane/licence.pdf","type":"id_card"}`)
	if rec.Code == http.StatusOK {
		t.Fatal("imported into a plain folder")
	}
	if _, err := os.Stat(filepath.Join(root, tree.ItemsDir)); err == nil {
		t.Fatal("items folder created")
	}
}

func TestPageIsServed(t *testing.T) {
	root, _ := folder(t, false)
	h := Handler(Settings{Root: root})
	for _, p := range []string{"/", "/app.js", "/app.css", "/ui/tokens.css"} {
		if rec := do(h, "GET", p, ""); rec.Code != http.StatusOK {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}

func TestRevisionHeadAndFieldsThroughThePage(t *testing.T) {
	root, _ := folder(t, true)
	h := Handler(Settings{Root: root})
	do(h, "POST", "/api/import", `{"path":"jane/licence.pdf","type":"id_card","fields":{"owner":"jane","country":"AU"}}`)
	must(t, os.WriteFile(filepath.Join(root, "jane", "renewed.pdf"), []byte("%PDF renewed"), 0o644))
	item := state(t, h).Items[0]
	first := item.Head
	if rec := do(h, "POST", "/api/revisions", `{"path":"jane/renewed.pdf","item":"`+item.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	item = state(t, h).Items[0]
	if len(item.Revisions) != 2 || item.Head == first {
		t.Fatalf("after add: %+v", item)
	}
	if rec := do(h, "POST", "/api/head", `{"item":"`+item.ID+`","digest":"`+first+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("head: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, "POST", "/api/fields", `{"item":"`+item.ID+`","fields":{"owner":"jane","country":"CN"}}`); rec.Code != http.StatusOK {
		t.Fatalf("fields: %d %s", rec.Code, rec.Body.String())
	}
	item = state(t, h).Items[0]
	if item.Head != first || item.Fields["country"] != "CN" {
		t.Fatalf("after head and fields: %+v", item)
	}
	if rec := do(h, "POST", "/api/revisions", `{"path":"../x.pdf","item":"`+item.ID+`"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unlisted path: %d", rec.Code)
	}
}
