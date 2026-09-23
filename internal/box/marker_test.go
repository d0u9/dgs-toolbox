package box_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/box"
)

// The gate exists because on macOS, writing under an unmounted /Volumes mount
// point silently creates a local directory. Without the check, a NAS that did
// not mount becomes a second, plausible-looking Box and nothing says so.
func TestARootWithNoMarkerIsNotABox(t *testing.T) {
	if err := box.RequireBox(t.TempDir(), ""); !errors.Is(err, box.ErrNotABox) {
		t.Fatalf("got %v, want ErrNotABox", err)
	}
}

func TestMarkerRecordsVersionAndDate(t *testing.T) {
	root := t.TempDir()
	made := time.Date(2026, 9, 22, 14, 30, 0, 0, time.UTC)
	if err := box.WriteMarker(root, "", made); err != nil {
		t.Fatalf("write: %v", err)
	}
	marker, err := box.ReadMarker(root, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// An empty marker could only answer whether this is a Box. One with
	// contents also answers which version made it, which is what a later format
	// change needs.
	if marker.Version != box.MarkerVersion {
		t.Errorf("version: got %d", marker.Version)
	}
	if marker.CreatedAt == "" {
		t.Error("no creation date")
	}
	// Not hidden: a Box whose metadata is invisible in Finder looks like a
	// folder of PDFs with nothing beside it.
	if filepath.Base(box.MarkerName)[0] == '.' {
		t.Error("the marker is a dotfile")
	}
}

func TestWriteMarkerRefusesAnExistingBox(t *testing.T) {
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	// Making a Box out of one that already exists would reset the version and
	// the creation date, which are the two things the file remembers.
	if err := box.WriteMarker(root, "", time.Now()); err == nil {
		t.Fatal("an existing Box was re-initialised")
	}
}

// A configured path would let the check be pointed somewhere other than the
// tree being written to, which is the one thing it exists to prevent.
func TestMarkerNameMustBeABareFilename(t *testing.T) {
	for _, name := range []string{"sub/dgs-box.yaml", "../dgs-box.yaml", "/etc/dgs-box.yaml"} {
		if _, err := box.MarkerPath(t.TempDir(), name); err == nil {
			t.Errorf("accepted %q as a marker name", name)
		}
	}
}

func TestNewerBoxVersionIsRefused(t *testing.T) {
	root := t.TempDir()
	path, err := box.MarkerPath(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("version: 99\ncreated_at: 2030-01-01T00:00:00Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := box.RequireBox(root, ""); err == nil {
		t.Fatal("a Box from a newer build was accepted")
	}
}
