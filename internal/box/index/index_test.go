package index_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/index"
	"dgs-toolbox/internal/box/publish"
	"dgs-toolbox/internal/box/sidecar"
)

// filledBox builds a small Box through the real publication path, so the cache
// is tested against the tree the tool actually writes rather than against a
// hand-made one.
func filledBox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := box.WriteMarker(root, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	inbox := t.TempDir()
	scans := []struct {
		name string
		body string
		kind string
		day  box.Date
	}{
		{"Scan_0012.pdf", "a boarding pass", "travel", box.Date{Year: 2026, Month: 9, Day: 22}},
		{"Scan_0013.pdf", "a power bill", "invoice", box.Date{Year: 2026, Month: 9, Day: 22}},
		{"Scan_0044.pdf", "a leaflet", "unsorted", box.Date{Year: 2026, Month: 9, Day: 23}},
	}
	for _, entry := range scans {
		path := filepath.Join(inbox, entry.name)
		if err := os.WriteFile(path, []byte(entry.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := publish.Publish(context.Background(), publish.Request{
			Root:       root,
			Source:     path,
			IntakeDate: entry.day,
			Digest:     digest.Whole([]byte(entry.body)),
			Sidecar:    sidecar.File{Kind: sidecar.KindPDF, Type: entry.kind},
		}); err != nil {
			t.Fatalf("publish %s: %v", entry.name, err)
		}
	}
	return root
}

// The only evidence the word "discardable" is true.
func TestRebuildEqualsOriginal(t *testing.T) {
	root := filledBox(t)
	first, orphans, err := index.Build(root)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans in a Box this tool just wrote: %+v", orphans)
	}
	if len(first.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(first.Entries))
	}
	second, _, err := index.Build(root)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Error("two builds of one Box produced different caches, so the cache holds something the Box does not")
	}
}

// Throwing the cache away and rebuilding must give back what a save-and-load
// round trip gives. Anything else means a field exists only in the cache.
func TestSaveLoadRoundTripEqualsARebuild(t *testing.T) {
	root := filledBox(t)
	cacheDir := t.TempDir()
	built, _, err := index.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	path := index.PathFor(cacheDir, root)
	if err := index.Save(path, built); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, usable, err := index.Load(path, root)
	if err != nil || !usable {
		t.Fatalf("load: %v usable=%v", err, usable)
	}
	builtJSON, _ := json.Marshal(built)
	loadedJSON, _ := json.Marshal(loaded)
	if string(builtJSON) != string(loadedJSON) {
		t.Error("a cache did not survive being written and read back")
	}
}

