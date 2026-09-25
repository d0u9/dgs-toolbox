package textcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/doc/ocr"
)

var digest = strings.Repeat("ab", 32)

func TestSaveAndLoad(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if _, _, ok := s.Load(digest, 0); ok {
		t.Fatal("empty cache answered")
	}
	page := ocr.Page{Source: ocr.SourceRecognised, Lines: []ocr.Line{{Text: "x", Box: ocr.Box{X: 0.1, W: 0.5, H: 0.1}}}}
	if err := s.Save(digest, 1, 3, page); err != nil {
		t.Fatal(err)
	}
	got, count, ok := s.Load(digest, 1)
	if !ok || count != 3 || got.Lines[0] != page.Lines[0] || got.Source != page.Source {
		t.Fatalf("%+v %d %v", got, count, ok)
	}
	if _, _, ok := s.Load(digest, 0); ok {
		t.Fatal("unsaved page answered")
	}
}

func TestBadEntriesAreMisses(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	path, _ := s.path(digest, 0)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	must(os.WriteFile(path, []byte("{broken"), 0o644))
	if _, _, ok := s.Load(digest, 0); ok {
		t.Fatal("broken entry answered")
	}
	if err := s.Save("../../etc", 0, 1, ocr.Page{}); err == nil {
		t.Fatal("a path was taken for a digest")
	}
	if err := (Store{}).Save(digest, 0, 1, ocr.Page{}); err == nil {
		t.Fatal("saved with no directory")
	}
}
