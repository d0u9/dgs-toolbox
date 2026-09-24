// Package publish puts one scan into a Box, and moves it when its dates say it
// belongs elsewhere.
//
// It is the only place a Box is written to during intake, and it holds the
// ordering that makes the result trustworthy: copy and verify first, then the
// final name, then the sidecar, then the log. Nothing before the verified
// rename is visible under a name that looks finished, and nothing after it is
// allowed to un-publish a file that passed.
//
// The source in the inbox is never deleted. Whether to empty a temporary folder
// is a decision for its owner, made once the Box is known to hold the files;
// box does not make it.
package publish

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
	"dgs-toolbox/internal/box/sidecar"
	"dgs-toolbox/internal/verifiedcopy"
)

// TrashDirectory is where a discarded scan goes, under the Box root. Nothing in
// a Box is deleted, so this is as far as removal goes, and the trash takes part
// in deduplication so the same judgement is not made twice.
const TrashDirectory = "trash"

// Request is one scan to publish.
type Request struct {
	// Root is the Box. It must already hold a marker; publishing refuses
	// otherwise rather than creating one.
	Root string
	// MarkerName overrides the marker's filename. Empty uses the default.
	MarkerName string
	// Source is the file in the inbox. It is read, never moved and never
	// deleted.
	Source string
	// IntakeDate is the day this is being taken in. It places the scan only
	// when the sidecar carries no event date; see FilingDate.
	IntakeDate box.Date
	// Digest is the whole-file digest from the one read, "sha256:" and hex. It
	// is checked against what the copy computes: a mismatch means the file
	// changed between being read and being copied, and publishing it would
	// store a sidecar that describes different bytes.
	Digest string
	// Sidecar is everything known about the scan. Digest, Size and the
	// published filename are filled in here rather than by the caller.
	Sidecar sidecar.File
	// Progress is passed through to the copy.
	Progress func(phase verifiedcopy.Phase, done, total int64)
	// Now is the clock, for the log entry. Zero means time.Now.
	Now time.Time
}

// Result is what was published.
type Result struct {
	// Path is the published file, absolute.
	Path string
	// RelativePath is the same file relative to the Box root, which is what the
	// log records: a Box that is moved or mounted somewhere else keeps its
	// history.
	RelativePath string
	// SidecarPath is the sidecar beside it.
	SidecarPath string
	// Size and Digest are what the verified copy produced.
	Size   int64
	Digest string
	// ShortDigest is the prefix used in the names, which is longer than the
	// default when a collision had to be broken.
	ShortDigest string
	// Sidecar is the record as written, with what Publish filled in: digest,
	// size, original filename and ingested_at. A caller keeping a copy of the
	// sidecar keeps this one, or its next save drops those fields.
	Sidecar sidecar.File
}

// Publish copies a scan into the Box and publishes it, in this order:
//
//  1. Copy into the filing date's directory under a .dgs-part name in that same
//     directory, hashing the source bytes during the copy.
//  2. Read the destination copy back independently and hash it.
//  3. Only if the digests match, rename it to its final name, which fails
//     rather than replace a file that appeared in the meantime.
//  4. Write the sidecar.
//  5. Append one line to the log.
//
// A failure at any step retries the whole file rather than resuming part of it.
// A partial copy is never published and never left under a name that looks
// final.
func Publish(ctx context.Context, request Request) (Result, error) {
	if err := box.RequireBox(request.Root, request.MarkerName); err != nil {
		return Result{}, err
	}
	if request.IntakeDate.Zero() {
		return Result{}, errors.New("publish: no intake date")
	}
	if strings.TrimSpace(request.Digest) == "" {
		return Result{}, errors.New("publish: no digest")
	}
	directory := filepath.Join(request.Root, DirectoryFor(FilingDate(request.Sidecar, request.IntakeDate)))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return Result{}, fmt.Errorf("create filing directory: %w", err)
	}

	stem, extension := splitName(filepath.Base(request.Source))
	// The prefix is extended until nothing in this directory already wears it.
	// Eight hex digits collide about as often as never, but "about as often as
	// never" over decades of a Box is not the same as never, and the failure it
	// would cause — two scans pointing at one sidecar — is silent.
	short, err := freePrefix(directory, request.Digest, stem, extension)
	if err != nil {
		return Result{}, err
	}
	destination := filepath.Join(directory, stem+"-"+short+extension)

	copied, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{
		Source:      request.Source,
		Destination: destination,
		// A Box does not preserve the source's modification time. A scan's
		// dates in the Box are in its sidecar, and the file's own timestamps
		// have been overwritten by copying and syncing long before it got here.
		Progress: request.Progress,
	})
	if err != nil {
		return Result{}, err
	}
	// The copy hashed the bytes it actually read. If that disagrees with what
	// the one read saw, the file changed underneath and the sidecar about to be
	// written describes different bytes.
	if got := digest.Prefix + copied.Digest; got != request.Digest {
		_ = os.Remove(destination)
		return Result{}, fmt.Errorf("source changed between reading and copying: %s != %s", request.Digest, got)
	}

	record := request.Sidecar
	record.Digest = request.Digest
	record.Size = copied.Size
	record.OriginalFilename = filepath.Base(request.Source)
	if record.IngestedAt.Zero() {
		record.IngestedAt = sidecar.Timestamp(clock(request).UTC().Format(time.RFC3339))
	}
	sidecarPath := sidecar.PathFor(directory, request.Digest, len(short))
	if err := sidecar.Save(sidecarPath, record); err != nil {
		// The file is verified and published; only its metadata failed. Removing
		// it now would throw away work that passed the integrity check to tidy
		// up a bookkeeping failure, so it stays and the error says what is
		// missing. A later run finds a file with no sidecar and reports it as an
		// orphan, which is recoverable; a deleted scan is not.
		return Result{}, fmt.Errorf("published %s but could not write its sidecar: %w", destination, err)
	}

	relative, err := filepath.Rel(request.Root, destination)
	if err != nil {
		relative = destination
	}
	entry := boxlog.Import(request.Digest, filepath.ToSlash(relative))
	entry.At = clock(request).UTC().Format(time.RFC3339)
	if err := boxlog.Append(boxlog.PathFor(request.Root), entry); err != nil {
		// Same reasoning: the scan is in the Box and its metadata is beside it.
		// A missing log line loses history, not data.
		return Result{}, fmt.Errorf("published %s but could not append to the log: %w", destination, err)
	}
	return Result{
		Path:         destination,
		RelativePath: filepath.ToSlash(relative),
		SidecarPath:  sidecarPath,
		Size:         copied.Size,
		Digest:       request.Digest,
		ShortDigest:  short,
		Sidecar:      record,
	}, nil
}

