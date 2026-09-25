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

	"dgs-toolbox/internal/doc/ocr"
	"dgs-toolbox/internal/doc/tree"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(content), 0o644))
}

// setup is a tree and, apart from it, a folder of scans in subfolders.
func setup(t *testing.T, init bool) (root, scans string) {
	t.Helper()
	root = t.TempDir()
	if init {
		must(t, tree.Init(root, time.Now()))
	}
	scans = t.TempDir()
	write(t, filepath.Join(scans, "jane", "licence.pdf"), "%PDF-1.4 x")
	write(t, filepath.Join(scans, "jane", "renewed.pdf"), "%PDF renewed")
	write(t, filepath.Join(scans, "notes.txt"), "n")
	write(t, filepath.Join(scans, ".hidden", "h.pdf"), "h")
	return root, scans
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

func source(t *testing.T, h http.Handler, dir string) []sourceFileJSON {
	t.Helper()
	var out struct{ Files []sourceFileJSON }
	rec := do(h, "GET", "/api/source?dir="+url.QueryEscape(dir), "")
	must(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out.Files
}

func q(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestSourceListsPDFsUnderTheFolder(t *testing.T) {
	root, scans := setup(t, false)
	files := source(t, Handler(Settings{Root: root}), scans)
	if len(files) != 2 || files[0].Path != "jane/licence.pdf" || files[1].Path != "jane/renewed.pdf" {
		t.Fatalf("files = %+v", files)
	}
	if rec := do(Handler(Settings{Root: root}), "GET", "/api/source?dir=relative", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("relative dir: %d", rec.Code)
	}
}

func TestSourceFileServesOnlyPDFsUnderTheFolder(t *testing.T) {
	root, scans := setup(t, false)
	h := Handler(Settings{Root: root})
	get := func(path string) *httptest.ResponseRecorder {
		return do(h, "GET", "/api/source/file?dir="+url.QueryEscape(scans)+"&path="+url.QueryEscape(path), "")
	}
	if rec := get("jane/licence.pdf"); rec.Code != http.StatusOK || rec.Body.String() != "%PDF-1.4 x" {
		t.Fatalf("PDF: %d %q", rec.Code, rec.Body.String())
	}
	outside := filepath.Join(t.TempDir(), "secret.pdf")
	write(t, outside, "secret")
	must(t, os.Symlink(outside, filepath.Join(scans, "link.pdf")))
	for _, p := range []string{"notes.txt", "../secret.pdf", outside, ".hidden/h.pdf", "link.pdf", ""} {
		if rec := get(p); rec.Code != http.StatusNotFound {
			t.Errorf("%q: got %d, want 404", p, rec.Code)
		}
	}
}

func TestImportRevisionHeadAndFields(t *testing.T) {
	root, scans := setup(t, true)
	h := Handler(Settings{Root: root})
	if raw := do(h, "GET", "/api/state", "").Body.String(); strings.Contains(raw, "null") {
		t.Fatalf("empty tree sends null: %s", raw)
	}
	body := `{"dir":` + q(scans) + `,"path":"jane/licence.pdf","type":"id_card","fields":{"owner":"jane","country":"AU"}}`
	if rec := do(h, "POST", "/api/import", body); rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, "POST", "/api/import", body); rec.Code != http.StatusConflict {
		t.Fatalf("second import: %d", rec.Code)
	}
	item := state(t, h).Items[0]
	files := source(t, h, scans)
	if files[0].Item != item.ID || files[1].Item != "" {
		t.Fatalf("marks: %+v", files)
	}
	first := item.Head
	rec := do(h, "POST", "/api/revisions", `{"dir":`+q(scans)+`,"path":"jane/renewed.pdf","item":"`+item.ID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	if item = state(t, h).Items[0]; len(item.Revisions) != 2 || item.Head == first {
		t.Fatalf("after add: %+v", item)
	}
	if rec := do(h, "GET", "/api/revision?item="+item.ID+"&digest="+first, ""); rec.Body.String() != "%PDF-1.4 x" {
		t.Fatalf("revision: %d", rec.Code)
	}
	do(h, "POST", "/api/head", `{"item":"`+item.ID+`","digest":"`+first+`"}`)
	do(h, "POST", "/api/fields", `{"item":"`+item.ID+`","fields":{"owner":"jane","country":"CN"}}`)
	if item = state(t, h).Items[0]; item.Head != first || item.Fields["country"] != "CN" {
		t.Fatalf("after head and fields: %+v", item)
	}
	// The tree's own items folder is not offered for import.
	if files := source(t, h, root); len(files) != 0 {
		t.Fatalf("tree root offers %+v", files)
	}
}

func TestImportRefusedOutsideATree(t *testing.T) {
	root, scans := setup(t, false)
	rec := do(Handler(Settings{Root: root}), "POST", "/api/import", `{"dir":`+q(scans)+`,"path":"jane/licence.pdf","type":"id_card"}`)
	if rec.Code == http.StatusOK {
		t.Fatal("imported into a plain folder")
	}
	if _, err := os.Stat(filepath.Join(root, tree.ItemsDir)); err == nil {
		t.Fatal("items folder created")
	}
}

func TestPagesAreServed(t *testing.T) {
	root, _ := setup(t, false)
	h := Handler(Settings{Root: root})
	if rec := do(h, "GET", "/", ""); rec.Code != http.StatusFound || rec.Header().Get("Location") != "/browse/" {
		t.Fatalf("/: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	for _, p := range []string{"/browse/", "/import/", "/common.js", "/browse.js", "/import.js", "/app.css", "/ui/filedialog.js", "/ui/files/dir"} {
		if rec := do(h, "GET", p, ""); rec.Code != http.StatusOK {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}

func TestTextAndSuggestions(t *testing.T) {
	root, scans := setup(t, true)
	reads := 0
	h := Handler(Settings{Root: root, Read: func(path string) (ocr.Result, error) {
		reads++
		return ocr.Result{Pages: []ocr.Page{{Text: "有效期限 2016.01.01-2036.01.01\n11010519491231002X", Source: ocr.SourceRecognised}}}, nil
	}})
	target := "/api/text?dir=" + url.QueryEscape(scans) + "&path=jane/licence.pdf"
	for range 2 {
		var out textJSON
		must(t, json.Unmarshal(do(h, "GET", target, "").Body.Bytes(), &out))
		if !out.Available || out.Suggestions["id_card"]["number"] != "11010519491231002X" || out.Suggestions["id_card"]["expires"] != "2036.01.01" {
			t.Fatalf("out = %+v", out)
		}
	}
	if reads != 1 {
		t.Fatalf("read %d times, want once", reads)
	}
	if rec := do(h, "GET", "/api/text?dir="+url.QueryEscape(scans)+"&path=notes.txt", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("non-PDF: %d", rec.Code)
	}
}

func TestTextWhereRecognitionIsMissing(t *testing.T) {
	root, scans := setup(t, true)
	h := Handler(Settings{Root: root, Read: func(string) (ocr.Result, error) { return ocr.Result{}, ocr.ErrUnavailable }})
	var out textJSON
	must(t, json.Unmarshal(do(h, "GET", "/api/text?dir="+url.QueryEscape(scans)+"&path=jane/licence.pdf", "").Body.Bytes(), &out))
	if out.Error == "" || out.Available {
		t.Fatalf("out = %+v", out)
	}
}
