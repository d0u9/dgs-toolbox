package tree

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
)

func cleanRecorded(p string) (string, error) {
	p = norm.NFC.String(p)
	if p == "" || p == "." || strings.ContainsAny(p, "\\\x00") || !filepath.IsLocal(filepath.FromSlash(p)) || path.Clean(p) != p {
		return "", fmt.Errorf("invalid recorded path %q", p)
	}
	switch strings.ToLower(strings.Split(p, "/")[0]) {
	case MetaDir, MarkerName, ChangeMarkName, ItemsDir, TrashDir, TemplatesDir, "outlines", "rules", "snapshots", "cases":
		return "", fmt.Errorf("recorded path %q uses a reserved tree location", p)
	}
	return p, nil
}

// recordedPath resolves each name in NFC without renaming files on disk. It
// checks resolved targets, including directory symlinks, before reading them.
// ponytail: scans each parent directory; add a name index only if profiling needs it.
// Missing files remain valid metadata; ReadPDF reports their absence.
func (s *Store) recordedPath(rel string) (string, error) {
	rel, err := cleanRecorded(filepath.ToSlash(rel))
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	treePath := func(p string) string {
		rel, _ := filepath.Rel(root, p)
		return filepath.Join(s.root, rel)
	}
	current := root
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", err
		}
		name := ""
		for _, entry := range entries {
			if norm.NFC.String(entry.Name()) == part {
				if name != "" {
					return "", fmt.Errorf("ambiguous Unicode path %q", rel)
				}
				name = entry.Name()
			}
		}
		if name == "" {
			return treePath(filepath.Join(current, filepath.FromSlash(strings.Join(parts[i:], "/")))), nil
		}
		candidate := filepath.Join(current, name)
		next, err := filepath.EvalSymlinks(candidate)
		if errors.Is(err, os.ErrNotExist) {
			// A dangling symlink still has a target that must stay in the tree.
			target, linkErr := os.Readlink(candidate)
			if linkErr != nil {
				return "", err
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(current, target)
			}
			inside, relErr := filepath.Rel(root, target)
			if relErr != nil || !filepath.IsLocal(inside) {
				return "", fmt.Errorf("recorded path %q resolves outside the tree", rel)
			}
			resolved, resolveErr := s.recordedPath(filepath.ToSlash(inside))
			if resolveErr != nil {
				return "", resolveErr
			}
			return filepath.Join(resolved, filepath.FromSlash(strings.Join(parts[i+1:], "/"))), nil
		}
		if err != nil {
			return "", err
		}
		inside, err := filepath.Rel(root, next)
		if err != nil || !filepath.IsLocal(inside) {
			return "", fmt.Errorf("recorded path %q resolves outside the tree", rel)
		}
		if _, err := cleanRecorded(filepath.ToSlash(inside)); inside != "." && err != nil {
			return "", err
		}
		current = next
	}
	return treePath(current), nil
}

// ValidateItemPaths checks the in-place sidecar invariant without requiring
// its files to exist. One digest has exactly one path within an Item.
func (s *Store) ValidateItemPaths(item *Item) error {
	for _, r := range item.Revisions {
		if r.Size != nil && *r.Size < 0 {
			return fmt.Errorf("negative file size")
		}
		for _, o := range r.Originals {
			if o.Size != nil && *o.Size < 0 {
				return fmt.Errorf("negative original size")
			}
		}
	}

	if !s.InPlace() {
		return nil
	}
	paths := map[string]string{}
	remember := func(digest string, p *string) error {
		clean, err := cleanRecorded(*p)
		if err != nil {
			return err
		}
		if previous, ok := paths[digest]; ok && previous != clean {
			return fmt.Errorf("Item %s: digest %s has conflicting recorded paths", item.ID, digest)
		}
		if _, err := s.recordedPath(clean); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		paths[digest], *p = clean, clean
		return nil
	}
	for i := range item.Revisions {
		r := &item.Revisions[i]
		if r.Digest != "" {
			if err := remember(r.Digest, &r.Path); err != nil {
				return err
			}
		}
		for j := range r.Originals {
			o := &r.Originals[j]
			if err := remember(o.Digest, &o.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

// ReadPDF resolves and checks a source immediately before preview or export.
func (s *Store) ReadPDF(item Item, digest string) (string, error) {
	if err := s.ValidateItemPaths(&item); err != nil {
		return "", err
	}
	if s.InPlace() {
		for _, r := range item.Revisions {
			if r.Digest == digest && r.Lost {
				return "", fmt.Errorf("Item %s: file is not at its recorded location: %s (earlier revision lost its file)", item.ID, r.Path)
			}
		}
	}
	rel := s.layout.PDF(Placing(item), digest)
	p := filepath.Join(s.root, rel)
	var err error
	if s.InPlace() {
		p, err = s.recordedPath(rel)
	}
	if err == nil {
		var info os.FileInfo
		info, err = os.Stat(p)
		if err == nil && !info.Mode().IsRegular() {
			return "", fmt.Errorf("Item %s: recorded file is not a regular file", item.ID)
		}
	}
	if err != nil {
		if s.InPlace() {
			err = fmt.Errorf("Item %s: file is not at its recorded location: %s: %w", item.ID, filepath.ToSlash(rel), err)
		}
		return "", err
	}
	return p, nil
}

// ReadFile resolves a stored object by Item ID for either layout.
func (s *Store) ReadFile(id, digest string) (string, error) {
	item := Item{ID: id}
	if s.InPlace() {
		var err error
		item, err = s.GetItem(id)
		if err != nil {
			return "", err
		}
	}
	return s.ReadPDF(item, digest)
}
