package tree_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/cases"
	"dgs-toolbox/internal/doc/check"
	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/merge"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

func TestSnapshotLifecycle(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	must(tree.Init(root, now))
	data := []byte("type: bank_card\nkind: document\nfields:\n  - key: owner\n    required: true\n    distinguishing: true\n  - key: card_name\n    required: true\n    distinguishing: true\n  - key: expires\n    type: month\n    per_revision: true\n")
	must(os.WriteFile(filepath.Join(root, tree.TemplatesDir, "bank_card.yaml"), data, 0600))
	tpl, err := tree.ParseTemplate(data)
	must(err)
	req := tree.ImportRequest{Root: root, Template: tpl, Fields: map[string]string{"owner": "doug", "card_name": "A", "expires": "2026-09"}, Now: now, Notes: "mobile", Tags: []string{"bank"}}
	item, err := tree.CreateWithoutPDF(req)
	must(err)
	initial := item.Current()
	if initial != tree.SnapshotID("bank_card", req.Fields, "") {
		t.Fatal("initial ID does not describe fields")
	}
	archived := cases.Case{Name: "cards", Status: cases.Open, Entries: []cases.Entry{{Item: item.ID}}}
	must(archived.Archive([]tree.Item{item}, now))
	if _, err := tree.CreateWithoutPDF(req); !errors.Is(err, tree.ErrTaken) {
		t.Fatalf("duplicate: %v", err)
	}
	report, err := check.Tree(context.Background(), root, nil)
	must(err)
	if len(report.Problems) != 0 || report.Checked != 0 {
		t.Fatalf("verify: %+v", report)
	}
	item, err = tree.AddWithoutPDF(root, item.ID, tpl, map[string]string{"expires": "2030-09"}, tree.RevisionMetadata{}, now.Add(time.Hour))
	must(err)
	renewal := item.Current()
	source := filepath.Join(t.TempDir(), "card.pdf")
	must(os.WriteFile(source, []byte("%PDF-1.4 test scan"), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tree.AttachPDF(ctx, root, item.ID, initial, source, now); err == nil {
		t.Fatal("cancelled attachment succeeded")
	}
	item, err = tree.AttachPDF(context.Background(), root, item.ID, initial, source, now.Add(2*time.Hour))
	must(err)
	attached := item.Current()
	blob := item.CurrentDigest()
	if len(item.Revisions) != 3 || attached == initial || attached == renewal || item.Revisions[0].Digest != "" ||
		item.FieldsAt(initial)["expires"] != "2026-09" || item.FieldsAt(renewal)["expires"] != "2030-09" ||
		item.CurrentFields()["expires"] != "2026-09" || item.Notes != "mobile" {
		t.Fatalf("snapshots: %+v", item)
	}
	selected, missing := archived.Items([]tree.Item{item})
	if len(missing) != 0 || selected[0].Current() != initial || selected[0].CurrentDigest() != "" {
		t.Fatal("Case moved to attached snapshot")
	}
	same, err := tree.AttachPDF(context.Background(), root, item.ID, attached, source, now)
	must(err)
	if len(same.Revisions) != 3 || len(same.History) != len(item.History) {
		t.Fatal("same PDF created a snapshot")
	}
	item, err = tree.SetFields(root, item.ID, attached, tpl, map[string]string{"owner": "doug", "card_name": "A", "expires": "2028-02"}, now.Add(3*time.Hour))
	must(err)
	edited := item.Current()
	if len(item.Revisions) != 4 || item.CurrentDigest() != blob || item.FieldsAt(attached)["expires"] != "2026-09" {
		t.Fatal("field edit mutated old snapshot")
	}
	same, err = tree.SetFields(root, item.ID, edited, tpl, item.CurrentFields(), now)
	must(err)
	if len(same.Revisions) != 4 || len(same.History) != len(item.History) {
		t.Fatal("unchanged fields made history")
	}
	item, err = tree.SetNotes(root, item.ID, "new note", now)
	must(err)
	item, err = tree.SetTags(root, item.ID, []string{"new tag"}, now)
	must(err)
	if item.Current() != edited || len(item.Revisions) != 4 {
		t.Fatal("metadata made a snapshot")
	}
	// Same PDF, different fields exports distinct snapshot identities and values.
	v := view.View{Name: "cards", Selection: view.All, Layout: "{revision}-{expires}.{ext}"}
	plan, err := view.Build(v, []tree.Item{item})
	must(err)
	if len(plan.Files) != 2 {
		t.Fatalf("export plan: %+v", plan)
	}
	target := t.TempDir()
	ep, err := export.Compute(context.Background(), target, []string{"cards"}, plan.Files)
	must(err)
	_, err = export.Apply(context.Background(), root, target, []string{"cards"}, ep, []tree.Item{item}, nil, nil)
	must(err)
	back, err := merge.FromTarget(target, now)
	must(err)
	if len(back.Items) != 1 || len(back.Items[0].Revisions) != 2 || back.Items[0].Current() != edited ||
		back.Items[0].FieldsAt(attached)["expires"] != "2026-09" || back.Items[0].CurrentFields()["expires"] != "2028-02" {
		t.Fatalf("export roundtrip: %+v", back.Items)
	}
	destination := t.TempDir()
	must(tree.Init(destination, now))
	src, err := merge.FromTree(root)
	must(err)
	mp, err := merge.Compute(destination, src)
	must(err)
	if len(mp.Problems) > 0 || len(mp.Conflicts) > 0 {
		t.Fatalf("merge: %+v", mp)
	}
	_, err = merge.Apply(context.Background(), destination, src, mp, nil, now)
	must(err)
	copied, _, err := tree.FindItem(destination, item.ID)
	must(err)
	if len(copied.Revisions) != 4 || copied.Current() != edited {
		t.Fatal("merge lost snapshots")
	}
	// A later snapshot merges without confusing HEAD's derived fields with
	// independently edited Item metadata.
	item, err = tree.SetFields(root, item.ID, item.Current(), tpl, map[string]string{"owner": "doug", "card_name": "A", "expires": "2033-03"}, now.Add(4*time.Hour))
	must(err)
	src, err = merge.FromTree(root)
	must(err)
	mp, err = merge.Compute(destination, src)
	must(err)
	if len(mp.Problems) > 0 || len(mp.Conflicts) > 0 {
		t.Fatalf("incremental merge: %+v", mp)
	}
	_, err = merge.Apply(context.Background(), destination, src, mp, nil, now)
	must(err)
	copied, _, err = tree.FindItem(destination, item.ID)
	must(err)
	if copied.Current() != item.Current() || copied.CurrentFields()["expires"] != "2033-03" {
		t.Fatal("merge lost current snapshot")
	}
	// Deleting a shared-blob revision saves metadata, not the still-used PDF.
	item, trash, err := tree.TrashRevision(root, item.ID, attached, now)
	must(err)
	if _, err := os.Stat(trash); err != nil {
		t.Fatal("no recoverable metadata")
	}
	digest, err := tree.FileDigest(tree.PDFPath(root, item.ID, blob))
	must(err)
	if digest != blob {
		t.Fatal("deleted another snapshot's PDF")
	}
	item, err = tree.SetHead(root, item.ID, initial, now)
	must(err)
	if item.CurrentFields()["expires"] != "2026-09" || item.CurrentDigest() != "" {
		t.Fatal("HEAD did not restore fields")
	}
	must(os.Remove(tree.PDFPath(root, item.ID, blob)))
	report, err = check.Tree(context.Background(), root, nil)
	must(err)
	if len(report.Problems) != 1 || report.Problems[0].Kind != check.Missing {
		t.Fatalf("missing PDF: %+v", report)
	}
}

func TestSnapshotHashAndLegacyFreeze(t *testing.T) {
	a := map[string]string{"owner": "A", "expires": "2030-01"}
	b := map[string]string{"expires": "2030-01", "owner": "A", "blank": ""}
	if tree.SnapshotID("card", a, "pdf") != tree.SnapshotID("card", b, "pdf") {
		t.Fatal("map order affects ID")
	}
	if tree.SnapshotID("card", a, "pdf") == tree.SnapshotID("card", a, "other") {
		t.Fatal("PDF missing from hash")
	}
	root := t.TempDir()
	now := time.Now()
	if err := tree.Init(root, now); err != nil {
		t.Fatal(err)
	}
	tpl := tree.Template{Type: "card", Kind: tree.KindDocument, Fields: []tree.Field{{Key: "owner"}, {Key: "expires", PerRevision: true}}}
	old := tree.Item{ID: "legacy", Type: "card", Kind: tree.KindDocument, Fields: map[string]string{"owner": "A"}, Head: "old", Revisions: []tree.Revision{{Digest: "old", Fields: map[string]string{"expires": "2030-01"}}}}
	if err := tree.WriteItem(root, old); err != nil {
		t.Fatal(err)
	}
	next, err := tree.SetFields(root, old.ID, "old", tpl, map[string]string{"owner": "B"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.FieldsAt("old")["owner"] != "A" || next.FieldsAt("old")["expires"] != "2030-01" || next.CurrentFields()["expires"] != "" || len(next.Revisions) != 2 {
		t.Fatalf("freeze: %+v", next)
	}
}

func TestHeadCannotRestoreTakenIdentity(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(tree.Init(root, now))
	templates, err := tree.LoadTemplates(root)
	must(err)
	tpl := templates[0]
	first, err := tree.CreateWithoutPDF(tree.ImportRequest{Root: root, Template: tpl, Now: now, Fields: map[string]string{"owner": "A", "country": "AU"}})
	must(err)
	old := first.Current()
	edited, err := tree.SetFields(root, first.ID, old, tpl, map[string]string{"owner": "B", "country": "AU"}, now)
	must(err)
	_, err = tree.CreateWithoutPDF(tree.ImportRequest{Root: root, Template: tpl, Now: now, Fields: map[string]string{"owner": "A", "country": "AU"}})
	must(err)
	if _, err := tree.SetHead(root, first.ID, old, now); !errors.Is(err, tree.ErrTaken) {
		t.Fatalf("restored conflicting identity: %v", err)
	}
	saved, _, err := tree.FindItem(root, first.ID)
	must(err)
	if saved.Current() != edited.Current() {
		t.Fatal("failed HEAD change wrote the sidecar")
	}
	saved.Revisions[len(saved.Revisions)-1].Fields["owner"] = "tampered"
	must(tree.WriteItem(root, saved))
	report, err := check.Tree(context.Background(), root, nil)
	must(err)
	if len(report.Problems) != 1 || report.Problems[0].Kind != check.BadSidecar {
		t.Fatalf("snapshot hash mismatch not detected: %+v", report)
	}
}
