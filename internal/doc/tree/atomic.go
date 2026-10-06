package tree

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data beside path and renames it into place, so a
// reader sees the old file or the new one, never half of one.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	part, err := os.CreateTemp(filepath.Dir(path), ".dgs-part-*")
	if err != nil {
		return err
	}
	defer os.Remove(part.Name())
	if _, err := part.Write(data); err != nil {
		part.Close()
		return err
	}
	if err := part.Sync(); err != nil {
		part.Close()
		return err
	}
	if err := part.Close(); err != nil {
		return err
	}
	if err := os.Chmod(part.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(part.Name(), path)
}
