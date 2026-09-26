package snapshot

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	rule := view.View{Name: "all", Selection: view.Head, Layout: "visa/{owner}.{ext}"}
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	s, err := Take("visa", "", &rule, items, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Files) != 2 || s.Rule != "all" || s.Files[0] != (File{Path: "visa/emma.pdf", Item: "A", Revision: "r2", Rule: "all"}) {
		t.Fatalf("%+v", s)
	}
	if empty, err := Take("mine", "", nil, items, nil, now); err != nil || len(empty.Files) != 0 {
		t.Fatalf("%v %+v", err, empty)
	}
	gap := view.View{Name: "gap", Selection: view.Head, Layout: "{country}.{ext}"}
	if _, err := Take("x", "", &gap, items, nil, now); err == nil {
		t.Fatal("a rule with PDFs it cannot place was taken")
	}

	// The Item renews and B leaves: the Snapshot keeps r2, and says B is lost.
	items[0].Revisions = append(items[0].Revisions, tree.Revision{ID: "r4", Digest: "d4"})
	items[0].Head = "r4"
	r := Resolve(s, items[:1])
	if len(r.Files) != 1 || r.Files[0].Digest != "d2" || r.Files[0].Revision != 2 || r.Files[0].View != "visa" || len(r.Lost) != 1 || r.Lost[0].Item != "B" {
		t.Fatalf("%+v", r)
	}

	root := t.TempDir()
	if err := Save(root, "", s, true); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, "", s, true); err == nil {
		t.Fatal("a second Snapshot of one name was saved")
	}
	// Its files are edited by hand after.
	edited := s
	edited.Files = []File{{Path: "emma.pdf", Item: "A", Revision: "r1"}}
	if err := Save(root, "", edited, false); err != nil {
		t.Fatal(err)
	}
	twice := edited
	twice.Files = append(twice.Files, File{Path: "EMMA.pdf", Item: "B", Revision: "r3"})
	if err := Save(root, "", twice, false); err == nil {
		t.Fatal("two files at one path were saved")
	}
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules", "ids.yaml"), []byte("name: ids\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clash := edited
	clash.Name = "ids"
	if err := Save(root, "visa", clash, false); err == nil {
		t.Fatal("a Snapshot took a rule's name")
	}
	list, err := Load(root)
	if err != nil || len(list) != 1 || len(list[0].Files) != 1 || list[0].Files[0].Revision != "r1" {
		t.Fatalf("%v %+v", err, list)
	}
	if _, err := Trash(root, "visa", now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, Dir, "visa.yaml")); !os.IsNotExist(err) {
		t.Fatal("still there")
	}
}

// A Snapshot taken from an Outline, before Snapshots stood alone, still reads.
func TestParseKeepsAnOldSnapshot(t *testing.T) {
	s, err := Parse([]byte("name: visa\ntaken: 2026-09-26T10:00:00Z\noutline: ids\nnode: a/b\nfolder: ~/v\nfiles:\n    - path: emma.pdf\n      item: A\n      revision: r2\n"))
	if err != nil || s.Outline != "ids" || len(s.Files) != 1 {
		t.Fatalf("%v %+v", err, s)
	}
}
