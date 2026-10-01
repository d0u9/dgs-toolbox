package verifiedcopy_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/verifiedcopy"
)

func write(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCopyPublishesVerifiedBytes(t *testing.T) {
	dir := t.TempDir()
	data := []byte(strings.Repeat("a scanned page\n", 5000))
	source := write(t, filepath.Join(dir, "in", "scan.pdf"), data)
	destination := filepath.Join(dir, "box", "2026", "2026-09-22", "scan.pdf")

	result, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{
		Source:      source,
		Destination: destination,
	})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	sum := sha256.Sum256(data)
	if result.Digest != hex.EncodeToString(sum[:]) {
		t.Errorf("digest: got %s", result.Digest)
	}
	if result.Size != int64(len(data)) {
		t.Errorf("size: got %d want %d", result.Size, len(data))
	}
	published, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read published: %v", err)
	}
	if string(published) != string(data) {
		t.Error("the published bytes are not the source bytes")
	}
}

// The contract: nothing wears the final name until the readback agrees, and
// nothing that failed is left under a name that looks finished.
func TestNothingIsLeftBehindOnFailure(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "box", "scan.pdf")
	_, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{
		Source:      filepath.Join(dir, "in", "missing.pdf"),
		Destination: destination,
	})
	if err == nil {
		t.Fatal("copying a file that is not there succeeded")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Error("the destination exists after a failure")
	}
	entries, _ := os.ReadDir(filepath.Dir(destination))
	for _, entry := range entries {
		if strings.Contains(entry.Name(), verifiedcopy.PartSuffix) {
			t.Errorf("a partial file was left behind: %s", entry.Name())
		}
	}
}

// Publication never replaces. The file that appeared may be another process's
// verified work, and overwriting it would destroy data to avoid an error.
func TestExistingDestinationIsRefused(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "scan.pdf"), []byte("new"))
	destination := write(t, filepath.Join(dir, "box", "scan.pdf"), []byte("already here"))

	_, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: destination})
	if !errors.Is(err, verifiedcopy.ErrDestinationExists) {
		t.Fatalf("got %v, want ErrDestinationExists", err)
	}
	kept, _ := os.ReadFile(destination)
	if string(kept) != "already here" {
		t.Error("the existing file was overwritten")
	}
}

// A leftover .dgs-part from an abandoned run is discarded, never resumed: there
// is no way to tell how much of it is right.
func TestStalePartIsDiscardedNotResumed(t *testing.T) {
	dir := t.TempDir()
	data := []byte("the real contents")
	source := write(t, filepath.Join(dir, "in", "scan.pdf"), data)
	destination := filepath.Join(dir, "box", "scan.pdf")
	stale := filepath.Join(dir, "box", "."+filepath.Base(destination)+verifiedcopy.PartSuffix)
	write(t, stale, []byte("half of some older attempt"))

	if _, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: destination}); err != nil {
		t.Fatalf("copy: %v", err)
	}
	published, _ := os.ReadFile(destination)
	if string(published) != string(data) {
		t.Errorf("the stale partial file was resumed: got %q", published)
	}
	if _, err := os.Lstat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Error("the stale partial file is still there")
	}
}

func TestProgressReportsBothPasses(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "scan.pdf"), []byte(strings.Repeat("x", 4096)))
	seen := map[verifiedcopy.Phase]bool{}
	_, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{
		Source:      source,
		Destination: filepath.Join(dir, "box", "scan.pdf"),
		Progress: func(phase verifiedcopy.Phase, done, total int64) {
			seen[phase] = true
			if done > total {
				t.Errorf("%s reported %d of %d", phase, done, total)
			}
		},
	})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	// Verifying reads the same bytes a second time. A caller that could not
	// tell the passes apart would show a bar that stalls at the end.
	for _, phase := range []verifiedcopy.Phase{verifiedcopy.PhaseCopying, verifiedcopy.PhaseVerifying, verifiedcopy.PhasePublished} {
		if !seen[phase] {
			t.Errorf("no progress reported for %s", phase)
		}
	}
}

func TestCancellationLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "scan.pdf"), []byte(strings.Repeat("x", 4<<20)))
	destination := filepath.Join(dir, "box", "scan.pdf")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{Source: source, Destination: destination})
	if err == nil {
		t.Fatal("a cancelled copy succeeded")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Error("a cancelled copy published something")
	}
}

func TestSourceMustBeARegularFile(t *testing.T) {
	dir := t.TempDir()
	// A symbolic link could point outside the tree that was scanned, and its
	// target could change between the scan and the copy.
	target := write(t, filepath.Join(dir, "in", "real.pdf"), []byte("data"))
	link := filepath.Join(dir, "in", "link.pdf")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	_, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{
		Source:      link,
		Destination: filepath.Join(dir, "box", "scan.pdf"),
	})
	if err == nil {
		t.Fatal("a symbolic link was copied")
	}
}

func TestPreserveModTimeIsOptional(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "scan.pdf"), []byte("data"))
	info, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(dir, "box", "kept.pdf")
	if _, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{
		Source: source, Destination: kept, PreserveModTime: true,
	}); err != nil {
		t.Fatalf("copy: %v", err)
	}
	keptInfo, err := os.Lstat(kept)
	if err != nil {
		t.Fatal(err)
	}
	if !keptInfo.ModTime().Equal(info.ModTime()) {
		t.Errorf("modification time not preserved: %v vs %v", keptInfo.ModTime(), info.ModTime())
	}
}

