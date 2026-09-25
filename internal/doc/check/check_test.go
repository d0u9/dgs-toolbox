package check

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

func importOne(t *testing.T, root, name, owner string) tree.Item {
	t.Helper()
	source := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(source, []byte("%PDF "+owner), 0o644); err != nil {
		t.Fatal(err)
	}
	templates, err := tree.LoadTemplates(root)
	if err != nil {
		t.Fatal(err)
	}
	item, err := tree.Import(context.Background(), tree.ImportRequest{
		Root: root, Source: source, Template: templates[0], Now: time.Now(),
		Fields: map[string]string{"owner": owner, "country": "AU"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func kinds(r Report) map[Kind]int {
	out := map[Kind]int{}
	for _, p := range r.Problems {
		out[p.Kind]++
	}
	return out
}

func TestCleanTree(t *testing.T) {
	root := t.TempDir()
	if err := tree.Init(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	importOne(t, root, "a.pdf", "jane")
	var seen int
	r, err := Tree(context.Background(), root, func(string, int, int) { seen++ })
	if err != nil || len(r.Problems) != 0 || r.Checked != 1 || seen != 1 || r.Items != 1 {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestEveryProblem(t *testing.T) {
	root := t.TempDir()
	if err := tree.Init(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	changed := importOne(t, root, "a.pdf", "jane")
	missing := importOne(t, root, "b.pdf", "tom")
	odd := importOne(t, root, "c.pdf", "ann")

	os.WriteFile(tree.PDFPath(root, changed.ID, changed.Head), []byte("altered"), 0o644)
	os.Remove(tree.PDFPath(root, missing.ID, missing.Head))
	os.WriteFile(filepath.Join(tree.Dir(root, odd.ID), "stray.pdf"), []byte("x"), 0o644)
	odd.Type, odd.Head = "passport", "0000"
	if err := tree.WriteItem(root, odd); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(root, tree.ItemsDir, "BROKEN")
	os.MkdirAll(broken, 0o755)
	os.WriteFile(filepath.Join(broken, tree.SidecarName), []byte("id: [unclosed"), 0o644)

	r, err := Tree(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[Kind]int{Changed: 1, Missing: 1, Unnamed: 1, UnknownType: 1, BadHead: 1, BadSidecar: 1}
	got := kinds(r)
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: got %d, want %d (%+v)", k, got[k], n, r.Problems)
		}
	}
}

func TestNotATree(t *testing.T) {
	if _, err := Tree(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatal("checked a folder with no marker")
	}
}
