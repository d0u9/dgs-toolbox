package merge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

const passport = `type: passport
kind: document
fields:
  - key: owner
    required: true
    distinguishing: true
  - key: number
`

const bill = `type: bill
kind: record
fields:
  - key: owner
`

var now = time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

func newTree(t *testing.T, templates ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := tree.Init(root, now); err != nil {
		t.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(root, tree.TemplatesDir, "*.yaml"))
	for _, path := range old {
		os.Remove(path)
	}
	for _, data := range templates {
		name, _, _ := strings.Cut(strings.TrimPrefix(data, "type: "), "\n")
		if err := os.WriteFile(filepath.Join(root, tree.TemplatesDir, name+".yaml"), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// put writes an Item whose revisions hold contents, HEAD on the last.
func put(t *testing.T, root string, item tree.Item, contents ...string) tree.Item {
	t.Helper()
	for _, c := range contents {
		sum := sha256.Sum256([]byte(c))
		d := hex.EncodeToString(sum[:])
		if err := os.MkdirAll(tree.Dir(root, item.ID), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tree.PDFPath(root, item.ID, d), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		item.Revisions = append(item.Revisions, tree.Revision{Digest: d, Added: now.Format(time.RFC3339)})
	}
	if item.Kind == tree.KindDocument {
		item.Head = item.Revisions[len(item.Revisions)-1].Digest
	}
	if err := tree.WriteItem(root, item); err != nil {
		t.Fatal(err)
	}
	return item
}

func digest(c string) string {
	sum := sha256.Sum256([]byte(c))
	return hex.EncodeToString(sum[:])
}

func plan(t *testing.T, root string, src Source) Plan {
	t.Helper()
	p, err := Compute(root, src)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func source(t *testing.T, root string) Source {
	t.Helper()
	src, err := FromTree(root)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestMergeMatchesAndAdds(t *testing.T) {
	full := newTree(t, passport, bill)
	sub := newTree(t, passport, bill)
	put(t, full, tree.Item{ID: "F1", Type: "passport", Kind: tree.KindDocument, Fields: map[string]string{"owner": "jane"}, Tags: []string{"home"}}, "old passport")
	put(t, full, tree.Item{ID: "F2", Type: "bill", Kind: tree.KindRecord, Fields: map[string]string{"owner": "jane"}}, "bill 1")
	// The sub-tree has the renewed passport, the same bill, and a new bill.
	put(t, sub, tree.Item{ID: "S1", Type: "passport", Kind: tree.KindDocument, Fields: map[string]string{"owner": "JANE"}, Notes: "renewed in Sydney", Tags: []string{"travel"}, Frequent: true, Retired: true, RetiredReason: "moved"}, "new passport")
	put(t, sub, tree.Item{ID: "S2", Type: "bill", Kind: tree.KindRecord, Fields: map[string]string{"owner": "jane"}}, "bill 1")
	put(t, sub, tree.Item{ID: "S3", Type: "bill", Kind: tree.KindRecord, Fields: map[string]string{"owner": "tom"}}, "bill 2")
	if err := view.Save(sub, view.View{Name: "all", Selection: view.Head, Layout: "{owner}/{type}.{ext}"}); err != nil {
		t.Fatal(err)
	}

	src := source(t, sub)
	p := plan(t, full, src)
	if len(p.New) != 1 || p.New[0].Item != "S3" || len(p.Changed) != 1 || p.Same != 1 || len(p.Views) != 1 || len(p.Conflicts) != 1 {
		t.Fatalf("plan: %+v", p)
	}
	c := p.Changed[0]
	if c.Item != "F1" || c.Head != digest("new passport") || len(c.Digests) != 1 || c.Notes != "renewed in Sydney" || len(c.Tags) != 1 || c.Tags[0] != "travel" || !c.Frequent || !c.Retired || c.RetiredReason != "moved" {
		t.Fatalf("change: %+v", c)
	}
	// owner differs in case only in the match, but the fields still differ.
	if len(p.Conflicts) != 1 || p.Conflicts[0].ID != "fields:F1" {
		t.Fatalf("conflicts: %+v", p.Conflicts)
	}
	if _, err := Apply(context.Background(), full, src, p, nil, time.Now()); err == nil {
		t.Fatal("applied without a choice")
	}
	result, err := Apply(context.Background(), full, src, p, map[string]string{"fields:F1": Ours}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.PDFs != 2 || result.Views != 1 {
		t.Fatalf("result: %+v", result)
	}
	f1, _, err := tree.FindItem(full, "F1")
	if err != nil || f1.Head != digest("new passport") || len(f1.Revisions) != 2 || f1.Fields["owner"] != "jane" || f1.Notes != "renewed in Sydney" || len(f1.Tags) != 2 || f1.Tags[0] != "home" || f1.Tags[1] != "travel" {
		t.Fatalf("F1: %v %+v", err, f1)
	}
	if !f1.Frequent || !f1.Retired || f1.RetiredReason != "moved" || len(f1.History) != 1 || f1.History[0].Action != "merge" || f1.History[0].At != now.Format(time.RFC3339) {
		t.Fatalf("F1 history: %+v", f1)
	}
	for _, key := range []string{"revisions", "head", "notes", "tags", "frequent", "retired"} {
		if _, ok := f1.History[0].Changes[key]; !ok {
			t.Errorf("merge event lacks %s: %+v", key, f1.History[0].Changes)
		}
	}
	s3, _, err := tree.FindItem(full, "S3")
	if err != nil {
		t.Fatal(err)
	}
	if len(s3.History) != 1 || s3.History[0].Action != "merge" {
		t.Fatalf("S3 history: %+v", s3.History)
	}

	// Merging again changes nothing.
	p = plan(t, full, src)
	if len(p.New)+len(p.Changed) != 0 || len(p.Views) != 0 {
		t.Fatalf("second plan: %+v", p)
	}
}

func TestHeadMovedOnBothSides(t *testing.T) {
	full := newTree(t, passport)
	sub := newTree(t, passport)
	fields := map[string]string{"owner": "jane"}
	put(t, full, tree.Item{ID: "F1", Type: "passport", Kind: tree.KindDocument, Fields: fields}, "a", "b")
	put(t, sub, tree.Item{ID: "S1", Type: "passport", Kind: tree.KindDocument, Fields: fields}, "a", "c")
	src := source(t, sub)
	p := plan(t, full, src)
	if len(p.Conflicts) != 1 || p.Conflicts[0].Kind != ConflictHead {
		t.Fatalf("plan: %+v", p)
	}
	if _, err := Apply(context.Background(), full, src, p, map[string]string{"head:F1": Theirs}, time.Now()); err != nil {
		t.Fatal(err)
	}
	f1, _, _ := tree.FindItem(full, "F1")
	if f1.Head != digest("c") || len(f1.Revisions) != 3 {
		t.Fatalf("F1: %+v", f1)
	}
}

func TestHeads(t *testing.T) {
	item := func(head string, ds ...string) tree.Item {
		it := tree.Item{Kind: tree.KindDocument, Head: head}
		for _, d := range ds {
			it.Revisions = append(it.Revisions, tree.Revision{Digest: d})
		}
		return it
	}
	for _, c := range []struct {
		ours, theirs tree.Item
		head         string
		conflict     bool
	}{
		{item("a", "a"), item("b", "a", "b"), "b", false},
		{item("b", "a", "b"), item("a", "a"), "b", false},
		{item("a", "a", "b"), item("b", "a", "b"), "", true},
		{item("b", "a", "b"), item("c", "a", "c"), "", true},
		{item("a", "a"), item("c", "c"), "c", false},
	} {
		head, conflict := heads(c.ours, c.theirs)
		if head != c.head || conflict != c.conflict {
			t.Errorf("heads(%v, %v) = %s %v", c.ours, c.theirs, head, conflict)
		}
	}
}

func TestTemplateConflictAndUnknownType(t *testing.T) {
	full := newTree(t, passport)
	sub := newTree(t, passport+"  - key: country\n")
	src := source(t, sub)
	p := plan(t, full, src)
	if len(p.Conflicts) != 1 || p.Conflicts[0].ID != "template:passport" {
		t.Fatalf("plan: %+v", p)
	}
	if _, err := Apply(context.Background(), full, src, p, map[string]string{"template:passport": Theirs}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(full, tree.TemplatesDir, "passport.yaml")); string(data) != passport+"  - key: country\n" {
		t.Fatalf("template: %s", data)
	}

	src.Items = []tree.Item{{ID: "X", Type: "visa", Kind: tree.KindRecord}}
	if p := plan(t, full, src); len(p.Problems) != 1 {
		t.Fatalf("unknown type: %+v", p)
	}
}

func TestImportBackFromTarget(t *testing.T) {
	full := newTree(t, passport)
	it := put(t, full, tree.Item{ID: "F1", Type: "passport", Kind: tree.KindDocument, Fields: map[string]string{"owner": "jane"}}, "a", "b")
	target := t.TempDir()
	files := []view.File{
		{Path: "jane/1.pdf", Item: "F1", Digest: digest("a"), Revision: 1},
		{Path: "jane/2.pdf", Item: "F1", Digest: digest("b"), Revision: 2},
	}
	ep, err := export.Compute(context.Background(), target, []string{"all"}, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := export.Apply(context.Background(), full, target, []string{"all"}, ep, []tree.Item{it}, nil, nil); err != nil {
		t.Fatal(err)
	}

	// Into an empty tree: the Item comes back with both revisions and HEAD.
	empty := newTree(t, passport)
	src, err := FromTarget(target, now)
	if err != nil {
		t.Fatal(err)
	}
	p := plan(t, empty, src)
	if len(p.New) != 1 {
		t.Fatalf("plan: %+v", p)
	}
	if _, err := Apply(context.Background(), empty, src, p, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	back, _, err := tree.FindItem(empty, "F1")
	if err != nil || len(back.Revisions) != 2 || back.Head != digest("b") || back.Fields["owner"] != "jane" {
		t.Fatalf("back: %v %+v", err, back)
	}
	// Into the tree it came from: nothing to do.
	if p := plan(t, full, src); len(p.New)+len(p.Changed)+len(p.Conflicts) != 0 || p.Same != 1 {
		t.Fatalf("same tree: %+v", p)
	}
}
