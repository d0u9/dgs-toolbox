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
