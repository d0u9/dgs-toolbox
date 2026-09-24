package web

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/maintain"
	"dgs-toolbox/internal/box/scanread"
	"dgs-toolbox/internal/box/sidecar"
)

// ApplyMany changes several scans in one request.
//
// One request rather than one per scan, because three hundred scans at one
// round trip each is the hour of work nobody finishes. Each record is still
// written and logged on its own: the log is what makes a batch recoverable,
// and therefore what makes batch editing safe enough to offer.
//
// A record that is refused does not undo the ones that succeeded. A batch is a
// convenience over a list of edits, not a transaction, and a Box has no way to
// roll a sidecar back that is better than the log.
func (e *Engine) ApplyMany(digests []string, edit Edit) (BatchResult, error) {
	var result BatchResult
	for _, scanDigest := range digests {
		edit.Digest = scanDigest
		scan, err := e.Apply(edit)
		if err != nil {
			result.Failed = append(result.Failed, BatchFailed{Digest: scanDigest, Error: err.Error()})
			continue
		}
		result.Changed = append(result.Changed, scan)
	}
	e.persist()
	return result, nil
}

// TrashMany discards several scans in one request, which is what makes a group
// of duplicates one decision rather than one per copy.
func (e *Engine) TrashMany(digests []string, reason string) (BatchResult, error) {
	var result BatchResult
	for _, scanDigest := range digests {
		if err := e.Trash(scanDigest, reason); err != nil {
			result.Failed = append(result.Failed, BatchFailed{Digest: scanDigest, Error: err.Error()})
		}
	}
	e.persist()
	return result, nil
}

// Adopt describes a scan that is in the Box with no sidecar.
//
// Dropping a file into the tree by hand is a deliberate act, so the answer is
// to take it in where it lies rather than to call it an error: the same
// metadata and thumbnail work as intake, and no copy. Its directory is never
// changed, because the directory records an intake date that adopting does not
// get to rewrite.
//
// The one thing that may change is the filename: a sidecar is paired with its
// scan by the digest prefix both names carry, so a file that arrived without
// one is renamed inside its own directory to gain it. Without that the sidecar
// written here would be an orphan the moment it was saved.
func (e *Engine) Adopt(path string, edit Edit) (Scan, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	if clean == "" || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return Scan{}, fmt.Errorf("%q is not a path inside the Box", path)
	}
	full := filepath.Join(e.root, clean)
	if !e.orphanScanLocked(filepath.ToSlash(clean)) {
		return Scan{}, fmt.Errorf("%s is not a scan without a sidecar", path)
	}

	result, err := scanread.ReadFile(full)
	if err != nil {
		return Scan{}, err
	}
	if result.Incomplete {
		return Scan{}, fmt.Errorf("%s is not adoptable: %s", path, result.IncompleteReason)
	}

	directory := filepath.Dir(full)
	short := digest.Short(result.Digest, sidecar.DefaultPrefix)
	scanPath, err := adoptedName(full, short)
	if err != nil {
		return Scan{}, err
	}
	sidecarPath := sidecar.PathFor(directory, result.Digest, len(short))
	if _, err := os.Lstat(sidecarPath); err == nil {
		return Scan{}, fmt.Errorf("%s already has a sidecar", filepath.Base(sidecarPath))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Scan{}, err
	}

	described, err := applyEdit(Scan{}, edit, e.currency)
	if err != nil {
		return Scan{}, err
	}
	record, err := recordFromScan(sidecar.File{}, described)
	if err != nil {
		return Scan{}, err
	}
	if strings.TrimSpace(record.Type) == "" {
		record.Type = "unsorted"
	}
	record.Digest = result.Digest
	record.Size = result.Size
	record.Kind = sidecar.Kind(result.Info.Kind)
	record.Pages = result.Info.Pages
	record.PageSize = result.Info.PageSize
	record.Producer = result.Info.Producer
	record.ScanCreatedAt = sidecar.Timestamp(result.Info.CreatedAt)
	record.OriginalFilename = filepath.Base(full)
	record.IngestedAt = sidecar.Timestamp(time.Now().UTC().Format(time.RFC3339))

	if scanPath != full {
		if err := os.Rename(full, scanPath); err != nil {
			return Scan{}, err
		}
	}
	if err := sidecar.Save(sidecarPath, record); err != nil {
		return Scan{}, err
	}
	entry := boxlog.Import(result.Digest, relativeTo(e.root, scanPath))
	entry.At = time.Now().Format(time.RFC3339)
	entry.Note = "adopted in place"
	if err := boxlog.Append(boxlog.PathFor(e.root), entry); err != nil {
		return Scan{}, fmt.Errorf("adopted %s but could not append to the log: %w", clean, err)
	}
	_ = e.thumbs.Save(result.Digest, result.Thumbs)

	e.reindexLocked()
	position, found := e.entryIndexLocked(result.Digest)
	if !found {
		return Scan{}, fmt.Errorf("adopted %s but it did not pair up", clean)
	}
	return decorate(scanFromEntry(e.cache.Entries[position]), box.Today(nil)), nil
}

