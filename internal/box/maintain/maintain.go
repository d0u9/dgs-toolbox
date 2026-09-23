// Package maintain is what the four housekeeping commands do, with no terminal
// attached: create a Box, rebuild its cache, check its bytes, and group its
// duplicates.
//
// They are separate because their costs differ by three orders of magnitude,
// and a name that hid that would mean either never dares be run — the cheap one
// because it might take an hour, the expensive one because it looks as though
// it has already happened.
//
//	Init     writes one file.
//	Reindex  reads sidecars only; the digest comes from the sidecar. Seconds.
//	Verify   reads every byte of every file to recompute digests. Tens of GB.
//	Dedupe   reads the index, and reads bytes only where it must.
package maintain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/dedupe"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/index"
)

// Init creates a Box.
//
// It is the only thing that writes the marker. No other command creates one as
// a side effect, because a gate any command could open is not a gate.
func Init(root, markerName string, now time.Time) error {
	if err := box.WriteMarker(root, markerName, now); err != nil {
		return err
	}
	return boxlog.Append(boxlog.PathFor(root), boxlog.Entry{
		Action: boxlog.ActionInit,
		At:     now.Format(time.RFC3339),
		Note:   fmt.Sprintf("Box format %d", box.MarkerVersion),
	})
}

// IndexResult is what a rebuild found.
type IndexResult struct {
	// Scans is how many sidecars were read.
	Scans int
	// Trashed is how many of those are in the trash. They stay indexed because
	// the trash takes part in deduplication.
	Trashed int
	// Orphans is everything that did not pair up, in both directions. Nothing
	// is repaired: guessing which side is wrong and acting on the guess is how
	// a tool destroys what it was asked to look after.
	Orphans []index.Orphan
	// Rebuilt says the cache was built from nothing rather than refreshed.
	Rebuilt bool
}

// Reindex brings a Box's cache up to date, or builds it from nothing.
//
// Sidecars only. This is seconds even on a NAS, and it is deliberately not
// Verify: the digest is taken from the sidecar and no scan's bytes are read.
func Reindex(root, markerName, cacheDir string) (IndexResult, error) {
	if err := box.RequireBox(root, markerName); err != nil {
		return IndexResult{}, err
	}
	path := index.PathFor(cacheDir, root)
	existing, usable, err := index.Load(path, root)
	if err != nil {
		return IndexResult{}, err
	}
	var cache index.Index
	var orphans []index.Orphan
	if usable {
		cache, orphans, err = index.Refresh(root, existing)
	} else {
		cache, orphans, err = index.Build(root)
	}
	if err != nil {
		return IndexResult{}, err
	}
	if err := index.Save(path, cache); err != nil {
		return IndexResult{}, err
	}
	result := IndexResult{Scans: len(cache.Entries), Orphans: orphans, Rebuilt: !usable}
	for _, entry := range cache.Entries {
		if entry.InTrash {
			result.Trashed++
		}
	}
	return result, nil
}

// Mismatch is one scan whose bytes no longer produce the digest recorded for
// them.
type Mismatch struct {
	// Path is relative to the Box root.
	Path string
	// Recorded is what the sidecar says, Actual what the bytes say now. Both
	// are kept: a report that only says "wrong" cannot be acted on.
	Recorded string
	Actual   string
	// Missing marks a sidecar whose file is no longer there, where Actual is
	// empty because there was nothing to read.
	Missing bool
}

// VerifyResult is what a verify pass found.
type VerifyResult struct {
	// Checked is how many files were read, Bytes how many bytes. Both are
	// reported because this is the expensive command and a person deciding
	// whether to run it again needs to know what it cost.
	Checked int
	Bytes   int64
	// Mismatches is every file whose bytes changed. Nothing is rewritten: this
	// command reports and never repairs.
	Mismatches []Mismatch
}

