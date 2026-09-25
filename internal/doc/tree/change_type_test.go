package tree

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestChangeTypePreservesPDFsAndArchivesOldFields(t *testing.T) {
	root := newTree(t)
	at := time.Date(2026, 9, 26, 12, 30, 0, 0, time.UTC)
	item := Item{ID: "A", Type: "id_card", Kind: KindDocument, Fields: map[string]string{"owner": "doug", "old_number": "42"}, Head: "new",
		Revisions: []Revision{{Digest: "old", Fields: map[string]string{"issued": "2020-01-01"}}, {Digest: "new", Fields: map[string]string{"issued": "2024-01-01"}}}}
	if err := WriteItem(root, item); err != nil {
		t.Fatal(err)
	}
	for _, digest := range []string{"old", "new"} {
		if err := os.WriteFile(PDFPath(root, "A", digest), []byte(digest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	target := Template{Type: "student_id", Kind: KindDocument, Fields: []Field{{Key: "owner", Required: true, Distinguishing: true}, {Key: "school", Required: true}, {Key: "issued", PerRevision: true}}}
	fields := map[string]string{"owner": "doug", "school": "Uni"}
	revs := map[string]map[string]string{"old": {"issued": "2020-01-01"}, "new": {"issued": "2024-01-01"}}
	changed, err := ChangeType(root, "A", target, fields, revs, at)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID != "A" || changed.Type != "student_id" || changed.Head != "new" || len(changed.Revisions) != 2 || changed.Fields["old_number"] != "" {
		t.Fatalf("changed: %+v", changed)
	}
	if len(changed.History) != 1 || changed.History[0].At != at.Format(time.RFC3339) || changed.History[0].PreviousFields["old_number"] != "42" {
		t.Fatalf("history: %+v", changed.History)
	}
	for _, digest := range []string{"old", "new"} {
		if data, err := os.ReadFile(PDFPath(root, "A", digest)); err != nil || string(data) != digest {
			t.Fatalf("PDF %s: %q %v", digest, data, err)
		}
	}
	saved, _, err := FindItem(root, "A")
	if err != nil || !reflect.DeepEqual(saved, changed) {
		t.Fatalf("saved: %+v %v", saved, err)
	}
	if _, err := ChangeType(root, "A", target, fields, revs, at); err == nil {
		t.Fatal("same type accepted")
	}
}

func TestChangeTypeRejectsMissingRevisionField(t *testing.T) {
	root := newTree(t)
	item := Item{ID: "A", Type: "old", Kind: KindDocument, Head: "x", Revisions: []Revision{{Digest: "x"}}}
	if err := WriteItem(root, item); err != nil {
		t.Fatal(err)
	}
	target := Template{Type: "new", Kind: KindDocument, Fields: []Field{{Key: "number", Required: true, PerRevision: true}}}
	if _, err := ChangeType(root, "A", target, nil, nil, time.Now()); err == nil {
		t.Fatal("missing value accepted")
	}
	saved, _, err := FindItem(root, "A")
	if err != nil || saved.Type != "old" || len(saved.History) != 0 {
		t.Fatalf("changed after rejection: %+v %v", saved, err)
	}
}

func TestFieldHistoryHasTimeAndBeforeAfter(t *testing.T) {
	root := newTree(t)
	tpl := Template{Type: "card", Kind: KindDocument, Fields: []Field{{Key: "owner", Required: true, Distinguishing: true}, {Key: "number", PerRevision: true}}}
	item := Item{ID: "A", Type: "card", Kind: KindDocument, Fields: map[string]string{"owner": "doug"}, Head: "x", Revisions: []Revision{{Digest: "x", Fields: map[string]string{"number": "1"}}}}
	if err := WriteItem(root, item); err != nil {
		t.Fatal(err)
	}
	changed, err := SetFields(root, "A", "x", tpl, map[string]string{"owner": "doug", "number": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.History) != 1 || changed.History[0].Changes["number"] != [2]string{"1", "2"} {
		t.Fatalf("history: %+v", changed.History)
	}
	if _, err := time.Parse(time.RFC3339, changed.History[0].At); err != nil {
		t.Fatalf("time: %v", err)
	}
}
