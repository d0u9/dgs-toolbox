package web

import (
	"dgs-toolbox/internal/doc/tree"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefinitionRoutes(t *testing.T) {
	root := t.TempDir()
	must(t, tree.Init(root, time.Now()))
	h := Handler(Settings{Root: root})
	requests := map[string]string{
		"templates": `{"data":"# preserve comment\ntype: alice\nkind: record\nfields:\n  - key: owner\n    required: true\n  - key: country\n    type: country\n    required: true\n"}`,
		"outlines":  `{"outline":{"name":"alice","rules":[]}}`,
		"rules":     `{"rule":{"name":"alice","file":"{owner}.{ext}"}}`,
		"snapshots": `{"previous":"alice","snapshot":{"name":"alice","taken":"2026-10-07T00:00:00Z","files":[]}}`,
		"cases":     `{"op":"new","name":"alice","title":"Synthetic case"}`,
	}
	// Snapshots are created with Take in normal use; prepare one to test the
	// shared save route's edit contract without an Item or PDF.
	write(t, filepath.Join(root, "snapshots", "alice.yaml"), "name: alice\ntaken: '2026-10-07T00:00:00Z'\nfiles: []\n")
	for _, kind := range []string{"templates", "outlines", "rules", "snapshots", "cases"} {
		t.Run(kind, func(t *testing.T) {
			path := "/api/defs/" + kind
			if rec := do(h, "POST", path, requests[kind]); rec.Code != 200 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			if rec := do(h, "GET", path, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), "alice") {
				t.Fatal(rec.Code, rec.Body.String())
			}
			source := filepath.Join(root, kind, "alice.yaml")
			before, err := os.ReadFile(source)
			must(t, err)
			rec := do(h, "POST", path+"/trash", `{"name":"alice"}`)
			if rec.Code != 200 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			var out struct {
				Trash string `json:"trash"`
			}
			must(t, json.Unmarshal(rec.Body.Bytes(), &out))
			got, err := os.ReadFile(out.Trash)
			must(t, err)
			if string(got) != string(before) || !strings.HasPrefix(out.Trash, filepath.Join(root, tree.TrashDir, kind)) {
				t.Fatal("trash response or bytes")
			}
			if _, err := os.Stat(source); !os.IsNotExist(err) {
				t.Fatal("source remains")
			}
		})
	}
	for _, path := range []string{"/api/defs/unknown", "/api/defs/outlines/unknown"} {
		if rec := do(h, "POST", path, `{}`); rec.Code != http.StatusNotFound {
			t.Fatal(rec.Code, rec.Body.String())
		}
	}
}

func TestMetadataPartialUpdatesAndFailedWrite(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	must(t, tree.Init(root, now))
	item := tree.Item{ID: "alice", Kind: tree.KindRecord, Type: "record", Notes: "keep", Tags: []string{"keep"}, Fields: map[string]string{"owner": "alice"}, Revisions: []tree.Revision{{ID: "r1"}}, Head: "r1"}
	must(t, testStore(t, root).WriteItem(item))
	h := Handler(Settings{Root: root})
	path := testStore(t, root).Sidecar(item.ID)
	before, err := os.ReadFile(path)
	must(t, err)
	for _, body := range []string{
		`{"item":"alice","notes":"changed","shared_with":["alice"]}`,
		`{"item":"alice","notes":"changed","digest":"missing","revision_tags":["tag"]}`,
		`{"item":"alice","revision_tags":[]}`,
	} {
		if rec := do(h, "POST", "/api/items/meta", body); rec.Code < 400 {
			t.Fatal("accepted invalid metadata", rec.Body.String())
		}
		got, err := os.ReadFile(path)
		must(t, err)
		if string(got) != string(before) {
			t.Fatal("failed validation wrote partial metadata")
		}
	}
	rec := do(h, "POST", "/api/items/meta", `{"item":"alice","notes":"","frequent":true,"shared_with":["bob"],"digest":"r1","revision_tags":["Tag","tag"]}`)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var saved tree.Item
	must(t, unmarshalItem(rec.Body.Bytes(), &saved))
	if saved.ID != "alice" || saved.Notes != "" || !saved.Frequent || len(saved.Tags) != 1 || saved.Tags[0] != "keep" || len(saved.SharedWith) != 1 || saved.SharedWith[0] != "bob" || len(saved.Revisions[0].Tags) != 1 {
		t.Fatalf("%+v", saved)
	}
	rec = do(h, "POST", "/api/items/meta", `{"item":"alice","frequent":false,"tags":[],"shared_with":[],"digest":"r1","revision_tags":[]}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	must(t, unmarshalItem(rec.Body.Bytes(), &saved))
	if saved.Frequent || len(saved.Tags) != 0 || len(saved.SharedWith) != 0 || len(saved.Revisions[0].Tags) != 0 {
		t.Fatalf("explicit empty/false: %+v", saved)
	}
}