// Verify reads every byte of every file in the Box and recomputes its digest.
//
// This is the expensive one — tens of gigabytes over a network filesystem — and
// it is the only thing that can answer whether the bytes are still what they
// were. Publication verifies at publication time and cannot promise anything
// about the years after it.
//
// Nothing is repaired. A file whose digest moved is reported with both digests
// and left exactly as it is: the tool has no way to know which of the two is
// the version somebody wants, and rewriting either one destroys the evidence.
func Verify(ctx context.Context, root, markerName string, progress func(path string, done, total int)) (VerifyResult, error) {
	if err := box.RequireBox(root, markerName); err != nil {
		return VerifyResult{}, err
	}
	cache, orphans, err := index.Build(root)
	if err != nil {
		return VerifyResult{}, err
	}
	result := VerifyResult{}
	// A sidecar whose scan is not beside it is the same finding as a scan whose
	// bytes changed: the Box no longer holds what it says it holds. Reporting
	// it only as an orphan would leave it out of the one command whose whole
	// job is to answer that question.
	for _, orphan := range orphans {
		if orphan.Kind != "sidecar" {
			continue
		}
		result.Mismatches = append(result.Mismatches, Mismatch{Path: orphan.Path, Missing: true})
	}
	total := len(cache.Entries)
	for position, entry := range cache.Entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if progress != nil {
			progress(entry.ScanPath, position, total)
		}
		if entry.ScanPath == "" {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     entry.SidecarPath,
				Recorded: entry.File.Digest,
				Missing:  true,
			})
			continue
		}
		path := filepath.Join(root, entry.ScanPath)
		file, err := os.Open(path)
		if err != nil {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     entry.ScanPath,
				Recorded: entry.File.Digest,
				Missing:  errors.Is(err, os.ErrNotExist),
			})
			continue
		}
		actual, hashErr := digest.WholeFrom(file)
		info, statErr := file.Stat()
		file.Close()
		if hashErr != nil {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path: entry.ScanPath, Recorded: entry.File.Digest,
			})
			continue
		}
		result.Checked++
		if statErr == nil {
			result.Bytes += info.Size()
		}
		if actual != entry.File.Digest {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     entry.ScanPath,
				Recorded: entry.File.Digest,
				Actual:   actual,
			})
		}
	}
	if progress != nil {
		progress("", total, total)
	}
	return result, nil
}

// DuplicateGroup is one document and every copy of it in the Box.
type DuplicateGroup struct {
	// Paths are relative to the Box root, in a fixed order.
	Paths []string
	// Trashed marks a group where at least one copy is already in the trash,
	// which says a decision about it has been made once already.
	Trashed bool
}

// DedupeResult is what a dedupe pass found.
type DedupeResult struct {
	Groups []DuplicateGroup
	// Copies is how many files could be discarded if every group were reduced
	// to one, which is the number worth showing: "12 groups" says nothing about
	// the size of the pile.
	Copies int
}

// Dedupe groups the scans in a Box that are the same piece of paper.
//
// It reads the index rather than the files, so it costs what Reindex costs and
// not what Verify costs. Both digests are already in the sidecars, which is why
// they were recorded there in the first place.
//
// Nothing is discarded. This reports; deciding what goes is a person's job, and
// the report is arranged so it is one decision per document rather than one per
// file.
func Dedupe(root, markerName string) (DedupeResult, error) {
	if err := box.RequireBox(root, markerName); err != nil {
		return DedupeResult{}, err
	}
	cache, _, err := index.Build(root)
	if err != nil {
		return DedupeResult{}, err
	}
	items := make([]dedupe.Item, 0, len(cache.Entries))
	paths := make(map[string]string, len(cache.Entries))
	for _, entry := range cache.Entries {
		id := entry.SidecarPath
		path := entry.ScanPath
		if path == "" {
			path = entry.SidecarPath
		}
		paths[id] = path
		items = append(items, dedupe.Item{
			ID:      id,
			Digest:  entry.File.Digest,
			InTrash: entry.InTrash,
		})
	}
	var result DedupeResult
	for _, group := range dedupe.Groups(items) {
		if !group.Duplicated() {
			continue
		}
		found := DuplicateGroup{Trashed: group.InTrash()}
		for _, item := range group.Items {
			found.Paths = append(found.Paths, paths[item.ID])
		}
		sort.Strings(found.Paths)
		result.Copies += len(found.Paths) - 1
		result.Groups = append(result.Groups, found)
	}
	sort.Slice(result.Groups, func(a, b int) bool {
		return result.Groups[a].Paths[0] < result.Groups[b].Paths[0]
	})
	return result, nil
}
