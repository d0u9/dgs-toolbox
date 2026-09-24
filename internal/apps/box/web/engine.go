package web

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/dedupe"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/index"
	"dgs-toolbox/internal/box/intakestate"
	"dgs-toolbox/internal/box/money"
	"dgs-toolbox/internal/box/publish"
	"dgs-toolbox/internal/box/readmemo"
	"dgs-toolbox/internal/box/scanmeta"
	"dgs-toolbox/internal/box/scanread"
	"dgs-toolbox/internal/box/sidecar"
	"dgs-toolbox/internal/box/thumb"
	"dgs-toolbox/internal/box/thumbcache"
)

// Engine is the real Source: a Box on disk, its inbox, and the discardable
// cache beside them.
//
// It composes and does not compute. The digests are internal/box/digest, the
// metadata scanmeta, the pictures thumb, the grouping dedupe, the writing
// publish; everything here is the order those go in and what the pages are
// told about the result.
type Engine struct {
	mutex sync.Mutex

	root       string
	inbox      string
	stateName  string
	markerName string
	currency   string
	zone       string
	// workers is how many inbox files are read at once. Reading a file is a
	// whole file over a network filesystem, and a handful at once is faster
	// than one and faster than many.
	workers int
	// trashKeepDays is box.trash.keep: advice about the trash, never a removal.
	trashKeepDays int

	cacheDir string
	cache    index.Index
	orphan   []index.Orphan
	thumbs   thumbcache.Store
	// reads remembers what reading each inbox file produced, so a restart
	// does not read the whole inbox over the network again.
	reads readmemo.Store
	// pages is the document being paged through; it has its own lock.
	pages pageMemo

	// pending is the inbox as the last scan of it saw it. It is built once,
	// because building it reads every candidate's bytes, and rebuilt only when
	// Rescan is called.
	pending []candidate
	scanned bool
	// state is what the last sitting decided, kept in the inbox. A hundred
	// scans are not sorted in one go, so a candidate already published,
	// rejected or found to be a duplicate is not offered again.
	state intakestate.File
}

// candidate is one file in the inbox, already read.
//
// The scan's own bytes are not kept: they are a few megabytes each and the only
// things wanted from them afterwards are the digests, the metadata and the two
// pictures, all of which are here.
type candidate struct {
	path string
	// relative is path inside the inbox, which is how the state names it: the
	// folder can be moved or renamed without invalidating every record in it.
	relative    string
	size        int64
	modTime     int64
	digest      string
	imageDigest string
	info        scanmeta.Info
	thumbs      thumb.Pair
	needsRender bool
	renderError string
	// incomplete marks a truncated or unparseable file. It is never taken in
	// and never silently skipped: a skip nobody is told about means believing
	// the inbox is empty when it is not.
	incomplete       bool
	incompleteReason string
	// edit is what the page has said about this scan before filing it. It is
	// held here rather than written anywhere, because nothing in the inbox is
	// in the Box yet and a half-described scan must not leave a sidecar behind.
	edit Scan
}

// NewEngine opens a Box. It refuses a root with no marker rather than creating
// one: a NAS that failed to mount would otherwise become a second, empty,
// entirely plausible-looking Box on the local disk.
func NewEngine(settings Settings) (*Engine, error) {
	root := strings.TrimSpace(settings.Root)
	if root == "" {
		return nil, errors.New("no Box: set box.root or pass --root")
	}
	if err := box.RequireBox(root, settings.MarkerName); err != nil {
		return nil, err
	}
	cacheDir := settings.CacheDir
	if strings.TrimSpace(cacheDir) == "" {
		return nil, errors.New("no cache directory for this Box")
	}
	cache, orphans, err := index.Open(cacheDir, root)
	if err != nil && cache.Version == 0 {
		return nil, err
	}
	engine := &Engine{
		root:       filepath.Clean(root),
		inbox:      strings.TrimSpace(settings.Inbox),
		stateName:  settings.StateFile,
		markerName: settings.MarkerName,
		currency:   settings.Currency,
		zone:       settings.Zone,
		cache:      cache,
		orphan:     orphans,
		cacheDir:   cacheDir,
		thumbs:     thumbcache.New(cacheDir, root),
		reads:      readmemo.New(cacheDir, strings.TrimSpace(settings.Inbox)),

		workers:       settings.Workers,
		trashKeepDays: settings.TrashKeepDays,
	}
	// Previews are dropped after box.preview.keep days without use. Sweeping
	// at startup rather than on a timer is enough: they are a cache, and the
	// only cost of keeping one a day longer is disk.
	_, _ = engine.thumbs.Sweep(settings.PreviewKeepDays, time.Now())
	return engine, nil
}

// Sample reports that this is a Box rather than a stand-in.
func (e *Engine) Sample() bool { return false }

// Locate is the absolute path of a filed scan's file in the Box. A scan only
// in the trash is found there, because a duplicate of something discarded is
// answered by looking at what was discarded.
func (e *Engine) Locate(digest string) (string, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	position, found := e.entryIndexLocked(digest)
	if !found {
		position, found = e.trashIndexLocked(digest)
	}
	if !found {
		// A scan still in intake is shown where it waits, in the inbox.
		if pending, waiting := e.pendingIndexLocked(digest); waiting {
			return e.pending[pending].path, nil
		}
		return "", fmt.Errorf("no scan %q", digest)
	}
	entry := e.cache.Entries[position]
	if entry.ScanPath == "" {
		return "", fmt.Errorf("%s has no file in the Box", digest)
	}
	return filepath.Join(e.root, entry.ScanPath), nil
}

