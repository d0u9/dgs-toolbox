package tree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// trashInPlace archives the original sidecar and only objects no remaining
// Item references. Every move is reversible until the sidecar is published.
func (st *Store) trashInPlace(item Item, remaining *Item, now time.Time) (to string, err error) {
	if err := st.Require(); err != nil {
		return "", err
	}
	items, err := st.LoadItems()
	if err != nil {
		return "", err
	}
	referenced := map[string]bool{}
	for _, other := range items {
		if other.ID == item.ID {
			continue
		}
		for digest := range Placing(other).Paths {
			referenced[digest] = true
		}
	}
	if remaining != nil {
		for digest := range Placing(*remaining).Paths {
			referenced[digest] = true
		}
	}
	paths := map[string]string{}
	for _, r := range item.Revisions {
		if !r.Lost && r.Digest != "" && !referenced[r.Digest] {
			paths[r.Digest] = r.Path
		}
		for _, o := range r.Originals {
			if !referenced[o.Digest] {
				paths[o.Digest] = o.Path
			}
		}
	}
	var digests []string
	for digest, rel := range paths {
		actual, _, readErr := st.recordedDigest(context.Background(), rel)
		if errors.Is(readErr, os.ErrNotExist) {
			delete(paths, digest)
			continue
		}
		if readErr != nil {
			return "", readErr
		}
		if actual != digest {
			return "", fmt.Errorf("%s: file no longer matches its recorded digest; check it before deletion", rel)
		}
		digests = append(digests, digest)
	}
	sort.Strings(digests)
	root, err := os.OpenRoot(st.root)
	if err != nil {
		return "", err
	}
	defer root.Close()
	trash, err := filepath.Rel(st.root, filepath.Join(st.meta, TrashDir))
	if err != nil {
		return "", err
	}
	to = filepath.Join(trash, item.ID+"-"+now.UTC().Format("20060102T150405.000000000Z"))
	if _, statErr := root.Lstat(to); !os.IsNotExist(statErr) {
		return "", fmt.Errorf("trash destination already exists")
	}
	if err := root.MkdirAll(trash, 0755); err != nil {
		return "", err
	}
	if err := root.Mkdir(to, 0755); err != nil {
		return "", err
	}
	var moved [][2]string
	defer func() {
		if err != nil {
			for i := len(moved) - 1; i >= 0; i-- {
				if undoErr := root.Rename(moved[i][1], moved[i][0]); undoErr != nil {
					err = errors.Join(err, fmt.Errorf("restore file: %w", undoErr))
				}
			}
			_ = root.Remove(to)
		}
	}()
	for _, digest := range digests {
		p, readErr := st.recordedPath(paths[digest])
		if readErr != nil {
			return "", readErr
		}
		from, readErr := filepath.Rel(st.root, p)
		if readErr != nil {
			return "", readErr
		}
		actual, _, readErr := st.recordedDigest(context.Background(), paths[digest])
		if readErr != nil {
			return "", readErr
		}
		if actual != digest {
			return "", fmt.Errorf("file changed during deletion")
		}
		dest := filepath.Join(to, digest+filepath.Ext(from))
		if err := root.Rename(from, dest); err != nil {
			return "", err
		}
		moved = append(moved, [2]string{from, dest})
	}
	sidecar, err := filepath.Rel(st.root, st.Sidecar(item.ID))
	if err != nil {
		return "", err
	}
	archived := filepath.Join(to, filepath.Base(sidecar))
	if err := root.Rename(sidecar, archived); err != nil {
		return "", err
	}
	moved = append(moved, [2]string{sidecar, archived})
	if remaining != nil {
		if err := st.WriteItem(*remaining); err != nil {
			return "", err
		}
	}
	return filepath.Join(st.root, to), nil
}
