package layout

import (
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestByItemPaths(t *testing.T) {
	l := ByItem()
	alice := Item{ID: "alice"}
	for got, want := range map[string]string{
		l.Sidecar("alice"):                       filepath.Join("items", "alice", "item.dgs-item.yaml"),
		l.PDF(alice, "digest"):                   filepath.Join("items", "alice", "digest.pdf"),
		l.Image(alice, "digest", "image", "jpg"): filepath.Join("items", "alice", "image.jpg"),
		l.(Folders).Folder("alice"):              filepath.Join("items", "alice"),
	} {
		if got != want {
			t.Fatalf("%s, want %s", got, want)
		}
	}
	if !l.Owned(l.PDF(alice, "digest")) || l.Owned("templates/alice.yaml") || l.Owned("items/../outside.pdf") {
		t.Fatal("ownership")
	}
	if l.Moves(alice, Item{ID: "alice", Fields: map[string]map[string]string{"digest": {"date": "2026-01-01"}}}) != nil {
		t.Fatal("by Item moved a file")
	}
}

func TestByItemSidecars(t *testing.T) {
	tree := fstest.MapFS{
		"items/alice/item.dgs-item.yaml":         {},
		"items/bob/item.dgs-item.yaml":           {},
		"items/.dgs-import-1/item.dgs-item.yaml": {},
		"items/carol/a.pdf":                      {},
		"templates/x.yaml":                       {},
	}
	got, err := ByItem().Sidecars(tree)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join("items", "alice", SidecarName), filepath.Join("items", "bob", SidecarName)}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sidecars = %v", got)
	}
}

func TestInPlacePathsAndSidecars(t *testing.T) {
	l := RecordedPaths()
	item := Item{ID: "alice", Paths: map[string]string{"digest": "Books/card.pdf", "image": "Books/card.jpg"}}
	if _, ok := l.(Folders); ok {
		t.Fatal("in-place implements Folders")
	}
	if l.Sidecar(item.ID) != filepath.Join("items", "alice.dgs-item.yaml") || l.PDF(item, "digest") != filepath.Join("Books", "card.pdf") || l.Image(item, "digest", "image", "jpg") != filepath.Join("Books", "card.jpg") {
		t.Fatal("paths")
	}
	found, err := l.Sidecars(fstest.MapFS{"items/alice.dgs-item.yaml": {}, "items/.pending.dgs-item.yaml": {}, "items/bob/item.dgs-item.yaml": {}, "Books/card.pdf": {}})
	if err != nil || len(found) != 1 || found[0] != l.Sidecar(item.ID) {
		t.Fatalf("sidecars: %v %v", found, err)
	}
	if !l.Owned("Books/card.pdf", item) || l.Owned("unimported.pdf", item) || l.Moves(item, item) != nil {
		t.Fatal("ownership/moves")
	}
}
