package outline

import (
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

func TestInPlaceSkeletonDefinitionsRoundTripAndFirstMountWins(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(0, 0)
	if err := tree.InitWithOptions(root, now, "in-place", 2); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	rule := view.View{Name: "identity", Folders: []view.Folder{{Name: "IDs", Take: view.Values{"id_card"}}}}
	if err := SaveRule(st, "", rule, now); err != nil {
		t.Fatal(err)
	}
	o := Outline{Name: "current", Mount: []Assembly{{Rule: "identity", At: "first"}, {Rule: "identity", At: "second"}}, Defaults: view.Defaults{Valid: "today"}, Pins: map[string]view.Pin{"second/IDs": {Items: []string{"alice"}, Numbers: map[string]int{"alice": 9}}}}
	if err := Save(st, "", o, now); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(st)
	if err != nil || len(loaded) != 1 || len(loaded[0].Mount) != 2 || loaded[0].Pins["second/IDs"].Numbers["alice"] != 9 {
		t.Fatal(loaded, err)
	}
	item := tree.Item{ID: "alice", Type: "id_card", Kind: tree.KindDocument, Head: "d", Fields: map[string]string{"name": "Card"}, Revisions: []tree.Revision{{Digest: "d", Path: "card.pdf"}}}
	types := view.TypesOf([]tree.Template{{Type: "id_card", Lineage: []string{"id_card"}}})
	group, err := GroupAt(loaded[0], nil, []tree.Item{item}, types, now)
	if err != nil || len(group.Files) != 1 || group.Files[0].Path != "second/IDs/09-Card.pdf" {
		t.Fatal(group, err)
	}
	loaded[0].Pins = nil
	group, err = GroupAt(loaded[0], nil, []tree.Item{item}, types, now)
	if err != nil || len(group.Files) != 1 || group.Files[0].Path != "first/IDs/01-Card.pdf" {
		t.Fatal(group, err)
	}
	if err := RenameRule(st, "identity", "papers", now); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(st)
	if err != nil || loaded[0].Mount[0].Rule != "papers" {
		t.Fatal("rename mount", loaded, err)
	}
	// Doc 1 format is deliberately refused in this tree.
	if err := st.WriteDefinition(RulesDir, "legacy", []byte("name: legacy\nselection: head\nfile: '{id}.pdf'\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRule(st, filepath.Join(root, RulesDir, "legacy.yaml")); err == nil {
		t.Fatal("legacy rule accepted")
	}
}

func TestInPlaceEmptySkeletonCanBeSavedAndReloaded(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(0, 0)
	if err := tree.InitWithOptions(root, now, "in-place", 2); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveRule(st, "", view.View{Name: "empty", Folders: []view.Folder{}}, now); err != nil {
		t.Fatal(err)
	}
	if err := Save(st, "", Outline{Name: "empty", Mount: []Assembly{}}, now); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(st)
	if err != nil || len(loaded) != 1 || loaded[0].Mount == nil {
		t.Fatal("empty mount lost", loaded, err)
	}
}
