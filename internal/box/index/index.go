// Package index is the Box's discardable cache: one JSON file holding a record
// per scan, read into memory at startup, and that is the whole query engine.
//
// A Box of a few thousand scans is a couple of megabytes of metadata. Filtering
// is a linear scan over a slice and sorting is sort.Slice, which at this size
// beats a database on speed, on lines of code and on dependencies — and the
// decision stays cheap to revisit precisely because everything here is
// derivable from the Box and can be thrown away.
//
// Three rules hold the whole design up:
//
//   - Nothing exists only in the cache. Every field comes from a sidecar or
//     from the file's own bytes. TestRebuildEqualsOriginal is the only evidence
//     the word "discardable" is true, so it is not an optional test.
//   - A version mismatch rebuilds and never migrates. Unparseable, truncated,
//     written by another schema — all the same answer. Migration code for a
//     discardable cache is code that can only be wrong.
//   - A lost write costs nothing. No WAL, no transactions, no careful fsync:
//     the file is written whole into a temporary file beside itself and
//     renamed. Losing it costs one rebuild.
//
// Rebuilding is not verifying. This package reads sidecars only and takes each
// digest from the sidecar, which is seconds on a NAS. Recomputing digests means
// reading every byte of every file, which is tens of gigabytes, and lives
// behind dgs box verify. Sharing one name would mean either never dares be run.
package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/sidecar"
)

// Version is the cache schema this build writes. A cache carrying any other
// number is rebuilt.
const Version = 2

// Name is the cache file, inside this Box's cache directory.
const Name = "index.json"

// Entry is one scan as the cache remembers it.
//
// The three staleness fields describe the sidecar, not the scan, because a
// startup walk reads sidecars and never touches a scan's bytes. A scan whose
// file was replaced but whose sidecar was not is a job for dgs box verify.
type Entry struct {
	// SidecarPath and ScanPath are relative to the Box root and use forward
	// slashes, so a cache survives the Box being mounted at another path — and
	// so two machines' caches describe the same Box in the same words.
	SidecarPath string `json:"sidecar_path"`
	// ScanPath is empty for a sidecar with no file beside it, which is an
	// orphan and is reported rather than dropped.
	ScanPath string `json:"scan_path,omitempty"`
	// Size and ModTime are the sidecar's, and together with SidecarPath are
	// what says whether this entry is still current.
	Size    int64 `json:"size"`
	ModTime int64 `json:"mod_time"`
	// InTrash marks an entry under trash/. The trash stays in the cache because
	// it takes part in deduplication: a file thrown away in March must not be
	// silently taken back in in September.
	InTrash bool `json:"in_trash,omitempty"`
	// File is the sidecar's content, which is the truth this is a copy of.
	File sidecar.File `json:"file"`
}

// Stale reports whether this entry describes a sidecar other than the one now
// on disk.
func (e Entry) Stale(size, modTime int64) bool {
	return e.Size != size || e.ModTime != modTime
}

// Index is the whole cache.
type Index struct {
	Version int     `json:"version"`
	Root    string  `json:"root"`
	Entries []Entry `json:"entries"`
}

// Orphan is something in the Box that does not pair up.
//
// Both directions are reported and neither is repaired. A sidecar with no scan
// may be the remains of a move; a scan with no sidecar has lost its only
// metadata. Guessing which is which, and acting on the guess, is how a tool
// destroys the thing it was asked to look after.
type Orphan struct {
	// Path is relative to the Box root.
	Path string
	// Kind is "sidecar" for a sidecar with no scan beside it, "scan" for a scan
	// with no sidecar, and "missing" for an entry in the cache whose file is no
	// longer on disk.
	Kind string
	// Reason is a sentence for a report.
	Reason string
}

// PathFor is the cache file for a Box root inside cacheDir.
//
// The root is part of the cache's path rather than of its contents, which is
// what lets two Boxes and two machines each keep a cache without ever having to
// agree on anything.
func PathFor(cacheDir, root string) string {
	return filepath.Join(cacheDir, box.CacheKey(root), Name)
}

// Load reads the cache at path.
//
// It returns usable=false for every reason a cache might not be usable —
// missing, truncated, unparseable, a version this build does not write, a root
// that is not the one asked for — because the answer to all of them is the same
// and it is Build. An error is returned only for something a rebuild would not
// fix, such as a permission problem.
func Load(path, root string) (Index, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Index{}, false, nil
		}
		return Index{}, false, err
	}
	var cache Index
	if err := json.Unmarshal(data, &cache); err != nil {
		return Index{}, false, nil
	}
	if cache.Version != Version {
		return Index{}, false, nil
	}
	if root != "" && cache.Root != filepath.Clean(root) {
		return Index{}, false, nil
	}
	return cache, true, nil
}

