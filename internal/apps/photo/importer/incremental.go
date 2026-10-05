package importer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"dgs-toolbox/internal/apps/photo/skipdirs"
	"dgs-toolbox/internal/verifiedcopy"
)

// canonicalPath resolves existing ancestors, including a root that is itself
// a link, without requiring a new destination directory to exist yet.
func canonicalPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// within reports whether path, or any existing directory above it, is the same
// file as root. Comparing identities rather than spellings also catches a
// case-insensitive volume, where /Volumes/Card and /volumes/card are one.
func within(root os.FileInfo, path string) bool {
	for {
		if info, err := os.Stat(path); err == nil && os.SameFile(root, info) {
			return true
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}

// ValidateRoots rejects overlapping roots and existing aliases before any
// incremental comparison or write. The destination can be absent.
func ValidateRoots(source, destination string) error {
	if source == "" || destination == "" {
		return errors.New("incremental import requires source and destination roots")
	}
	src, err := canonicalPath(source)
	if err != nil {
		return err
	}
	dst, err := canonicalPath(destination)
	if err != nil {
		return err
	}
	si, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source root: %w", err)
	}
	if !si.IsDir() {
		return errors.New("source root must be a directory")
	}
	di, err := os.Stat(dst)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("destination root: %w", err)
	}
	if err == nil && !di.IsDir() {
		return errors.New("destination root must be a directory")
	}
	if di != nil && os.SameFile(si, di) {
		return errors.New("source and destination roots are aliases")
	}
	if inside(src, dst) || inside(dst, src) || within(si, dst) || (di != nil && within(di, src)) {
		return errors.New("source and destination roots overlap")
	}
	return nil
}

// IsImportArtifact identifies engine-owned bookkeeping and unpublished files.
// It does not exclude arbitrary dotfiles or folders.
func IsImportArtifact(name, stateName string) bool {
	return name == stateName || name == ".dgs-state" ||
		(strings.HasPrefix(name, ".") && strings.HasSuffix(name, verifiedcopy.PartSuffix)) ||
		((strings.HasPrefix(name, "."+stateName+".") || strings.HasPrefix(name, "..dgs-state.")) && strings.HasSuffix(name, ".tmp"))
}

type candidate struct {
	path    string
	info    os.FileInfo
	mu      sync.Mutex
	checked verifiedcopy.Result
	done    bool
}

type contentIndex struct {
	root   string
	bySize map[int64][]*candidate
	byPath map[string]*candidate
	hints  map[string][]FileResult
}

func pathKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func newContentIndex(ctx context.Context, plan Plan, prior State, events chan<- Event) (*contentIndex, error) {
	if err := ValidateRoots(plan.SourceRoot, plan.DestinationRoot); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(plan.DestinationRoot)
	if err != nil {
		return nil, err
	}
	index := &contentIndex{root: root, bySize: make(map[int64][]*candidate), byPath: make(map[string]*candidate), hints: make(map[string][]FileResult)}
	for _, file := range prior.Files {
		if file.Phase != PhaseComplete && file.Phase != PhaseAlreadyPresent && file.Phase != PhaseSourceKept {
			continue
		}
		planned := file.PlannedDestination
		if planned == "" {
			planned = file.Destination
		}
		if planned == "" || file.Destination == "" {
			continue
		}
		key := pathKey(planned)
		index.hints[key] = append(index.hints[key], file)
	}
	stateName := filepath.Base(plan.StatePath)
	if plan.StatePath == "" {
		stateName = ".dgs-state"
	}
	// Walk the resolved root so choosing a root symlink works, but never follow
	// a link below it. Snapshot membership never grows with this run's writes.
	walkRoot, err := canonicalPath(root)
	if err != nil {
		return nil, err
	}
	listed := 0
	var limit throttle
	report := func(final bool) {
		if final || limit.allow(PhaseInventory, int64(listed), int64(listed)+1) {
			emit(ctx, events, Event{Index: -1, Phase: PhaseInventory, Bytes: int64(listed)})
		}
	}
	err = filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := plan.Controller.wait(ctx); err != nil {
			return err
		}
		// Trash, snapshots and NAS metadata hold no durable copy: a photo
		// found only there is not imported, and a Move must not delete its
		// source on that evidence. They are skipped whether readable or not.
		if path != walkRoot && entry != nil && entry.IsDir() && skipdirs.Skip(entry.Name()) {
			return filepath.SkipDir
		}
		if walkErr != nil {
			if path == walkRoot && errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("destination inventory incomplete: %w", walkErr)
		}
		if entry.IsDir() || !entry.Type().IsRegular() || IsImportArtifact(entry.Name(), stateName) {
			return nil
		}
		listed++
		report(false)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(walkRoot, path)
		if err != nil {
			return err
		}
		c := &candidate{path: filepath.Join(root, rel), info: info}
		index.bySize[info.Size()] = append(index.bySize[info.Size()], c)
		index.byPath[c.path] = c
		return nil
	})
	report(true)
	return index, err
}