// DirectoryFor is the path inside a Box that a scan filed under date belongs
// to, relative to the root: <YYYY>/<MM>, or <YYYY> alone for a date known only
// to the year.
//
// A Box is looked through by when things happened — "the 2019 tax receipts" —
// so the tree follows the date on the document. A month level keeps a year of
// scans to directories of a readable size without a folder per day.
func DirectoryFor(date box.Date) string {
	if date.Month == 0 {
		return fmt.Sprintf("%04d", date.Year)
	}
	return filepath.Join(fmt.Sprintf("%04d", date.Year), fmt.Sprintf("%02d", date.Month))
}

// FilingDate is the date that places a scan: its event date, or for a file
// split into documents the latest event date among the file and its
// documents, so a pile of a year's statements lands in the year it ends.
// A scan with no event date anywhere falls back to fallback, which is the
// day it was taken in.
func FilingDate(record sidecar.File, fallback box.Date) box.Date {
	latest := record.EventDate
	for _, document := range record.Documents {
		if latest.Zero() || document.EventDate.Last().After(latest.Last()) {
			latest = document.EventDate
		}
	}
	if latest.Zero() {
		return fallback
	}
	return latest
}

// IntakeDateOf is the day a filed scan was taken in, read from its sidecar in
// the local zone. It is the zero Date when the sidecar does not say.
func IntakeDateOf(record sidecar.File) box.Date {
	instant, err := time.Parse(time.RFC3339, strings.TrimSpace(string(record.IngestedAt)))
	if err != nil {
		return box.Date{}
	}
	return box.DateOf(instant, nil)
}

// freePrefix finds the shortest digest prefix, from the default length up, that
// no file in this directory is already using.
func freePrefix(directory, fullDigest, stem, extension string) (string, error) {
	hexLength := len(digest.Short(fullDigest, 0))
	for length := sidecar.DefaultPrefix; length <= hexLength; length++ {
		short := digest.Short(fullDigest, length)
		file := filepath.Join(directory, stem+"-"+short+extension)
		record := sidecar.PathFor(directory, fullDigest, length)
		fileTaken, err := exists(file)
		if err != nil {
			return "", err
		}
		recordTaken, err := exists(record)
		if err != nil {
			return "", err
		}
		if !fileTaken && !recordTaken {
			return short, nil
		}
	}
	return "", fmt.Errorf("no free name for %s in %s", fullDigest, directory)
}

func exists(path string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return false, nil
}

// splitName separates a filename into the stem that arrived and its extension.
// The stem is kept first in the published name so the directory still sorts the
// way the scanner named things.
func splitName(name string) (stem, extension string) {
	extension = filepath.Ext(name)
	return strings.TrimSuffix(name, extension), extension
}

func clock(request Request) time.Time {
	if request.Now.IsZero() {
		return time.Now()
	}
	return request.Now
}
