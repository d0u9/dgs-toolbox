package filebrowse

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckThenRenameRefusesTakenName(t *testing.T) {
	dir := t.TempDir()
	from, taken, free := filepath.Join(dir, "a.gpx"), filepath.Join(dir, "b.gpx"), filepath.Join(dir, "c.gpx")
	for _, path := range []string{from, taken} {
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkThenRename(from, taken); !errors.Is(err, os.ErrExist) {
		t.Fatalf("taken name: %v", err)
	}
	if data, _ := os.ReadFile(taken); string(data) != taken {
		t.Fatal("taken file was replaced")
	}
	if err := checkThenRename(from, free); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(free); err != nil {
		t.Fatal(err)
	}
}
