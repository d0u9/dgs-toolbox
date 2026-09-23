// Package verifiedcopy copies one file and refuses to publish it under its
// final name until bytes read back independently from the destination produce
// the same SHA-256 digest as the bytes that were read from the source.
//
// This is the integrity contract Photo Import states and dgs box inherits, and
// it is the highest-priority requirement in either: a file that appears under
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
	"syscall"
)

// PartSuffix is what an unpublished copy is named while it is being written and
// verified. It is the one name dgs writes that begins with a dot: it exists for
// seconds and is not something anyone should open.
const PartSuffix = ".dgs-part"

// Phase is where a copy has got to. A caller showing progress needs to tell
// copying from verifying, because the second pass reads the same number of
// bytes again and a single bar would appear to stall at the end.
type Phase string

const (
	PhaseCopying   Phase = "copying"
	PhaseVerifying Phase = "verifying"
	PhasePublished Phase = "published"
)

// Request is one file to copy.
type Request struct {
	// Source and Destination are paths. Destination's directory is created if
	// it is missing.
	Source      string
	Destination string
	// PreserveModTime applies the source's modification time to the copy before
	// it is published, so the final name never denotes a file with incomplete
	// metadata. A Box does not want this — a scan's intake date is a fact about
	// the Box, not about the file it came from — and Photo Import does.
	PreserveModTime bool
	// Progress, when set, is called as bytes move. It must not block: it runs
	// on the copying goroutine.
	Progress func(phase Phase, done, total int64)
	// Wait, when set, is called between blocks and is how a caller pauses or
	// cancels mid-file. Returning an error abandons the copy.
	Wait func(ctx context.Context) error
}

// Result is a completed, verified, published copy.
type Result struct {
	Size int64
	// Digest is the SHA-256 both passes agreed on, lowercase hex with no
	// algorithm prefix. It is what identifies the file from here on.
	Digest string
}

// ErrDestinationExists is returned when something is already at the final path.
// Publication never replaces a file: the one that appeared may be another
// process's verified work, and overwriting it would destroy data to avoid an
// error message.
var ErrDestinationExists = errors.New("destination exists")

