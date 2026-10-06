package tree

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// A PDF added without New revision corrects HEAD's revision; with it, it
// begins one. Editing fields corrects the revision edited.
func TestOnlyTheOwnerBeginsARevision(t *testing.T) {
	root := newTree(t)
	item := importOne(t, root, "old.pdf", "%PDF old", map[string]string{"owner": "emma", "country": "AU"})
	first := item.Head
	rescan := write(t, filepath.Join(root, "rescan.pdf"), "%PDF rescan")
	item, err := testStoreAt(root).AddRevisionWithMetadata(context.Background(), item.ID, rescan, idCard(t, root), nil, RevisionMetadata{}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := item.Papers(); len(item.Revisions) != 2 || !slices.Equal(got, []string{first}) {
		t.Fatalf("a rescan is a correction: papers %v of %d snapshots", got, len(item.Revisions))
	}
	renewed := write(t, filepath.Join(root, "new.pdf"), "%PDF new")
	item, err = testStoreAt(root).AddRevisionWithMetadata(context.Background(), item.ID, renewed, idCard(t, root), nil, RevisionMetadata{NewRevision: true}, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := item.Papers(); len(got) != 2 || got[1] != item.Head || item.RevisionNumber(item.Head) != 2 {
		t.Fatalf("a renewal begins revision 2: papers %v head %s", got, item.Head)
	}
	fields := maps.Clone(item.CurrentFields())
	fields["country"] = "CN"
	item, err = testStoreAt(root).SetFields(item.ID, "", idCard(t, root), fields, now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if item.RevisionNumber(item.Head) != 2 || len(item.Papers()) != 2 {
		t.Fatalf("an edit keeps its revision: number %d papers %v", item.RevisionNumber(item.Head), item.Papers())
	}
	again, err := testStoreAt(root).GetItem(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range again.Revisions {
		if r.Paper == "" {
			t.Fatalf("snapshot %s was written without its paper", r.Ref())
		}
	}
}

func TestMakeRevisionAndMakeCorrectionUndoEachOther(t *testing.T) {
	root := newTree(t)
	item := importOne(t, root, "old.pdf", "%PDF old", map[string]string{"owner": "emma", "country": "AU"})
	rescan := write(t, filepath.Join(root, "rescan.pdf"), "%PDF rescan")
	item, err := testStoreAt(root).AddRevision(context.Background(), item.ID, rescan, idCard(t, root), nil, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second := item.Head
	if _, err := testStoreAt(root).MakeCorrection(item.ID, second, now); err == nil {
		t.Fatal("a correction made a correction again")
	}
	item, err = testStoreAt(root).MakeRevision(item.ID, second, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Papers()) != 2 || item.Head != second {
		t.Fatalf("make revision: papers %v head %s", item.Papers(), item.Head)
	}
	if _, err := testStoreAt(root).MakeCorrection(item.ID, item.Papers()[0], now); err == nil {
		t.Fatal("the first revision joined none")
	}
	item, err = testStoreAt(root).MakeCorrection(item.ID, second, now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Papers()) != 1 || item.History[len(item.History)-1].Action != "make_correction" {
		t.Fatalf("make correction: papers %v", item.Papers())
	}
}

// A sidecar without papers is read by its history: imports begin, edits
// correct the revision HEAD was on, make_head moves that.
func TestPapersAreReadFromHistory(t *testing.T) {
	item := Item{Kind: KindDocument}
	for _, ref := range []string{"a", "b", "c", "d", "e", "f"} {
		item.Revisions = append(item.Revisions, Revision{ID: ref, Snapshot: true})
	}
	for _, e := range [][2]string{{"import", "a"}, {"edit_fields", "b"}, {"import_revision", "c"}, {"change_type", "d"}, {"make_head", "a"}, {"attach_pdf", "e"}} {
		item.History = append(item.History, HistoryEvent{Action: e[0], Digest: e[1]})
	}
	got := item.papers()
	want := map[string]string{"a": "a", "b": "a", "c": "c", "d": "c", "e": "a", "f": "f"}
	if !maps.Equal(got, want) {
		t.Fatalf("papers = %v, want %v", got, want)
	}
	item.Head = "b"
	latest := item.Latest()
	if !latest["b"] || !latest["d"] || !latest["f"] || len(latest) != 3 {
		t.Fatalf("latest = %v: HEAD for its own revision, the newest for the rest", latest)
	}
	if n := item.RevisionNumber("e"); n != 1 {
		t.Fatalf("revision number of e = %d", n)
	}
}

func TestARecordHasOneRevision(t *testing.T) {
	item := Item{Kind: KindRecord, History: []HistoryEvent{{Action: "import", Digest: "a"}, {Action: "import_revision", Digest: "b"}},
		Revisions: []Revision{{ID: "a", Snapshot: true}, {ID: "b", Snapshot: true, Paper: "b"}}}
	if got := item.Papers(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("papers = %v", got)
	}
	if correcting(item, "a", true) != "a" {
		t.Fatal("a record began a revision")
	}
}