// Scans is everything filed in the Box, newest first, with the trash left out:
// the trash takes part in deduplication but is not something to browse.
func (e *Engine) Scans() []Scan {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	today := box.Today(nil)
	fromInbox := e.fromInboxLocked()
	scans := make([]Scan, 0, len(e.cache.Entries))
	for _, entry := range e.cache.Entries {
		if entry.InTrash {
			continue
		}
		scan := decorate(scanFromEntry(entry), today)
		_, scan.Unfilable = fromInbox[entry.File.Digest]
		scans = append(scans, scan)
	}
	sort.SliceStable(scans, func(i, j int) bool {
		left, right := sortKey(scans[i]), sortKey(scans[j])
		if left == right {
			return scans[i].Filename < scans[j].Filename
		}
		return left > right
	})
	return scans
}

// Pending is what intake still has to deal with, in scan-time order.
//
// Scan-time order rather than filename order because what was scanned
// consecutively is related — the flight, the boarding pass and the hotel
// receipt sit next to each other — and that adjacency is what makes describing
// a range of scans at once correct rather than a guess.
func (e *Engine) Pending() []Scan {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if !e.scanned {
		e.rescanLocked()
	}
	today := box.Today(nil)
	known := e.duplicatesLocked()
	scans := make([]Scan, 0, len(e.pending))
	for _, entry := range e.pending {
		scan := entry.edit
		scan.Digest = entry.digest
		scan.Filename = filepath.Base(entry.path)
		scan.Kind = string(entry.info.Kind)
		scan.Pages = entry.info.Pages
		scan.PageSize = entry.info.PageSize
		scan.Producer = entry.info.Producer
		scan.ScannedAt = entry.info.CreatedAt
		scan.NeedsRender = entry.needsRender
		scan.Colour, scan.DPI = pictureFacts(entry.info)
		if match, found := known[entry.digest]; found {
			scan.DuplicateOf, scan.DuplicatePath, scan.TrashedAt = match.digest, match.path, match.trashedAt
		} else if match, found := known[entry.imageDigest]; found && entry.imageDigest != "" {
			scan.DuplicateOf, scan.DuplicatePath, scan.TrashedAt = match.digest, match.path, match.trashedAt
		}
		scans = append(scans, decorate(scan, today))
	}
	sort.SliceStable(scans, func(i, j int) bool { return scans[i].ScannedAt < scans[j].ScannedAt })
	return scans
}

// match is a digest already in the Box, where its file is relative to the Box
// root, and, when it was discarded, when.
type match struct {
	digest    string
	path      string
	trashedAt string
}

// duplicatesLocked indexes the Box by both digests, so a candidate can be
// matched on either.
//
// The trash is included. Rejecting something during intake and being shown it
// again on the next run — needing the same judgement a second time — is exactly
// the tedium this tool exists to remove.
func (e *Engine) duplicatesLocked() map[string]match {
	known := make(map[string]match, len(e.cache.Entries)*2)
	for _, entry := range e.cache.Entries {
		// A scan unfiled to be filed again is not a judgement already made:
		// its copy in the trash is how it was taken back, not a rejection.
		if entry.InTrash && entry.File.Reason == UnfiledReason {
			continue
		}
		found := match{digest: entry.File.Digest, path: entry.ScanPath}
		if entry.InTrash {
			found.trashedAt = string(entry.File.TrashedAt)
		}
		if entry.File.Digest != "" {
			known[entry.File.Digest] = found
		}
	}
	return known
}

// Exceptions is what cannot be explained about the tree.
func (e *Engine) Exceptions() []Exception {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	exceptions := make([]Exception, 0, len(e.orphan))
	for _, orphan := range e.orphan {
		exception := Exception{Path: orphan.Path, Detail: orphan.Reason}
		switch orphan.Kind {
		case "scan":
			exception.Kind = "no sidecar"
			// The one exception with an answer rather than a report: a file put
			// there by hand can be described where it lies.
			exception.Adoptable = true
		case "sidecar":
			exception.Kind = "no scan"
		default:
			exception.Kind = orphan.Kind
		}
		exceptions = append(exceptions, exception)
	}
	return exceptions
}

