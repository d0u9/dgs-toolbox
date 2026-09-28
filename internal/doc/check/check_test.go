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
	importOne(t, root, "a.pdf", "emma")
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
	changed := importOne(t, root, "a.pdf", "emma")
	missing := importOne(t, root, "b.pdf", "tom")
	odd := importOne(t, root, "c.pdf", "ann")

	os.WriteFile(tree.PDFPath(root, changed.ID, changed.CurrentDigest()), []byte("altered"), 0o644)
	os.Remove(tree.PDFPath(root, missing.ID, missing.CurrentDigest()))
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

func TestFilesBesideItems(t *testing.T) {
	root := t.TempDir()
	if err := tree.Init(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	item := importOne(t, root, "a.pdf", "emma")
	write := func(dir, name, body string) {
		os.MkdirAll(filepath.Join(root, dir), 0o755)
		if err := os.WriteFile(filepath.Join(root, dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("rules", "ids.yaml", "name: ids\nlayout: \"{type}.{ext}\"\n")
	write("rules", "broken.yaml", "name: [unclosed")
	write("snapshots", "visa.yaml", "name: visa\ntaken: 2026-01-02T03:04:05Z\nfiles:\n"+
		"  - {path: a.pdf, item: "+item.ID+", revision: "+item.Head+"}\n"+
		"  - {path: b.pdf, item: GONE, revision: 1111}\n"+
		"  - {path: c.pdf, item: "+item.ID+", revision: 0000}\n")
	write("outlines", "phone.yaml", "name: phone\nrules: [ids]\nsnapshots: [{name: visa}, {name: lost}]\n")
	write("outlines", "wrong.yaml", "name: other\nrules: [ids]\n")
	write("cases", "lease.yaml", "name: lease\nstatus: open\nopened: 2026-01-02T03:04:05Z\nentries:\n  - {item: GONE, added: 2026-01-02T03:04:05Z}\nneeds: []\n")

	r, err := Tree(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[Kind]int{BadFile: 2, Dangling: 4}
	got := kinds(r)
	if len(got) != len(want) {
		t.Errorf("kinds %v, want %v (%+v)", got, want, r.Problems)
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: got %d, want %d (%+v)", k, got[k], n, r.Problems)
		}
	}
}