func (c *candidate) check(ctx context.Context, index *contentIndex, plan Plan, base Event, events chan<- Event) (verifiedcopy.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := plan.Controller.wait(ctx); err != nil {
		return verifiedcopy.Result{}, err
	}
	info, err := verifiedcopy.FileInfo(index.root, c.path)
	if err != nil {
		return verifiedcopy.Result{}, err
	}
	if !verifiedcopy.SameVersion(info, c.info) {
		return verifiedcopy.Result{}, errors.New("destination candidate changed since inventory")
	}
	if c.done {
		return c.checked, nil
	}
	base.Destination, base.Total = c.path, info.Size()
	var limit throttle
	limit.allow(PhaseCheckingDestination, 0, info.Size())
	emit(ctx, events, withProgress(base, PhaseCheckingDestination, 0))
	checked, err := verifiedcopy.Check(ctx, index.root, c.path, plan.Controller.wait, func(done int64) {
		if limit.allow(PhaseCheckingDestination, done, info.Size()) {
			emit(ctx, events, withProgress(base, PhaseCheckingDestination, done))
		}
	})
	if err != nil {
		// Only a digest is shared. A failed read may be transient, so the
		// next source of this size reads the candidate again.
		return verifiedcopy.Result{}, err
	}
	c.checked, c.done = checked, true
	return c.checked, nil
}

func (index *contentIndex) match(ctx context.Context, job Job, plan Plan, base Event, events chan<- Event) (string, verifiedcopy.Result, string, error) {
	info, err := verifiedcopy.FileInfo(plan.SourceRoot, job.Source)
	if err != nil {
		return "", verifiedcopy.Result{}, "", err
	}
	candidates := append([]*candidate(nil), index.bySize[info.Size()]...)
	planned := job.Destination
	if base.Index < len(plan.planned) {
		planned = plan.planned[base.Index]
	}
	planned = pathKey(planned)
	hints := make(map[string]string)
	warning := ""
	for _, file := range index.hints[planned] {
		path := pathKey(file.Destination)
		hints[path] = file.DestinationHash
		if c := index.byPath[path]; c != nil && c.info.Size() != file.Size {
			warning = "previously recorded destination size differs from its import state"
		}
	}
	// An empty file has no content to recognise: every empty file matches it,
	// a placeholder as much as a photo that failed to write. Only its own name,
	// or where this state recorded it, can hold it, so a Move never deletes it
	// on the evidence of an unrelated empty file.
	if info.Size() == 0 {
		own := candidates[:0]
		for _, c := range candidates {
			if _, recorded := hints[c.path]; recorded || c.path == planned {
				own = append(own, c)
			}
		}
		candidates = own
	}
	if len(candidates) == 0 {
		return "", verifiedcopy.Result{}, warning, nil
	}
	rank := func(path string) int {
		if _, ok := hints[path]; ok {
			return 0
		}
		if path == planned {
			return 1
		}
		if strings.EqualFold(filepath.Base(path), filepath.Base(job.Source)) {
			return 2
		}
		return 3
	}
	sort.SliceStable(candidates, func(i, j int) bool { return rank(candidates[i].path) < rank(candidates[j].path) })
	base.Total = info.Size()
	var limit throttle
	limit.allow(PhaseCheckingSource, 0, info.Size())
	emit(ctx, events, withProgress(base, PhaseCheckingSource, 0))
	source, err := verifiedcopy.Check(ctx, plan.SourceRoot, job.Source, plan.Controller.wait, func(done int64) {
		if limit.allow(PhaseCheckingSource, done, info.Size()) {
			emit(ctx, events, withProgress(base, PhaseCheckingSource, done))
		}
	})
	if err != nil {
		return "", verifiedcopy.Result{}, "", err
	}
	var firstErr error
	base.SourceHash = source.Digest
	for _, c := range candidates {
		if err := plan.Controller.wait(ctx); err != nil {
			return "", source, warning, err
		}
		if os.SameFile(source.Source, c.info) {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s aliases source; it is not an independent copy", c.path)
			}
			continue
		}
		checked, err := c.check(ctx, index, plan, base, events)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", source, warning, ctxErr
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", c.path, err)
			}
			continue
		}
		if expected := hints[c.path]; expected != "" && checked.Digest != expected {
			warning = "previously recorded destination no longer matches its recorded SHA-256"
		}
		if checked.Digest == source.Digest {
			current, err := verifiedcopy.FileInfo(plan.SourceRoot, job.Source)
			if err != nil || !verifiedcopy.SameVersion(current, source.Source) {
				return "", source, warning, errors.New("source changed during comparison")
			}
			return c.path, source, warning, nil
		}
	}
	if firstErr != nil {
		return "", source, warning, fmt.Errorf("existing copy undetermined: %w", firstErr)
	}
	return "", source, warning, nil
}

func deleteMatchedSource(ctx context.Context, sourcePath, destination string, plan Plan, source verifiedcopy.Result) error {
	if err := verifiedcopy.CheckRoot(plan.DestinationRoot, plan.expectedRoot); err != nil {
		return fmt.Errorf("%w; source kept", err)
	}
	verified, err := verifiedcopy.Check(ctx, plan.DestinationRoot, destination, plan.Controller.wait, nil)
	if err != nil {
		return fmt.Errorf("existing destination revalidation: %w", err)
	}
	if verified.Size != source.Size || verified.Digest != source.Digest {
		return errors.New("existing destination changed; source kept")
	}
	// The source itself, reached through a hard link, is no second copy.
	if os.SameFile(verified.Source, source.Source) {
		return errors.New("existing destination is the source itself; source kept")
	}
	if err := plan.Controller.wait(ctx); err != nil {
		return err
	}
	return deletePinnedSource(plan, sourcePath, source.Source)
}