// Apply changes one scan's metadata.
//
// For a filed scan this is one sidecar rewrite and one log line, and a rename
// when the event date now places the scan in another month. The digest stays
// valid, because the bytes are untouched. For a pending one it changes nothing on
// disk at all — there is nothing in the Box yet to change.
func (e *Engine) Apply(edit Edit) (Scan, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if position, found := e.pendingIndexLocked(edit.Digest); found {
		// The page count is the file's, not the draft's, and a split is
		// checked against it.
		base := e.pending[position].edit
		base.Pages = e.pending[position].info.Pages
		updated, err := applyEdit(base, edit, e.currency)
		if err != nil {
			return Scan{}, err
		}
		e.pending[position].edit = updated
		// The description is held in the inbox's state, not written into the
		// Box: nothing here is in the Box yet, and a half-described scan must
		// not leave a sidecar behind. It is kept so that closing the tool
		// halfway through a pile does not throw away what was typed.
		e.rememberDraftLocked(position, updated)
		return e.pendingScanLocked(position), nil
	}
	position, found := e.entryIndexLocked(edit.Digest)
	if !found {
		return Scan{}, fmt.Errorf("no scan %q", edit.Digest)
	}
	entry := e.cache.Entries[position]
	before := scanFromEntry(entry)
	updated, err := applyEdit(before, edit, e.currency)
	if err != nil {
		return Scan{}, err
	}
	record, err := recordFromScan(entry.File, updated)
	if err != nil {
		return Scan{}, err
	}
	record.EditedAt = sidecar.Timestamp(time.Now().UTC().Format(time.RFC3339))
	path := filepath.Join(e.root, entry.SidecarPath)
	if err := sidecar.Save(path, record); err != nil {
		return Scan{}, err
	}
	// The log is appended after the sidecar, because the sidecar is the truth
	// and the log is the history of how it got that way. A line describing a
	// change that failed to be written would be a history of something that
	// did not happen.
	for _, change := range changes(before, updated) {
		change.Digest = entry.File.Digest
		if err := boxlog.Append(boxlog.PathFor(e.root), change); err != nil {
			return Scan{}, fmt.Errorf("saved %s but could not append to the log: %w", entry.SidecarPath, err)
		}
	}
	e.cache.Entries[position].File = record
	// A changed event date can place the scan in another month. The sidecar
	// is saved first, so a move that fails leaves the scan described rightly
	// where it was, and the next edit tries again.
	if entry.ScanPath != "" && !entry.InTrash {
		prefix := strings.TrimSuffix(filepath.Base(entry.SidecarPath), sidecar.Suffix)
		moved, err := publish.Move(publish.MoveRequest{
			Root:       e.root,
			MarkerName: e.markerName,
			Path:       filepath.Join(e.root, entry.ScanPath),
			Digest:     entry.File.Digest,
			Prefix:     len(prefix),
			Record:     record,
		})
		if err != nil {
			e.refreshStalenessLocked(position)
			e.saveCacheLocked()
			return Scan{}, fmt.Errorf("saved %s but could not move it: %w", entry.SidecarPath, err)
		}
		if moved.Moved {
			e.cache.Entries[position].ScanPath = relativeTo(e.root, moved.Path)
			e.cache.Entries[position].SidecarPath = relativeTo(e.root, moved.SidecarPath)
		}
	}
	e.refreshStalenessLocked(position)
	e.saveCacheLocked()
	return decorate(scanFromEntry(e.cache.Entries[position]), box.Today(nil)), nil
}

// File takes a pending scan into the Box: the only call that copies bytes.
func (e *Engine) File(digest string, edit Edit) (Scan, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	position, found := e.pendingIndexLocked(digest)
	if !found {
		return Scan{}, fmt.Errorf("no pending scan %q", digest)
	}
	entry := e.pending[position]
	edit.Digest = digest
	base := entry.edit
	base.Pages = entry.info.Pages
	described, err := applyEdit(base, edit, e.currency)
	if err != nil {
		return Scan{}, err
	}
	record, err := recordFromScan(sidecar.File{}, described)
	if err != nil {
		return Scan{}, err
	}
	record.Kind = sidecar.Kind(entry.info.Kind)
	record.Pages = entry.info.Pages
	record.PageSize = entry.info.PageSize
	record.Producer = entry.info.Producer
	record.ScanCreatedAt = sidecar.Timestamp(entry.info.CreatedAt)
	// unsorted is always a legal outcome. Intake never blocks on a decision,
	// which is what makes an inbox drainable.
	if strings.TrimSpace(record.Type) == "" {
		record.Type = "unsorted"
	}

	result, err := publish.Publish(context.Background(), publish.Request{
		Root:       e.root,
		MarkerName: e.markerName,
		Source:     entry.path,
		IntakeDate: box.Today(nil),
		Digest:     entry.digest,
		Sidecar:    record,
	})
	if err != nil {
		return Scan{}, err
	}
	// The pictures are written after the scan is published, not before: a
	// thumbnail of something that failed to publish is a picture of nothing.
	// Failing to write one is not a reason to un-file a verified scan, so the
	// error is dropped and the page simply has no picture until the next read.
	_ = e.thumbs.Save(entry.digest, entry.thumbs)

	e.state.Set(intakestate.Record{
		Path: entry.relative, Size: entry.size, ModTime: entry.modTime,
		Digest: entry.digest, ImageDigest: entry.imageDigest,
		State: intakestate.Published, PublishedPath: result.RelativePath,
	}, time.Now())
	e.saveStateLocked()

	e.pending = append(e.pending[:position], e.pending[position+1:]...)
	info, err := os.Lstat(result.SidecarPath)
	newEntry := index.Entry{
		SidecarPath: relativeTo(e.root, result.SidecarPath),
		ScanPath:    relativeTo(e.root, result.Path),
		File:        result.Sidecar,
	}
	if err == nil {
		newEntry.Size, newEntry.ModTime = info.Size(), info.ModTime().UnixNano()
	}
	e.cache.Entries = append(e.cache.Entries, newEntry)
	e.saveCacheLocked()
	return decorate(scanFromEntry(newEntry), box.Today(nil)), nil
}

// UnfiledReason is the reason a scan taken back out of the Box is discarded
// with. Duplicate detection passes over a trashed copy carrying it, so the
// scan comes back to intake as a scan and not as something thrown away.
const UnfiledReason = "unfiled to be filed again"

// fromInboxLocked is every digest published from the configured inbox whose
// file is still there, which is what can be unfiled.
func (e *Engine) fromInboxLocked() map[string]intakestate.Record {
	if !e.scanned && e.inbox != "" {
		e.state = e.loadStateLocked()
	}
	out := map[string]intakestate.Record{}
	for _, record := range e.state.Records {
		if record.State == intakestate.Published && record.Digest != "" {
			out[record.Digest] = record
		}
	}
	return out
}

