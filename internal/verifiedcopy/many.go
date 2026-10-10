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
	"sync"
	"sync/atomic"
)

// PipelineDepth is how many source blocks CopyMany holds in flight. A slow
// destination may fall this far behind the fastest before the source read
// waits for it, which bounds memory at PipelineDepth × DefaultBufferSize.
const PipelineDepth = 4

// openSource opens the source for its single read; tests count the opens.
var openSource = os.Open

// createdTemporary sees each target's temporary file once it is open; tests
// use it to make one target's writes fail mid-copy.
var createdTemporary = func(destination string, file *os.File) {}

// syncPublishedDirectory flushes a verified publication; tests delay that sync.
var syncPublishedDirectory = SyncDirectory

// Target is one destination of CopyMany.
type Target struct {
	Destination string
	// Root and ExpectedRoot mean what they mean in Request, for this target.
	Root         string
	ExpectedRoot os.FileInfo
}

// ManyRequest copies one source into several destinations with one read.
// The source-side fields mean what they mean in Request.
type ManyRequest struct {
	Source          string
	ExpectedSource  os.FileInfo
	ExpectedDigest  string
	PreserveModTime bool
	Mode            os.FileMode
	Targets         []Target
	// Progress, when set, is called as bytes move in each target. It must not
	// block.
	Progress func(target int, phase Phase, done, total int64)
	// Wait, when set, is called between blocks of the source read and of each
	// readback. Calls of Progress and Wait never overlap, though they come
	// from several goroutines: a paused Wait holds every target still.
	Wait func(ctx context.Context) error
	// Complete is called exactly once per target, as soon as its outcome is
	// final, after publication or partial cleanup. Result is empty on failure.
	// Calls are serialized independently of Progress and Wait, so a paused
	// target cannot prevent another target from recording its completion.
	// A successful Complete precedes that target's PhasePublished progress;
	// the progress notification may wait until another target resumes.
	Complete func(target int, result Result, err error)
}

