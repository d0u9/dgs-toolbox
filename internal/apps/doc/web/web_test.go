package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	if item = state(t, h).Items[0]; item.Head != first || item.Fields["country"] != "中国" {
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
	var mu sync.Mutex
	reads := 0
	h := Handler(Settings{Root: root, CacheDir: t.TempDir(), ReadPage: func(path string, page int) (ocr.Page, int, error) {
		mu.Lock()
		reads++
		mu.Unlock()
		return ocr.Page{Source: ocr.SourceRecognised, Lines: []ocr.Line{
			{Text: "有效期限 2016.01.01-2036.01.01"}, {Text: "11010519491231002X"}}}, 2, nil
	}})
	target := "/api/text?dir=" + url.QueryEscape(scans) + "&path=jane/licence.pdf&page=1"
	for range 2 {
		var out textJSON
		must(t, json.Unmarshal(do(h, "GET", target, "").Body.Bytes(), &out))
		got := out.Suggestions["id_card"]
		if !out.Available || out.Count != 2 || got["number"] != (suggestion{"11010519491231002X", 1, 1}) || got["expires"] != (suggestion{"2036.01.01", 1, 0}) {
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

func TestAheadReadsInTheBackground(t *testing.T) {
	root, scans := setup(t, true)
	read := make(chan string, 8)
	cache := t.TempDir()
	h := Handler(Settings{Root: root, CacheDir: cache, ReadPage: func(path string, page int) (ocr.Page, int, error) {
		read <- filepath.Base(path)
		return ocr.Page{}, 1, nil
	}})
	rec := do(h, "POST", "/api/ahead", `{"dir":`+q(scans)+`,"paths":["jane/renewed.pdf","notes.txt","../x.pdf"]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"queued":1`) {
		t.Fatalf("ahead: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case got := <-read:
		if got != "renewed.pdf" {
			t.Fatalf("read %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing read in advance")
	}
	// The worker stores the page after reading it; wait for that to finish, so
	// the cache folder is not being written while the test removes it.
	for deadline := time.Now().Add(2 * time.Second); ; {
		kept, _ := filepath.Glob(filepath.Join(cache, "*", "*", "*", "*.json"))
		parts, _ := filepath.Glob(filepath.Join(cache, "*", "*", "*", ".page-*"))
		if len(kept) > 0 && len(parts) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the page read in advance was never stored")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTextWhereRecognitionIsMissing(t *testing.T) {
	root, scans := setup(t, true)
	h := Handler(Settings{Root: root, ReadPage: func(string, int) (ocr.Page, int, error) { return ocr.Page{}, 0, ocr.ErrUnavailable }})
	var out textJSON
	must(t, json.Unmarshal(do(h, "GET", "/api/text?dir="+url.QueryEscape(scans)+"&path=jane/licence.pdf", "").Body.Bytes(), &out))
	if out.Error == "" || out.Available {
		t.Fatalf("out = %+v", out)
	}
}

func TestViewsSavePlanDelete(t *testing.T) {
	root, scans := setup(t, true)
	h := Handler(Settings{Root: root})
	body := `{"dir":` + q(scans) + `,"path":"jane/licence.pdf","type":"id_card","fields":{"owner":"jane","country":"AU"}}`
	if rec := do(h, "POST", "/api/import", body); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	v := `{"name":"important","selection":"head","layout":"{country}/{owner}/{type}.{ext}","query":{"type":["id_card"]}}`
	rec := do(h, "POST", "/api/views/plan", v)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"path":"澳大利亚/jane/id_card.pdf"`) {
		t.Fatal(rec.Body.String())
	}
	rec = do(h, "POST", "/api/views/plan", `{"name":"x","layout":"{number}/{type}.{ext}"}`)
	if !strings.Contains(rec.Body.String(), `"keys":["number"]`) {
		t.Fatal(rec.Body.String())
	}
	if rec := do(h, "POST", "/api/views", `{"view":`+v+`}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	renamed := strings.Replace(v, "important", "kept", 1)
	if rec := do(h, "POST", "/api/views", `{"view":`+renamed+`,"previous":"important"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var list viewsJSON
	must(t, json.Unmarshal(do(h, "GET", "/api/views", "").Body.Bytes(), &list))
	if len(list.Views) != 1 || list.Views[0].Name != "kept" || list.Keys[0] != "country" {
		t.Fatalf("%+v", list)
	}
	if rec := do(h, "POST", "/api/views/delete", `{"name":"kept"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := do(h, "POST", "/api/views", `{"view":{"name":"Bad","selection":"head","layout":"x"}}`); rec.Code != 400 {
		t.Fatal("bad view saved")
	}
}

func TestNotes(t *testing.T) {
	root, scans := setup(t, true)
	h := Handler(Settings{Root: root})
	rec := do(h, "POST", "/api/import", `{"dir":`+q(scans)+`,"path":"jane/licence.pdf","type":"id_card","fields":{"owner":"jane","country":"AU"}}`)
	var item tree.Item
	must(t, json.Unmarshal(rec.Body.Bytes(), &item))
	rec = do(h, "POST", "/api/notes", `{"item":"`+item.ID+`","notes":" kept for the visa "}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"notes":"kept for the visa"`) {
		t.Fatal(rec.Body.String())
	}
	if do(h, "POST", "/api/notes", `{"item":"NOPE","notes":"x"}`).Code != http.StatusNotFound {
		t.Fatal("notes on a missing Item")
	}
}

func TestTypeFromFiledItems(t *testing.T) {
	root, scans := setup(t, true)
	h := Handler(Settings{Root: root, CacheDir: t.TempDir(), ReadPage: func(path string, page int) (ocr.Page, int, error) {
		return ocr.Page{Lines: []ocr.Line{{Text: "居民身份证 姓名 住址 公民身份号码"}}}, 1, nil
	}})
	text := func(path string) textJSON {
		var out textJSON
		must(t, json.Unmarshal(do(h, "GET", "/api/text?dir="+url.QueryEscape(scans)+"&path="+path+"&page=0", "").Body.Bytes(), &out))
		return out
	}
	if out := text("jane/licence.pdf"); out.Type != "" || len(out.Types) != 0 {
		t.Fatalf("nothing filed, yet %+v", out.Types)
	}
	rec := do(h, "POST", "/api/import", `{"dir":`+q(scans)+`,"path":"jane/licence.pdf","type":"id_card","fields":{"owner":"jane","country":"AU"}}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if out := text("jane/renewed.pdf"); out.Type != "id_card" || out.Types[0].Score < 0.99 {
		t.Fatalf("%+v", out.Types)
	}
	if out := text("jane/licence.pdf"); out.Type != "" {
		t.Fatal("a filed PDF was compared with itself")
	}
}

func TestCasesAndTheirExport(t *testing.T) {
	root, scans := setup(t, true)
	h := Handler(Settings{Root: root})
	body := `{"dir":` + q(scans) + `,"path":"jane/licence.pdf","type":"id_card","fields":{"owner":"jane","country":"AU"}}`
	if rec := do(h, "POST", "/api/import", body); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	id := state(t, h).Items[0].ID
	for _, change := range []string{
		`{"op":"new","name":"lease","title":"Lease"}`,
		`{"op":"need","name":"lease","text":"photo ID"}`,
		`{"op":"meet","name":"lease","need":0,"item":"` + id + `"}`,
	} {
		if rec := do(h, "POST", "/api/cases/change", change); rec.Code != 200 {
			t.Fatalf("%s: %s", change, rec.Body.String())
		}
	}
	if rec := do(h, "POST", "/api/cases/change", `{"op":"add","name":"lease","item":"NOPE"}`); rec.Code != 400 {
		t.Fatal("an Item the tree lacks went in")
	}
	out := t.TempDir()
	plan := do(h, "POST", "/api/cases/export/plan", `{"name":"lease","folder":`+q(out)+`}`)
	if !strings.Contains(plan.Body.String(), `"ready":true`) || !strings.Contains(plan.Body.String(), `"id_card.pdf"`) {
		t.Fatal(plan.Body.String())
	}
	if rec := do(h, "POST", "/api/cases/export", `{"name":"lease","folder":`+q(out)+`}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(out, "id_card.pdf")); err != nil || string(data) != "%PDF-1.4 x" {
		t.Fatalf("exported: %q %v", data, err)
	}
	if rec := do(h, "POST", "/api/cases/change", `{"op":"archive","name":"lease"}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"digest":`) {
		t.Fatal(rec.Body.String())
	}
	if rec := do(h, "POST", "/api/cases/change", `{"op":"remove","name":"lease","item":"`+id+`"}`); rec.Code != 400 {
		t.Fatal("an archived Case changed")
	}
	if rec := do(h, "POST", "/api/cases/change", `{"op":"delete","name":"lease"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "trash", "cases", "lease-*.yaml")); len(matches) != 1 {
		t.Fatal("a deleted Case is not in the trash")
	}
}

func TestTreesAreChosenByName(t *testing.T) {
	papers, scans := setup(t, true)
	books, _ := setup(t, true)
	h := Handler(Settings{Trees: []Tree{{Name: "papers", Root: papers}, {Name: "books", Root: books}}})
	body := `{"dir":` + q(scans) + `,"path":"jane/licence.pdf","type":"id_card","fields":{"owner":"jane","country":"AU"}}`
	if rec := do(h, "POST", "/api/import?tree=books", body); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var s stateJSON
	must(t, json.Unmarshal(do(h, "GET", "/api/state?tree=books", "").Body.Bytes(), &s))
	if s.Name != "books" || len(s.Items) != 1 || len(s.Trees) != 2 {
		t.Fatalf("books: %+v", s)
	}
	if first := state(t, h); first.Name != "papers" || len(first.Items) != 0 {
		t.Fatalf("no tree named is the first: %+v", first)
	}
	if rec := do(h, "GET", "/api/state?tree=nope", ""); rec.Code != 404 {
		t.Fatal("an unknown tree answered")
	}
}
