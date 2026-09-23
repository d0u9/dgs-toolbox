package maintain_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/maintain"
	"dgs-toolbox/internal/box/publish"
	"dgs-toolbox/internal/box/sidecar"
)

var intake = box.Date{Year: 2026, Month: 9, Day: 22}

func newBox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := maintain.Init(root, "", time.Now()); err != nil {
		t.Fatalf("init: %v", err)
	}
	return root
}

func file(t *testing.T, root, name, body string) publish.Result {
	t.Helper()
	inbox := t.TempDir()
	path := filepath.Join(inbox, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := publish.Publish(context.Background(), publish.Request{
		Root: root, Source: path, IntakeDate: intake, Digest: digest.Whole([]byte(body)),
		Sidecar: sidecar.File{Kind: sidecar.KindPDF, Type: "unsorted"},
	})
	if err != nil {
		t.Fatalf("publish %s: %v", name, err)
	}
	return result
}

func TestInitWritesTheMarkerAndLogsIt(t *testing.T) {
	root := t.TempDir()
	if err := maintain.Init(root, "", time.Now()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := box.RequireBox(root, ""); err != nil {
		t.Fatalf("the new Box is not a Box: %v", err)
	}
	entries, err := boxlog.Read(boxlog.PathFor(root))
	if err != nil || len(entries) != 1 || entries[0].Action != boxlog.ActionInit {
		t.Errorf("init log: %+v %v", entries, err)
	}
	// Doing it twice would reset the version and the creation date, which are
	// the two things the marker remembers.
	if err := maintain.Init(root, "", time.Now()); err == nil {
		t.Error("an existing Box was re-initialised")
	}
}

func TestReindexCountsAndReportsOrphans(t *testing.T) {
	root := newBox(t)
	file(t, root, "Scan_0012.pdf", "a boarding pass")
	file(t, root, "Scan_0013.pdf", "a power bill")
	// A file put there by hand, with no sidecar.
	day := filepath.Join(root, "2026", "2026-09-22")
	if err := os.WriteFile(filepath.Join(day, "Scan_0099-deadbeef.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cacheDir := t.TempDir()
	result, err := maintain.Reindex(root, "", cacheDir)
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if result.Scans != 2 {
		t.Errorf("scans: got %d want 2", result.Scans)
	}
	if !result.Rebuilt {
		t.Error("the first run should have built the cache from nothing")
	}
	if len(result.Orphans) != 1 || result.Orphans[0].Kind != "scan" {
		t.Errorf("orphans: %+v", result.Orphans)
	}
	// The second run refreshes rather than rebuilds.
	again, err := maintain.Reindex(root, "", cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Rebuilt {
		t.Error("an existing cache was rebuilt instead of refreshed")
	}
	if again.Scans != result.Scans {
		t.Errorf("the second run found %d scans, the first %d", again.Scans, result.Scans)
	}
}

// Reindex takes the digest from the sidecar and never reads a scan's bytes.
// That is the whole difference from Verify, and it is what makes it seconds.
func TestReindexDoesNotNoticeChangedBytes(t *testing.T) {
	root := newBox(t)
	published := file(t, root, "Scan_0012.pdf", "the original bytes")
	if err := os.WriteFile(published.Path, []byte("something else entirely"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := maintain.Reindex(root, "", t.TempDir())
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if result.Scans != 1 || len(result.Orphans) != 0 {
		t.Errorf("reindex looked at the bytes: %+v", result)
	}
}

// Verify is the one that reads every byte, and it reports without repairing.
func TestVerifyFindsChangedBytesAndRepairsNothing(t *testing.T) {
	root := newBox(t)
	good := file(t, root, "Scan_0012.pdf", "a boarding pass")
	bad := file(t, root, "Scan_0013.pdf", "a power bill")
	if err := os.WriteFile(bad.Path, []byte("rot"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := maintain.Verify(context.Background(), root, "", nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Checked != 2 {
		t.Errorf("checked: got %d want 2", result.Checked)
	}
	if result.Bytes == 0 {
		t.Error("no bytes counted, so there is nothing to tell a person what it cost")
	}
	if len(result.Mismatches) != 1 {
		t.Fatalf("mismatches: %+v", result.Mismatches)
	}
	mismatch := result.Mismatches[0]
	// Both digests, because a report that only says "wrong" cannot be acted on.
	if mismatch.Recorded == "" || mismatch.Actual == "" {
		t.Errorf("the report does not say what changed to what: %+v", mismatch)
	}
	if mismatch.Recorded == mismatch.Actual {
		t.Error("a file was reported that did not change")
	}
	// Nothing was rewritten: the tool cannot know which version somebody wants.
	content, _ := os.ReadFile(bad.Path)
	if string(content) != "rot" {
		t.Error("verify rewrote a file")
	}
	record, err := sidecar.Load(bad.SidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if record.Digest != mismatch.Recorded {
		t.Error("verify rewrote a sidecar")
	}
	// The good one is not reported.
	for _, found := range result.Mismatches {
		if strings.Contains(found.Path, filepath.Base(good.Path)) {
			t.Errorf("an intact file was reported: %+v", found)
		}
	}
}

func TestVerifyReportsAMissingFile(t *testing.T) {
	root := newBox(t)
	published := file(t, root, "Scan_0012.pdf", "a boarding pass")
	if err := os.Remove(published.Path); err != nil {
		t.Fatal(err)
	}
	result, err := maintain.Verify(context.Background(), root, "", nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(result.Mismatches) != 1 || !result.Mismatches[0].Missing {
		t.Fatalf("a missing file was not reported as missing: %+v", result.Mismatches)
	}
	if result.Mismatches[0].Actual != "" {
		t.Error("a digest was reported for a file that is not there")
	}
}

func TestVerifyReportsProgress(t *testing.T) {
	root := newBox(t)
	file(t, root, "Scan_0012.pdf", "one")
	file(t, root, "Scan_0013.pdf", "two")
	var calls int
	var lastDone, lastTotal int
	if _, err := maintain.Verify(context.Background(), root, "", func(_ string, done, total int) {
		calls++
		lastDone, lastTotal = done, total
	}); err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("no progress was reported for the expensive command")
	}
	if lastDone != lastTotal || lastTotal != 2 {
		t.Errorf("the last report was %d of %d", lastDone, lastTotal)
	}
}

func TestVerifyIsCancellable(t *testing.T) {
	root := newBox(t)
	file(t, root, "Scan_0012.pdf", "one")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := maintain.Verify(ctx, root, "", nil); err == nil {
		t.Fatal("a cancelled verify ran to the end")
	}
}

// One decision per document, not one per file.
func TestDedupeGroupsCopies(t *testing.T) {
	root := newBox(t)
	file(t, root, "Scan_0012.pdf", "the same page")
	file(t, root, "Scan_0012 copy.pdf", "the same page")
	file(t, root, "Scan_0013.pdf", "a different page")

	result, err := maintain.Dedupe(root, "")
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("groups: %+v", result.Groups)
	}
	if len(result.Groups[0].Paths) != 2 {
		t.Errorf("group members: %+v", result.Groups[0])
	}
	// "12 groups" says nothing about the size of the pile; this is the number
	// worth showing.
	if result.Copies != 1 {
		t.Errorf("copies: got %d want 1", result.Copies)
	}
}

// A match against the trash says a decision has already been made once.
func TestDedupeMarksTheTrash(t *testing.T) {
	root := newBox(t)
	first := file(t, root, "Scan_0012.pdf", "the same page")
	if _, err := publish.Discard(publish.DiscardRequest{
		Root: root, Path: first.Path, Digest: first.Digest,
		Prefix: len(first.ShortDigest), DiscardDate: box.Date{Year: 2026, Month: 9, Day: 23},
		Reason: "thrown away once already",
	}); err != nil {
		t.Fatal(err)
	}
	file(t, root, "Scan_0012 again.pdf", "the same page")

	result, err := maintain.Dedupe(root, "")
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("groups: %+v", result.Groups)
	}
	if !result.Groups[0].Trashed {
		t.Error("the group does not say one copy was already discarded")
	}
}

// Dedupe reads the index, not the files: it costs what Reindex costs.
func TestDedupeNeedsNoInputAndFindsNothingInACleanBox(t *testing.T) {
	root := newBox(t)
	file(t, root, "Scan_0012.pdf", "one")
	file(t, root, "Scan_0013.pdf", "two")
	result, err := maintain.Dedupe(root, "")
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if len(result.Groups) != 0 || result.Copies != 0 {
		t.Errorf("duplicates in a Box that has none: %+v", result)
	}
}

// Every one of these refuses a directory that is not a Box.
func TestAllRefuseSomethingThatIsNotABox(t *testing.T) {
	root := t.TempDir()
	if _, err := maintain.Reindex(root, "", t.TempDir()); err == nil {
		t.Error("reindex accepted a directory that is not a Box")
	}
	if _, err := maintain.Verify(context.Background(), root, "", nil); err == nil {
		t.Error("verify accepted a directory that is not a Box")
	}
	if _, err := maintain.Dedupe(root, ""); err == nil {
		t.Error("dedupe accepted a directory that is not a Box")
	}
}