// CopyMany reads Source once and writes every block to each target's own
// temporary file. Each target then reads its copy back independently and is
// published only when that readback matches the one source digest: the
// contract of Copy holds for every target separately.
//
// Targets fail independently. A target that cannot be prepared, written, read
// back or published loses only its own partial file; the others go on. The
// source read stops early only when no target is left. A source that cannot
// be read, or changes under the read, fails every target.
//
// The returned errors have one entry per target, nil where that target was
// published. Result describes the source and is meaningful when at least one
// entry is nil.
func CopyMany(ctx context.Context, request ManyRequest) (Result, []error) {
	errs := make([]error, len(request.Targets))
	// Completion must remain available while a Wait holds the progress mutex.
	// Its callback can hand a finished target to a state-writing coordinator.
	var completionMu sync.Mutex
	completed := make([]bool, len(errs))
	complete := func(target int, result Result, err error) {
		if request.Complete == nil {
			return
		}
		completionMu.Lock()
		defer completionMu.Unlock()
		completed[target] = true
		request.Complete(target, result, err)
	}
	defer func() {
		// Early source failures have no target goroutines to report them.
		for i, err := range errs {
			if !completed[i] {
				complete(i, Result{}, err)
			}
		}
	}()
	failAll := func(err error) (Result, []error) {
		for i := range errs {
			if errs[i] == nil {
				errs[i] = err
			}
		}
		return Result{}, errs
	}
	if request.Source == "" {
		return failAll(errors.New("copy: source and destination are both required"))
	}
	for i, target := range request.Targets {
		if target.Destination == "" {
			errs[i] = errors.New("copy: source and destination are both required")
		}
	}
	// Non-following metadata operations throughout: a symbolic link in the
	// source could read outside the tree that was scanned, and one at the
	// destination would write through to somewhere nobody chose.
	info, err := os.Lstat(request.Source)
	if err != nil {
		return failAll(fmt.Errorf("source metadata: %w", err))
	}
	if !info.Mode().IsRegular() {
		return failAll(fmt.Errorf("source is not a regular file: %s", request.Source))
	}
	if request.ExpectedSource != nil && !SameVersion(info, request.ExpectedSource) {
		return failAll(errors.New("source changed since comparison"))
	}
	copies := make([]*targetCopy, len(request.Targets))
	for i, target := range request.Targets {
		if errs[i] != nil {
			continue
		}
		if errs[i] = prepareTarget(target); errs[i] == nil {
			copies[i] = &targetCopy{index: i, target: target}
		}
	}
	for i, err := range errs {
		if err != nil {
			complete(i, Result{}, err)
		}
	}
	if !anyCopy(copies) {
		return Result{}, errs
	}

	source, err := openSource(request.Source)
	if err != nil {
		return failAll(fmt.Errorf("open source: %w", err))
	}
	defer source.Close()
	opened, err := source.Stat()
	if err == nil && !SameVersion(info, opened) {
		// A network filesystem such as SMB may answer the first Lstat of a
		// file just written from a stale cache, before the server's own
		// modification time. A second Lstat, taken now the file is open,
		// must find the very file opened, unchanged, to go on.
		if again, againErr := os.Lstat(request.Source); againErr == nil && os.SameFile(info, again) && SameVersion(again, opened) {
			info = again
		}
	}
	if err != nil || !SameVersion(info, opened) {
		return failAll(errors.New("source changed while opening"))
	}
	for i, c := range copies {
		if c == nil {
			continue
		}
		if errs[i] = c.create(); errs[i] != nil {
			copies[i] = nil
			complete(i, Result{}, errs[i])
		}
	}
	if !anyCopy(copies) {
		return Result{}, errs
	}

	var callbacks sync.Mutex
	report := func(target int, phase Phase, done int64) {
		if request.Progress != nil {
			callbacks.Lock()
			defer callbacks.Unlock()
			request.Progress(target, phase, done, info.Size())
		}
	}
	wait := func(ctx context.Context) error {
		if request.Wait == nil {
			return waitFor(ctx, nil)
		}
		callbacks.Lock()
		defer callbacks.Unlock()
		return request.Wait(ctx)
	}
	var alive atomic.Int32
	var group sync.WaitGroup
	pool := make(chan []byte, PipelineDepth)
	for range PipelineDepth {
		pool <- make([]byte, DefaultBufferSize)
	}
	// sourceDone closes once the read is over; expected and sourceErr are
	// set before it closes and only read after.
	sourceDone := make(chan struct{})
	var expected string
	var sourceErr error
	for _, c := range copies {
		if c == nil {
			continue
		}
		alive.Add(1)
		c.blocks = make(chan *block, PipelineDepth)
		group.Add(1)
		go func() {
			defer group.Done()
			report(c.index, PhaseCopying, 0)
			c.write(&alive, func(done int64) { report(c.index, PhaseCopying, done) })
			<-sourceDone
			if c.err == nil && sourceErr != nil {
				c.err = sourceErr
			}
			if c.err == nil {
				c.err = c.finish(ctx, request, wait, info, expected, func(phase Phase, done, _ int64) { report(c.index, phase, done) })
			}
			if c.err != nil && c.partial != "" {
				_ = os.Remove(c.partial)
			}
			errs[c.index] = c.err
			result := Result{}
			if c.err == nil {
				result = Result{Source: info, Size: info.Size(), Digest: expected}
			}
			complete(c.index, result, c.err)
			if c.err == nil {
				// Publication is already durable at the filesystem API boundary. Send
				// its outcome before progress, which may wait behind a paused Wait.
				report(c.index, PhasePublished, info.Size())
			}
		}()
	}

	digest := sha256.New()
	for {
		if alive.Load() == 0 {
			// Every target has its own error already; this one only ends the read.
			sourceErr = errors.New("no destination left")
			break
		}
		if err := wait(ctx); err != nil {
			sourceErr = err
			break
		}
		buffer := <-pool
		read, readErr := source.Read(buffer)
		if read > 0 {
			digest.Write(buffer[:read])
			next := &block{data: buffer[:read], pool: pool}
			next.refs.Store(int32(len(request.Targets)))
			for _, c := range copies {
				if c == nil {
					next.release()
					continue
				}
				c.blocks <- next
			}
		} else {
			pool <- buffer
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			sourceErr = fmt.Errorf("read source: %w", readErr)
			break
		}
	}
	for _, c := range copies {
		if c != nil {
			close(c.blocks)
		}
	}
	if sourceErr == nil {
		// A source that changed under the copy makes the digest a digest of
		// nothing in particular — partly the old file, partly the new one.
		after, err := os.Lstat(request.Source)
		switch {
		case err != nil:
			sourceErr = fmt.Errorf("source metadata after copy: %w", err)
		case !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()):
			sourceErr = errors.New("source changed during copy")
		}
	}
	if sourceErr == nil {
		expected = hex.EncodeToString(digest.Sum(nil))
		if request.ExpectedDigest != "" && expected != request.ExpectedDigest {
			sourceErr = errors.New("source digest changed since comparison")
		}
	}
	// Completion callbacks may perform Move deletion. Release the read handle
	// before allowing targets to publish and report, including on platforms
	// that refuse deletion of an open file.
	_ = source.Close()
	close(sourceDone)
	group.Wait()
	return Result{Source: info, Size: info.Size(), Digest: expected}, errs
}

