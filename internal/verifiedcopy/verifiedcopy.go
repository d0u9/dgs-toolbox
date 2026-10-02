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
	// Progress, when set, is called as bytes move. It must not block: it runs
	// on the copying goroutine.
	Progress func(phase Phase, done, total int64)
	// Wait, when set, is called between blocks and is how a caller pauses or
	// cancels mid-file. Returning an error abandons the copy.
	Wait func(ctx context.Context) error
}

// DefaultPhotoMode is the permission Photo Import publishes with: readable by
// whoever can read the library, writable by its owner.
const DefaultPhotoMode os.FileMode = 0o644

// Result is a completed, verified, published copy.
type Result struct {
	// Source is the source's metadata as it was read. A caller that deletes the
	// source afterwards compares against it first: the digest describes these
	// bytes and no others.
	Source os.FileInfo
	Size   int64
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
	if err := ensureDirectory(request.Root, directory); err != nil {
		return fail(fmt.Errorf("create destination directory: %w", err))
	}
	if err := clearStale(directory, filepath.Base(request.Destination)); err != nil {
		return fail(err)
	}

	source, err := os.Open(request.Source)
	if err != nil {
		return fail(fmt.Errorf("open source: %w", err))
	}
	defer source.Close()
	// The temporary file goes in the destination's own directory, so publishing
	// is a rename within one filesystem and cannot turn into a second copy.
	// CreateTemp creates exclusively under a name nobody else holds.
	destination, err := os.CreateTemp(directory, "."+filepath.Base(request.Destination)+".*"+PartSuffix)
	if err != nil {
		return fail(fmt.Errorf("open destination temporary file: %w", err))
	}
	temporary := destination.Name()
	partial = temporary
	// written is the file this copy wrote. Everything after the write —
	// readback, metadata, publication — is checked against it, because they
	// all go through a path and a path can come to name another file.
	written, err := destination.Stat()
	if err != nil {
		destination.Close()
		return fail(fmt.Errorf("temporary file metadata: %w", err))
	}

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
	verified, actual, err := readBack(ctx, request, temporary, written, buffer, info.Size())
	if err != nil {
		return fail(err)
	}
	if verified != info.Size() || actual != expected {
		return fail(fmt.Errorf("source and destination SHA-256 mismatch: %s != %s", expected, actual))
	}

	if request.PreserveModTime || request.Mode != 0 {
		// Applied while the file still has its unpublished name, so the final
		// name never denotes a file with incomplete metadata.
		if request.Mode != 0 {
			if err := os.Chmod(temporary, request.Mode.Perm()); err != nil {
				return fail(fmt.Errorf("set destination permission: %w", err))
			}
		}
		if request.PreserveModTime {
			if err := os.Chtimes(temporary, info.ModTime(), info.ModTime()); err != nil {
				return fail(fmt.Errorf("preserve source modification time: %w", err))
			}
		}
		if err := syncMetadata(temporary); err != nil {
			return fail(err)
		}
	}

	// The name about to be published must still be the file that was written
	// and read back. Nothing else creates this name, but anything may remove
	// it, and a rename of whatever stands there would publish unverified bytes.
	if current, err := os.Lstat(temporary); err != nil {
		return fail(fmt.Errorf("temporary metadata before publish: %w", err))
	} else if !os.SameFile(current, written) {
		return fail(fmt.Errorf("temporary file was replaced before publish: %s", temporary))
	}
	// Publication never replaces: a file that appeared at the final name while
	// this one was being verified is somebody else's work.
	if err := Publish(temporary, request.Destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fail(fmt.Errorf("%w: appeared during copy: %s", ErrDestinationExists, request.Destination))
		}
		return fail(fmt.Errorf("publish destination: %w", err))
	}
	partial = ""
	if err := SyncDirectory(directory); err != nil {
		return Result{}, fmt.Errorf("sync destination directory: %w", err)
	}
	report(request, PhasePublished, info.Size(), info.Size())
	return Result{Source: info, Size: info.Size(), Digest: expected}, nil
}

func readBack(ctx context.Context, request Request, path string, written os.FileInfo, buffer []byte, total int64) (int64, string, error) {
	file, err := os.Open(path)
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

// ensureDirectory creates directory. Under a root, every level below it is
// checked without following links and made one at a time, so a symbolic link
// planted inside the tree is refused rather than written through.
func ensureDirectory(root, directory string) error {
	if root == "" {
		return os.MkdirAll(directory, 0o755)
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("%s is outside %s", directory, root)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
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
