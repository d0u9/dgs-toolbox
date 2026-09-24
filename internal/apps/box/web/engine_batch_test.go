package web_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boxweb "dgs-toolbox/internal/apps/box/web"
	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/index"
	"dgs-toolbox/internal/box/sidecar"
)

// fileScans puts count scans in the inbox and files them, returning their
// digests in the Box.
func fileScans(t *testing.T, engine *boxweb.Engine, inbox string, bodies ...string) []string {
	t.Helper()
	for position, body := range bodies {
		putScan(t, inbox, "Scan_"+string(rune('a'+position))+".pdf", []byte(body))
	}
	engine.Rescan()
	var digests []string
	for _, pending := range engine.Pending() {
		filed, err := engine.File(pending.Digest, boxweb.Edit{})
		if err != nil {
			t.Fatalf("file: %v", err)
		}
		digests = append(digests, filed.Digest)
	}
	return digests
}

// A batch is one request and still one log line per record, because the log is
// what makes three hundred records changed at once recoverable.
func TestApplyManyChangesEachAndLogsEach(t *testing.T) {
	engine, root, inbox := engineBox(t)
	digests := fileScans(t, engine, inbox, "a receipt", "another receipt", "a third")

	receipt := "receipt"
	result, err := engine.ApplyMany(digests, boxweb.Edit{Type: &receipt})
	if err != nil {
		t.Fatalf("apply many: %v", err)
	}
	if len(result.Changed) != 3 || len(result.Failed) != 0 {
		t.Fatalf("batch: %d changed, %+v failed", len(result.Changed), result.Failed)
	}
	for _, scan := range engine.Scans() {
		if scan.Type != "receipt" {
			t.Errorf("%s was not changed: %q", scan.Digest, scan.Type)
		}
	}
	entries, err := boxlog.Read(boxlog.PathFor(root))
	if err != nil {
		t.Fatal(err)
	}
	edits := 0
	for _, entry := range entries {
		if entry.Action == boxlog.ActionEdit && entry.Field == "type" {
			edits++
		}
	}
	if edits != 3 {
		t.Errorf("got %d type edits in the log, want 3", edits)
	}
}

// One refused record does not undo the ones that succeeded. A batch is a
// convenience over a list of edits, not a transaction.
func TestApplyManyReportsEachFailureOnItsOwn(t *testing.T) {
	engine, _, inbox := engineBox(t)
	digests := fileScans(t, engine, inbox, "a receipt", "another receipt")

	receipt := "receipt"
	result, err := engine.ApplyMany(append(digests, "sha256:nothing"), boxweb.Edit{Type: &receipt})
	if err != nil {
		t.Fatalf("apply many: %v", err)
	}
	if len(result.Changed) != 2 {
		t.Errorf("a missing scan undid the real ones: %d changed", len(result.Changed))
	}
	if len(result.Failed) != 1 || result.Failed[0].Digest != "sha256:nothing" {
		t.Errorf("the failure was not reported: %+v", result.Failed)
	}
}

// Duplicates are one decision, not one per copy.
func TestTrashManyDiscardsTheWholeGroup(t *testing.T) {
	engine, root, inbox := engineBox(t)
	digests := fileScans(t, engine, inbox, "one copy", "a second document")

	result, err := engine.TrashMany(digests, "duplicate")
	if err != nil {
		t.Fatalf("trash many: %v", err)
	}
	if len(result.Failed) != 0 {
		t.Fatalf("failed: %+v", result.Failed)
	}
	if got := len(engine.Scans()); got != 0 {
		t.Errorf("%d scans are still browsable after being discarded", got)
	}
	// Nothing is deleted: they are in the trash, where deduplication still
	// sees them.
	if _, err := os.Lstat(filepath.Join(root, "trash")); err != nil {
		t.Errorf("nothing was moved into the trash: %v", err)
	}
	summary := engine.TrashSummary()
	if summary.Count != 2 {
		t.Errorf("trash summary: %+v", summary)
	}
}

