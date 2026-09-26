package web

import (
	"dgs-toolbox/internal/doc/tree"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestImportVisaReplacement(t *testing.T) {
	root, _ := setup(t, true)
	write(t, filepath.Join(root, tree.TemplatesDir, "visa.yaml"), "type: visa\nkind: record\nfields:\n  - key: owner\n    required: true\n    distinguishing: true\n  - key: country\n    type: country\n  - key: number\n    required: true\n    distinguishing: true\n")
	h := Handler(Settings{Root: root})
	rec := do(h, "POST", "/api/import", `{"no_pdf":true,"type":"visa","fields":{"owner":"doug","country":"AU","number":"old"}}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var old tree.Item
	must(t, json.Unmarshal(rec.Body.Bytes(), &old))
	rec = do(h, "POST", "/api/import", `{"no_pdf":true,"type":"visa","replaces":`+q(old.ID)+`,"fields":{"owner":"jane","country":"AU","number":"invalid"}}`)
	if rec.Code != 409 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	items, err := tree.LoadItems(root)
	must(t, err)
	if len(items) != 1 {
		t.Fatal("invalid replacement created an item")
	}
	rec = do(h, "POST", "/api/import", `{"no_pdf":true,"type":"visa","replaces":`+q(old.ID)+`,"fields":{"owner":"doug","country":"AU","number":"new"}}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var next tree.Item
	must(t, json.Unmarshal(rec.Body.Bytes(), &next))
	updated, _, err := tree.FindItem(root, old.ID)
	must(t, err)
	if !updated.Retired || updated.SupersededBy != next.ID {
		t.Fatal("missing relationship")
	}
	rec = do(h, "POST", "/api/supersession", `{"item":`+q(old.ID)+`}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	updated, _, err = tree.FindItem(root, old.ID)
	must(t, err)
	if updated.Retired || updated.SupersededBy != "" {
		t.Fatal("undo failed")
	}
}
