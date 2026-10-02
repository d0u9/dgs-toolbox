package pagecache

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var digest = strings.Repeat("ab", 32)

func TestSavedPageLoadsBack(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if _, ok := s.Load(digest, 1, "thumb"); ok {
		t.Fatal("an empty cache answered")
	}
	if err := s.Save(digest, 1, "thumb", []byte("jpeg")); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Load(digest, 1, "thumb")
	if !ok || !bytes.Equal(got, []byte("jpeg")) {
		t.Fatalf("Load = %q %v", got, ok)
	}
	if _, ok := s.Load(digest, 1, "page"); ok {
		t.Fatal("another size answered")
	}
	if _, ok := s.Load(digest, 2, "thumb"); ok {
		t.Fatal("another page answered")
	}
}

func TestBadKeysAndNoDirKeepNothing(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, c := range []struct {
		digest string
		page   int
		size   string
	}{{"../x", 1, "thumb"}, {digest, 0, "thumb"}, {digest, 1, "../x"}} {
		if err := s.Save(c.digest, c.page, c.size, []byte("x")); err == nil {
			t.Errorf("saved %+v", c)
		}
	}
	if err := (Store{}).Save(digest, 1, "thumb", []byte("x")); err == nil {
		t.Error("saved with no directory")
	}
}

func TestAnUnreadableEntryIsAMiss(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	path := filepath.Join(s.Dir, Version, digest[:2], digest, "1-thumb.jpg")
	if err := os.MkdirAll(path, 0o755); err != nil { // a directory where the file goes
		t.Fatal(err)
	}
	if _, ok := s.Load(digest, 1, "thumb"); ok {
		t.Fatal("a directory read as a page")
	}
}
