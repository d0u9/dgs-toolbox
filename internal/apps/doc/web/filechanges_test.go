package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/check"
	"dgs-toolbox/internal/doc/tree"
)

func TestInPlaceFileCheckAPIRecordsChangesAndKeepsLockedReferences(t *testing.T) {
	root := t.TempDir()
	must(t, tree.InitWithLayout(root, time.Unix(0, 0), "in-place"))
	st, err := tree.Open(root)
	must(t, err)
	source := filepath.Join(root, "paper.pdf")
	write(t, source, "synthetic original PDF")
	digest, err := tree.FileDigest(source)
	must(t, err)
	for _, id := range []string{"alice", "bob"} {
		item := tree.Item{ID: id, Type: "id_card", Kind: tree.KindDocument, Head: digest, Fields: map[string]string{"owner": id, "country": "AU"}, Locked: id == "bob", Revisions: []tree.Revision{{Digest: digest, Path: "paper.pdf"}}}
		must(t, st.WriteItem(item))
	}
	h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
	write(t, source, "synthetic edited PDF")
	rec := do(h, "POST", "/api/files/check", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var result struct {
		Report   check.Report `json:"report"`
		Recorded []string     `json:"recorded"`
	}
	must(t, json.Unmarshal(rec.Body.Bytes(), &result))
	if len(result.Recorded) != 1 || result.Recorded[0] != "alice" || len(result.Report.Problems) != 1 || !result.Report.Problems[0].Locked {
		t.Fatalf("check %+v", result)
	}
	items := state(t, h).Items
	var alice, bob tree.Item
	for _, i := range items {
		if i.ID == "alice" {
			alice = i
		} else {
			bob = i
		}
	}
	if len(alice.Revisions) != 2 || !alice.Revisions[0].Lost || len(bob.Revisions) != 1 || bob.Revisions[0].Lost {
		t.Fatal("cache or locked references", items)
	}
	rec = do(h, "GET", "/api/revision?item=alice&digest="+digest, "")
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "earlier revision lost") {
		t.Fatal("lost preview", rec.Body.String())
	}
	rec = do(h, "POST", "/api/files/check", `{}`)
	must(t, json.Unmarshal(rec.Body.Bytes(), &result))
	if len(result.Recorded) != 0 || len(state(t, h).Items[0].Revisions) != 2 {
		t.Fatal("repeat created revisions")
	}
	rec = do(h, "POST", "/api/items/meta", `{"item":"bob","locked":false}`)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = do(h, "POST", "/api/files/check", `{}`)
	must(t, json.Unmarshal(rec.Body.Bytes(), &result))
	if len(result.Recorded) != 1 || result.Recorded[0] != "bob" {
		t.Fatal("unlock did not allow recording", rec.Body.String())
	}
}

func TestInPlaceMovedFileAPIRequiresCurrentConfirmation(t *testing.T) {
	root := t.TempDir()
	must(t, tree.InitWithLayout(root, time.Unix(0, 0), "in-place"))
	st, err := tree.Open(root)
	must(t, err)
	source := filepath.Join(root, "paper.pdf")
	write(t, source, "synthetic original PDF")
	digest, err := tree.FileDigest(source)
	must(t, err)
	item := tree.Item{ID: "alice", Type: "id_card", Kind: tree.KindDocument, Head: digest, Revisions: []tree.Revision{{Digest: digest, Path: "paper.pdf"}}}
	must(t, st.WriteItem(item))
	h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
	moved := filepath.Join(root, "renamed.pdf")
	must(t, os.Rename(source, moved))
	rec := do(h, "POST", "/api/files/check", `{}`)
	var result struct {
		Report check.Report `json:"report"`
	}
	must(t, json.Unmarshal(rec.Body.Bytes(), &result))
	if rec.Code != http.StatusOK || len(result.Report.Problems) != 1 || result.Report.Problems[0].Kind != check.Moved || len(result.Report.Problems[0].Candidates) != 1 {
		t.Fatal("move report", rec.Body.String())
	}
	if state(t, h).Items[0].Revisions[0].Path != "paper.pdf" {
		t.Fatal("check confirmed a move")
	}
	body := `{"item":"alice","digest":"` + digest + `","previous":"paper.pdf","path":"renamed.pdf"}`
	write(t, moved, "synthetic changed candidate")
	rec = do(h, "POST", "/api/files/confirm", body)
	if rec.Code == http.StatusOK || state(t, h).Items[0].Revisions[0].Path != "paper.pdf" {
		t.Fatal("stale confirmation wrote")
	}
	write(t, moved, "synthetic original PDF")
	rec = do(h, "POST", "/api/files/confirm", body)
	if rec.Code != http.StatusOK || state(t, h).Items[0].Revisions[0].Path != "renamed.pdf" {
		t.Fatal("confirmation/cache", rec.Body.String())
	}
	rec = do(h, "POST", "/api/files/check", `{}`)
	must(t, json.Unmarshal(rec.Body.Bytes(), &result))
	if len(result.Report.Problems) != 0 {
		t.Fatal("confirmed move still reported", rec.Body.String())
	}
	// Controls are embedded in the served page/module, without a new app.
	for path, expected := range map[string]string{"/browse/": "id=\"check-files\"", "/browse.js": "/api/files/confirm"} {
		rec = do(h, "GET", path, "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), expected) {
			t.Fatal("missing page control", path, rec.Code)
		}
	}
}

func TestByItemRejectsV3WriteActions(t *testing.T) {
	root := t.TempDir()
	must(t, tree.Init(root, time.Unix(0, 0)))
	h := Handler(Settings{Root: root, CacheDir: t.TempDir()})
	if rec := do(h, "POST", "/api/files/check", `{}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	for path, body := range map[string]string{"/api/files/confirm": `{"item":"alice"}`, "/api/items/meta": `{"item":"alice","locked":true}`} {
		if rec := do(h, "POST", path, body); rec.Code == http.StatusOK {
			t.Fatal("by-Item V3 action accepted", path)
		}
	}
}
