package tree_test

import (
	"dgs-toolbox/internal/doc/tree"
	"os"
	"testing"
	"time"
)

func TestVisaSupersession(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	if err := tree.Init(root, now); err != nil {
		t.Fatal(err)
	}
	visa := "type: visa\nkind: record\nfields:\n  - key: owner\n    required: true\n  - key: country\n    type: country\n"
	if err := os.WriteFile(testStore(t, root).TemplatePath("visa"), []byte(visa), 0600); err != nil {
		t.Fatal(err)
	}
	put := func(id, owner, country string) tree.Item {
		it := tree.Item{ID: id, Type: "visa", Kind: tree.KindRecord, Fields: map[string]string{"owner": owner, "country": country}, Revisions: []tree.Revision{{ID: "snapshot-" + id, Snapshot: true, Fields: map[string]string{"number": id}}}}
		if err := testStore(t, root).WriteItem(it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	old := put("OLD", "alex", "澳大利亚")
	put("NEW", "alex", "AU")
	put("OTHER", "emma", "AU")
	put("COUNTRY", "alex", "CN")
	for _, id := range []string{"OLD", "OTHER", "COUNTRY", "missing"} {
		if _, err := testStore(t, root).Supersede("OLD", id, now); err == nil {
			t.Fatalf("accepted invalid successor %s", id)
		}
	}
	updated, err := testStore(t, root).Supersede("OLD", "NEW", now)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Retired || updated.SupersededBy != "NEW" || updated.Current() != old.Current() || len(updated.Revisions) != 1 {
		t.Fatalf("bad replacement: %+v", updated)
	}
	if _, err := testStore(t, root).SetRetired("OLD", false, "", now); err == nil {
		t.Fatal("unretire bypassed replacement")
	}
	if _, err := testStore(t, root).Supersede("NEW", "OLD", now); err == nil {
		t.Fatal("cycle allowed")
	}
	put("ANOTHER", "alex", "AU")
	if _, err := testStore(t, root).Supersede("ANOTHER", "NEW", now); err == nil {
		t.Fatal("two predecessors allowed")
	}
	restored, err := testStore(t, root).UndoSupersession("OLD", now)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Retired || restored.SupersededBy != "" || restored.Current() != old.Current() || len(restored.History) != 2 {
		t.Fatalf("bad undo: %+v", restored)
	}
	if _, err := testStore(t, root).Supersede("ANOTHER", "NEW", now); err != nil {
		t.Fatal(err)
	}
}
