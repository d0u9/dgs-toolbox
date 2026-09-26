package tree_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

// cardTree is a tree with one document Template: owner names the card, and
// number changes with each revision.
func cardTree(t *testing.T) (string, tree.Template) {
	t.Helper()
	root := t.TempDir()
	if err := tree.Init(root, time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	data := []byte("type: card\nkind: document\nfields:\n  - key: owner\n    required: true\n    distinguishing: true\n  - key: number\n    per_revision: true\n")
	if err := os.WriteFile(filepath.Join(root, tree.TemplatesDir, "card.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	tpl, err := tree.ParseTemplate(data)
	if err != nil {
		t.Fatal(err)
	}
	return root, tpl
}

func scan(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scan.pdf")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A renewal must not publish a snapshot another document already names,
// even when the sidecar came to collide outside dgs (a hand edit, a merge).
func TestRenewalCannotTakeAnotherDocumentsIdentity(t *testing.T) {
	root, tpl := cardTree(t)
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	if _, err := tree.CreateWithoutPDF(tree.ImportRequest{Root: root, Template: tpl, Fields: map[string]string{"owner": "alex", "number": "1"}, Now: now}); err != nil {
		t.Fatal(err)
	}
	second, err := tree.CreateWithoutPDF(tree.ImportRequest{Root: root, Template: tpl, Fields: map[string]string{"owner": "ann", "number": "2"}, Now: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	second.Fields["owner"] = "alex"
	second.Revisions[0].Fields["owner"] = "alex"
	if err := tree.WriteItem(root, second); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.AddWithoutPDF(root, second.ID, tpl, map[string]string{"number": "3"}, tree.RevisionMetadata{}, now.Add(time.Hour)); !errors.Is(err, tree.ErrTaken) {
		t.Fatalf("AddWithoutPDF: %v", err)
	}
	source := scan(t, "%PDF card 3")
	if _, err := tree.AddRevision(context.Background(), root, second.ID, source, tpl, map[string]string{"number": "3"}, now.Add(time.Hour)); !errors.Is(err, tree.ErrTaken) {
		t.Fatalf("AddRevision: %v", err)
	}
	digest, err := tree.FileDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tree.PDFPath(root, second.ID, digest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refused revision left its PDF: %v", err)
	}
}

func TestReturningToSnapshotKeepsItsRevisionTags(t *testing.T) {
	root, tpl := cardTree(t)
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	item, err := tree.CreateWithoutPDF(tree.ImportRequest{Root: root, Template: tpl, Fields: map[string]string{"owner": "alex", "number": "1"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	source := scan(t, "%PDF renewal")
	item, err = tree.AddRevisionWithMetadata(context.Background(), root, item.ID, source, tpl, map[string]string{"number": "2"}, tree.RevisionMetadata{RevisionTags: []string{"original"}}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	ref := item.Current()
	notes := "edited"
	item, err = tree.AddRevisionWithMetadata(context.Background(), root, item.ID, source, tpl, map[string]string{"number": "2"}, tree.RevisionMetadata{Notes: &notes}, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if item.Current() != ref {
		t.Fatalf("HEAD %s, want reused %s", item.Current(), ref)
	}
	r, _ := item.Revision(ref)
	if !slices.Equal(r.Tags, []string{"original"}) {
		t.Fatalf("revision tags %v, want kept", r.Tags)
	}
	item, err = tree.AddRevisionWithMetadata(context.Background(), root, item.ID, source, tpl, map[string]string{"number": "2"}, tree.RevisionMetadata{RevisionTags: []string{"added"}}, now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r, _ = item.Revision(ref)
	if !slices.Equal(r.Tags, []string{"added", "original"}) && !slices.Equal(r.Tags, []string{"original", "added"}) {
		t.Fatalf("revision tags %v, want both", r.Tags)
	}
}

func TestSnapshotsSharingPDFKeepTheirOwnTagsAndFields(t *testing.T) {
	root, tpl := cardTree(t)
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	item, err := tree.Import(context.Background(), tree.ImportRequest{Root: root, Template: tpl, Source: scan(t, "%PDF shared"), Fields: map[string]string{"owner": "alex", "number": "1"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	first := item.Current()
	pdf := item.CurrentDigest()
	if _, err := tree.SetRevisionTags(root, item.ID, first, []string{"old-number"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	item, err = tree.SetFields(root, item.ID, first, tpl, map[string]string{"owner": "alex", "number": "9"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second := item.Current()
	if second == first || item.CurrentDigest() != pdf {
		t.Fatal("edit did not make a second snapshot of the same PDF")
	}
	if got := item.TagsAt(second); slices.Contains(got, "old-number") {
		t.Fatalf("second snapshot has the first one's tag: %v", got)
	}
	if got := item.TagsAt(pdf); slices.Contains(got, "old-number") {
		t.Fatalf("PDF digest resolves to a snapshot's tags: %v", got)
	}
	if got := item.FieldsAt(first)["number"]; got != "1" {
		t.Fatalf("first snapshot number %q", got)
	}
}

func TestSupersessionOwnerIgnoresSurroundingSpace(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	if err := tree.Init(root, now); err != nil {
		t.Fatal(err)
	}
	old := tree.Item{ID: "old", Type: "visa", Kind: tree.KindDocument, Fields: map[string]string{"owner": "alex ", "country": "AU"}}
	if err := tree.WriteItem(root, old); err != nil {
		t.Fatal(err)
	}
	if err := tree.ValidateSupersession(root, "old", "visa", map[string]string{"owner": " alex", "country": "AU"}); err != nil {
		t.Fatal(err)
	}
}
