package publish_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/publish"
	"dgs-toolbox/internal/box/sidecar"
)

func newBox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatalf("init: %v", err)
	}
	return root
}

func scan(t *testing.T, dir, name string, data []byte) (string, string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, digest.Whole(data)
}

var intake = box.Date{Year: 2026, Month: 9, Day: 22}

func TestPublishFilesByIntakeDate(t *testing.T) {
	root := newBox(t)
	source, want := scan(t, filepath.Join(t.TempDir(), "inbox"), "Scan_0012.pdf", []byte("a boarding pass"))

	result, err := publish.Publish(context.Background(), publish.Request{
		Root:       root,
		Source:     source,
		IntakeDate: intake,
		Digest:     want,
		Sidecar:    sidecar.File{Kind: sidecar.KindPDF, Type: "travel"},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	// Only the intake date is in the path. Type, amount and expiry all change,
	// so a path built from any of them would make every correction a file move.
	if got := filepath.ToSlash(result.RelativePath); !strings.HasPrefix(got, "2026/2026-09-22/") {
		t.Errorf("relative path: got %q", got)
	}
	// The stem it arrived with comes first, so the directory still sorts the
	// way the scanner named things; the digest makes the name unique.
	if name := filepath.Base(result.Path); name != "Scan_0012-"+digest.Short(want, 8)+".pdf" {
		t.Errorf("filename: got %q", name)
	}
	if result.Digest != want {
		t.Errorf("digest: got %s want %s", result.Digest, want)
	}
	published, err := os.ReadFile(result.Path)
	if err != nil || string(published) != "a boarding pass" {
		t.Errorf("published bytes: %q %v", published, err)
	}
	// The source is never deleted. Whether to empty the inbox is its owner's
	// decision, made once the Box is known to hold the files.
	if _, err := os.Lstat(source); err != nil {
		t.Errorf("the source was touched: %v", err)
	}
}

func TestPublishWritesSidecarAndLog(t *testing.T) {
	root := newBox(t)
	source, want := scan(t, filepath.Join(t.TempDir(), "inbox"), "Scan_0013.pdf", []byte("an invoice"))

	result, err := publish.Publish(context.Background(), publish.Request{
		Root:       root,
		Source:     source,
		IntakeDate: intake,
		Digest:     want,
		Sidecar:    sidecar.File{Kind: sidecar.KindPDF, Type: "invoice", Description: "power bill"},
		Now:        time.Date(2026, 9, 22, 14, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	record, err := sidecar.Load(result.SidecarPath)
	if err != nil {
		t.Fatalf("load sidecar: %v", err)
	}
	if record.Digest != want || record.Size != int64(len("an invoice")) {
		t.Errorf("sidecar bytes fields: %+v", record)
	}
	if record.OriginalFilename != "Scan_0013.pdf" {
		t.Errorf("the arriving stem was not recorded: %q", record.OriginalFilename)
	}
	if record.Description != "power bill" || record.Type != "invoice" {
		t.Errorf("what the caller said was lost: %+v", record)
	}
	if record.IngestedAt.Zero() {
		t.Error("no intake time")
	}
	// The sidecar sits beside the file, named from the digest alone.
	if filepath.Dir(result.SidecarPath) != filepath.Dir(result.Path) {
		t.Error("the sidecar is not beside its scan")
	}

	entries, err := boxlog.Read(boxlog.PathFor(root))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log entries: got %d", len(entries))
	}
	if entries[0].Action != boxlog.ActionImport || entries[0].Digest != want {
		t.Errorf("log entry: %+v", entries[0])
	}
	// The log records the path relative to the root, so a Box that is mounted
	// somewhere else keeps its history.
	if filepath.IsAbs(entries[0].Path) {
		t.Errorf("the log recorded an absolute path: %q", entries[0].Path)
	}
}

// The gate: without it, a NAS that failed to mount becomes a second, empty,
// entirely plausible-looking Box on the local disk.
func TestPublishRefusesWithoutAMarker(t *testing.T) {
	root := t.TempDir()
	source, want := scan(t, filepath.Join(t.TempDir(), "inbox"), "Scan.pdf", []byte("x"))
	_, err := publish.Publish(context.Background(), publish.Request{
		Root: root, Source: source, IntakeDate: intake, Digest: want,
	})
	if !errors.Is(err, box.ErrNotABox) {
		t.Fatalf("got %v, want ErrNotABox", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("something was written into a directory that is not a Box: %v", entries)
	}
}

// A sidecar that describes different bytes than the file beside it is worse
// than no sidecar: nothing downstream can tell.
func TestPublishRefusesADigestThatDoesNotMatch(t *testing.T) {
	root := newBox(t)
	source, _ := scan(t, filepath.Join(t.TempDir(), "inbox"), "Scan.pdf", []byte("the real bytes"))
	_, err := publish.Publish(context.Background(), publish.Request{
		Root:       root,
		Source:     source,
		IntakeDate: intake,
		Digest:     digest.Whole([]byte("some other bytes")),
	})
	if err == nil {
		t.Fatal("a mismatched digest was published")
	}
	if !strings.Contains(err.Error(), "changed between reading and copying") {
		t.Errorf("error does not say what went wrong: %v", err)
	}
	// Nothing is left in the Box under any name.
	day := filepath.Join(root, "2026", "2026-09-22")
	if entries, err := os.ReadDir(day); err == nil && len(entries) != 0 {
		t.Errorf("the failed publication left %v", entries)
	}
}

// Eight digits collide about as often as never, and "about as often as never"
// over the life of a Box is not never. The failure it causes — two scans
// pointing at one sidecar — is silent, so the prefix grows instead.
func TestCollidingNamesExtendThePrefix(t *testing.T) {
	root := newBox(t)
	inbox := filepath.Join(t.TempDir(), "inbox")
	source, want := scan(t, inbox, "Scan_0012.pdf", []byte("a page"))

	day := filepath.Join(root, "2026", "2026-09-22")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	// Something is already wearing the eight-digit name.
	taken := filepath.Join(day, "Scan_0012-"+digest.Short(want, 8)+".pdf")
	if err := os.WriteFile(taken, []byte("someone else"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := publish.Publish(context.Background(), publish.Request{
		Root: root, Source: source, IntakeDate: intake, Digest: want,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(result.ShortDigest) <= 8 {
		t.Errorf("the prefix did not grow: %q", result.ShortDigest)
	}
	if result.Path == taken {
		t.Fatal("the existing file was overwritten")
	}
	kept, _ := os.ReadFile(taken)
	if string(kept) != "someone else" {
		t.Error("the existing file was changed")
	}
	// The sidecar wears the same longer prefix, so the pair still matches.
	if !strings.Contains(filepath.Base(result.SidecarPath), result.ShortDigest) {
		t.Errorf("sidecar %q does not use the prefix %q", result.SidecarPath, result.ShortDigest)
	}
}

func TestDirectoryForUsesIntakeDateOnly(t *testing.T) {
	got := publish.DirectoryFor(box.Date{Year: 2019, Month: 3, Day: 11})
	if filepath.ToSlash(got) != "2019/2019-03-11" {
		t.Errorf("got %q", got)
	}
}

// Nothing is deleted. Discarding is a same-volume rename inside the Box, and
// the sidecar travels with the file so the digest stays known.
func TestDiscardMovesIntoTheTrash(t *testing.T) {
	root := newBox(t)
	source, want := scan(t, filepath.Join(t.TempDir(), "inbox"), "Scan_0044.pdf", []byte("a leaflet"))
	published, err := publish.Publish(context.Background(), publish.Request{
		Root: root, Source: source, IntakeDate: intake, Digest: want,
		Sidecar: sidecar.File{Kind: sidecar.KindPDF, Type: "unsorted"},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	discarded, err := publish.Discard(publish.DiscardRequest{
		Root:        root,
		Path:        published.Path,
		Digest:      want,
		Prefix:      len(published.ShortDigest),
		DiscardDate: box.Date{Year: 2026, Month: 9, Day: 23},
		Reason:      "a duplicate of the one filed in March",
		Now:         time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("discard: %v", err)
	}
	// Bucketed by the day it was thrown away, which is how emptying works.
	if !strings.Contains(filepath.ToSlash(discarded.Path), "/trash/2026-09-23/") {
		t.Errorf("trash path: got %q", discarded.Path)
	}
	if _, err := os.Lstat(published.Path); !errors.Is(err, os.ErrNotExist) {
		t.Error("the scan is still in its old place")
	}
	if content, _ := os.ReadFile(discarded.Path); string(content) != "a leaflet" {
		t.Error("the bytes did not survive the move")
	}

	record, err := sidecar.Load(discarded.SidecarPath)
	if err != nil {
		t.Fatalf("load trash sidecar: %v", err)
	}
	// The digest stays known, so the trash takes part in deduplication: the
	// same judgement is never asked for twice.
	if record.Digest != want {
		t.Errorf("the digest was lost: %+v", record)
	}
	if record.TrashedAt.Zero() || record.Reason == "" {
		t.Errorf("the trash sidecar does not say when or why: %+v", record)
	}
	if !strings.Contains(record.TrashedFrom, "2026-09-22") {
		t.Errorf("the trash sidecar does not say where it came from: %q", record.TrashedFrom)
	}
	if _, err := os.Lstat(published.SidecarPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("the old sidecar was left behind, which would be reported as an orphan")
	}

	entries, _ := boxlog.Read(boxlog.PathFor(root))
	if len(entries) != 2 || entries[1].Action != boxlog.ActionDiscard {
		t.Errorf("the discard was not logged: %+v", entries)
	}
}

// The same scan filed, unfiled, filed again and unfiled again on one day
// meets its own earlier copy in the trash. Both are kept, the second one
// beside the first in a numbered directory.
func TestDiscardTwiceInOneDayKeepsBoth(t *testing.T) {
	root := newBox(t)
	source, want := scan(t, filepath.Join(t.TempDir(), "inbox"), "Scan_0045.pdf", []byte("a ticket"))
	day := box.Date{Year: 2026, Month: 9, Day: 23}
	var paths []string
	for _, kind := range []string{"object", "ticket"} {
		published, err := publish.Publish(context.Background(), publish.Request{
			Root: root, Source: source, IntakeDate: intake, Digest: want,
			Sidecar: sidecar.File{Kind: sidecar.KindPDF, Type: kind},
		})
		if err != nil {
			t.Fatalf("publish %s: %v", kind, err)
		}
		discarded, err := publish.Discard(publish.DiscardRequest{
			Root: root, Path: published.Path, Digest: want, Prefix: len(published.ShortDigest),
			DiscardDate: day, Reason: "unfiled", Now: time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("discard %s: %v", kind, err)
		}
		record, err := sidecar.Load(discarded.SidecarPath)
		if err != nil || record.Type != kind {
			t.Fatalf("sidecar for %s: %+v %v", kind, record, err)
		}
		paths = append(paths, filepath.ToSlash(discarded.Path))
	}
	if !strings.Contains(paths[0], "/trash/2026-09-23/Scan") || !strings.Contains(paths[1], "/trash/2026-09-23/2/Scan") {
		t.Errorf("trash paths: %q", paths)
	}
}

func TestDiscardRefusesAFileOutsideTheBox(t *testing.T) {
	root := newBox(t)
	outside, want := scan(t, filepath.Join(t.TempDir(), "elsewhere"), "Scan.pdf", []byte("x"))
	_, err := publish.Discard(publish.DiscardRequest{
		Root: root, Path: outside, Digest: want,
		DiscardDate: box.Date{Year: 2026, Month: 9, Day: 23},
	})
	if err == nil {
		t.Fatal("a file outside the Box was moved into its trash")
	}
}