// Adopt describes a scan that is in the Box with no sidecar. It stays in its
// own directory, because that directory records an intake date adopting does
// not get to rewrite.
func TestAdoptTakesInAScanWhereItLies(t *testing.T) {
	_, root, _ := engineBox(t)
	day := filepath.Join(root, "2024", "2024-02-03")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(day, "by-hand.pdf"), []byte("dropped in by hand"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The engine has to have seen it as an orphan before it can be adopted.
	engine2, err := boxweb.NewEngine(boxweb.Settings{Root: root, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	exceptions := engine2.Exceptions()
	if len(exceptions) != 1 || !exceptions[0].Adoptable {
		t.Fatalf("the file was not reported as adoptable: %+v", exceptions)
	}

	letter := "letter"
	scan, err := engine2.Adopt(exceptions[0].Path, boxweb.Edit{Type: &letter})
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if scan.Type != "letter" {
		t.Errorf("type: %q", scan.Type)
	}
	entries, err := os.ReadDir(day)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(names) != 2 {
		t.Fatalf("the directory holds %v, want the scan and one sidecar", names)
	}
	// Nothing left the directory it was in, and nothing was copied: the scan
	// is the same file, renamed only to carry the digest its sidecar is named
	// from.
	for _, name := range names {
		if strings.HasSuffix(name, sidecar.Suffix) {
			continue
		}
		if !strings.HasPrefix(name, "by-hand-") || !strings.HasSuffix(name, ".pdf") {
			t.Errorf("%s is not the file that was there", name)
		}
	}
	if len(engine2.Exceptions()) != 0 {
		t.Errorf("it is still an exception after being adopted: %+v", engine2.Exceptions())
	}
	if got := len(engine2.Scans()); got != 1 {
		t.Errorf("the Box holds %d scans after adopting one", got)
	}
}

// A path the Box did not report as an orphan is refused: a request that could
// name any path is a request to write a sidecar somewhere nobody asked for one.
func TestAdoptRefusesAPathItNeverReported(t *testing.T) {
	engine, _, _ := engineBox(t)
	for _, path := range []string{"../outside.pdf", "/etc/hosts", "2024/2024-02-03/invented.pdf"} {
		if _, err := engine.Adopt(path, boxweb.Edit{}); err == nil {
			t.Errorf("%s was adopted", path)
		}
	}
}

// Verify reads the bytes and reports what no longer matches. It repairs
// nothing: rewriting either digest destroys the only evidence.
func TestVerifyReportsAMismatchAndRewritesNothing(t *testing.T) {
	engine, root, inbox := engineBox(t)
	fileScans(t, engine, inbox, "a receipt")
	scans := engine.Scans()
	if len(scans) != 1 {
		t.Fatalf("scans: %+v", scans)
	}

	if found, err := engine.Verify(context.Background()); err != nil || len(found) != 0 {
		t.Fatalf("a Box nobody touched: %+v %v", found, err)
	}

	day := filepath.Join(root, time.Now().Format("2006"), time.Now().Format("01"))
	entries, err := os.ReadDir(day)
	if err != nil {
		t.Fatal(err)
	}
	var scanPath, sidecarPath string
	for _, entry := range entries {
		full := filepath.Join(day, entry.Name())
		if strings.HasSuffix(entry.Name(), sidecar.Suffix) {
			sidecarPath = full
			continue
		}
		scanPath = full
	}
	before, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scanPath, []byte("bit rot, or somebody's editor"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := engine.Verify(context.Background())
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(found) != 1 || found[0].Kind != "digest mismatch" {
		t.Fatalf("mismatch: %+v", found)
	}
	after, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the sidecar was rewritten to match the new bytes")
	}
}

// The cache is written after every change, so the next start does not pay for
// a rebuild it does not need. Losing it still costs nothing.
func TestTheCacheIsWrittenAfterEveryChange(t *testing.T) {
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	inbox, cacheDir := t.TempDir(), t.TempDir()
	engine, err := boxweb.NewEngine(boxweb.Settings{Root: root, Inbox: inbox, CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	digests := fileScans(t, engine, inbox, "a receipt")

	data, err := os.ReadFile(index.PathFor(cacheDir, root))
	if err != nil {
		t.Fatalf("no cache was written: %v", err)
	}
	var cached index.Index
	if err := json.Unmarshal(data, &cached); err != nil {
		t.Fatal(err)
	}
	if len(cached.Entries) != 1 || cached.Entries[0].File.Digest != digests[0] {
		t.Errorf("the cache does not describe what was filed: %+v", cached.Entries)
	}
}