// Save writes the cache whole.
//
// A temporary file beside it and a rename, with no sync of either: this is a
// cache, a lost write costs one rebuild, and paying for durability here would
// buy nothing that Build does not already give for free.
func Save(path string, cache Index) error {
	cache.Version = Version
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, "."+Name+".*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

// Build reads every sidecar in the Box and returns a cache and whatever did not
// pair up.
//
// Sidecars only. The digest comes from the sidecar, never from the file's
// bytes: that is what makes this seconds rather than tens of gigabytes, and it
// is the whole difference between dgs box index and dgs box verify.
func Build(root string) (Index, []Orphan, error) {
	root = filepath.Clean(root)
	found, orphans, err := walk(root)
	if err != nil {
		return Index{}, nil, err
	}
	entries := make([]Entry, 0, len(found))
	for _, pair := range found {
		entry, err := readEntry(root, pair)
		if err != nil {
			// A sidecar this build cannot read is reported, not fatal. One bad
			// file must not make the other three thousand unreachable.
			orphans = append(orphans, Orphan{Path: pair.sidecar, Kind: "sidecar", Reason: err.Error()})
			continue
		}
		entries = append(entries, entry)
	}
	sortEntries(entries)
	sortOrphans(orphans)
	return Index{Version: Version, Root: root, Entries: entries}, orphans, nil
}

// Refresh brings an existing cache up to date without rereading what has not
// changed.
//
// The walk reads the directory listing only — names, sizes and modification
// times — and reopens just the sidecars whose triple moved. An entry whose file
// is gone is reported as an orphan and dropped from the returned cache; it is
// not silently kept, because a cache that remembers files nobody has is a cache
// that answers questions wrongly. A sidecar that reappears under a new path
// with the same digest was moved, and is not reported.
func Refresh(root string, existing Index) (Index, []Orphan, error) {
	root = filepath.Clean(root)
	found, orphans, err := walk(root)
	if err != nil {
		return Index{}, nil, err
	}
	previous := make(map[string]Entry, len(existing.Entries))
	for _, entry := range existing.Entries {
		previous[entry.SidecarPath] = entry
	}
	seen := make(map[string]bool, len(found))
	entries := make([]Entry, 0, len(found))
	for _, pair := range found {
		relative := relative(root, pair.sidecar)
		seen[relative] = true
		if kept, ok := previous[relative]; ok && !kept.Stale(pair.size, pair.modTime) {
			// The scan beside it may have been renamed even when the sidecar
			// was not touched, so that one field is refreshed from the walk.
			kept.ScanPath = relativeOrEmpty(root, pair.scan)
			kept.InTrash = pair.inTrash
			entries = append(entries, kept)
			continue
		}
		entry, err := readEntry(root, pair)
		if err != nil {
			orphans = append(orphans, Orphan{Path: relative, Kind: "sidecar", Reason: err.Error()})
			continue
		}
		entries = append(entries, entry)
	}
	// A sidecar that appeared under a new path with the same digest was moved —
	// into the trash, or back out of it — not lost. Reporting a move the tool
	// made itself as a missing file tells the person something is wrong when it
	// is not. Only new paths count: a copy the cache already knew, such as a
	// duplicate discarded earlier, must not hide the loss of the original.
	moved := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if _, known := previous[entry.SidecarPath]; !known && entry.File.Digest != "" {
			moved[entry.File.Digest] = true
		}
	}
	for _, entry := range existing.Entries {
		if !seen[entry.SidecarPath] && !moved[entry.File.Digest] {
			orphans = append(orphans, Orphan{
				Path:   entry.SidecarPath,
				Kind:   "missing",
				Reason: "in the cache but no longer in the Box",
			})
		}
	}
	sortEntries(entries)
	sortOrphans(orphans)
	return Index{Version: Version, Root: root, Entries: entries}, orphans, nil
}

// pair is one sidecar and the scan beside it, as the directory listing shows
// them.
type pair struct {
	sidecar string
	scan    string
	size    int64
	modTime int64
	inTrash bool
}

