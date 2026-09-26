package snapshot

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

func TestTakeResolveAndKeep(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "passport", Kind: tree.KindDocument, Fields: map[string]string{"owner": "emma"},
			Revisions: []tree.Revision{{ID: "r1", Digest: "d1"}, {ID: "r2", Digest: "d2"}}, Head: "r2"},
		{ID: "B", Type: "passport", Kind: tree.KindDocument, Fields: map[string]string{"owner": "tom"},
			Revisions: []tree.Revision{{ID: "r3", Digest: "d3"}}, Head: "r3"},
	}
	o := outline.Outline{Name: "ids", Rules: []view.View{{Name: "all", Selection: view.Head, Layout: "visa/{owner}.{ext}"}}}
	g, err := outline.Group(o, items, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	s, err := Take("visa", "", o, g, "visa", items, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Files) != 2 || s.Files[0] != (File{Path: "emma.pdf", Item: "A", Revision: "r2", Rule: "all"}) {
		t.Fatalf("%+v", s.Files)
	}
	if _, err := Take("x", "", o, g, "nowhere", items, now); err == nil {
		t.Fatal("an empty folder was taken")
	}

	// The Item renews and B leaves: the Snapshot keeps r2, and says B is lost.
	items[0].Revisions = append(items[0].Revisions, tree.Revision{ID: "r4", Digest: "d4"})
	items[0].Head = "r4"
	r := Resolve(s, items[:1])
	if len(r.Files) != 1 || r.Files[0].Digest != "d2" || r.Files[0].Revision != 2 || r.Files[0].View != "visa" || len(r.Lost) != 1 || r.Lost[0].Item != "B" {
		t.Fatalf("%+v", r)
	}
	if r.Root.Count != 1 || r.Root.Files[0].Path != "emma.pdf" {
		t.Fatalf("%+v", r.Root)
	}

	root := t.TempDir()
	if err := Save(root, "", s, true); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, "", s, true); err == nil {
		t.Fatal("a second Snapshot of one name was saved")
	}
	changed := s
	changed.Files = changed.Files[:1]
	if err := Save(root, "visa", changed, false); err == nil {
		t.Fatal("a taken Snapshot's files changed")
	}
	renamed := s
	renamed.Name, renamed.About, renamed.Folder = "visa-2026", "handed in", "~/v"
	if err := Save(root, "visa", renamed, false); err != nil {
		t.Fatal(err)
	}
	list, err := Load(root)
	if err != nil || len(list) != 1 || list[0].Name != "visa-2026" || list[0].About != "handed in" {
		t.Fatalf("%v %+v", err, list)
	}
	if err := outline.Save(root, "", outline.Outline{Name: "visa-2026"}); err == nil {
		t.Fatal("an Outline took a Snapshot's name")
	}
	if _, err := Trash(root, "visa-2026", now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, Dir, "visa-2026.yaml")); !os.IsNotExist(err) {
		t.Fatal("still there")
	}
}