// Unfile takes a filed scan back to intake, for a scan filed wrongly as a
// whole. Nothing is deleted: the Box's copy goes to the trash, as any discard
// does, and the scan returns to the inbox's list with everything its sidecar
// said as the draft, to be corrected and filed again. It needs the inbox file
// the scan came from: the Box's copy is not moved back out.
func (e *Engine) Unfile(digest string) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	record, found := e.fromInboxLocked()[digest]
	if !found {
		return fmt.Errorf("%s did not come from this inbox, or its file is gone from it: correct it here instead", digest)
	}
	position, found := e.entryIndexLocked(digest)
	if !found {
		return fmt.Errorf("no scan %q", digest)
	}
	draft := e.cache.Entries[position].File
	if err := e.trashLocked(digest, UnfiledReason); err != nil {
		return err
	}
	record.State, record.PublishedPath, record.At = intakestate.Classified, "", ""
	record.Draft = &draft
	e.state.Set(record, time.Now())
	e.saveStateLocked()
	e.rescanLocked()
	return nil
}

// Rejected lists what was turned away at intake and is still in the inbox,
// newest first. A rejected file whose owner has since removed it is gone from
// the state too, so everything here can be restored.
func (e *Engine) Rejected() []RejectedScan {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if !e.scanned {
		e.rescanLocked()
	}
	var found []RejectedScan
	for _, record := range e.state.Records {
		if record.State != intakestate.Rejected {
			continue
		}
		found = append(found, RejectedScan{
			Digest: record.Digest, Filename: filepath.Base(filepath.FromSlash(record.Path)),
			Reason: record.Reason, At: record.At,
		})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].At > found[j].At })
	return found
}

// RejectedFile reads a rejected scan from the inbox. The file is named by the
// state's record, and its bytes must still hash to the digest the record was
// made for: a file replaced under the same name is not the one rejected.
func (e *Engine) RejectedFile(want string) ([]byte, string, string, error) {
	e.mutex.Lock()
	var relative string
	for _, record := range e.state.Records {
		if record.Digest == want && record.State == intakestate.Rejected {
			relative = record.Path
		}
	}
	inbox := e.inbox
	e.mutex.Unlock()
	if relative == "" {
		return nil, "", "", fmt.Errorf("no rejected scan %q", want)
	}
	body, err := os.ReadFile(filepath.Join(inbox, filepath.FromSlash(relative)))
	if err != nil {
		return nil, "", "", err
	}
	if digest.Whole(body) != want {
		return nil, "", "", fmt.Errorf("%s has changed since it was rejected", relative)
	}
	name := filepath.Base(filepath.FromSlash(relative))
	return body, name, mime.TypeByExtension(strings.ToLower(filepath.Ext(name))), nil
}

// Restore takes back a rejection. Rejecting touched nothing but the inbox's
// state, so neither does this: the record goes back to pending and the inbox
// is read again, which brings the scan back into the list.
func (e *Engine) Restore(digest string) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	for _, record := range e.state.Records {
		if record.Digest != digest || record.State != intakestate.Rejected {
			continue
		}
		record.State, record.Reason, record.At = intakestate.Pending, "", ""
		e.state.Set(record, time.Now())
		e.saveStateLocked()
		e.rescanLocked()
		return nil
	}
	return fmt.Errorf("no rejected scan %q", digest)
}

// Trash moves a filed scan into the Box's trash. Nothing is deleted.
func (e *Engine) Trash(digest, reason string) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.trashLocked(digest, reason)
}

func (e *Engine) trashLocked(digest, reason string) error {
	// A pending scan is not in the Box, so rejecting one is a decision about
	// the inbox and touches nothing. The source file stays where its owner put
	// it; emptying a temporary folder is their call, not this tool's.
	if position, found := e.pendingIndexLocked(digest); found {
		entry := e.pending[position]
		e.state.Set(intakestate.Record{
			Path: entry.relative, Size: entry.size, ModTime: entry.modTime,
			Digest: entry.digest, ImageDigest: entry.imageDigest,
			State: intakestate.Rejected, Reason: reason,
		}, time.Now())
		e.saveStateLocked()
		e.pending = append(e.pending[:position], e.pending[position+1:]...)
		return nil
	}
	position, found := e.entryIndexLocked(digest)
	if !found {
		return fmt.Errorf("no scan %q", digest)
	}
	entry := e.cache.Entries[position]
	if entry.ScanPath == "" {
		return fmt.Errorf("%s has no file to discard", digest)
	}
	prefix := strings.TrimSuffix(filepath.Base(entry.SidecarPath), sidecar.Suffix)
	if _, err := publish.Discard(publish.DiscardRequest{
		Root:        e.root,
		MarkerName:  e.markerName,
		Path:        filepath.Join(e.root, entry.ScanPath),
		Digest:      entry.File.Digest,
		Prefix:      len(prefix),
		DiscardDate: box.Today(nil),
		Reason:      reason,
	}); err != nil {
		return err
	}
	// Refreshing from disk rather than editing the entry in place: Discard
	// moved two files and wrote a third, and guessing where they all landed is
	// how a cache starts describing a Box that does not exist.
	e.reindexLocked()
	e.saveCacheLocked()
	return nil
}