// adoptedName is what the file has to be called for its sidecar to find it: its
// own name when it already ends in this digest's prefix, and the same name with
// the prefix added when it does not. Nothing outside the file's own directory
// is touched.
func adoptedName(full, short string) (string, error) {
	directory := filepath.Dir(full)
	base := filepath.Base(full)
	extension := filepath.Ext(base)
	stem := strings.TrimSuffix(base, extension)
	if strings.HasSuffix(stem, "-"+short) {
		return full, nil
	}
	wanted := filepath.Join(directory, stem+"-"+short+extension)
	if _, err := os.Lstat(wanted); err == nil {
		return "", fmt.Errorf("%s already exists", filepath.Base(wanted))
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return wanted, nil
}

// orphanScanLocked reports whether this path is one of the files the last walk
// found with no sidecar. Adopting is offered for those and for nothing else: a
// request naming any other path is a request to write a sidecar somewhere the
// Box did not ask for one.
func (e *Engine) orphanScanLocked(relative string) bool {
	for _, orphan := range e.orphan {
		if orphan.Kind == "scan" && orphan.Path == relative {
			return true
		}
	}
	return false
}

// TrashSummary is what trash/ holds. It is advice and nothing else: box never
// removes a file, so emptying the trash stays a decision made by hand.
func (e *Engine) TrashSummary() TrashSummary {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	summary := TrashSummary{KeepDays: e.trashKeepDays}
	cutoff := ""
	if summary.KeepDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -summary.KeepDays).Format("2006-01-02")
	}
	for _, entry := range e.cache.Entries {
		if !entry.InTrash {
			continue
		}
		summary.Count++
		day := string(entry.File.TrashedAt)
		if len(day) >= 10 {
			day = day[:10]
			if summary.Oldest == "" || day < summary.Oldest {
				summary.Oldest = day
			}
			if cutoff != "" && day < cutoff {
				summary.Overdue++
			}
		}
	}
	return summary
}

// Verify reads every byte of every file in the Box and reports what no longer
// matches the digest recorded for it.
//
// It repairs nothing. The tool cannot know which of the two versions somebody
// wants, and rewriting either one destroys the only evidence that anything
// happened — which on a NAS without snapshots may be the first sign of bit rot.
func (e *Engine) Verify(ctx context.Context) ([]Exception, error) {
	e.mutex.Lock()
	root, marker := e.root, e.markerName
	e.mutex.Unlock()
	result, err := maintain.Verify(ctx, root, marker, nil)
	if err != nil {
		return nil, err
	}
	found := make([]Exception, 0, len(result.Mismatches))
	for _, mismatch := range result.Mismatches {
		exception := Exception{Kind: "digest mismatch", Path: mismatch.Path}
		switch {
		case mismatch.Missing:
			exception.Kind = "no scan"
			exception.Detail = "The sidecar describes a file that is not there. Nothing has been rewritten."
		default:
			exception.Detail = fmt.Sprintf(
				"Recorded as %s. The bytes now give %s. Nothing has been rewritten.",
				mismatch.Recorded, mismatch.Actual,
			)
		}
		found = append(found, exception)
	}
	return found, nil
}

// persist writes the cache back after a batch. Losing it costs one rebuild, so
// a failure here is not worth failing a batch of verified writes over.
func (e *Engine) persist() {
	_ = e.Save()
}

// Redraw drops a filed scan's stored pictures and draws the first page again
// from its file, the way intake drew it; later pages are drawn again when
// they are next looked at. An empty digest redraws every filed scan, which
// reads every file, so the lock is held only to decide what to read.
func (e *Engine) Redraw(ctx context.Context, digest string) (RedrawResult, error) {
	type target struct{ digest, path string }
	e.mutex.Lock()
	var targets []target
	for _, entry := range e.cache.Entries {
		if entry.InTrash || entry.ScanPath == "" {
			continue
		}
		if digest != "" && entry.File.Digest != digest {
			continue
		}
		targets = append(targets, target{entry.File.Digest, filepath.Join(e.root, entry.ScanPath)})
	}
	thumbs := e.thumbs
	e.mutex.Unlock()
	if digest != "" && len(targets) == 0 {
		return RedrawResult{}, fmt.Errorf("no filed scan %q", digest)
	}

	result := RedrawResult{Failed: []Exception{}}
	for _, item := range targets {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		e.pages.forget(item.digest)
		if _, err := thumbs.Drop(item.digest); err != nil {
			result.Failed = append(result.Failed, Exception{Kind: "redraw", Path: item.path, Detail: err.Error()})
			continue
		}
		read, err := scanread.ReadFile(item.path)
		if err == nil && read.NeedsRender {
			err = fmt.Errorf("no picture: %s", read.RenderError)
		}
		if err == nil {
			err = thumbs.Save(item.digest, read.Thumbs)
		}
		if err != nil {
			result.Failed = append(result.Failed, Exception{Kind: "redraw", Path: item.path, Detail: err.Error()})
			continue
		}
		result.Redrawn++
	}
	return result, nil
}