// partFiles lists the unpublished copies in dir.
func partFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), verifiedcopy.PartSuffix) {
			parts = append(parts, filepath.Join(dir, entry.Name()))
		}
	}
	return parts
}

// verifyingWait runs once, at the first pause of the readback pass.
func verifyingWait(once func()) (func(verifiedcopy.Phase, int64, int64), func(context.Context) error) {
	var phase verifiedcopy.Phase
	done := false
	return func(p verifiedcopy.Phase, _, _ int64) { phase = p },
		func(context.Context) error {
			if phase == verifiedcopy.PhaseVerifying && !done {
				done = true
				once()
			}
			return nil
		}
}

// Another writer replacing the unpublished copy while it is read back must not
// get its bytes published under the final name: the readback verified this
// copy's file, and only that file may be published.
func TestReplacedTemporaryIsNeverPublished(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "IMG.JPG"), []byte(strings.Repeat("X", 3<<20)))
	destination := filepath.Join(dir, "out", "IMG.JPG")
	progress, wait := verifyingWait(func() {
		for _, part := range partFiles(t, filepath.Dir(destination)) {
			os.Remove(part)
			write(t, part, []byte("PARTIAL-OTHER"))
		}
	})
	_, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: destination, Progress: progress, Wait: wait})
	if err == nil {
		t.Fatal("copy succeeded although its temporary file was replaced")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("something was published: %v", err)
	}
}

// Two copies of one final name never share a temporary path.
func TestTemporaryNameIsUniquePerCopy(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "IMG.JPG"), []byte("photo"))
	destination := filepath.Join(dir, "out", "IMG.JPG")
	var seen string
	progress, wait := verifyingWait(func() {
		parts := partFiles(t, filepath.Dir(destination))
		if len(parts) != 1 {
			t.Fatalf("parts = %v", parts)
		}
		seen = filepath.Base(parts[0])
	})
	if _, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: destination, Progress: progress, Wait: wait}); err != nil {
		t.Fatal(err)
	}
	if seen == ".IMG.JPG"+verifiedcopy.PartSuffix || !strings.HasPrefix(seen, ".IMG.JPG.") {
		t.Fatalf("temporary name %q is not unique to the copy", seen)
	}
}

// A file appearing at the final name during verification is never replaced.
func TestPublicationNeverReplacesAFileThatAppeared(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "IMG.JPG"), []byte("new photo"))
	destination := filepath.Join(dir, "out", "IMG.JPG")
	progress, wait := verifyingWait(func() { write(t, destination, []byte("someone else's")) })
	_, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: destination, Progress: progress, Wait: wait})
	if !errors.Is(err, verifiedcopy.ErrDestinationExists) {
		t.Fatalf("err = %v, want ErrDestinationExists", err)
	}
	if data, _ := os.ReadFile(destination); string(data) != "someone else's" {
		t.Fatalf("existing file replaced: %q", data)
	}
	if parts := partFiles(t, filepath.Dir(destination)); len(parts) != 0 {
		t.Fatalf("left behind %v", parts)
	}
}

func TestStaleNumberedPartIsDiscarded(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "scan.pdf"), []byte("real"))
	destination := filepath.Join(dir, "box", "scan.pdf")
	stale := write(t, filepath.Join(dir, "box", ".scan.pdf.123456"+verifiedcopy.PartSuffix), []byte("old"))
	unrelated := write(t, filepath.Join(dir, "box", ".scan.pdf.notes"+verifiedcopy.PartSuffix), []byte("keep"))
	if _, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: destination}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Error("stale numbered part is still there")
	}
	if _, err := os.Lstat(unrelated); err != nil {
		t.Error("a file that is not a part of this name was removed")
	}
}

func TestModeIsAppliedBeforePublication(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "IMG.JPG"), []byte("photo"))
	destination := filepath.Join(dir, "out", "IMG.JPG")
	result, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: destination, Mode: verifiedcopy.DefaultPhotoMode})
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(destination)
	if info.Mode().Perm() != verifiedcopy.DefaultPhotoMode {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	if result.Source == nil || result.Source.Size() != 5 {
		t.Fatalf("source metadata = %v", result.Source)
	}
}

// Under a Root, a symbolic link inside the tree is refused, not written through.
func TestRootRefusesSymlinkedDirectory(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "in", "IMG.JPG"), []byte("photo"))
	root, elsewhere := filepath.Join(dir, "library"), filepath.Join(dir, "elsewhere")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(root, "DCIM")); err != nil {
		t.Fatal(err)
	}
	_, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: filepath.Join(root, "DCIM", "sub", "IMG.JPG"), Root: root})
	if err == nil || !strings.Contains(err.Error(), "not a real directory") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("wrote through the link: %v", entries)
	}
	if _, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: filepath.Join(root, "a", "b", "IMG.JPG"), Root: root}); err != nil {
		t.Fatalf("nested real directories: %v", err)
	}
}
