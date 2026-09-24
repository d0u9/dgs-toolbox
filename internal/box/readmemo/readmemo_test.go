package readmemo_test

import (
	"testing"

	"dgs-toolbox/internal/box/readmemo"
	"dgs-toolbox/internal/box/scanread"
	"dgs-toolbox/internal/box/thumb"
)

func sample() scanread.Result {
	result := scanread.Result{Size: 10, Digest: "abc", ImageDigest: "def"}
	result.Info.Pages = 3
	result.Thumbs = thumb.Pair{Grid: []byte("g"), Preview: []byte("p")}
	return result
}

func TestLoadReturnsWhatWasSaved(t *testing.T) {
	store := readmemo.New(t.TempDir(), "/inbox")
	if err := store.Save("a/b.pdf", 10, 99, sample()); err != nil {
		t.Fatal(err)
	}
	got, ok := store.Load("a/b.pdf", 10, 99)
	if !ok {
		t.Fatal("miss")
	}
	if got.Digest != "abc" || got.ImageDigest != "def" || got.Info.Pages != 3 ||
		string(got.Thumbs.Grid) != "g" || string(got.Thumbs.Preview) != "p" {
		t.Errorf("got %+v", got)
	}
}

func TestChangedFileIsAMiss(t *testing.T) {
	store := readmemo.New(t.TempDir(), "/inbox")
	_ = store.Save("a.pdf", 10, 99, sample())
	if _, ok := store.Load("a.pdf", 11, 99); ok {
		t.Error("size changed but memo hit")
	}
	if _, ok := store.Load("a.pdf", 10, 100); ok {
		t.Error("mod time changed but memo hit")
	}
	if _, ok := store.Load("other.pdf", 10, 99); ok {
		t.Error("other path hit")
	}
}

func TestKeepDropsAbsentFiles(t *testing.T) {
	store := readmemo.New(t.TempDir(), "/inbox")
	_ = store.Save("keep.pdf", 10, 1, sample())
	_ = store.Save("gone.pdf", 10, 1, sample())
	if err := store.Keep(map[string]bool{"keep.pdf": true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Load("keep.pdf", 10, 1); !ok {
		t.Error("kept memo lost")
	}
	if _, ok := store.Load("gone.pdf", 10, 1); ok {
		t.Error("absent file still remembered")
	}
}
