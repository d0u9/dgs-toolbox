package web_test

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boxweb "dgs-toolbox/internal/apps/box/web"
	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/sidecar"
	"dgs-toolbox/internal/box/thumb"
	"dgs-toolbox/internal/box/thumbcache"
)

// engineBox builds a real Box and inbox on disk and opens the engine over them.
func engineBox(t *testing.T) (*boxweb.Engine, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	inbox := t.TempDir()
	settings := boxweb.Settings{
		Root:     root,
		Inbox:    inbox,
		CacheDir: t.TempDir(),
		Currency: "AUD",
	}
	engine, err := boxweb.NewEngine(settings)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	return engine, root, inbox
}

func putScan(t *testing.T, inbox, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(inbox, name), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The banner exists so nobody describes scans that do not exist. A real Box
// must never present itself as a sample, or the other way round.
func TestARealBoxIsNotASample(t *testing.T) {
	engine, _, _ := engineBox(t)
	if engine.Sample() {
		t.Fatal("a real Box called itself a sample")
	}
	if !boxweb.NewSample("AUD").Sample() {
		t.Fatal("the stand-in did not say it was one")
	}
}

// A root with no marker is refused rather than turned into a Box, and the
// server falls back to the stand-in instead of failing to start.
func TestARootWithNoMarkerIsRefused(t *testing.T) {
	settings := boxweb.Settings{Root: t.TempDir(), CacheDir: t.TempDir()}
	if _, err := boxweb.NewEngine(settings); err == nil {
		t.Fatal("a directory that is not a Box was opened as one")
	}
	if source := boxweb.OpenSource(settings); !source.Sample() {
		t.Error("the fallback did not say it was a sample")
	}
	entries, _ := os.ReadDir(settings.Root)
	if len(entries) != 0 {
		t.Errorf("something was written into a directory that is not a Box: %v", entries)
	}
}

func TestPendingReadsTheInbox(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0012.pdf", []byte("a boarding pass"))
	putScan(t, inbox, "Scan_0013.pdf", []byte("a power bill"))
	engine.Rescan()

	pending := engine.Pending()
	if len(pending) != 2 {
		t.Fatalf("got %d pending, want 2", len(pending))
	}
	for _, scan := range pending {
		if scan.Digest == "" {
			t.Errorf("a pending scan has no digest: %+v", scan)
		}
		if scan.Filename == "" {
			t.Errorf("a pending scan has no filename: %+v", scan)
		}
	}
}

// Two files in one inbox holding the same bytes are one decision, not two.
func TestPendingCollapsesDuplicatesInTheInbox(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0012.pdf", []byte("the same page"))
	putScan(t, inbox, "Scan_0012 copy.pdf", []byte("the same page"))
	engine.Rescan()
	if got := len(engine.Pending()); got != 1 {
		t.Fatalf("got %d pending, want 1", got)
	}
}

func TestFilePublishesAndTheScanAppears(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0012.pdf", []byte("a boarding pass"))
	engine.Rescan()
	pending := engine.Pending()
	if len(pending) != 1 {
		t.Fatalf("got %d pending", len(pending))
	}

	travel := "travel"
	description := "Haneda to Sydney"
	filed, err := engine.File(pending[0].Digest, boxweb.Edit{Type: &travel, Description: &description})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if filed.Type != "travel" || filed.Description != "Haneda to Sydney" {
		t.Errorf("what was typed was lost: %+v", filed)
	}
	if len(engine.Pending()) != 0 {
		t.Error("the scan is still pending after being filed")
	}
	scans := engine.Scans()
	if len(scans) != 1 || scans[0].Digest != filed.Digest {
		t.Fatalf("the Box does not hold it: %+v", scans)
	}
	// It is really on disk, under the intake date, with its sidecar beside it.
	day := filepath.Join(root, time.Now().Format("2006"), time.Now().Format("01"))
	entries, err := os.ReadDir(day)
	if err != nil {
		t.Fatalf("read %s: %v", day, err)
	}
	var scanFiles, sidecars int
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".pdf" {
			scanFiles++
		}
		if len(entry.Name()) > len(sidecar.Suffix) && entry.Name()[len(entry.Name())-len(sidecar.Suffix):] == sidecar.Suffix {
			sidecars++
		}
	}
	if scanFiles != 1 || sidecars != 1 {
		t.Errorf("on disk: %d scans and %d sidecars", scanFiles, sidecars)
	}
	// The source in the inbox is not deleted: emptying a temporary folder is
	// its owner's decision.
	if _, err := os.Lstat(filepath.Join(inbox, "Scan_0012.pdf")); err != nil {
		t.Errorf("the inbox was emptied: %v", err)
	}
	// And it was logged.
	entriesLog, err := boxlog.Read(boxlog.PathFor(root))
	if err != nil || len(entriesLog) != 1 || entriesLog[0].Action != boxlog.ActionImport {
		t.Errorf("import log: %+v %v", entriesLog, err)
	}
}

