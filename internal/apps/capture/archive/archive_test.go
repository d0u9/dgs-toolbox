package archive

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeCapture(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create capture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"schema":"v1"}`), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	return dir
}

func TestMoveRelocatesTheCaptureDirectory(t *testing.T) {
	root := t.TempDir()
	capture := writeCapture(t, root, "2026-09-01-note")
	archiveRoot := filepath.Join(t.TempDir(), "Archive")

	destination, err := Move(capture, archiveRoot)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if want := filepath.Join(archiveRoot, "2026-09-01-note"); destination != want {
		t.Fatalf("destination = %q, want %q", destination, want)
	}
	if _, err := os.Stat(filepath.Join(destination, "index.json")); err != nil {
		t.Fatalf("archived index: %v", err)
	}
	if _, err := os.Stat(capture); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capture still in the root: %v", err)
	}
}

func TestMoveRefusesAnOccupiedDestination(t *testing.T) {
	root := t.TempDir()
	capture := writeCapture(t, root, "same-name")
	archiveRoot := t.TempDir()
	writeCapture(t, archiveRoot, "same-name")

	if _, err := Move(capture, archiveRoot); !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if _, err := os.Stat(capture); err != nil {
		t.Fatalf("capture should stay where it is: %v", err)
	}
}

func TestMoveRefusesWithoutAnArchiveFolder(t *testing.T) {
	capture := writeCapture(t, t.TempDir(), "note")
	if _, err := Move(capture, ""); err == nil {
		t.Fatal("expected an error with no archive folder")
	}
}

// The copy path is what runs when the archive is on another volume, which a
// test cannot arrange; it is exercised directly instead.
func TestCopyTreeCopiesNestedFiles(t *testing.T) {
	source := writeCapture(t, t.TempDir(), "note")
	if err := os.MkdirAll(filepath.Join(source, "files"), 0o755); err != nil {
		t.Fatalf("create subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "files", "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "note")
	if err := copyTree(source, destination); err != nil {
		t.Fatalf("copy: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "files", "a.txt"))
	if err != nil || string(data) != "hello" {
		t.Fatalf("copied attachment = %q, %v", data, err)
	}
}

func TestDestinationKeepsTheDirectoryName(t *testing.T) {
	if got := Destination("/archive", "/root/2026-09-01-note/"); got != "/archive/2026-09-01-note" {
		t.Fatalf("destination = %q", got)
	}
}
