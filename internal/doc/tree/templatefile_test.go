package tree

import (
	"os"
	"testing"
	"time"
)

func TestSaveAndTrashTemplate(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	if err := Init(root, now); err != nil {
		t.Fatal(err)
	}
	bill := "# bills\ntype: bill\nkind: record\nfields:\n  - key: owner\n"
	if _, err := SaveTemplate(root, "", []byte(bill), now); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(TemplatePath(root, "bill")); string(data) != bill {
		t.Fatalf("written as typed: %q", data)
	}
	if _, err := SaveTemplate(root, "", []byte(bill), now); err == nil {
		t.Fatal("a second new bill was accepted")
	}
	if _, err := SaveTemplate(root, "", []byte("type: x\nkind: record\nfields: []\nbogus: 1\n"), now); err == nil {
		t.Fatal("an unknown key was accepted")
	}

	// In use: no rename, no change of kind, no trash.
	if err := WriteItem(root, Item{ID: "A", Type: "bill", Kind: KindRecord, Revisions: []Revision{{Digest: "d"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTemplate(root, "bill", []byte("type: invoice\nkind: record\nfields: []\n"), now); err == nil {
		t.Fatal("renamed a type in use")
	}
	if _, err := SaveTemplate(root, "bill", []byte("type: bill\nkind: document\nfields: []\n"), now); err == nil {
		t.Fatal("changed the kind of a type in use")
	}
	if _, err := SaveTemplate(root, "bill", []byte("type: bill\nkind: record\nfields:\n  - key: amount\n"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := TrashTemplate(root, "bill", now); err == nil {
		t.Fatal("trashed a type in use")
	}

	// Unused: renamed, and the old file goes to the trash.
	if _, err := SaveTemplate(root, "id_card", []byte("type: card\nkind: document\nfields: []\n"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(TemplatePath(root, "id_card")); !os.IsNotExist(err) {
		t.Fatal("the renamed Template's old file is still there")
	}
	to, err := TrashTemplate(root, "card", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(to); err != nil {
		t.Fatal(err)
	}
	templates, err := LoadTemplates(root)
	if err != nil || len(templates) != 1 || templates[0].Type != "bill" {
		t.Fatalf("left: %v %+v", err, templates)
	}
}