// Image is a scan's picture.
//
// A filed scan's picture comes from the cache and is drawn from the scan again
// if it is not there. A pending one's first page was made during the read of
// the inbox; any other page of it is drawn from the inbox file when asked for.
//
// size is what the page asks for — "thumb" or "preview" — or the cache's own
// name for a size. Anything that is not a preview is a grid thumbnail.
func (e *Engine) Image(digest, size string, page int) ([]byte, string, error) {
	size = cacheSize(size)
	if page < 1 {
		page = 1
	}
	// The lock covers only deciding where the picture comes from. Reading a
	// scan and drawing a page take long enough that holding it would queue
	// every thumbnail in the strip behind the page being turned to.
	e.mutex.Lock()
	var path string
	pending := false
	if position, found := e.pendingIndexLocked(digest); found {
		entry := e.pending[position]
		if page == 1 {
			picture := entry.thumbs.Grid
			if size == thumbcache.SizePreview {
				picture = entry.thumbs.Preview
			}
			e.mutex.Unlock()
			if len(picture) == 0 {
				return nil, "", fmt.Errorf("%s has no picture: %s", digest, entry.renderError)
			}
			return picture, thumbcache.MediaType, nil
		}
		if page > entry.info.Pages {
			e.mutex.Unlock()
			return nil, "", fmt.Errorf("%s has no page %d", digest, page)
		}
		path, pending = entry.path, true
	} else {
		if data, err := e.thumbs.LoadPage(digest, size, page); err == nil {
			e.mutex.Unlock()
			return data, thumbcache.MediaType, nil
		} else if !errors.Is(err, thumbcache.ErrNotStored) {
			e.mutex.Unlock()
			return nil, "", err
		}
		position, found := e.entryIndexLocked(digest)
		if !found {
			e.mutex.Unlock()
			return nil, "", fmt.Errorf("no scan %q", digest)
		}
		entry := e.cache.Entries[position]
		if entry.ScanPath == "" {
			e.mutex.Unlock()
			return nil, "", fmt.Errorf("%s has no file to draw from", digest)
		}
		path = filepath.Join(e.root, entry.ScanPath)
	}
	thumbs := e.thumbs
	e.mutex.Unlock()

	// A page past the first of something not yet in the Box is not saved to
	// the cache: the cache belongs to the Box, and a scan that is rejected
	// must not leave pictures of itself behind. It is still kept in memory
	// with the rest of the document being looked at.
	var save func(int, thumb.Pair)
	if !pending {
		save = func(page int, pair thumb.Pair) {
			if page > 1 {
				_ = thumbs.SavePage(digest, page, pair)
			}
		}
	}
	// The first page of a filed scan missing from the cache is drawn the way
	// the inbox read drew it, so the grid's picture does not change: it falls
	// back to a scan's only picture when page one has none.
	if page == 1 && !pending {
		result, err := scanread.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
		if result.NeedsRender {
			return nil, "", fmt.Errorf("%s has no picture: %s", digest, result.RenderError)
		}
		_ = thumbs.Save(digest, result.Thumbs)
		return pick(result.Thumbs, size), thumbcache.MediaType, nil
	}
	pair, err := e.pages.draw(digest, path, page, save)
	if err != nil {
		return nil, "", fmt.Errorf("%s page %d: %w", digest, page, err)
	}
	return pick(pair, size), thumbcache.MediaType, nil
}

// pageMemo holds the one document being paged through, parsed once, and every
// page of it drawn so far. Turning a page otherwise reads the whole scan again,
// over a network filesystem, and decodes every picture in it, for one page.
//
// One document, because a person looks at one at a time; the next scan asked
// for replaces it, so memory stays bounded by the largest scan.
//
// Once a document is open, the rest of its pages are drawn in the background,
// nearest the one asked for first, so turning finds them ready. A filed scan's
// pages are saved to the cache as they are drawn; a pending one's are not.
type pageMemo struct {
	mutex  sync.Mutex
	digest string
	pages  *scanmeta.Pages
	drawn  map[int]thumb.Pair
	// save stores a drawn page, or is nil for a scan not yet in the Box.
	save func(page int, pair thumb.Pair)
}

// forget lets go of digest if it is the document held, so its pages are read
// again from the file rather than from memory.
func (m *pageMemo) forget(digest string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.digest == digest {
		m.digest, m.pages, m.drawn = "", nil, nil
	}
}

// draw returns one page of the scan at path, opening the scan only when it is
// not the document already held.
func (m *pageMemo) draw(digest, path string, page int, save func(int, thumb.Pair)) (thumb.Pair, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.digest != digest {
		data, err := os.ReadFile(path)
		if err != nil {
			return thumb.Pair{}, err
		}
		if scanmeta.LooksLikePDF(data) && !scanmeta.Complete(data) {
			return thumb.Pair{}, errors.New("the file does not end with %EOF, so it is truncated or was copied badly")
		}
		pages, err := scanmeta.OpenPages(data)
		if err != nil {
			return thumb.Pair{}, err
		}
		m.digest, m.pages, m.drawn, m.save = digest, pages, map[int]thumb.Pair{}, save
		go m.drawRest(digest, page)
	}
	return m.drawLocked(page)
}

func (m *pageMemo) drawLocked(page int) (thumb.Pair, error) {
	if pair, found := m.drawn[page]; found {
		return pair, nil
	}
	picture, err := m.pages.Page(page)
	if err != nil {
		return thumb.Pair{}, err
	}
	pair, err := thumb.RenderPicture(picture)
	if err != nil {
		return thumb.Pair{}, err
	}
	m.drawn[page] = pair
	if m.save != nil {
		m.save(page, pair)
	}
	return pair, nil
}

// drawRest draws every page of digest outward from start, one page per hold
// of the lock so a page asked for is never kept waiting behind the whole
// document. It stops as soon as another document replaces this one.
func (m *pageMemo) drawRest(digest string, start int) {
	m.mutex.Lock()
	count := 0
	if m.digest == digest {
		count = m.pages.Count()
	}
	m.mutex.Unlock()
	for distance := 1; distance < count; distance++ {
		for _, page := range []int{start + distance, start - distance} {
			if page < 1 || page > count {
				continue
			}
			m.mutex.Lock()
			if m.digest != digest {
				m.mutex.Unlock()
				return
			}
			_, _ = m.drawLocked(page)
			m.mutex.Unlock()
		}
	}
}

