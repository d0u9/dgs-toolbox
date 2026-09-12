// Package archive moves a Capture directory out of the Capture root once it has
// been organized. It is deliberately not an Action: an Action writes a Capture
// somewhere, while archiving takes the Capture itself out of reach, and a
// Capture that has been archived can no longer be worked on.
package archive

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrExists is returned when the destination already holds a directory of that
// name. A Capture is never merged into one that is already there: two Captures
// sharing a folder name is a question only the reader can answer, and guessing
// at it would bury one of them inside the other.
var ErrExists = errors.New("a directory of that name is already archived")

// Destination is where a Capture would land under the given archive root. It
// keeps its own directory name: the name is what the index and the record refer
// to each other by, and renaming on the way out would make the archive read
// differently from the record of how it was organized.
func Destination(root, capturePath string) string {
	return filepath.Join(root, filepath.Base(filepath.Clean(capturePath)))
}

// Move takes one Capture directory to the archive root and returns where it
// landed. The move is a rename where the filesystem allows one, and a copy
// followed by a removal where it does not — an archive on another volume is the
// normal case, not an exception. A copy that fails part way through leaves the
// original in place and removes what it had written, so a failed archive is a
// Capture still in the root rather than half of one in two places.
func Move(capturePath, root string) (string, error) {
	if root == "" {
		return "", errors.New("no archive folder is configured")
	}
	source, err := filepath.Abs(capturePath)
	if err != nil {
		return "", err
	}
	destination := Destination(root, source)
	if _, err := os.Lstat(destination); err == nil {
		return "", fmt.Errorf("%s: %w", filepath.Base(destination), ErrExists)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create archive folder: %w", err)
	}
	if err := os.Rename(source, destination); err == nil {
		return destination, nil
	}
	if err := copyTree(source, destination); err != nil {
		// The destination is the half-written copy, so it is what gets removed:
		// the Capture itself has not been touched yet.
		_ = os.RemoveAll(destination)
		return "", err
	}
	if err := os.RemoveAll(source); err != nil {
		return "", fmt.Errorf("remove archived capture: %w", err)
	}
	return destination, nil
}

// copyTree copies a Capture directory across a filesystem boundary. A Capture
// is a flat directory of regular files in practice, but the walk handles
// nesting rather than assuming it away; anything that is neither a directory
// nor a regular file — a symlink, a device — refuses, because copying it is a
// decision this tool has not been asked to make.
func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: not a regular file", relative)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
