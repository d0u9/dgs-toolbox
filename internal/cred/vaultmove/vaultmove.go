// Package vaultmove renames a vault entry and moves it to another folder of
// the same vault. An entry is the .age file and the recipient record beside
// it, which must keep its name to stay the file's record, so the two always
// move together. The rules are in docs/apps/cred/vault.md.
package vaultmove

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/cred/record"
)

// Suffix is what a vault file is named with.
const Suffix = ".age"

// Result is what a move did, as paths relative to the vault root, in slash
// form.
type Result struct {
	From string
	To   string
	// Record is the record that moved with the file, empty when there is none.
	Record string
	// CreatedDirs are the directories made for the destination.
	CreatedDirs []string
}

// CheckName reports why name cannot be a vault file name, or nil. The rules
// are the vault's listing rules: a single name, ending in .age, not hidden.
func CheckName(name string) error {
	switch {
	case name == "" || name == "." || name == "..":
		return errors.New("the name must be a file name")
	case strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator):
		return errors.New("the name must be a single file name, without a folder")
	case strings.HasPrefix(name, "."):
		return errors.New("a name starting with a dot is not listed in the vault")
	case !strings.HasSuffix(name, Suffix):
		return fmt.Errorf("the name must end in %s, or the vault will not list it", Suffix)
	case name == Suffix:
		return errors.New("the name must have something before " + Suffix)
	}
	return nil
}

// CheckFolder reports why folder cannot hold a vault file, or nil. It is a
// path relative to the vault root; "" and "." are the root itself.
func CheckFolder(folder string) error {
	clean := path.Clean("/" + filepath.ToSlash(folder))
	if clean == "/" {
		return nil
	}
	for _, part := range strings.Split(strings.TrimPrefix(clean, "/"), "/") {
		if strings.HasPrefix(part, ".") {
			return fmt.Errorf("%s is hidden, so the vault does not list what is in it", part)
		}
	}
	return nil
}

// Move renames from to to, both relative to the vault root, in slash form. The
// destination's folder is created when it is missing. It refuses a destination
// that exists, one outside the vault, and a record that would be left behind
// by a name the vault does not list.
func Move(root, from, to string) (Result, error) {
	result := Result{From: path.Clean(filepath.ToSlash(from)), To: path.Clean(filepath.ToSlash(to))}
	if root == "" {
		return result, errors.New("no vault folder")
	}
	source, err := within(root, filepath.ToSlash(from))
	if err != nil {
		return result, err
	}
	destination, err := within(root, filepath.ToSlash(to))
	if err != nil {
		return result, err
	}
	if err := CheckName(path.Base(result.To)); err != nil {
		return result, err
	}
	if err := CheckFolder(path.Dir(result.To)); err != nil {
		return result, err
	}
	if result.From == result.To {
		return result, errors.New("it is already there")
	}
	if _, err := os.Lstat(source); err != nil {
		return result, err
	}
	// A destination that differs only in case is the same file on a
	// case-insensitive filesystem, which a rename of one file still handles.
	sameFile := strings.EqualFold(source, destination)
	if !sameFile {
		for _, p := range []string{destination, record.PathFor(destination)} {
			if _, err := os.Lstat(p); err == nil {
				return result, fmt.Errorf("%s already exists", relative(root, p))
			}
		}
	}
	created, err := makeDirs(root, filepath.Dir(destination))
	result.CreatedDirs = created
	if err != nil {
		return result, err
	}
	if err := os.Rename(source, destination); err != nil {
		return result, err
	}
	sourceRecord := record.PathFor(source)
	if _, err := os.Lstat(sourceRecord); err != nil {
		return result, nil
	}
	if err := os.Rename(sourceRecord, record.PathFor(destination)); err != nil {
		return result, fmt.Errorf("%s moved, but its record did not: %w", result.To, err)
	}
	result.Record = record.PathFor(result.To)
	return result, nil
}

// within resolves a path relative to the vault root and refuses one that
// leaves it. A .. is refused rather than cleaned away: cleaning it would move
// the file somewhere the path does not say.
func within(root, rel string) (string, error) {
	slash := filepath.ToSlash(rel)
	for _, part := range strings.Split(slash, "/") {
		if part == ".." {
			return "", fmt.Errorf("%s leaves the vault", slash)
		}
	}
	clean := path.Clean("/" + slash)
	if clean == "/" {
		return "", errors.New("the vault folder itself is not a file")
	}
	return filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(clean, "/"))), nil
}

// makeDirs creates the destination folder, reporting the directories it made
// so a failed move can say what it left behind.
func makeDirs(root, dir string) ([]string, error) {
	var missing []string
	for current := dir; len(current) > len(root); current = filepath.Dir(current) {
		if _, err := os.Stat(current); err == nil {
			break
		}
		missing = append([]string{current}, missing...)
	}
	if len(missing) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	made := make([]string, len(missing))
	for i, p := range missing {
		made[i] = relative(root, p)
	}
	return made, nil
}

func relative(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(rel)
	}
	return p
}
