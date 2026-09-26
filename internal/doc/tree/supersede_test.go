package tree_test

import (
	"dgs-toolbox/internal/doc/tree"
	"testing"
	"time"
)

func TestVisaSupersession(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	if err := tree.Init(root, now); err != nil {
		t.Fatal(err)
	}
	put := func(id, owner, country string) tree.Item {
		it := tree.Item{ID: id, Type: "visa", Kind: tree.KindRecord, Fields: map[string]string{"owner": owner, "country": country}, Revisions: []tree.Revision{{ID: "snapshot-" + id, Snapshot: true, Fields: map[string]string{"number": id}}}}
		if err := tree.WriteItem(root, it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	old := put("OLD", "doug", "澳大利亚")
	put("NEW", "doug", "AU")
	put("OTHER", "jane", "AU")
	put("COUNTRY", "doug", "CN")
	for _, id := range []string{"OLD", "OTHER", "COUNTRY", "missing"} {
		if _, err := tree.Supersede(root, "OLD", id, now); err == nil {
			t.Fatalf("accepted invalid successor %s", id)
		}
	}
	updated, err := tree.Supersede(root, "OLD", "NEW", now)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Retired || updated.SupersededBy != "NEW" || updated.Current() != old.Current() || len(updated.Revisions) != 1 {
		t.Fatalf("bad replacement: %+v", updated)
	}
	if _, err := tree.SetRetired(root, "OLD", false, "", now); err == nil {
		t.Fatal("unretire bypassed replacement")
	}
	if _, err := tree.Supersede(root, "NEW", "OLD", now); err == nil {
		t.Fatal("cycle allowed")
	}
	put("ANOTHER", "doug", "AU")
	if _, err := tree.Supersede(root, "ANOTHER", "NEW", now); err == nil {
		t.Fatal("two predecessors allowed")
	}
	restored, err := tree.UndoSupersession(root, "OLD", now)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Retired || restored.SupersededBy != "" || restored.Current() != old.Current() || len(restored.History) != 2 {
		t.Fatalf("bad undo: %+v", restored)
	}
	if _, err := tree.Supersede(root, "ANOTHER", "NEW", now); err != nil {
		t.Fatal(err)
	}
}
