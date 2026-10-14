// Package verifiedcopy copies one file and refuses to publish it under its
// final name until bytes read back independently from the destination produce
// the same SHA-256 digest as the bytes that were read from the source.
//
// This is the integrity contract Photo Import requires: a file that appears under
// its final name has been verified, and a file that has not been verified never
// wears that name. Everything else — progress reporting, retries, what the
// caller does afterwards — is arranged around that and belongs to the caller.
//
// What it does not promise: anything after publication. The bytes may rot on
// the disk later, which is what a verify pass is for. Nor does it promise more
// than the user-space filesystem API gives, which matters more than usual here
// because a destination is normally a NAS over SMB or AFP, where flushing
// semantics are weaker than on a local disk.
package verifiedcopy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// PartSuffix ends the name an unpublished copy has while it is being written and
// verified: .<final name>.<random digits>.dgs-part. It is the one name dgs writes
// that begins with a dot: it exists for seconds and is not something anyone
// should open.
//
// The random part makes the name belong to one copy. Two copies of the same
// final name — two dgs processes importing two cards that both hold
// IMG_0001.JPG — never write, read back or publish through a shared path, so
// neither can publish bytes the other wrote.
const PartSuffix = ".dgs-part"

// DefaultBufferSize bounds each sequential read and the interval between pause
// or cancellation checks in Copy and Check.
const DefaultBufferSize = 1024 * 1024

// Phase is where a copy has got to. A caller showing progress needs to tell
// copying from verifying, because the second pass reads the same number of
// bytes again and a single bar would appear to stall at the end.
type Phase string

const (
	PhaseCopying   Phase = "copying"
	PhaseVerifying Phase = "verifying"
	// PhasePublishing follows a successful size and digest comparison and
	// precedes destination metadata application and final-name publication.
	PhasePublishing Phase = "publishing"
	PhasePublished  Phase = "published"
)

// Request is one file to copy.
type Request struct {
	// Source and Destination are paths. Destination's directory is created if
	// it is missing.
	Source      string
	Destination string
	// ExpectedSource and ExpectedDigest optionally bind a copy to an earlier
	// observation. Both are checked before publication; omitted values leave
	// callers that have not pre-read source unchanged.
	ExpectedSource os.FileInfo
	ExpectedDigest string
	// PreserveModTime applies the source's modification time to the copy before
	// it is published, so the final name never denotes a file with incomplete
	// metadata. Photo Import preserves the source modification time.
	PreserveModTime bool
	// Mode, when non-zero, is the permission the copy is published with. Zero
	// keeps the owner-only 0600 the unpublished copy is created with. Photo
	// Import publishes DefaultPhotoMode so a shared library can read it.
	Mode os.FileMode
	// Root, when set, is the directory Destination must lie under. Every
	// directory between Root and Destination is checked with non-following
	// metadata calls and created one level at a time, so a symbolic link
	// inside the tree is refused instead of written through. Root itself may
	// be a link: it is what the user chose.
	Root string
	// ExpectedRoot optionally pins an existing Root's identity. A pinned root
	// is never created, and is rechecked before creating files and publishing.
	ExpectedRoot os.FileInfo
	// Progress, when set, is called as bytes move. It must not block. Calls of
	// Progress and Wait never overlap, but need not come from one goroutine.
	Progress func(phase Phase, done, total int64)
	// Wait, when set, is called between blocks and is how a caller pauses or
	// cancels mid-file. Returning an error abandons the copy.
	Wait func(ctx context.Context) error
}

// DefaultPhotoMode is the permission Photo Import publishes with: readable by
// whoever can read the library, writable by its owner.
const DefaultPhotoMode os.FileMode = 0o644

// Result describes the bytes and file identity observed by Copy or Check.
// Copy returns it only after verified publication; Check is a read-only digest.
type Result struct {
	// Source is the source's metadata as it was read. A caller that deletes the
	// source afterwards compares against it first: the digest describes these
	// bytes and no others.
	Source os.FileInfo
	Size   int64
	// Digest is lowercase SHA-256 hex with no algorithm prefix. Copy's source
	// stream and independent destination readback must both agree on it.
	Digest string
}

// ErrDestinationExists is returned when something is already at the final path.
// Publication never replaces a file: the one that appeared may be another
// process's verified work, and overwriting it would destroy data to avoid an
// error message.
var ErrDestinationExists = errors.New("destination exists")

// Copy copies Source to Destination and publishes it only after an independent
// readback agrees. It is CopyMany with one target.
//
// Every failure removes the partial file. A partial copy is never published and
// is never left under a name that looks final, so a retry is always a whole
// file from byte zero and never a resumption of something unverifiable.
func Copy(ctx context.Context, request Request) (Result, error) {
	if request.Source == "" || request.Destination == "" {
		return Result{}, errors.New("copy: source and destination are both required")
	}
	many := ManyRequest{
		Source:          request.Source,
		ExpectedSource:  request.ExpectedSource,
		ExpectedDigest:  request.ExpectedDigest,
		PreserveModTime: request.PreserveModTime,
		Mode:            request.Mode,
		Targets:         []Target{{Destination: request.Destination, Root: request.Root, ExpectedRoot: request.ExpectedRoot}},
		Wait:            request.Wait,
	}
	if request.Progress != nil {
		many.Progress = func(_ int, phase Phase, done, total int64) { request.Progress(phase, done, total) }
	}
	result, errs := CopyMany(ctx, many)
	if errs[0] != nil {
		return Result{}, errs[0]
	}
	return result, nil
}