// cacheSize turns the name a page asks for into the cache's own.
func cacheSize(size string) string {
	if size == "preview" || size == thumbcache.SizePreview {
		return thumbcache.SizePreview
	}
	return thumbcache.SizeGrid
}

func pick(pair thumb.Pair, size string) []byte {
	if size == thumbcache.SizePreview {
		return pair.Preview
	}
	return pair.Grid
}

// Rescan reads the inbox again.
func (e *Engine) Rescan() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.rescanLocked()
}

// rescanLocked reads the inbox, skipping what a previous sitting already
// settled.
//
// One read per file that has to be read: hashing already pulls every byte
// across, so the digests, the metadata and both pictures come out of that same
// read. A candidate the state already knows — published, rejected, a duplicate,
// incomplete — is not read at all, which is what makes coming back to a
// half-sorted inbox cheap.
//
// A file that cannot be read is left out rather than failing the scan. One
// unreadable file must not hide the other two hundred.
func (e *Engine) rescanLocked() {
	e.scanned = true
	e.pending = nil
	if e.inbox == "" {
		return
	}
	e.state = e.loadStateLocked()
	known := e.state.Index()
	present := map[string]bool{}
	now := time.Now()

	// The walk itself is a directory listing and costs nothing. It collects
	// what has to be read; the reading is the expensive part and is done after
	// it, several files at a time.
	var toRead []candidate
	_ = filepath.WalkDir(e.inbox, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == e.stateFileName() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		relative := relativeTo(e.inbox, path)
		present[relative] = true

		if record, found := known[relative]; found && !record.Stale(info.Size(), info.ModTime().UnixNano()) {
			if record.State.Settled() && e.stillSettledLocked(record) {
				// Decided last time, and the file has not changed since.
				// Reading it again would cost a whole file over the network to
				// arrive at the same answer.
				return nil
			}
		}
		toRead = append(toRead, candidate{path: path, relative: relative, size: info.Size(), modTime: info.ModTime().UnixNano()})
		return nil
	})

	candidates := readCandidates(toRead, e.workers, e.reads)
	_ = e.reads.Keep(present)
	for position := range candidates {
		// A description typed last time and not yet filed is picked back up.
		if record, ok := known[candidates[position].relative]; ok &&
			record.State == intakestate.Classified && record.Draft != nil {
			candidates[position].edit = scanFromDraft(*record.Draft)
		}
	}

	// Records for files their owner moved, renamed or deleted describe nothing.
	// What was published stays in the Box regardless: the Box is the truth and
	// this file never was.
	e.state.Prune(present)

	// Two files in one inbox holding the same bytes are one decision, not two.
	kept := collapse(candidates)
	remaining := kept[:0]
	for _, found := range kept {
		switch {
		case found.incomplete:
			e.state.Set(intakestate.Record{
				Path: found.relative, Size: found.size, ModTime: found.modTime,
				Digest: found.digest, State: intakestate.Incomplete,
				Reason: found.incompleteReason,
			}, now)
		default:
			e.state.Set(intakestate.Record{
				Path: found.relative, Size: found.size, ModTime: found.modTime,
				Digest: found.digest, ImageDigest: found.imageDigest,
				State: intakestate.Pending,
			}, now)
			remaining = append(remaining, found)
		}
	}
	e.pending = remaining
	e.saveStateLocked()
}

// readCandidates reads each file once — both digests, the metadata and both
// pictures out of that one read — with at most workers of them in flight.
//
// A handful at a time rather than one, because over a network filesystem a
// single reader spends most of its time waiting; and rather than all of them,
// because a hundred concurrent whole-file reads make the same filesystem
// slower, not faster. box.workers is the handful.
//
// A file that cannot be read is left out rather than failing the scan. One
// unreadable file must not hide the other two hundred.
func readCandidates(toRead []candidate, workers int, memo readmemo.Store) []candidate {
	if workers < 1 {
		workers = 1
	}
	if workers > len(toRead) {
		workers = len(toRead)
	}
	if len(toRead) == 0 {
		return nil
	}
	read := make([]*candidate, len(toRead))
	jobs := make(chan int)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for position := range jobs {
				found := toRead[position]
				result, ok := memo.Load(found.relative, found.size, found.modTime)
				if !ok {
					var err error
					result, err = scanread.ReadFile(found.path)
					if err != nil {
						continue
					}
					_ = memo.Save(found.relative, found.size, found.modTime, result)
				}
				result.Info.Images = nil
				found.size = result.Size
				found.digest = result.Digest
				found.imageDigest = result.ImageDigest
				found.info = result.Info
				found.thumbs = result.Thumbs
				found.needsRender = result.NeedsRender
				found.renderError = result.RenderError
				found.incomplete = result.Incomplete
				found.incompleteReason = result.IncompleteReason
				read[position] = &found
			}
		}()
	}
	for position := range toRead {
		jobs <- position
	}
	close(jobs)
	group.Wait()

	// Back into the walk's order, so the result does not depend on which
	// worker finished first.
	candidates := make([]candidate, 0, len(toRead))
	for _, found := range read {
		if found != nil {
			candidates = append(candidates, *found)
		}
	}
	return candidates
}