// block is one source read shared by every target. The last target to finish
// with it returns its buffer to the pool.
type block struct {
	data []byte
	refs atomic.Int32
	pool chan<- []byte
}

func (b *block) release() {
	if b.refs.Add(-1) == 0 {
		b.pool <- b.data[:cap(b.data)]
	}
}

func anyCopy(copies []*targetCopy) bool {
	for _, c := range copies {
		if c != nil {
			return true
		}
	}
	return false
}

// targetCopy is one target's side of CopyMany: its temporary file, the
// identity it was created with, and its outcome. Only its own goroutine
// touches it once writing starts.
type targetCopy struct {
	index   int
	target  Target
	file    *os.File
	partial string
	written os.FileInfo
	blocks  chan *block
	err     error
}

// prepareTarget checks a target before the source is opened: nothing stands
// at its final name and its directory exists under the pinned root.
func prepareTarget(target Target) error {
	if _, err := os.Lstat(target.Destination); err == nil {
		return fmt.Errorf("%w: %s", ErrDestinationExists, target.Destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("destination metadata: %w", err)
	}
	directory := filepath.Dir(target.Destination)
	if err := ensureDirectory(target.Root, directory, target.ExpectedRoot); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	if err := CheckRoot(target.Root, target.ExpectedRoot); err != nil {
		return err
	}
	return clearStale(directory, filepath.Base(target.Destination))
}

// create opens the temporary file. It goes in the destination's own
// directory, so publishing is a rename within one filesystem and cannot turn
// into a second copy. CreateTemp creates exclusively under a name nobody else
// holds.
func (c *targetCopy) create() error {
	if err := CheckRoot(c.target.Root, c.target.ExpectedRoot); err != nil {
		return err
	}
	directory := filepath.Dir(c.target.Destination)
	file, err := os.CreateTemp(directory, "."+filepath.Base(c.target.Destination)+".*"+PartSuffix)
	if err != nil {
		return fmt.Errorf("open destination temporary file: %w", err)
	}
	c.file, c.partial = file, file.Name()
	// Best effort: a filesystem that refuses still gets a readback, served
	// however the operating system chooses.
	_ = bypassCache(file)
	// written is the file this copy wrote. Everything after the write —
	// readback, metadata, publication — is checked against it, because they
	// all go through a path and a path can come to name another file.
	if c.written, err = file.Stat(); err != nil {
		file.Close()
		_ = os.Remove(c.partial)
		return fmt.Errorf("temporary file metadata: %w", err)
	}
	createdTemporary(c.target.Destination, file)
	return nil
}

// write drains this target's blocks. After a failed write it keeps draining
// without writing, so the shared buffers return to the pool.
func (c *targetCopy) write(alive *atomic.Int32, progress func(int64)) {
	var written int64
	for next := range c.blocks {
		if c.err == nil {
			if _, err := c.file.Write(next.data); err != nil {
				c.err = fmt.Errorf("write destination: %w", err)
				alive.Add(-1)
			} else {
				written += int64(len(next.data))
				progress(written)
			}
		}
		next.release()
	}
	if c.err != nil {
		c.file.Close()
		return
	}
	if err := c.file.Sync(); err != nil {
		c.file.Close()
		c.err = fmt.Errorf("sync destination: %w", err)
		return
	}
	_ = dropCache(c.file)
	if err := c.file.Close(); err != nil {
		c.err = fmt.Errorf("close destination: %w", err)
	}
}

// finish reads the copy back, applies metadata and publishes it.
func (c *targetCopy) finish(ctx context.Context, request ManyRequest, wait func(context.Context) error, info os.FileInfo, expected string, progress func(Phase, int64, int64)) error {
	// The verification pass opens the destination again and reads it back. It
	// is not the buffer that was just written, and the handles ask the system
	// not to answer from its cache (F_NOCACHE on macOS, POSIX_FADV_DONTNEED on
	// Linux): reopening alone would read the pages this process just wrote.
	// A filesystem may ignore that request, which is the API's boundary.
	progress(PhaseVerifying, 0, info.Size())
	buffer := make([]byte, DefaultBufferSize)
	verified, actual, err := readBack(ctx, wait, progress, c.partial, c.written, buffer, info.Size())
	if err != nil {
		return err
	}
	if verified != info.Size() || actual != expected {
		return fmt.Errorf("source and destination SHA-256 mismatch: %s != %s", expected, actual)
	}
	// Byte progress alone cannot establish verification. Announce the
	// publishing phase only after the independent readback actually matched.
	progress(PhasePublishing, info.Size(), info.Size())
	// A caller may pause or cancel at this new pre-publication boundary.
	if err := waitFor(ctx, wait); err != nil {
		return err
	}
	if request.PreserveModTime || request.Mode != 0 {
		// Applied while the file still has its unpublished name, so the final
		// name never denotes a file with incomplete metadata.
		if request.Mode != 0 {
			if err := os.Chmod(c.partial, request.Mode.Perm()); err != nil {
				return fmt.Errorf("set destination permission: %w", err)
			}
		}
		if request.PreserveModTime {
			if err := os.Chtimes(c.partial, info.ModTime(), info.ModTime()); err != nil {
				return fmt.Errorf("preserve source modification time: %w", err)
			}
		}
		if err := syncMetadata(c.partial); err != nil {
			return err
		}
	}
	// The name about to be published must still be the file that was written
	// and read back. Nothing else creates this name, but anything may remove
	// it, and a rename of whatever stands there would publish unverified bytes.
	if current, err := os.Lstat(c.partial); err != nil {
		return fmt.Errorf("temporary metadata before publish: %w", err)
	} else if !os.SameFile(current, c.written) {
		return fmt.Errorf("temporary file was replaced before publish: %s", c.partial)
	}
	// Publication never replaces: a file that appeared at the final name while
	// this one was being verified is somebody else's work.
	if err := CheckRoot(c.target.Root, c.target.ExpectedRoot); err != nil {
		return err
	}
	if err := Publish(c.partial, c.target.Destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: appeared during copy: %s", ErrDestinationExists, c.target.Destination)
		}
		return fmt.Errorf("publish destination: %w", err)
	}
	// The partial is removed by path. When a pinned Root was replaced or
	// detached meanwhile, that path no longer names it and it stays behind in
	// the detached tree: unpublished and harmless, skipped by inventories,
	// and cleared by the next copy of the same name in that directory.
	c.partial = ""
	if err := syncPublishedDirectory(filepath.Dir(c.target.Destination)); err != nil {
		return fmt.Errorf("sync destination directory: %w", err)
	}
	return nil
}