// openReadBack opens the independent verification handle; tests delay an open.
var openReadBack = os.Open

func readBack(ctx context.Context, wait func(context.Context) error, progress func(Phase, int64, int64), path string, written os.FileInfo, buffer []byte, total int64) (int64, string, error) {
	file, err := openReadBack(path)
	if err != nil {
		return 0, "", fmt.Errorf("open destination for verification: %w", err)
	}
	// A readback of some other file would verify that file, not this copy.
	if opened, err := file.Stat(); err != nil {
		file.Close()
		return 0, "", fmt.Errorf("destination metadata for verification: %w", err)
	} else if !os.SameFile(opened, written) {
		file.Close()
		return 0, "", fmt.Errorf("temporary file was replaced before verification: %s", path)
	}
	_ = bypassCache(file)
	_ = dropCache(file)
	digest := sha256.New()
	var read int64
	for {
		if err := waitFor(ctx, wait); err != nil {
			file.Close()
			return 0, "", err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			digest.Write(buffer[:n])
			read += int64(n)
			progress(PhaseVerifying, read, total)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			file.Close()
			return 0, "", fmt.Errorf("read destination for verification: %w", readErr)
		}
	}
	// The digest is of the bytes already read, and a read-only handle holds
	// nothing to flush, so an error closing it says nothing about the file.
	// macOS SMB mounts return EBADF here after a complete read; failing the
	// copy on it would refuse a destination that has just been verified.
	_ = file.Close()
	return read, hex.EncodeToString(digest.Sum(nil)), nil
}

// clearStale removes leftover temporary files of this final name from abandoned
// runs: the legacy fixed name and any .<name>.<digits>.dgs-part. A partial copy
// is never resumed: there is no way to tell how much of it is right.
//
// A live copy by another process matches too. Removing it makes that copy fail
// its identity check before publication, never publish something else, so the
// cost of a mistaken removal is a retry, not a wrong file.
func clearStale(directory, name string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read destination directory: %w", err)
	}
	prefix := "." + name + "."
	for _, entry := range entries {
		candidate := entry.Name()
		if candidate != "."+name+PartSuffix {
			middle, ok := strings.CutPrefix(candidate, prefix)
			if !ok {
				continue
			}
			middle, ok = strings.CutSuffix(middle, PartSuffix)
			if !ok || middle == "" || strings.Trim(middle, "0123456789") != "" {
				continue
			}
		}
		temporary := filepath.Join(directory, candidate)
		info, err := os.Lstat(temporary)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("temporary metadata: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("temporary path is not a regular file: %s", temporary)
		}
		if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale temporary file: %w", err)
		}
	}
	return nil
}

// EnsureDirectory creates directory. Under a root, every level below it is
// checked without following links and made one at a time, so a symbolic link
// planted inside the tree is refused rather than written through.
func EnsureDirectory(root, directory string) error {
	return ensureDirectory(root, directory, nil)
}

func ensureDirectory(root, directory string, expected os.FileInfo) error {
	if err := CheckRoot(root, expected); err != nil {
		return err
	}
	if root == "" {
		directory = filepath.Clean(directory)
		if info, err := os.Stat(directory); err == nil {
			if !info.IsDir() {
				return fmt.Errorf("not a directory: %s", directory)
			}
			return syncDirectory(filepath.Dir(directory))
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := ensureDirectory("", filepath.Dir(directory), nil); err != nil {
			return err
		}
		if err := os.Mkdir(directory, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		return syncDirectory(filepath.Dir(directory))
	}

	relative, err := filepath.Rel(root, directory)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("%s is outside %s", directory, root)
	}
	if expected == nil {
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			if err := ensureDirectory("", root, nil); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	if relative == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if err := CheckRoot(root, expected); err != nil {
			return err
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			if info, err = os.Lstat(current); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("not a real directory: %s", current)
		}
		// Retry the sync even for an existing directory left by a failed attempt.
		if err := syncDirectory(filepath.Dir(current)); err != nil {
			return fmt.Errorf("sync destination parent: %w", err)
		}
	}
	return nil
}

// syncMetadata flushes a change made after the bytes were written.
//
// The handle is reopened read-write because some network filesystems — SMB and
// NAS mounts in particular — refuse fsync on a read-only descriptor even to the
// process that created the file. Ignoring the error instead would weaken the
// contract rather than satisfy it.
func syncMetadata(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open destination to sync metadata: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync destination metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close destination metadata: %w", err)
	}
	return nil
}

// syncDirectory lets filesystem tests inject directory-sync failures.
var syncDirectory = SyncDirectory

// SyncDirectory flushes a directory entry, where the platform has such a thing.
// A filesystem that has no answer to the question is not a failure — that is
// what the two ignored errors are — and this is the edge of what the user-space
// API can promise either way.
func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
		return err
	}
	return nil
}

func waitFor(ctx context.Context, wait func(context.Context) error) error {
	if wait != nil {
		return wait(ctx)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
