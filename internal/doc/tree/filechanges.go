package tree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

// FindFilePaths searches only safe tree locations for the requested content.
// ponytail: scans on demand, filtering by recorded size; index only if checks become too slow.
func (st *Store) FindFilePaths(ctx context.Context, digests []string) (map[string][]string, error) {
	found := map[string][]string{}
	if !st.InPlace() {
		return found, nil
	}
	wanted := map[string]bool{}
	for _, d := range digests {
		wanted[d] = true
	}
	if len(wanted) == 0 {
		return found, nil
	}
	sizes := map[int64]bool{}
	known := map[string]bool{}
	items, err := st.LoadItems()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		for _, r := range item.Revisions {
			if wanted[r.Digest] && r.Size != nil {
				sizes[*r.Size] = true
				known[r.Digest] = true
			}
			for _, o := range r.Originals {
				if wanted[o.Digest] && o.Size != nil {
					sizes[*o.Size] = true
					known[o.Digest] = true
				}
			}
		}
	}
	allSizes := len(known) != len(wanted)
	root, err := filepath.EvalSymlinks(st.root)
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel, err = cleanRecorded(filepath.ToSlash(rel))
		if err != nil {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".dgs-part") {
			return nil
		}
		resolved, err := st.recordedPath(rel)
		if err != nil {
			return nil
		} // Unsafe and dangling links are not candidates.
		info, err := os.Stat(resolved)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if !allSizes && !sizes[info.Size()] {
			return nil
		}
		digest, _, err := st.recordedDigest(ctx, rel)
		if err != nil {
			return err
		}
		if wanted[digest] {
			found[digest] = append(found[digest], rel)
		}
		return nil
	})
	return found, err
}

// ConfirmFilePath rechecks an offer before updating every reference within
// the Item. No files are moved and no other Item's sidecar is written.
func (st *Store) ConfirmFilePath(ctx context.Context, id, digest, previous, next string, now time.Time) (Item, error) {
	if err := st.Require(); err != nil {
		return Item{}, err
	}
	if !st.InPlace() {
		return Item{}, unsupportedLayout()
	}
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	item, err := st.GetItem(id)
	if err != nil {
		return Item{}, err
	}
	previous, err = cleanRecorded(previous)
	if err != nil {
		return Item{}, err
	}
	if Placing(item).Paths[digest] != previous {
		return Item{}, fmt.Errorf("recorded path changed since the check")
	}
	next, err = cleanRecorded(next)
	if err != nil {
		return Item{}, err
	}
	actual, _, err := st.recordedDigest(ctx, next)
	if err != nil {
		return Item{}, err
	}
	if actual != digest {
		return Item{}, fmt.Errorf("file changed since the check")
	}
	for i := range item.Revisions {
		r := &item.Revisions[i]
		if r.Digest == digest {
			r.Path, r.Lost = next, false
		}
		for j := range r.Originals {
			if r.Originals[j].Digest == digest {
				r.Originals[j].Path = next
			}
		}
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "confirm_file_path", Digest: digest, Changes: map[string][2]string{"path": {previous, next}}})
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	return item, st.WriteItem(item)
}

// RecordFileChange records content edited outside doc. It rechecks both the
// sidecar and bytes; locked Items and stale reports cannot create snapshots.
func (st *Store) RecordFileChange(ctx context.Context, id, digest, recorded, expected string, now time.Time) (Item, error) {
	if err := st.Require(); err != nil {
		return Item{}, err
	}
	if !st.InPlace() {
		return Item{}, unsupportedLayout()
	}
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	item, err := st.GetItem(id)
	if err != nil {
		return Item{}, err
	}
	if item.Locked {
		return Item{}, fmt.Errorf("locked Item: restore %s from a backup", recorded)
	}
	var base *Revision
	head := item.Current()
	for i := range item.Revisions {
		r := &item.Revisions[i]
		if r.Digest == digest && !r.Lost && r.Path == recorded {
			base = r
			if r.Ref() == head {
				break
			}
		}
	}
	if base == nil {
		return Item{}, fmt.Errorf("revision changed since the check")
	}
	actual, size, err := st.recordedDigest(ctx, recorded)
	if err != nil {
		return Item{}, err
	}
	if actual != expected || actual == digest {
		return Item{}, fmt.Errorf("file changed since the check")
	}
	fields, typ, source := item.FieldsAt(base.Ref()), base.Type, base.Source
	if typ == "" {
		typ = item.Type
	}
	// Returning to content already kept must retain the one-path-per-digest rule.
	if old, ok := Placing(item).Paths[actual]; ok && old != recorded {
		return Item{}, fmt.Errorf("changed content already has another recorded path in this Item")
	}
	ref := item.saveSnapshot(typ, fields, actual, source, "", now)
	for i := range item.Revisions {
		r := &item.Revisions[i]
		switch r.Digest {
		case digest:
			r.Lost = true
		case actual:
			r.Path, r.Lost = recorded, false
			r.Size = new(size)
		}
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "file_changed", Digest: ref, Changes: map[string][2]string{"digest": {digest, actual}}})
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	return item, st.WriteItem(item)
}

// recordedDigest uses an os.Root handle so a replaced symlink cannot make
// confirmation or change recording read outside the tree.
func (st *Store) recordedDigest(ctx context.Context, rel string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	p, err := st.recordedPath(rel)
	if err != nil {
		return "", 0, err
	}
	local, err := filepath.Rel(st.root, p)
	if err != nil {
		return "", 0, err
	}
	root, err := os.OpenRoot(st.root)
	if err != nil {
		return "", 0, err
	}
	defer root.Close()
	info, err := root.Stat(local)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("recorded file is not a regular file")
	}
	file, err := root.Open(local)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if !verifiedcopy.SameVersion(info, opened) {
		return "", 0, fmt.Errorf("file changed while it was being checked")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", 0, err
	}
	after, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	current, err := root.Stat(local)
	if err != nil {
		return "", 0, err
	}
	if !verifiedcopy.SameVersion(opened, after) || !verifiedcopy.SameVersion(after, current) {
		return "", 0, fmt.Errorf("file changed while it was being checked")
	}
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), opened.Size(), nil
}