// Copy copies Source to Destination and publishes it only after an independent
// readback agrees.
//
// Every failure removes the partial file. A partial copy is never published and
// is never left under a name that looks final, so a retry is always a whole
// file from byte zero and never a resumption of something unverifiable.
func Copy(ctx context.Context, request Request) (Result, error) {
	if request.Source == "" || request.Destination == "" {
		return Result{}, errors.New("copy: source and destination are both required")
	}
	// partial holds the temporary path once it belongs to this copy, so every
	// failure path below discards it rather than leaving an unverifiable
	// remnant in the destination directory.
	var partial string
	fail := func(err error) (Result, error) {
		if partial != "" {
			_ = os.Remove(partial)
		}
		return Result{}, err
	}

	// Non-following metadata operations throughout: a symbolic link in the
	// source could read outside the tree that was scanned, and one at the
	// destination would write through to somewhere nobody chose.
	info, err := os.Lstat(request.Source)
	if err != nil {
		return fail(fmt.Errorf("source metadata: %w", err))
	}
	if !info.Mode().IsRegular() {
		return fail(fmt.Errorf("source is not a regular file: %s", request.Source))
	}
	if _, err := os.Lstat(request.Destination); err == nil {
		return fail(fmt.Errorf("%w: %s", ErrDestinationExists, request.Destination))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("destination metadata: %w", err))
	}
	directory := filepath.Dir(request.Destination)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fail(fmt.Errorf("create destination directory: %w", err))
	}
	// The temporary file goes in the destination's own directory, so publishing
	// is a rename within one filesystem and cannot turn into a second copy.
	temporary := filepath.Join(directory, "."+filepath.Base(request.Destination)+PartSuffix)
	if err := clearStale(temporary); err != nil {
		return fail(err)
	}

	source, err := os.Open(request.Source)
	if err != nil {
		return fail(fmt.Errorf("open source: %w", err))
	}
	defer source.Close()
	// Exclusive creation, so an unrelated file that happens to sit at the
	// temporary path is never silently written into.
	destination, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fail(fmt.Errorf("open destination temporary file: %w", err))
	}
	partial = temporary

	sourceDigest := sha256.New()
	buffer := make([]byte, 1024*1024)
	var copied int64
	report(request, PhaseCopying, 0, info.Size())
	for {
		if err := waitFor(ctx, request); err != nil {
			destination.Close()
			return fail(err)
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			chunk := buffer[:read]
			if _, err := destination.Write(chunk); err != nil {
				destination.Close()
				return fail(fmt.Errorf("write destination: %w", err))
			}
			sourceDigest.Write(chunk)
			copied += int64(read)
			report(request, PhaseCopying, copied, info.Size())
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			destination.Close()
			return fail(fmt.Errorf("read source: %w", readErr))
		}
	}
	if err := destination.Sync(); err != nil {
		destination.Close()
		return fail(fmt.Errorf("sync destination: %w", err))
	}
	if err := destination.Close(); err != nil {
		return fail(fmt.Errorf("close destination: %w", err))
	}

	// A source that changed under the copy makes the digest a digest of nothing
	// in particular — partly the old file, partly the new one.
	after, err := os.Lstat(request.Source)
	if err != nil {
		return fail(fmt.Errorf("source metadata after copy: %w", err))
	}
	if !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return fail(errors.New("source changed during copy"))
	}

	// The verification pass opens the destination again and reads it back. It
	// is not the buffer that was just written: a digest taken over bytes still
	// in memory verifies memory, not the disk.
	expected := hex.EncodeToString(sourceDigest.Sum(nil))
	report(request, PhaseVerifying, 0, info.Size())
	verified, actual, err := readBack(ctx, request, temporary, buffer, info.Size())
	if err != nil {
		return fail(err)
	}
	if verified != info.Size() || actual != expected {
		return fail(fmt.Errorf("source and destination SHA-256 mismatch: %s != %s", expected, actual))
	}

	if request.PreserveModTime {
		// Applied while the file still has its unpublished name, so the final
		// name never denotes a file with incomplete metadata.
		if err := os.Chtimes(temporary, info.ModTime(), info.ModTime()); err != nil {
			return fail(fmt.Errorf("preserve source modification time: %w", err))
		}
		if err := syncMetadata(temporary); err != nil {
			return fail(err)
		}
	}

	// Checked again immediately before the rename: a file that appeared while
	// this one was being verified is somebody else's work and must not be
	// destroyed to save an error message.
	if _, err := os.Lstat(request.Destination); err == nil {
		return fail(fmt.Errorf("%w: appeared during copy: %s", ErrDestinationExists, request.Destination))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("destination metadata before publish: %w", err))
	}
	if err := os.Rename(temporary, request.Destination); err != nil {
		return fail(fmt.Errorf("publish destination: %w", err))
	}
	partial = ""
	if err := SyncDirectory(directory); err != nil {
		return Result{}, fmt.Errorf("sync destination directory: %w", err)
	}
	report(request, PhasePublished, info.Size(), info.Size())
	return Result{Size: info.Size(), Digest: expected}, nil
}

func readBack(ctx context.Context, request Request, path string, buffer []byte, total int64) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("open destination for verification: %w", err)
	}
	digest := sha256.New()
	var read int64
	for {
		if err := waitFor(ctx, request); err != nil {
			file.Close()
			return 0, "", err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			digest.Write(buffer[:n])
			read += int64(n)
			report(request, PhaseVerifying, read, total)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			file.Close()
			return 0, "", fmt.Errorf("read destination for verification: %w", readErr)
		}
	}
	if err := file.Close(); err != nil {
		return 0, "", fmt.Errorf("close destination verification: %w", err)
	}
	return read, hex.EncodeToString(digest.Sum(nil)), nil
}

// clearStale removes a leftover temporary file from an abandoned run. A partial
// copy is never resumed: there is no way to tell how much of it is right.
func clearStale(temporary string) error {
	info, err := os.Lstat(temporary)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("temporary metadata: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("temporary path is not a regular file: %s", temporary)
	}
	if err := os.Remove(temporary); err != nil {
		return fmt.Errorf("remove stale temporary file: %w", err)
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

func report(request Request, phase Phase, done, total int64) {
	if request.Progress != nil {
		request.Progress(phase, done, total)
	}
}

func waitFor(ctx context.Context, request Request) error {
	if request.Wait != nil {
		return request.Wait(ctx)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
