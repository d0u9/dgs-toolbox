package tree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

func newTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := Init(root, now); err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func idCard(t *testing.T, root string) Template {
	t.Helper()
	templates, err := LoadTemplates(root)
	if err != nil || len(templates) != 1 || templates[0].Type != "id_card" {
		t.Fatalf("templates = %+v, %v", templates, err)
	}
	return templates[0]
}

func TestInitWritesMarkerAndExampleOnce(t *testing.T) {
	root := newTree(t)
	if err := Require(root); err != nil {
		t.Fatal(err)
	}
	idCard(t, root)
	if err := Init(root, now); err == nil {
		t.Fatal("second Init succeeded")
	}
	if err := Require(t.TempDir()); !errors.Is(err, ErrNotATree) {
		t.Fatalf("Require on a plain folder = %v", err)
	}
}

func TestLoadTemplatesRefusesBadOnes(t *testing.T) {
	for name, content := range map[string]string{
		"wrong_name.yaml": "type: other\nkind: record\n",
		"bad_kind.yaml":   "type: bad_kind\nkind: thing\n",
		"loose.yaml":      "type: loose\nkind: document\nfields:\n  - key: a\n    distinguishing: true\n",
		"unknown.yaml":    "type: unknown\nkind: record\ncolour: red\n",
	} {
		root := t.TempDir()
		write(t, filepath.Join(root, TemplatesDir, name), content)
		if _, err := LoadTemplates(root); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestImportMakesAnItemAndLeavesTheSource(t *testing.T) {
	root := newTree(t)
	source := write(t, filepath.Join(root, "scan.pdf"), "%PDF jane")
	item, err := Import(context.Background(), ImportRequest{
		Root: root, Source: source, Template: idCard(t, root), Now: now,
		Fields: map[string]string{"owner": " jane ", "country": "AU", "number": ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Head == "" || item.Head != item.Revisions[0].Digest || item.Fields["owner"] != "jane" {
		t.Fatalf("item = %+v", item)
	}
	if _, ok := item.Fields["number"]; ok {
		t.Error("empty field kept")
	}
	if data, err := os.ReadFile(PDFPath(root, item.ID, item.Head)); err != nil || string(data) != "%PDF jane" {
		t.Fatalf("stored PDF: %q %v", data, err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("source was touched")
	}
	items, err := LoadItems(root)
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("LoadItems = %+v, %v", items, err)
	}
}

func TestImportRefuses(t *testing.T) {
	root := newTree(t)
	tpl := idCard(t, root)
	first := write(t, filepath.Join(root, "a.pdf"), "%PDF a")
	if _, err := Import(context.Background(), ImportRequest{Root: root, Source: first, Template: tpl, Now: now,
		Fields: map[string]string{"owner": "jane", "country": "AU"}}); err != nil {
		t.Fatal(err)
	}
	other := write(t, filepath.Join(root, "b.pdf"), "%PDF b")
	cases := []struct {
		name   string
		source string
		fields map[string]string
		want   string
	}{
		{"same PDF", first, map[string]string{"owner": "doug", "country": "AU"}, "already in the tree"},
		{"same document", other, map[string]string{"owner": "Jane", "country": "au"}, "exists"},
		{"missing", other, map[string]string{"owner": "doug"}, "missing country"},
		{"unknown key", other, map[string]string{"owner": "doug", "country": "CN", "colour": "red"}, "no field colour"},
	}
	for _, c := range cases {
		_, err := Import(context.Background(), ImportRequest{Root: root, Source: c.source, Template: tpl, Now: now, Fields: c.fields})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
	// A different country is a different document.
	if _, err := Import(context.Background(), ImportRequest{Root: root, Source: other, Template: tpl, Now: now,
		Fields: map[string]string{"owner": "jane", "country": "CN"}}); err != nil {
		t.Fatal(err)
	}
}

func TestImportNeedsATree(t *testing.T) {
	root := t.TempDir()
	source := write(t, filepath.Join(root, "a.pdf"), "x")
	_, err := Import(context.Background(), ImportRequest{Root: root, Source: source, Template: Template{Type: "x", Kind: KindRecord}, Now: now})
	if !errors.Is(err, ErrNotATree) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewIDIsAULID(t *testing.T) {
	a, _ := NewID(now)
	b, _ := NewID(now.Add(time.Millisecond))
	if len(a) != 26 || a == b || a[:10] >= b[:10] {
		t.Fatalf("ids %s %s", a, b)
	}
	if a[:10] != "01M3C03V80" {
		t.Errorf("time part = %s", a[:10])
	}
}
