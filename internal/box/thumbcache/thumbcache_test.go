package thumbcache_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/box/thumb"
	"dgs-toolbox/internal/box/thumbcache"
)

const testDigest = "sha256:a1b2c3d4e5f60718293a4b5c6d7e8f90"

func TestSaveAndLoadBothSizes(t *testing.T) {
	store := thumbcache.New(t.TempDir(), "/Volumes/nas/Box")
	pair := thumb.Pair{Grid: []byte("grid bytes"), Preview: []byte("preview bytes")}
	if err := store.Save(testDigest, pair); err != nil {
		t.Fatalf("save: %v", err)
	}
	for size, want := range map[string]string{
		thumbcache.SizeGrid:    "grid bytes",
		thumbcache.SizePreview: "preview bytes",
	} {
		got, err := store.Load(testDigest, size)
		if err != nil {
			t.Fatalf("load %s: %v", size, err)
		}
		if string(got) != want {
			t.Errorf("%s: got %q want %q", size, got, want)
		}
	}
}

// A few thousand files in one directory is slow in every file browser and bad
// on some filesystems.
func TestPathsAreSharded(t *testing.T) {
	store := thumbcache.New("/cache", "/Volumes/nas/Box")
	path, err := store.PathFor(testDigest, thumbcache.SizeGrid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(path), "/thumbs/a1/a1b2c3d4-300.jpg") {
		t.Errorf("got %q", path)
	}
	preview, err := store.PathFor(testDigest, thumbcache.SizePreview)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(preview), "/previews/a1/a1b2c3d4-1600.jpg") {
		t.Errorf("got %q", preview)
	}
}

// The two trees expire differently, so sweeping one must not be able to reach
// the other.
func TestGridAndPreviewLiveApart(t *testing.T) {
	store := thumbcache.New("/cache", "/Volumes/nas/Box")
	grid, _ := store.PathFor(testDigest, thumbcache.SizeGrid)
	preview, _ := store.PathFor(testDigest, thumbcache.SizePreview)
	if filepath.Dir(grid) == filepath.Dir(preview) {
		t.Error("both sizes are in one directory")
	}
}

func TestMissingPictureIsNamed(t *testing.T) {
	store := thumbcache.New(t.TempDir(), "/Volumes/nas/Box")
	// A scan marked needs-render has no picture, and asking for one is an
	// ordinary outcome rather than a failure.
	if _, err := store.Load(testDigest, thumbcache.SizeGrid); !errors.Is(err, thumbcache.ErrNotStored) {
		t.Fatalf("got %v, want ErrNotStored", err)
	}
}

func TestUnknownSizeIsRefused(t *testing.T) {
	store := thumbcache.New("/cache", "/Volumes/nas/Box")
	if _, err := store.PathFor(testDigest, "4000"); err == nil {
		t.Fatal("an unknown size was given a path")
	}
}

// Grid thumbnails are never swept; previews go after box.preview.keep days
// without use.
func TestSweepDropsOldPreviewsOnly(t *testing.T) {
	cacheDir := t.TempDir()
	store := thumbcache.New(cacheDir, "/Volumes/nas/Box")
	if err := store.Save(testDigest, thumb.Pair{Grid: []byte("g"), Preview: []byte("p")}); err != nil {
		t.Fatal(err)
	}
	preview, _ := store.PathFor(testDigest, thumbcache.SizePreview)
	old := time.Now().AddDate(0, 0, -60)
	if err := os.Chtimes(preview, old, old); err != nil {
		t.Fatal(err)
	}
	dropped, err := store.Sweep(30, time.Now())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if dropped != 1 {
		t.Errorf("dropped %d, want 1", dropped)
	}
	if _, err := os.Lstat(preview); !errors.Is(err, os.ErrNotExist) {
		t.Error("the old preview is still there")
	}
	if _, err := store.Load(testDigest, thumbcache.SizeGrid); err != nil {
		t.Errorf("the grid thumbnail was swept: %v", err)
	}
}

// box.preview.keep = 0 means keep previews indefinitely.
func TestSweepOfZeroKeepsEverything(t *testing.T) {
	cacheDir := t.TempDir()
	store := thumbcache.New(cacheDir, "/Volumes/nas/Box")
	if err := store.Save(testDigest, thumb.Pair{Preview: []byte("p")}); err != nil {
		t.Fatal(err)
	}
	preview, _ := store.PathFor(testDigest, thumbcache.SizePreview)
	old := time.Now().AddDate(0, 0, -900)
	os.Chtimes(preview, old, old)
	dropped, err := store.Sweep(0, time.Now())
	if err != nil || dropped != 0 {
		t.Fatalf("dropped %d, err %v", dropped, err)
	}
	if _, err := os.Lstat(preview); err != nil {
		t.Error("a preview was swept although keep is 0")
	}
}

// Using a preview is what keeps it: one being looked at today must not be
// dropped tonight.
func TestLoadingAPreviewKeepsIt(t *testing.T) {
	cacheDir := t.TempDir()
	store := thumbcache.New(cacheDir, "/Volumes/nas/Box")
	if err := store.Save(testDigest, thumb.Pair{Preview: []byte("p")}); err != nil {
		t.Fatal(err)
	}
	preview, _ := store.PathFor(testDigest, thumbcache.SizePreview)
	old := time.Now().AddDate(0, 0, -60)
	os.Chtimes(preview, old, old)
	if _, err := store.Load(testDigest, thumbcache.SizePreview); err != nil {
		t.Fatal(err)
	}
	dropped, err := store.Sweep(30, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 0 {
		t.Error("a preview that was just used was swept")
	}
}

// Sweeping a cache that is not there is not a failure.
func TestSweepingNothingIsFine(t *testing.T) {
	store := thumbcache.New(t.TempDir(), "/Volumes/nas/Box")
	if dropped, err := store.Sweep(30, time.Now()); err != nil || dropped != 0 {
		t.Fatalf("dropped %d, err %v", dropped, err)
	}
}

// Page 1 keeps the name an older build wrote, and every later page expires
// with the previews whatever its size.
func TestPagesAfterTheFirstLiveWithPreviews(t *testing.T) {
	store := thumbcache.New("/cache", "/Volumes/nas/Box")
	first, err := store.PathForPage(testDigest, thumbcache.SizeGrid, 1)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	old, _ := store.PathFor(testDigest, thumbcache.SizeGrid)
	if first != old {
		t.Errorf("page 1 moved: %s, was %s", first, old)
	}
	second, err := store.PathForPage(testDigest, thumbcache.SizeGrid, 2)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if !strings.Contains(second, string(filepath.Separator)+"previews"+string(filepath.Separator)) || !strings.Contains(second, "-p2-") {
		t.Errorf("page 2 grid thumbnail at %s", second)
	}
}

func TestSaveAndLoadAPage(t *testing.T) {
	store := thumbcache.New(t.TempDir(), "/Volumes/nas/Box")
	if err := store.SavePage(testDigest, 3, thumb.Pair{Grid: []byte("p3")}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.LoadPage(testDigest, thumbcache.SizeGrid, 3)
	if err != nil || string(got) != "p3" {
		t.Fatalf("load page 3: %q %v", got, err)
	}
	if _, err := store.Load(testDigest, thumbcache.SizeGrid); !errors.Is(err, thumbcache.ErrNotStored) {
		t.Errorf("page 3 answered for page 1: %v", err)
	}
}