// unsorted is always a legal outcome: intake never blocks on a decision, which
// is what makes an inbox drainable.
func TestFilingWithNoTypeIsUnsorted(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0099.pdf", []byte("something nobody can name yet"))
	engine.Rescan()
	filed, err := engine.File(engine.Pending()[0].Digest, boxweb.Edit{})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if filed.Type != "unsorted" {
		t.Errorf("type: got %q want unsorted", filed.Type)
	}
}

// An event date files the scan under that date's year and month, and the scan
// and its sidecar move there together; the index finds them where they went.
func TestApplyMovesAScanToItsEventMonth(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0015.pdf", []byte("an old receipt"))
	engine.Rescan()
	filed, err := engine.File(engine.Pending()[0].Digest, boxweb.Edit{})
	if err != nil {
		t.Fatal(err)
	}
	date := "2019-03-11"
	updated, err := engine.Apply(boxweb.Edit{Digest: filed.Digest, EventDate: &date})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	moved, err := os.ReadDir(filepath.Join(root, "2019", "03"))
	if err != nil || len(moved) != 2 {
		t.Fatalf("2019/03 holds %v (%v), want the scan and its sidecar", moved, err)
	}
	if updated.EventDate != date {
		t.Errorf("event date: got %q", updated.EventDate)
	}
	reopened, err := boxweb.NewEngine(boxweb.Settings{Root: root, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if scans := reopened.Scans(); len(scans) != 1 || scans[0].EventDate != date {
		t.Errorf("after reopening: %+v", scans)
	}
	if exceptions := reopened.Exceptions(); len(exceptions) != 0 {
		t.Errorf("exceptions after the move: %+v", exceptions)
	}
}

// Correcting a classification is one sidecar rewrite and one log line. The file
// does not move and the digest stays valid.
func TestApplyRewritesTheSidecarAndLogsIt(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0013.pdf", []byte("a power bill"))
	engine.Rescan()
	receipt := "receipt"
	filed, err := engine.File(engine.Pending()[0].Digest, boxweb.Edit{Type: &receipt})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(filepath.Join(root, time.Now().Format("2006"), time.Now().Format("01")))
	if err != nil {
		t.Fatal(err)
	}

	invoice := "statement"
	updated, err := engine.Apply(boxweb.Edit{Digest: filed.Digest, Type: &invoice})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if updated.Type != "statement" {
		t.Errorf("type: got %q", updated.Type)
	}
	// Changing the type un-reviews it: a guess and a confirmation must stay
	// distinguishable.
	if updated.Reviewed {
		t.Error("a reclassified scan is still marked reviewed")
	}
	// A type is not a date, so nothing moved.
	after, err := os.ReadDir(filepath.Join(root, time.Now().Format("2006"), time.Now().Format("01")))
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Errorf("the directory changed shape: %d then %d entries", len(before), len(after))
	}
	// The change survives a reopen, because it is in the sidecar and not only
	// in memory.
	reopened, err := boxweb.NewEngine(boxweb.Settings{Root: root, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	scans := reopened.Scans()
	if len(scans) != 1 || scans[0].Type != "statement" {
		t.Errorf("after reopening: %+v", scans)
	}
	// And the log says what moved, from what, to what.
	logged, err := boxlog.Read(boxlog.PathFor(root))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, entry := range logged {
		if entry.Action == boxlog.ActionEdit && entry.Field == "type" && entry.From == "receipt" && entry.To == "statement" {
			found = true
		}
	}
	if !found {
		t.Errorf("the edit was not logged usefully: %+v", logged)
	}
}

// A type this build does not register is refused on the way in.
func TestUnknownTypeIsRefused(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan.pdf", []byte("x"))
	engine.Rescan()
	nonsense := "not-a-real-type"
	if _, err := engine.File(engine.Pending()[0].Digest, boxweb.Edit{Type: &nonsense}); err == nil {
		t.Fatal("an unknown type was accepted")
	}
}

// Nothing is deleted: discarding moves the file into the trash, where it stays
// known so the same judgement is not asked for twice.
func TestTrashMovesAndKeeps(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0044.pdf", []byte("a leaflet"))
	engine.Rescan()
	filed, err := engine.File(engine.Pending()[0].Digest, boxweb.Edit{})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Trash(filed.Digest, "a duplicate"); err != nil {
		t.Fatalf("trash: %v", err)
	}
	if got := len(engine.Scans()); got != 0 {
		t.Errorf("the Box still lists %d scans", got)
	}
	// The bytes are in trash/, not gone.
	trashed := filepath.Join(root, "trash", time.Now().Format("2006-01-02"))
	entries, err := os.ReadDir(trashed)
	if err != nil || len(entries) == 0 {
		t.Fatalf("nothing in the trash: %v %v", entries, err)
	}
	// And putting the same file back in the inbox is recognised, with when it
	// was thrown away, rather than asked about again.
	engine.Rescan()
	pending := engine.Pending()
	if len(pending) != 1 {
		t.Fatalf("got %d pending", len(pending))
	}
	if pending[0].DuplicateOf == "" {
		t.Error("the scan was not recognised as one already seen")
	}
	if pending[0].TrashedAt == "" {
		t.Error("nothing said it had been thrown away")
	}
	if path, err := engine.Locate(pending[0].DuplicateOf); err != nil || !strings.Contains(filepath.ToSlash(path), "/trash/") {
		t.Errorf("the discarded copy cannot be found: %q %v", path, err)
	}
}

// A scan already filed and put back in the inbox is a duplicate, not a second
// document.
func TestAFiledScanIsRecognisedInTheInbox(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0012.pdf", []byte("a boarding pass"))
	engine.Rescan()
	if _, err := engine.File(engine.Pending()[0].Digest, boxweb.Edit{}); err != nil {
		t.Fatal(err)
	}
	putScan(t, inbox, "Scan_0012 again.pdf", []byte("a boarding pass"))
	engine.Rescan()
	pending := engine.Pending()
	if len(pending) != 1 {
		t.Fatalf("got %d pending", len(pending))
	}
	if pending[0].DuplicateOf == "" {
		t.Error("a scan already in the Box was not recognised")
	}
	// Where the copy already filed is, so it can be found without a search.
	located, _ := engine.Locate(pending[0].DuplicateOf)
	if pending[0].DuplicatePath == "" || !strings.HasSuffix(located, filepath.FromSlash(pending[0].DuplicatePath)) {
		t.Errorf("duplicate path %q, filed at %q", pending[0].DuplicatePath, located)
	}
	if _, err := engine.Locate(pending[0].DuplicateOf); err != nil {
		t.Errorf("the match cannot be revealed: %v", err)
	}
}

// Redrawing drops a broken picture and draws it again from the file.
func TestRedrawMendsABrokenThumbnail(t *testing.T) {
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	inbox := t.TempDir()
	cacheDir := t.TempDir()
	engine, err := boxweb.NewEngine(boxweb.Settings{Root: root, Inbox: inbox, CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	if err := jpeg.Encode(&picture, image.NewGray(image.Rect(0, 0, 40, 60)), nil); err != nil {
		t.Fatal(err)
	}
	putScan(t, inbox, "Scan_0001.jpg", picture.Bytes())
	engine.Rescan()
	filed, err := engine.File(engine.Pending()[0].Digest, boxweb.Edit{})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	store := thumbcache.New(cacheDir, root)
	if err := store.Save(filed.Digest, thumb.Pair{Grid: []byte("broken"), Preview: []byte("broken")}); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Redraw(context.Background(), filed.Digest)
	if err != nil || result.Redrawn != 1 || len(result.Failed) != 0 {
		t.Fatalf("redraw: %+v %v", result, err)
	}
	got, _, err := engine.Image(filed.Digest, thumbcache.SizeGrid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(got)); err != nil {
		t.Errorf("the thumbnail is still broken: %v", err)
	}
	if _, err := engine.Redraw(context.Background(), "sha256:nothere"); err == nil {
		t.Error("redrawing a scan that is not filed was not refused")
	}
	if all, err := engine.Redraw(context.Background(), ""); err != nil || all.Redrawn != 1 {
		t.Errorf("redraw all: %+v %v", all, err)
	}
}

// The pictures land in the cache, outside the Box.
func TestPicturesAreCachedOutsideTheBox(t *testing.T) {
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	inbox := t.TempDir()
	cacheDir := t.TempDir()
	engine, err := boxweb.NewEngine(boxweb.Settings{Root: root, Inbox: inbox, CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	// A file with no picture in it is filed all the same and simply has none:
	// an inbox that cannot be drained is worse than a Box with a missing
	// thumbnail.
	putScan(t, inbox, "Scan_0001.pdf", []byte("no picture in here"))
	engine.Rescan()
	pending := engine.Pending()
	if len(pending) != 1 {
		t.Fatalf("got %d pending", len(pending))
	}
	if !pending[0].NeedsRender {
		t.Error("a file with no picture was not marked needs-render")
	}
	filed, err := engine.File(pending[0].Digest, boxweb.Edit{})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if _, _, err := engine.Image(filed.Digest, thumbcache.SizeGrid, 1); err == nil {
		t.Error("a picture was produced for a scan that has none")
	}
	// Whatever the cache holds, it holds it outside the Box.
	if _, err := os.Lstat(filepath.Join(root, "index.json")); err == nil {
		t.Error("the cache was written into the Box")
	}
}

// Every field in the cache comes from the Box, so throwing it away changes
// nothing a page can see.
func TestThrowingTheCacheAwayChangesNothing(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0012.pdf", []byte("a boarding pass"))
	putScan(t, inbox, "Scan_0013.pdf", []byte("a power bill"))
	engine.Rescan()
	for _, scan := range engine.Pending() {
		if _, err := engine.File(scan.Digest, boxweb.Edit{}); err != nil {
			t.Fatal(err)
		}
	}
	before := engine.Scans()

	fresh, err := boxweb.NewEngine(boxweb.Settings{Root: root, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	after := fresh.Scans()
	if len(before) != len(after) {
		t.Fatalf("%d scans before, %d after the cache was thrown away", len(before), len(after))
	}
	for i := range before {
		if before[i].Digest != after[i].Digest || before[i].Type != after[i].Type {
			t.Errorf("scan %d differs: %+v vs %+v", i, before[i], after[i])
		}
	}
}

// A hundred scans are not sorted in one sitting.
func TestResumingSkipsWhatWasAlreadyDecided(t *testing.T) {
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	inbox := t.TempDir()
	settings := boxweb.Settings{Root: root, Inbox: inbox, CacheDir: t.TempDir()}
	engine, err := boxweb.NewEngine(settings)
	if err != nil {
		t.Fatal(err)
	}
	putScan(t, inbox, "Scan_0012.pdf", []byte("a boarding pass"))
	putScan(t, inbox, "Scan_0013.pdf", []byte("a power bill"))
	putScan(t, inbox, "Scan_0014.pdf", []byte("a leaflet"))
	engine.Rescan()
	pending := engine.Pending()
	if len(pending) != 3 {
		t.Fatalf("got %d pending", len(pending))
	}
	// One filed, one rejected, one left alone.
	if _, err := engine.File(pending[0].Digest, boxweb.Edit{}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Trash(pending[1].Digest, "not wanted"); err != nil {
		t.Fatal(err)
	}

	// Closing the tool and coming back is a fresh engine over the same folders.
	again, err := boxweb.NewEngine(settings)
	if err != nil {
		t.Fatal(err)
	}
	remaining := again.Pending()
	if len(remaining) != 1 {
		t.Fatalf("got %d pending after coming back, want 1: %+v", len(remaining), remaining)
	}
	if remaining[0].Digest != pending[2].Digest {
		t.Errorf("the wrong scan came back: %+v", remaining[0])
	}
	// The state is in the inbox, not in the Box.
	if _, err := os.Lstat(filepath.Join(inbox, "dgs-box-state.json")); err != nil {
		t.Errorf("no state file in the inbox: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "dgs-box-state.json")); err == nil {
		t.Error("the state was written into the Box")
	}
}

// Closing the tool halfway through describing something must not throw away
// what was typed.
func TestADraftSurvivesComingBack(t *testing.T) {
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	inbox := t.TempDir()
	settings := boxweb.Settings{Root: root, Inbox: inbox, CacheDir: t.TempDir()}
	engine, err := boxweb.NewEngine(settings)
	if err != nil {
		t.Fatal(err)
	}
	putScan(t, inbox, "Scan_0012.pdf", []byte("a boarding pass"))
	engine.Rescan()
	pending := engine.Pending()
	travel := "travel"
	description := "Haneda to Sydney"
	if _, err := engine.Apply(boxweb.Edit{Digest: pending[0].Digest, Type: &travel, Description: &description}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Nothing was written into the Box: it is not in the Box yet.
	if entries, _ := os.ReadDir(filepath.Join(root, time.Now().Format("2006"))); len(entries) != 0 {
		t.Errorf("a scan that was only described landed in the Box: %v", entries)
	}

	again, err := boxweb.NewEngine(settings)
	if err != nil {
		t.Fatal(err)
	}
	resumed := again.Pending()
	if len(resumed) != 1 {
		t.Fatalf("got %d pending", len(resumed))
	}
	if resumed[0].Type != "travel" || resumed[0].Description != "Haneda to Sydney" {
		t.Errorf("the draft was lost: %+v", resumed[0])
	}
}

// A file that changed since it was decided is not the file that was decided.
func TestAChangedFileIsOfferedAgain(t *testing.T) {
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	inbox := t.TempDir()
	settings := boxweb.Settings{Root: root, Inbox: inbox, CacheDir: t.TempDir()}
	engine, err := boxweb.NewEngine(settings)
	if err != nil {
		t.Fatal(err)
	}
	putScan(t, inbox, "Scan_0012.pdf", []byte("the first version"))
	engine.Rescan()
	if err := engine.Trash(engine.Pending()[0].Digest, "not wanted"); err != nil {
		t.Fatal(err)
	}
	if got := len(engine.Pending()); got != 0 {
		t.Fatalf("got %d pending after rejecting", got)
	}
	// Rescanned at the same path, with different bytes.
	putScan(t, inbox, "Scan_0012.pdf", []byte("a completely different scan this time"))
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(inbox, "Scan_0012.pdf"), future, future); err != nil {
		t.Fatal(err)
	}
	engine.Rescan()
	if got := len(engine.Pending()); got != 1 {
		t.Errorf("a changed file was not offered again: %d pending", got)
	}
}

// Truncated files, bad copies and damaged old backups are reported, never
// silently skipped: a skip nobody is told about means believing the inbox is
// empty when it is not.
func TestIncompleteFilesAreReportedNotSkipped(t *testing.T) {
	engine, _, inbox := engineBox(t)
	// A PDF header with nothing behind it: exactly what a truncated copy looks
	// like.
	putScan(t, inbox, "Scan_0012.pdf", []byte("%PDF-1.7\nand then the copy stopped"))
	putScan(t, inbox, "Scan_0013.pdf", []byte("a perfectly ordinary image is not a PDF"))
	engine.Rescan()

	if got := len(engine.Pending()); got != 1 {
		t.Errorf("a truncated file was offered for filing: %d pending", got)
	}
	incomplete := engine.Incomplete()
	if len(incomplete) != 1 {
		t.Fatalf("incomplete: %+v", incomplete)
	}
	if incomplete[0].Detail == "" {
		t.Error("nothing said what was wrong with it")
	}
	if !strings.Contains(incomplete[0].Path, "Scan_0012") {
		t.Errorf("the wrong file was reported: %+v", incomplete[0])
	}
}

// A file the owner moved out of the inbox describes nothing any more.
func TestVanishedCandidatesAreForgotten(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0012.pdf", []byte("a boarding pass"))
	engine.Rescan()
	if err := engine.Trash(engine.Pending()[0].Digest, "not wanted"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(inbox, "Scan_0012.pdf")); err != nil {
		t.Fatal(err)
	}
	engine.Rescan()
	state, err := os.ReadFile(filepath.Join(inbox, "dgs-box-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "Scan_0012.pdf") {
		t.Errorf("a record was kept for a file that is gone:\n%s", state)
	}
}

// A scan rejected by mistake is still in the inbox and comes back whole.
func TestARejectedScanCanBeRestored(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0001.pdf", threePages(t))
	engine.Rescan()
	digest := engine.Pending()[0].Digest
	if err := engine.Trash(digest, "rejected at intake"); err != nil {
		t.Fatal(err)
	}
	if len(engine.Pending()) != 0 {
		t.Fatal("still pending after reject")
	}
	rejected := engine.Rejected()
	if len(rejected) != 1 || rejected[0].Digest != digest || rejected[0].Filename != "Scan_0001.pdf" {
		t.Fatalf("rejected: %+v", rejected)
	}
	body, name, mediaType, err := engine.RejectedFile(digest)
	if err != nil || name != "Scan_0001.pdf" || mediaType != "application/pdf" || len(body) == 0 {
		t.Fatalf("rejected file: %d bytes, %q, %q, %v", len(body), name, mediaType, err)
	}
	if err := engine.Restore(digest); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := engine.RejectedFile(digest); err == nil {
		t.Error("gave the file of a scan no longer rejected")
	}
	if pending := engine.Pending(); len(pending) != 1 || pending[0].Digest != digest {
		t.Fatalf("pending after restore: %+v", pending)
	}
	if len(engine.Rejected()) != 0 {
		t.Error("still listed as rejected")
	}
	if err := engine.Restore(digest); err == nil {
		t.Error("restored a scan that was not rejected")
	}
}

// A scan filed wrongly as a whole goes back to intake with what it was
// described as, and is not mistaken for something thrown away.
func TestAFiledScanCanBeUnfiled(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0001.pdf", threePages(t))
	engine.Rescan()
	digest := engine.Pending()[0].Digest
	description := "wrong one"
	if _, err := engine.File(digest, boxweb.Edit{Description: &description}); err != nil {
		t.Fatal(err)
	}
	scans := engine.Scans()
	if len(scans) != 1 || !scans[0].Unfilable {
		t.Fatalf("filed: %+v", scans)
	}
	if err := engine.Unfile(digest); err != nil {
		t.Fatal(err)
	}
	if len(engine.Scans()) != 0 {
		t.Error("still in the Box after unfiling")
	}
	pending := engine.Pending()
	if len(pending) != 1 || pending[0].Description != "wrong one" {
		t.Fatalf("pending: %+v", pending)
	}
	if pending[0].DuplicateOf != "" || pending[0].TrashedAt != "" {
		t.Errorf("unfiled scan flagged as a duplicate: %+v", pending[0])
	}
	if _, err := os.Stat(filepath.Join(root, "trash")); err != nil {
		t.Errorf("no trash: %v", err)
	}
	// And it files again.
	if _, err := engine.File(digest, boxweb.Edit{}); err != nil {
		t.Fatalf("file again: %v", err)
	}
	if err := engine.Unfile("sha256:nothing"); err == nil {
		t.Error("unfiled a scan that is not there")
	}
}

// A scan edited right after it is filed keeps what filing recorded: the edit
// starts from the sidecar as written, not the draft that went into Publish.
func TestAnEditAfterFilingKeepsWhenItWasAdded(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0013.pdf", []byte("a receipt"))
	engine.Rescan()
	filed, err := engine.File(engine.Pending()[0].Digest, boxweb.Edit{})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if filed.IngestedAt == "" || filed.EditedAt == "" {
		t.Fatalf("filed scan has no added time: %+v", filed)
	}
	description := "later"
	if _, err := engine.Apply(boxweb.Edit{Digest: filed.Digest, Description: &description}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*", "*", "*"+sidecar.Suffix))
	if len(matches) != 1 {
		t.Fatalf("sidecars: %v", matches)
	}
	record, err := sidecar.Load(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(record.IngestedAt) != filed.IngestedAt || record.OriginalFilename != "Scan_0013.pdf" {
		t.Errorf("the edit dropped what filing wrote: ingested_at %q, original_filename %q", record.IngestedAt, record.OriginalFilename)
	}
}

// A scan still in intake is located in the inbox, so it can be shown there.
func TestAPendingScanIsLocatedInTheInbox(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0014.pdf", []byte("a letter"))
	engine.Rescan()
	pending := engine.Pending()
	if len(pending) != 1 {
		t.Fatalf("got %d pending", len(pending))
	}
	path, err := engine.Locate(pending[0].Digest)
	if err != nil || path != filepath.Join(inbox, "Scan_0014.pdf") {
		t.Errorf("located %q, %v", path, err)
	}
}

// Another folder can be opened as the inbox for the run; its subfolders are
// how the page draws it, and a folder overlapping the Box is refused.
func TestAnotherFolderCanBeTheInbox(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0012.pdf", []byte("a boarding pass"))
	if got := engine.Pending(); len(got) != 1 {
		t.Fatalf("first inbox: %d", len(got))
	}
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "2024", "trip"), 0o755); err != nil {
		t.Fatal(err)
	}
	putScan(t, other, filepath.Join("2024", "trip", "a.pdf"), []byte("a ticket"))
	putScan(t, other, "b.pdf", []byte("a receipt"))
	if err := engine.SetInbox(other); err != nil {
		t.Fatal(err)
	}
	got := engine.Pending()
	paths := map[string]bool{}
	for _, s := range got {
		paths[s.InboxPath] = true
	}
	if engine.Inbox() != other || len(got) != 2 || !paths["2024/trip/a.pdf"] || !paths["b.pdf"] {
		t.Fatalf("other inbox: %s %+v", engine.Inbox(), paths)
	}
	for _, bad := range []string{root, filepath.Join(root, "x"), filepath.Dir(root), "relative"} {
		if filepath.IsAbs(bad) {
			os.MkdirAll(bad, 0o755)
		}
		if err := engine.SetInbox(bad); err == nil {
			t.Fatalf("%s was taken as the inbox", bad)
		}
	}
}
