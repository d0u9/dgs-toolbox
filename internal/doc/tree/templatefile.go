package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TemplatePath is where the Template of a type is kept.
func TemplatePath(root, typ string) string {
	return filepath.Join(root, TemplatesDir, typ+".yaml")
}

// SaveTemplate writes a Template file's content as it was typed, comments
// included. previous is the type being edited, or "" for a new Template.
//
// A new type must not exist yet. A type Items use cannot be renamed, nor its
// kind changed: every sidecar names its type and kind. A renamed Template's
// old file goes to the trash.
func SaveTemplate(root, previous string, data []byte, now time.Time) (Template, error) {
	if err := Require(root); err != nil {
		return Template{}, err
	}
	t, err := ParseTemplate(data)
	if err != nil {
		return Template{}, err
	}
	items, err := LoadItems(root)
	if err != nil {
		return Template{}, err
	}
	if previous != t.Type {
		if _, err := os.Lstat(TemplatePath(root, t.Type)); err == nil {
			return Template{}, fmt.Errorf("a Template of type %s already exists", t.Type)
		}
	}
	if previous != "" {
		for _, item := range items {
			if item.Type != previous {
				continue
			}
			if previous != t.Type {
				return Template{}, fmt.Errorf("Items of type %s exist, so it cannot be renamed", previous)
			}
			if item.Kind != t.Kind {
				return Template{}, fmt.Errorf("Items of type %s are %ss, so its kind cannot change", previous, item.Kind)
			}
		}
	}
	if err := writeFileAtomic(TemplatePath(root, t.Type), data); err != nil {
		return Template{}, err
	}
	if previous != "" && previous != t.Type {
		if _, err := TrashTemplate(root, previous, now); err != nil {
			return t, err
		}
	}
	return t, nil
}

// TrashTemplate moves a Template no Item uses into the trash, and answers
// where it went.
func TrashTemplate(root, typ string, now time.Time) (string, error) {
	if err := Require(root); err != nil {
		return "", err
	}
	items, err := LoadItems(root)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Type == typ {
			return "", fmt.Errorf("Items of type %s exist: delete them first", typ)
		}
	}
	from := TemplatePath(root, typ)
	if _, err := os.Lstat(from); err != nil {
		return "", fmt.Errorf("no Template of type %s", typ)
	}
	dir := filepath.Join(root, TrashDir, TemplatesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	to := filepath.Join(dir, typ+"-"+now.UTC().Format("20060102T150405Z")+".yaml")
	if _, err := os.Lstat(to); err == nil {
		return "", fmt.Errorf("%s is already in the trash", to)
	}
	return to, os.Rename(from, to)
}

// writeFileAtomic writes data beside path and renames it into place, so a
// reader sees the old file or the new one, never half of one.
func writeFileAtomic(path string, data []byte) error {
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
