// Package publish puts one scan into a Box.
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
	// IntakeDate is the day this is being taken in, and is the only thing the
	// destination path encodes. Type, amount and expiry all change later, so a
	// path built from any of them would turn every correction into a file move.
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
}

// Publish copies a scan into the Box and publishes it, in this order:
//
//  1. Copy into the intake date's directory under a .dgs-part name in that same
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
	directory := filepath.Join(request.Root, DirectoryFor(request.IntakeDate))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return Result{}, fmt.Errorf("create intake directory: %w", err)
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
		// A Box does not preserve the source's modification time. A scan's date
		// in the Box is its intake date, which is a fact about the Box, and the
		// file's own timestamps have been overwritten by copying and syncing
		// long before it got here.
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
		record.IngestedAt = sidecar.Timestamp(clock(request).Format(time.RFC3339))
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
	entry.At = clock(request).Format(time.RFC3339)
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
	}, nil
}

// DirectoryFor is the path inside a Box that a scan taken in on date belongs
// to, relative to the root: <YYYY>/<YYYY-MM-DD>.
//
// The year level exists so a directory listing of the root stays readable after
// a decade of daily folders. Bucketing by intake date rather than by the date
// on the document is deliberate: the intake date is a fact about this Box, is
// known exactly, is never revised, and groups a batch usefully, because what
// was scanned on one day is usually related.
func DirectoryFor(date box.Date) string {
	return filepath.Join(fmt.Sprintf("%04d", date.Year), date.String())
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