// Incomplete is what was read and not taken in: truncated files, bad copies,
// damaged old backups. It is reported rather than skipped, because a skip
// nobody is told about means believing the inbox is empty when it is not.
func (e *Engine) Incomplete() []Exception {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if !e.scanned {
		e.rescanLocked()
	}
	var found []Exception
	for _, record := range e.state.Records {
		if record.State != intakestate.Incomplete {
			continue
		}
		found = append(found, Exception{
			Kind:   "incomplete",
			Path:   record.Path,
			Detail: record.Reason,
		})
	}
	return found
}

// stillSettledLocked checks a settled decision against the Box, which is the
// truth the state is only a note about.
//
// A candidate recorded as published whose scan has since been discarded is not
// settled any more: the Box no longer holds it, and the person who threw it
// away may well want to be asked again. The state is a note about a pile of
// files; when it disagrees with the Box, the Box wins.
func (e *Engine) stillSettledLocked(record intakestate.Record) bool {
	if record.State != intakestate.Published {
		return true
	}
	_, inBox := e.entryIndexLocked(record.Digest)
	return inBox
}

func (e *Engine) stateFileName() string {
	if strings.TrimSpace(e.stateName) == "" {
		return intakestate.Name
	}
	return e.stateName
}

func (e *Engine) statePathLocked() (string, bool) {
	if e.inbox == "" {
		return "", false
	}
	path, err := intakestate.PathFor(e.inbox, e.stateName)
	if err != nil {
		return "", false
	}
	return path, true
}

func (e *Engine) loadStateLocked() intakestate.File {
	path, ok := e.statePathLocked()
	if !ok {
		return intakestate.File{Version: intakestate.Version}
	}
	return intakestate.Load(path)
}

// saveStateLocked writes the state back. It is deliberately not fatal: losing
// it costs re-reading the inbox, and refusing to show a person their scans
// because a temporary folder is read-only would be the wrong trade.
func (e *Engine) saveStateLocked() {
	path, ok := e.statePathLocked()
	if !ok {
		return
	}
	_ = intakestate.Save(path, e.state)
}

// scanFromDraft picks a description back up where it was left, so closing the
// tool halfway through a pile does not throw away what was already typed.
func scanFromDraft(draft sidecar.File) Scan {
	scan := Scan{
		Type:          draft.Type,
		Reviewed:      draft.Reviewed,
		Description:   draft.Description,
		Tags:          draft.Tags,
		EventDate:     draft.EventDate.String(),
		EventZone:     draft.EventZone,
		ExpiresAt:     draft.ExpiresAt.String(),
		ExpiryCleared: draft.ExpiryCleared,
		NeedsSplit:    draft.NeedsSplit,
	}
	scan.Documents, scan.IgnoredPages = splitFromRecord(draft)
	if draft.Total.Valid() {
		scan.Total = draft.Total.String()
	}
	return scan
}

// collapse drops inbox candidates that repeat one another, keeping the first in
// path order so the choice does not move between runs.
func collapse(candidates []candidate) []candidate {
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })
	items := make([]dedupe.Item, 0, len(candidates))
	byID := make(map[string]candidate, len(candidates))
	for _, entry := range candidates {
		items = append(items, dedupe.Item{ID: entry.path, Digest: entry.digest, ImageDigest: entry.imageDigest})
		byID[entry.path] = entry
	}
	kept := make([]candidate, 0, len(candidates))
	for _, group := range dedupe.Groups(items) {
		kept = append(kept, byID[group.Items[0].ID])
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].path < kept[j].path })
	return kept
}

// rememberDraftLocked stores what has been said about a candidate that is not
// in the Box yet.
func (e *Engine) rememberDraftLocked(position int, described Scan) {
	entry := e.pending[position]
	draft, err := recordFromScan(sidecar.File{}, described)
	if err != nil {
		return
	}
	e.state.Set(intakestate.Record{
		Path: entry.relative, Size: entry.size, ModTime: entry.modTime,
		Digest: entry.digest, ImageDigest: entry.imageDigest,
		State: intakestate.Classified, Draft: &draft,
	}, time.Now())
	e.saveStateLocked()
}

func (e *Engine) pendingIndexLocked(digest string) (int, bool) {
	for position, entry := range e.pending {
		if entry.digest == digest {
			return position, true
		}
	}
	return 0, false
}

// trashIndexLocked finds the newest trash entry for a digest.
func (e *Engine) trashIndexLocked(digest string) (int, bool) {
	for position := len(e.cache.Entries) - 1; position >= 0; position-- {
		entry := e.cache.Entries[position]
		if entry.File.Digest == digest && entry.InTrash && entry.ScanPath != "" {
			return position, true
		}
	}
	return 0, false
}

func (e *Engine) entryIndexLocked(digest string) (int, bool) {
	for position, entry := range e.cache.Entries {
		if entry.File.Digest == digest && !entry.InTrash {
			return position, true
		}
	}
	return 0, false
}

func (e *Engine) pendingScanLocked(position int) Scan {
	entry := e.pending[position]
	scan := entry.edit
	scan.Digest = entry.digest
	scan.Filename = filepath.Base(entry.path)
	scan.Kind = string(entry.info.Kind)
	scan.ScannedAt = entry.info.CreatedAt
	return decorate(scan, box.Today(nil))
}

func (e *Engine) refreshStalenessLocked(position int) {
	path := filepath.Join(e.root, e.cache.Entries[position].SidecarPath)
	if info, err := os.Lstat(path); err == nil {
		e.cache.Entries[position].Size = info.Size()
		e.cache.Entries[position].ModTime = info.ModTime().UnixNano()
	}
}

func (e *Engine) reindexLocked() {
	cache, orphans, err := index.Refresh(e.root, e.cache)
	if err != nil {
		return
	}
	e.cache, e.orphan = cache, orphans
}

