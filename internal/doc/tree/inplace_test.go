package tree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

func inPlaceStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	if err := InitWithLayout(root, time.Unix(0, 0), "in-place"); err != nil {
		t.Fatal(err)
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Require(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Folder("alice"); ok {
		t.Fatal("in-place has Item folders")
	}
	return s
}

func TestInPlaceReadAndSnapshot(t *testing.T) {
	s := inPlaceStore(t)
	name := "Books/café.pdf"
	disk := filepath.Join(s.Root(), norm.NFD.String(name))
	if err := os.MkdirAll(filepath.Dir(disk), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disk, []byte("synthetic PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := FileDigest(disk)
	if err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(s.Root(), "Books", "page.jpg")
	if err := os.WriteFile(originalPath, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	imageDigest, err := FileDigest(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	original := Original{Digest: imageDigest, Extension: ".jpg", Path: "Books/page.jpg"}
	item := Item{ID: "alice", Type: "id_card", Kind: KindDocument, Fields: map[string]string{"owner": "alice"}, Revisions: []Revision{{Digest: digest, Path: norm.NFD.String(name), Originals: []Original{original}}}}
	if err := s.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	items, err := s.Items()
	if err != nil || len(items) != 1 {
		t.Fatalf("Items: %v %v", items, err)
	}
	item = items[0]
	if item.Revisions[0].Path != name {
		t.Fatal("path was not NFC")
	}
	p, err := s.ReadPDF(item, digest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil || string(data) != "synthetic PDF" {
		t.Fatal("wrong PDF", err)
	}
	image, err := os.ReadFile(s.Image(item, digest, original))
	if err != nil || string(image) != "synthetic original" {
		t.Fatal("original path", err)
	}
	if err := os.Symlink("Books", filepath.Join(s.Root(), "shelf")); err != nil {
		t.Fatal(err)
	}
	linked := item
	linked.Revisions = []Revision{{Digest: digest, Path: "shelf/café.pdf"}}
	if _, err := s.ReadPDF(linked, digest); err != nil {
		t.Fatal("internal symlink", err)
	}
	if !s.Owned(norm.NFD.String(name), item) || s.Owned("unimported.pdf", item) {
		t.Fatal("ownership")
	}
	template := Template{Type: item.Type, Kind: KindDocument, Fields: []Field{{Key: "owner"}}}
	item, err = s.SetFields(item.ID, item.Current(), template, map[string]string{"owner": "bob"}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Revisions) != 2 || item.Revisions[1].Path != name || len(item.Revisions[1].Originals) != 1 || item.Revisions[1].Originals[0].Path != original.Path {
		t.Fatal("snapshot lost path")
	}
	before, _ := os.ReadFile(s.Sidecar(item.ID))
	operations := []func() error{
		func() error {
			_, err := s.AttachFileAs(context.Background(), item.ID, "", nil, false, false, time.Now())
			return err
		},
	}
	for _, op := range operations {
		if err := op(); err == nil || !strings.Contains(err.Error(), "not supported yet") {
			t.Fatalf("guard: %v", err)
		}
	}
	after, _ := os.ReadFile(s.Sidecar(item.ID))
	if string(before) != string(after) {
		t.Fatal("guard changed sidecar")
	}
	if _, err := os.Stat(filepath.Join(s.Root(), TrashDir)); !os.IsNotExist(err) {
		t.Fatal("guard created trash")
	}
	if err := os.Remove(disk); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadPDF(item, digest); err == nil || !strings.Contains(err.Error(), "file is not at its recorded location") {
		t.Fatalf("missing: %v", err)
	}
}

func TestInPlaceRejectsUnsafeAndConflictingPaths(t *testing.T) {
	s := inPlaceStore(t)
	for _, name := range []string{"", ".", "ITEMS/card.pdf", "../outside.pdf", "/absolute.pdf", "a/../b.pdf", "a\\b.pdf", MarkerName, "items/a.pdf", "trash/a.pdf", "templates/a.pdf", "outlines/a.pdf", "rules/a.pdf", "snapshots/a.pdf", "cases/a.pdf"} {
		item := Item{ID: "alice", Revisions: []Revision{{Digest: "digest", Path: name}}}
		if err := s.WriteItem(item); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	item := Item{ID: "alice", Revisions: []Revision{{Digest: "digest", Path: "a.pdf"}, {Digest: "digest", Path: "b.pdf"}}}
	data, _ := yaml.Marshal(item)
	if err := os.WriteFile(s.Sidecar(item.ID), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetItem(item.ID); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("conflict: %v", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(s.Root(), "outside")); err != nil {
		t.Fatal(err)
	}
	item.Revisions = []Revision{{Digest: "digest", Path: "outside/missing.pdf"}}
	if err := s.WriteItem(item); err == nil {
		t.Fatal("outside symlink accepted")
	}
	if err := os.Symlink(filepath.Join(s.Root(), TemplatesDir), filepath.Join(s.Root(), "alias")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "missing.pdf")
	if err := os.Symlink(outside, filepath.Join(s.Root(), "dangling.pdf")); err != nil {
		t.Fatal(err)
	}
	item.Revisions[0].Path = "dangling.pdf"
	if err := s.WriteItem(item); err == nil {
		t.Fatal("dangling outside symlink accepted")
	}
	item.Revisions[0].Path = "alias/missing.pdf"
	if err := s.WriteItem(item); err == nil {
		t.Fatal("reserved symlink accepted")
	}
}