// walk lists the Box and pairs sidecars with scans by the digest prefix in
// their names.
//
// Pairing on the digest rather than on the filename is deliberate: macOS hands
// out names in NFD while most software writes NFC, so the same name can exist
// as two byte sequences that look identical. A pairing that depended on those
// bytes would report real sidecars as orphans, and an orphan report nobody
// trusts is worse than none.
func walk(root string) ([]pair, []Orphan, error) {
	type directory struct {
		sidecars map[string]pair     // digest prefix to sidecar
		scans    map[string][]string // digest prefix to scan paths
	}
	directories := map[string]*directory{}
	skip := map[string]bool{box.MarkerName: true, boxlog.Name: true}
	var unpaired []Orphan

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if skip[name] || strings.HasPrefix(name, ".") {
			return nil
		}
		parent := filepath.Dir(path)
		bucket := directories[parent]
		if bucket == nil {
			bucket = &directory{sidecars: map[string]pair{}, scans: map[string][]string{}}
			directories[parent] = bucket
		}
		if strings.HasSuffix(name, sidecar.Suffix) {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			prefix := strings.TrimSuffix(name, sidecar.Suffix)
			bucket.sidecars[prefix] = pair{
				sidecar: path,
				size:    info.Size(),
				modTime: info.ModTime().UnixNano(),
				inTrash: underTrash(root, path),
			}
			return nil
		}
		if prefix, ok := scanPrefix(name); ok {
			bucket.scans[prefix] = append(bucket.scans[prefix], path)
			return nil
		}
		// A file whose name carries no digest prefix cannot be paired with any
		// sidecar, so it is reported rather than passed over. Dropping a scan
		// into the tree by hand is a deliberate act and the answer to it is
		// adopt; a file the walk ignored would instead be a file nobody is ever
		// told about.
		unpaired = append(unpaired, Orphan{
			Path:   relative(root, path),
			Kind:   "scan",
			Reason: "no sidecar beside it, and its name carries no digest",
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	var pairs []pair
	orphans := unpaired
	for _, bucket := range directories {
		matched := map[string]bool{}
		for prefix, found := range bucket.sidecars {
			scans := bucket.scans[prefix]
			if len(scans) == 0 {
				orphans = append(orphans, Orphan{
					Path:   relative(root, found.sidecar),
					Kind:   "sidecar",
					Reason: "no scan beside it",
				})
				continue
			}
			sort.Strings(scans)
			matched[prefix] = true
			found.scan = scans[0]
			pairs = append(pairs, found)
		}
		for prefix, scans := range bucket.scans {
			if matched[prefix] {
				continue
			}
			for _, scan := range scans {
				orphans = append(orphans, Orphan{
					Path:   relative(root, scan),
					Kind:   "scan",
					Reason: "no sidecar beside it",
				})
			}
		}
	}
	sort.Slice(pairs, func(a, b int) bool { return pairs[a].sidecar < pairs[b].sidecar })
	return pairs, orphans, nil
}

// scanPrefix reads the digest prefix off a published filename —
// Scan_0012-a1b2c3d4.pdf gives a1b2c3d4 — and reports whether there was one.
func scanPrefix(name string) (string, bool) {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	cut := strings.LastIndex(stem, "-")
	if cut < 0 || cut == len(stem)-1 {
		return "", false
	}
	prefix := stem[cut+1:]
	if !isHex(prefix) {
		return "", false
	}
	return prefix, true
}

func isHex(text string) bool {
	if text == "" {
		return false
	}
	for _, character := range text {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'a' && character <= 'f':
		default:
			return false
		}
	}
	return true
}

func readEntry(root string, found pair) (Entry, error) {
	record, err := sidecar.Load(found.sidecar)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		SidecarPath: relative(root, found.sidecar),
		ScanPath:    relativeOrEmpty(root, found.scan),
		Size:        found.size,
		ModTime:     found.modTime,
		InTrash:     found.inTrash,
		File:        record,
	}, nil
}

func underTrash(root, path string) bool {
	return strings.HasPrefix(relative(root, path), "trash/")
}

func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func relativeOrEmpty(root, path string) string {
	if path == "" {
		return ""
	}
	return relative(root, path)
}

// sortEntries puts the cache in a fixed order so two builds of one Box produce
// byte-identical files. Without it the rebuild test would compare two correct
// caches and call them different.
func sortEntries(entries []Entry) {
	sort.Slice(entries, func(a, b int) bool { return entries[a].SidecarPath < entries[b].SidecarPath })
}

func sortOrphans(orphans []Orphan) {
	sort.Slice(orphans, func(a, b int) bool {
		if orphans[a].Path != orphans[b].Path {
			return orphans[a].Path < orphans[b].Path
		}
		return orphans[a].Kind < orphans[b].Kind
	})
}

// Open loads the cache for a Box and brings it up to date, rebuilding from
// scratch when there is nothing usable to start from. It is what a command
// calls at startup.
func Open(cacheDir, root string) (Index, []Orphan, error) {
	path := PathFor(cacheDir, root)
	existing, usable, err := Load(path, root)
	if err != nil {
		return Index{}, nil, err
	}
	var cache Index
	var orphans []Orphan
	if usable {
		cache, orphans, err = Refresh(root, existing)
	} else {
		cache, orphans, err = Build(root)
	}
	if err != nil {
		return Index{}, nil, err
	}
	if err := Save(path, cache); err != nil {
		// A cache that could not be written is still a cache that works for
		// this run. Failing the command over it would make an unwritable cache
		// directory look like an unreadable Box.
		return cache, orphans, fmt.Errorf("index built but not cached: %w", err)
	}
	return cache, orphans, nil
}