// scanFromEntry turns a cache entry into what the pages see.
func scanFromEntry(entry index.Entry) Scan {
	record := entry.File
	scan := Scan{
		Digest:        record.Digest,
		Filename:      filepath.Base(entry.ScanPath),
		Kind:          string(record.Kind),
		Pages:         record.Pages,
		PageSize:      record.PageSize,
		Producer:      record.Producer,
		ScannedAt:     string(record.ScanCreatedAt),
		Type:          record.Type,
		Reviewed:      record.Reviewed,
		Description:   record.Description,
		Tags:          record.Tags,
		EventDate:     record.EventDate.String(),
		EventZone:     record.EventZone,
		ExpiresAt:     record.ExpiresAt.String(),
		ExpiryCleared: record.ExpiryCleared,
		NeedsSplit:    record.NeedsSplit,
		TrashedAt:     string(record.TrashedAt),
		IngestedAt:    string(record.IngestedAt),
	}
	// Never edited since it was filed, it was last changed when it was added.
	scan.EditedAt = string(record.EditedAt)
	if scan.EditedAt == "" {
		scan.EditedAt = scan.IngestedAt
	}
	scan.Documents, scan.IgnoredPages = splitFromRecord(record)
	if scan.Filename == "." || scan.Filename == string(filepath.Separator) {
		scan.Filename = record.OriginalFilename
	}
	if record.Total.Valid() {
		scan.Total = record.Total.String()
	}
	return scan
}

// recordFromScan folds what the page said back into a sidecar, leaving every
// field the page does not own — the digest, the size, what was derived from the
// bytes — exactly as it was.
func recordFromScan(record sidecar.File, scan Scan) (sidecar.File, error) {
	record.Type = scan.Type
	record.Reviewed = scan.Reviewed
	record.Description = scan.Description
	record.Tags = scan.Tags
	record.EventZone = scan.EventZone
	record.ExpiryCleared = scan.ExpiryCleared
	record.NeedsSplit = scan.NeedsSplit
	documents, ignored, err := splitToRecord(scan)
	if err != nil {
		return sidecar.File{}, err
	}
	record.Documents, record.IgnoredPages = documents, ignored

	eventDate, err := parseOptionalEventDate(scan.EventDate)
	if err != nil {
		return sidecar.File{}, err
	}
	record.EventDate = eventDate
	expiresAt, err := parseOptionalDate(scan.ExpiresAt)
	if err != nil {
		return sidecar.File{}, err
	}
	record.ExpiresAt = expiresAt

	if strings.TrimSpace(scan.Total) == "" {
		record.Total = money.Amount{}
	} else {
		amount, err := money.Parse(scan.Total, "")
		if err != nil {
			return sidecar.File{}, err
		}
		record.Total = amount
	}
	return record, nil
}

func parseOptionalEventDate(text string) (box.Date, error) {
	if strings.TrimSpace(text) == "" {
		return box.Date{}, nil
	}
	return box.ParseEventDate(text)
}

func parseOptionalDate(text string) (box.Date, error) {
	if strings.TrimSpace(text) == "" {
		return box.Date{}, nil
	}
	return box.ParseDate(text)
}

// changes is one log line per field that actually moved.
//
// Per field rather than one line per edit, because the log is what makes a
// three-hundred-record batch edit recoverable and "type: receipt to invoice" is
// what someone undoing one needs to read.
func changes(before, after Scan) []boxlog.Entry {
	var entries []boxlog.Entry
	add := func(field, from, to string) {
		if from != to {
			entries = append(entries, boxlog.Edit("", field, from, to))
		}
	}
	add("type", before.Type, after.Type)
	add("description", before.Description, after.Description)
	add("event_date", before.EventDate, after.EventDate)
	add("event_tz", before.EventZone, after.EventZone)
	add("expires_at", before.ExpiresAt, after.ExpiresAt)
	add("total", before.Total, after.Total)
	add("tags", strings.Join(before.Tags, ","), strings.Join(after.Tags, ","))
	add("reviewed", boolText(before.Reviewed), boolText(after.Reviewed))
	add("documents", splitText(before.Documents), splitText(after.Documents))
	add("ignored_pages", before.IgnoredPages, after.IgnoredPages)
	return entries
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// pictureFacts reads the colour mode and resolution off the first image, which
// record a judgement someone already made: monochrome at 200 dpi went through
// the feeder in a batch, colour at 600 dpi was placed on the glass.
func pictureFacts(info scanmeta.Info) (string, int) {
	if len(info.Images) == 0 {
		return "", 0
	}
	first := info.Images[0]
	dpi, _ := first.DPI(info.Width, info.Height)
	return first.ColorMode(), dpi
}

func relativeTo(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

// Save writes the cache back. Losing it costs one rebuild and nothing else.
func (e *Engine) Save() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.saveCacheLocked()
}

// saveCacheLocked writes the cache after a change to it.
//
// Every write does this rather than only the way out, because a process that
// is closed by its window being shut never gets a way out — and a cache that
// is only ever written once makes every start pay for a Refresh it should not
// need. The error is dropped: a Box on a machine whose cache directory is
// unwritable still works, one rebuild at a time.
func (e *Engine) saveCacheLocked() error {
	sortEntriesByPath(e.cache.Entries)
	return index.Save(index.PathFor(e.cacheDir, e.root), e.cache)
}

func sortEntriesByPath(entries []index.Entry) {
	sort.Slice(entries, func(a, b int) bool { return entries[a].SidecarPath < entries[b].SidecarPath })
}
