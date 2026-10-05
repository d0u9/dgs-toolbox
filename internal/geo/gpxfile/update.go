package gpxfile

import (
	"bytes"
	"os"
	"path/filepath"
)

// Update applies a group of edits to a private, same-folder copy of a GPX
// created by dgs, then publishes that copy with one rename. A failed edit
// leaves the original unchanged, so retrying cannot repeat a partial append
// or deletion. Existing edit functions can be used on the staged path.
// Publication has the same filesystem durability boundary as replaceFile.
func Update(path string, edit func(staged string) error) error {
	data, _, err := readOurs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	stagePath := staged.Name()
	defer os.Remove(stagePath)
	if _, err := staged.Write(data); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Chmod(info.Mode().Perm()); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := edit(stagePath); err != nil {
		return err
	}
	updated, _, err := readOurs(stagePath)
	if err != nil {
		return err
	}
	if bytes.Equal(data, updated) {
		return nil
	}
	return os.Rename(stagePath, path)
}