// Unparseable, truncated, another schema — all the same answer, and the answer
// is Build. Migration code for a discardable cache is code that can only be
// wrong.
func TestUnusableCachesAreRebuiltNotMigrated(t *testing.T) {
	root := filledBox(t)
	cacheDir := t.TempDir()
	path := index.PathFor(cacheDir, root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"truncated":    `{"version":1,"root":"` + root + `","entr`,
		"not json":     `this is not a cache`,
		"older schema": `{"version":0,"root":"` + root + `","entries":[]}`,
		"newer schema": `{"version":99,"root":"` + root + `","entries":[]}`,
		"another Box":  `{"version":1,"root":"/somewhere/else","entries":[]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, usable, err := index.Load(path, root); usable || err != nil {
				t.Fatalf("usable=%v err=%v, want an unusable cache and no error", usable, err)
			}
			// Open turns that into a rebuild rather than a failure.
			cache, _, err := index.Open(cacheDir, root)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if len(cache.Entries) != 3 {
				t.Errorf("rebuild produced %d entries", len(cache.Entries))
			}
		})
	}
}

func TestAMissingCacheIsNotAnError(t *testing.T) {
	root := filledBox(t)
	_, usable, err := index.Load(index.PathFor(t.TempDir(), root), root)
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if usable {
		t.Error("a cache that is not there was called usable")
	}
}

// A startup walk reads the listing only and reopens just the sidecars whose
// triple moved. An untouched sidecar keeps whatever the cache already had.
func TestRefreshOnlyRereadsWhatChanged(t *testing.T) {
	root := filledBox(t)
	cache, _, err := index.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	// Mark every entry so an entry that came back from disk is obvious.
	for i := range cache.Entries {
		cache.Entries[i].File.Description = "from the cache"
	}
	changed := cache.Entries[0]
	record, err := sidecar.Load(filepath.Join(root, changed.SidecarPath))
	if err != nil {
		t.Fatal(err)
	}
	record.Description = "from the sidecar"
	// The modification time has to move for the triple to change; some
	// filesystems have coarse enough timestamps that an immediate rewrite looks
	// identical.
	if err := sidecar.Save(filepath.Join(root, changed.SidecarPath), record); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(root, changed.SidecarPath), future, future); err != nil {
		t.Fatal(err)
	}

	refreshed, orphans, err := index.Refresh(root, cache)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans: %+v", orphans)
	}
	var reread, kept int
	for _, entry := range refreshed.Entries {
		switch entry.File.Description {
		case "from the sidecar":
			reread++
		case "from the cache":
			kept++
		default:
			t.Errorf("unexpected entry %+v", entry)
		}
	}
	if reread != 1 {
		t.Errorf("%d sidecars were reread, want 1", reread)
	}
	if kept != 2 {
		t.Errorf("%d entries were kept from the cache, want 2", kept)
	}
}

// A cache that remembers files nobody has answers questions wrongly, so the
// entry goes — but it is reported, not dropped in silence.
func TestAVanishedFileIsReported(t *testing.T) {
	root := filledBox(t)
	cache, _, err := index.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	gone := cache.Entries[0]
	if err := os.Remove(filepath.Join(root, gone.SidecarPath)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, gone.ScanPath)); err != nil {
		t.Fatal(err)
	}
	refreshed, orphans, err := index.Refresh(root, cache)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(refreshed.Entries) != 2 {
		t.Errorf("entries: got %d want 2", len(refreshed.Entries))
	}
	var reported bool
	for _, orphan := range orphans {
		if orphan.Kind == "missing" && orphan.Path == gone.SidecarPath {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the vanished entry was dropped silently: %+v", orphans)
	}
}

// Both directions are reported and neither is repaired. Guessing which is which
// and acting on the guess is how a tool destroys what it was asked to look
// after.
func TestOrphansInBothDirections(t *testing.T) {
	root := filledBox(t)
	day := filepath.Join(root, "2026", "09")
	// A scan with no sidecar.
	if err := os.WriteFile(filepath.Join(day, "Scan_0099-deadbeef.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A sidecar with no scan.
	lonely := filepath.Join(day, "cafebabe"+sidecar.Suffix)
	if err := sidecar.Save(lonely, sidecar.File{Digest: "sha256:cafebabe", Type: "unsorted"}); err != nil {
		t.Fatal(err)
	}
	_, orphans, err := index.Build(root)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	kinds := map[string]int{}
	for _, orphan := range orphans {
		kinds[orphan.Kind]++
	}
	if kinds["scan"] != 1 {
		t.Errorf("scans with no sidecar: got %d want 1 (%+v)", kinds["scan"], orphans)
	}
	if kinds["sidecar"] != 1 {
		t.Errorf("sidecars with no scan: got %d want 1 (%+v)", kinds["sidecar"], orphans)
	}
	// Nothing was repaired.
	if _, err := os.Lstat(lonely); err != nil {
		t.Error("the lonely sidecar was removed")
	}
}

// The trash stays in the cache: it takes part in deduplication, so a file
// thrown away in March is not silently taken back in in September.
func TestTheTrashIsIndexedAndMarked(t *testing.T) {
	root := filledBox(t)
	cache, _, err := index.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	victim := cache.Entries[0]
	if _, err := publish.Discard(publish.DiscardRequest{
		Root:        root,
		Path:        filepath.Join(root, victim.ScanPath),
		Digest:      victim.File.Digest,
		Prefix:      len(strings.TrimSuffix(filepath.Base(victim.SidecarPath), sidecar.Suffix)),
		DiscardDate: box.Date{Year: 2026, Month: 9, Day: 24},
		Reason:      "a duplicate",
	}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	rebuilt, orphans, err := index.Build(root)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans after a discard: %+v", orphans)
	}
	if len(rebuilt.Entries) != 3 {
		t.Fatalf("entries: got %d want 3 — the discarded scan must stay known", len(rebuilt.Entries))
	}
	var trashed int
	for _, entry := range rebuilt.Entries {
		if entry.InTrash {
			trashed++
			if entry.File.Digest != victim.File.Digest {
				t.Errorf("the trashed entry lost its digest: %+v", entry.File)
			}
		}
	}
	if trashed != 1 {
		t.Errorf("entries marked as trash: got %d want 1", trashed)
	}
}

// The marker and the log are the Box's own files, not scans.
func TestBoxOwnFilesAreNotIndexed(t *testing.T) {
	root := filledBox(t)
	_, orphans, err := index.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, orphan := range orphans {
		if strings.Contains(orphan.Path, box.MarkerName) || strings.Contains(orphan.Path, "dgs-box-log") {
			t.Errorf("a Box file was treated as a scan: %+v", orphan)
		}
	}
}

// The cache lives outside the Box: everything in the tree is truth, and a
// database file on a network filesystem is a way to lose data.
func TestTheCacheIsNotInTheBox(t *testing.T) {
	root := filledBox(t)
	cacheDir := t.TempDir()
	if _, _, err := index.Open(cacheDir, root); err != nil {
		t.Fatalf("open: %v", err)
	}
	path := index.PathFor(cacheDir, root)
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("no cache was written: %v", err)
	}
	if strings.HasPrefix(path, root) {
		t.Errorf("the cache was written inside the Box: %s", path)
	}
	// Keyed by the root, so two Boxes never share one.
	other := index.PathFor(cacheDir, t.TempDir())
	if filepath.Dir(other) == filepath.Dir(path) {
		t.Error("two Boxes share a cache directory")
	}
}

// Relative paths, so a Box mounted somewhere else keeps its cache usable and
// two machines describe it in the same words.
func TestPathsAreRelativeAndSlashed(t *testing.T) {
	root := filledBox(t)
	cache, _, err := index.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range cache.Entries {
		if filepath.IsAbs(entry.SidecarPath) || filepath.IsAbs(entry.ScanPath) {
			t.Errorf("absolute path in the cache: %+v", entry)
		}
		if strings.Contains(entry.SidecarPath, `\`) {
			t.Errorf("a backslash in a cache path: %q", entry.SidecarPath)
		}
	}
}
