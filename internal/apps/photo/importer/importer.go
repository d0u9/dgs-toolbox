// Package importer implements integrity-verified photo transfers independently
// from the Bubble Tea UI.
package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

const StateVersion = 2

// DefaultStateInterval is how often, at most, the state file is rewritten
// while a batch runs. It is always written when the batch starts and ends.
// Rewriting the whole file after every photo made bookkeeping grow with the
// square of the batch and stalled workers on a NAS.
const DefaultStateInterval = 2 * time.Second

// ProgressInterval is the least time between two progress events of one
// transfer step. Starts, phase changes and completions are always sent.
const ProgressInterval = 100 * time.Millisecond

// MaxWorkers bounds the transfer pool. More parallel readers rarely help a
// card reader or a NAS, and each worker holds its own buffers and handles.
const MaxWorkers = 8

type Operation string

const (
	Copy Operation = "copy"
	Move Operation = "move"
)

type ConflictPolicy string

const (
	Skip     ConflictPolicy = "skip"
	KeepBoth ConflictPolicy = "keep-both"
	Replace  ConflictPolicy = "replace"
)

type Phase string

const (
	PhasePending Phase = "pending"
	// PhaseInventory events carry Index -1 and, in Bytes, the number of
	// destination files listed so far by the incremental inventory.
	PhaseInventory           Phase = "inventory"
	PhaseCheckingSource      Phase = "checking-source"
	PhaseCheckingDestination Phase = "checking-destination"
	PhaseAlreadyPresent      Phase = "already-present"
	PhaseUndetermined        Phase = "undetermined"
	PhaseCopying             Phase = "copying"
	PhaseVerifying           Phase = "verifying"
	PhasePublishing          Phase = "publishing"
	PhaseDeletingSource      Phase = "deleting-source"
	PhaseComplete            Phase = "complete"
	// PhaseSourceKept is a Move whose destination was verified and published
	// but whose source was not deleted: the source is read-only, or changed
	// after it was copied. The photo is imported; only the move is unfinished,
	// and a later run with the same state deletes the source once both sides
	// still match the recorded digest.
	PhaseSourceKept Phase = "source-kept"
	PhaseSkipped    Phase = "skipped"
	PhaseFailed     Phase = "failed"
)

type Job struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// ConflictNotImported is the Warning of an incremental Skip: the name is
// taken by different content, so this photo was not imported.
const ConflictNotImported = "same name, different content: not imported"

type Plan struct {
	Jobs      []Job
	Operation Operation
	Conflict  ConflictPolicy
	Workers   int
	StatePath string
	// Incremental compares source content with the destination inventory taken
	// before this run publishes anything. SourceRoot is required in this mode.
	Incremental bool
	SourceRoot  string
	// DestinationRoot, when set, is the tree every Destination lies under.
	// Directories below it are never followed through a symbolic link.
	DestinationRoot string
	// expectedRoot pins roots selected by multi-target preflight. A vanished
	// or replaced mount must not be recreated by transfer or state writes.
	expectedRoot os.FileInfo
	worker       int
	Controller   *Controller
	// StateInterval overrides DefaultStateInterval when positive.
	StateInterval time.Duration
	// planned holds each job's Destination before Keep both renamed it, so
	// the state file can still recognise the job on a later run.
	planned []string
	index   *contentIndex
	// resume holds, per job index, a state record whose destination still has
	// the recorded size. A worker confirms it by digest before trusting it.
	resume map[int]FileResult
	// names holds every destination name Keep both has handed out, shared by
	// the workers so two of them never choose the same new name.
	names *reservations
	// claimErrors holds jobs for which Keep both could not choose a name.
	claimErrors map[int]error
	// saveState writes the state file; tests count its calls.
	saveState func(string, *State) error
	// copyMany defaults to verifiedcopy.CopyMany; tests can delay one target.
	copyMany func(context.Context, verifiedcopy.ManyRequest) (verifiedcopy.Result, []error)
}

// reservations hands out Keep both names that no other job holds and that
// do not exist on disk.
type reservations struct {
	mu    sync.Mutex
	names map[string]struct{}
}

func newReservations() *reservations {
	return &reservations{names: make(map[string]struct{})}
}

// MaxKeepBothNames bounds the "<stem>-N<ext>" names Keep both tries for one
// file, so a directory that answers every name as taken ends in an error.
const MaxKeepBothNames = 10000

// claim reserves path itself when it is free, or else the first free
// "<stem>-N<ext>" beside it. A name whose metadata cannot be read — a path
// component that is a file, a directory without permission, a name too long —
// is neither free nor taken, so it ends the search with an error instead of
// counting as taken: otherwise every candidate would, and claim never returns.
func (r *reservations) claim(path string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	free := func(candidate string) (bool, error) {
		if _, taken := r.names[candidate]; taken {
			return false, nil
		}
		_, err := os.Lstat(candidate)
		if err == nil {
			return false, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("keep both name: %w", err)
	}
	ext := filepath.Ext(path)
	stem := filepath.Base(path)[:len(filepath.Base(path))-len(ext)]
	for index := 1; index <= MaxKeepBothNames; index++ {
		candidate := path
		if index > 1 {
			candidate = filepath.Join(filepath.Dir(path), fmt.Sprintf("%s-%d%s", stem, index, ext))
		}
		ok, err := free(candidate)
		if err != nil {
			return "", err
		}
		if ok {
			r.names[candidate] = struct{}{}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("keep both: no free name for %s after %d tries", path, MaxKeepBothNames)
}

// throttle limits progress events of one transfer step to ProgressInterval.
type throttle struct {
	phase Phase
	last  time.Time
}

func (t *throttle) allow(phase Phase, done, total int64) bool {
	now := time.Now()
	if phase != t.phase || done == 0 || done >= total || now.Sub(t.last) >= ProgressInterval {
		t.phase, t.last = phase, now
		return true
	}
	return false
}

// Controller pauses transfers between bounded I/O chunks. A paused transfer
// keeps its open files but performs no further reads or writes until resumed.
type Controller struct {
	mu     sync.Mutex
	resume chan struct{}
}

func (c *Controller) Pause() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resume == nil {
		c.resume = make(chan struct{})
	}
}

func (c *Controller) Resume() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resume != nil {
		close(c.resume)
		c.resume = nil
	}
}

func (c *Controller) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	c.mu.Lock()
	resume := c.resume
	c.mu.Unlock()
	if resume == nil {
		return nil
	}
	select {
	case <-resume:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

type Event struct {
	// Worker is the one-based whole-file worker; zero denotes batch events.
	Worker          int
	Index           int
	Phase           Phase
	Source          string
	Destination     string
	Bytes           int64
	Total           int64
	SourceHash      string
	DestinationHash string
	Err             error
	Existing        bool
}

type FileResult struct {
	Index              int    `json:"index"`
	Source             string `json:"source"`
	Destination        string `json:"destination"`
	PlannedDestination string `json:"planned_destination,omitempty"`
	Size               int64  `json:"size"`
	SourceModTime      int64  `json:"source_mod_time_unix_nano,omitempty"`
	SourceHash         string `json:"source_hash,omitempty"`
	DestinationHash    string `json:"destination_hash,omitempty"`
	Phase              Phase  `json:"phase"`
	Error              string `json:"error,omitempty"`
	// Existing distinguishes prior copies (including source-kept outcomes)
	// from files published by this run.
	Existing bool   `json:"existing,omitempty"`
	Warning  string `json:"warning,omitempty"`
	// verified is this run's destination version, never trusted from state.
	verified os.FileInfo
}

// Result carries the per-file transfer outcomes. StateError reports a failure
// to persist the import-state file, which is bookkeeping rather than transfer
// I/O: files that were verified and published stay complete, and only whole-file
// resume for a later retry is degraded.
type Result struct {
	Files      []FileResult
	StateError error
}

type State struct {
	Version       int          `json:"version"`
	HashAlgorithm string       `json:"hash_algorithm"`
	UpdatedAt     time.Time    `json:"updated_at"`
	Operation     Operation    `json:"operation"`
	Files         []FileResult `json:"files"`
}

type outcome struct {
	result FileResult
	event  Event
}

// Run executes a bounded pool and serializes state-file updates. Events may be
// nil. Replace is rejected until its rollback contract is explicitly settled.
func Run(ctx context.Context, plan Plan, events chan<- Event) Result {
	return runTargets(ctx, []Plan{plan}, []chan<- Event{events})[0]
}

// targetRun is one destination of a batch: its plan, state file and results.
// Only the coordinator goroutine touches state, result and dirty.
type targetRun struct {
	plan   Plan
	events chan<- Event
	state  State
	result Result
	dirty  bool
	// active is false when the target failed before any file was started.
	active bool
}

// failAll ends a target before any file started: every job gets phase and err.
func (t *targetRun) failAll(ctx context.Context, phase Phase, err error) *targetRun {
	for i, job := range t.plan.Jobs {
		t.result.Files[i] = FileResult{Index: i, Source: job.Source, Destination: job.Destination, Phase: phase, Error: err.Error()}
		emit(ctx, t.events, Event{Index: i, Source: job.Source, Destination: job.Destination, Phase: phase, Err: err})
	}
	return t
}

// setupTarget validates and pins a target, takes its inventory and resume
// records, and writes its initial state. An inactive result is final.
func setupTarget(ctx context.Context, plan Plan, events chan<- Event) *targetRun {
	t := &targetRun{plan: plan, events: events, result: Result{Files: make([]FileResult, len(plan.Jobs))}}
	if err := verifiedcopy.CheckRoot(plan.DestinationRoot, plan.expectedRoot); err != nil {
		return t.failAll(ctx, PhaseFailed, err)
	}
	t.state = State{Version: StateVersion, HashAlgorithm: "SHA-256", Operation: plan.Operation, Files: make([]FileResult, len(plan.Jobs))}
	if plan.saveState == nil {
		plan.saveState = writeState
	}
	// Overlapping roots make a Copy duplicate the card onto itself and a
	// Move rename it in place, whatever the mode.
	if plan.SourceRoot != "" && plan.DestinationRoot != "" {
		if err := ValidateRoots(plan.SourceRoot, plan.DestinationRoot); err != nil {
			return t.failAll(ctx, PhaseFailed, err)
		}
	}
	// Every caller's destination is pinned for the batch, not only the
	// command line's: a root that vanishes mid-run — a NAS unmounted under
	// the workspace — must not be recreated on the local disk. A root that
	// does not exist yet is created once, here, and pinned from then on.
	if plan.DestinationRoot != "" && plan.expectedRoot == nil {
		identity, err := pinRoot(plan.DestinationRoot)
		if err != nil {
			return t.failAll(ctx, PhaseFailed, err)
		}
		plan.expectedRoot = identity
	}
	if plan.DestinationRoot != "" {
		if err := verifiedcopy.SyncDirectory(filepath.Dir(plan.DestinationRoot)); err != nil {
			return t.failAll(ctx, PhaseFailed, fmt.Errorf("sync destination root parent: %w", err))
		}
	}
	prior := loadState(plan.StatePath)
	if plan.Incremental {
		var err error
		plan.index, err = newContentIndex(ctx, plan, prior, events)
		if err != nil {
			t.plan = plan
			return t.failAll(ctx, PhaseUndetermined, err)
		}
	}
	plan.planned = make([]string, len(plan.Jobs))
	plan.resume = make(map[int]FileResult)
	reserve := make([]int, 0, len(plan.Jobs))
	// Resume is matched against each job's own Destination, before Keep both
	// can rename it: a file this batch already published now stands at that
	// name, and renaming first would make it look like a conflict to copy past.
	// Only metadata is compared here; the digests are read by the workers,
	// where pause, cancellation and progress apply.
	for i, job := range plan.Jobs {
		plan.planned[i] = job.Destination
		pending := FileResult{Index: i, Source: job.Source, Destination: job.Destination, Phase: PhasePending}
		t.result.Files[i], t.state.Files[i] = pending, pending
		if !plan.Incremental {
			if record, ok := resumeRecord(prior, job); ok {
				record.Index = i
				plan.resume[i] = record
				// The record stays the state's evidence until a worker
				// confirms or refutes it, so an interrupted run loses nothing.
				t.state.Files[i] = record
				continue
			}
		}
		reserve = append(reserve, i)
	}
	plan = reserveDestinations(plan, reserve)
	for _, i := range reserve {
		pending := FileResult{Index: i, Source: plan.Jobs[i].Source, Destination: plan.Jobs[i].Destination, Phase: PhasePending}
		if plan.Jobs[i].Destination != plan.planned[i] {
			pending.PlannedDestination = plan.planned[i]
		}
		t.result.Files[i], t.state.Files[i] = pending, pending
	}
	// Records of earlier batches into this Destination stay, so importing
	// another card in between does not cost the first one its resume.
	t.state.Files = append(t.state.Files, carriedRecords(prior, plan.Jobs)...)
	t.plan, t.active = plan, true
	t.dirty = !t.save()
	return t
}

func (t *targetRun) save() bool {
	if t.plan.StatePath == "" {
		return true
	}
	// A file that reached the destination under its final name has already
	// satisfied the integrity contract. Failing to record that is a
	// bookkeeping failure, so keep the transfer outcome intact and report the
	// state error once for the batch.
	err := verifiedcopy.CheckRoot(t.plan.DestinationRoot, t.plan.expectedRoot)
	if err == nil {
		err = t.plan.saveState(t.plan.StatePath, &t.state)
	}
	if err != nil {
		if t.result.StateError == nil {
			t.result.StateError = fmt.Errorf("write state: %w", err)
		}
		return false
	}
	return true
}

// runTargets imports the same jobs into every plan's destination at once.
// Each file is one unit of work for the whole batch: a worker prepares it in
// every target, then copies it into all that need it from one source read.
// All plans must describe the same jobs; Move is honoured only with one plan,
// because a source can be deleted only after every target holds its copy.
func runTargets(ctx context.Context, plans []Plan, events []chan<- Event) []Result {
	runs := make([]*targetRun, len(plans))
	active := false
	for i, plan := range plans {
		runs[i] = setupTarget(ctx, plan, events[i])
		active = active || runs[i].active
	}
	results := func() []Result {
		out := make([]Result, len(runs))
		for i, run := range runs {
			out[i] = run.result
		}
		return out
	}
	jobs := len(plans[0].Jobs)
	if !active {
		return results()
	}
	workers := min(max(plans[0].Workers, 1), MaxWorkers)
	if workers > jobs && jobs > 0 {
		workers = jobs
	}
	interval := plans[0].StateInterval
	if interval <= 0 {
		interval = DefaultStateInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	queue := make(chan int)
	type targetOutcome struct {
		target int
		outcome
	}
	outcomes := make(chan targetOutcome)
	var group sync.WaitGroup
	for worker := range workers {
		// Workers get only each target's plan and events, taken before they
		// start: state, results and dirty belong to the coordinator.
		workerRuns := make([]*targetRun, len(runs))
		for i, run := range runs {
			if run.active {
				plan := run.plan
				plan.worker = worker + 1
				workerRuns[i] = &targetRun{plan: plan, events: run.events, active: true}
			}
		}
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range queue {
				transferAll(ctx, index, workerRuns, func(target int, finished outcome) {
					outcomes <- targetOutcome{target: target, outcome: finished}
				})
			}
		}()
	}
	go func() {
		defer close(queue)
		for index := range jobs {
			select {
			case queue <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { group.Wait(); close(outcomes) }()

	// One coordinator owns state and saves pending transitions on ticks, even
	// while the next file is slow or paused. Clean state is not rewritten on
	// ticks; failed writes stay dirty so a later tick can retry them.
	for outcomes != nil {
		select {
		case finished, ok := <-outcomes:
			if !ok {
				outcomes = nil
				continue
			}
			run := runs[finished.target]
			run.result.Files[finished.result.Index] = finished.result
			if _, resuming := run.plan.resume[finished.result.Index]; !resuming || ctx.Err() == nil || finished.result.Phase != PhaseFailed {
				// An interrupted resume refutes nothing: keep its earlier record.
				run.state.Files[finished.result.Index] = finished.result
				run.dirty = true
			}
			emit(ctx, run.events, finished.event)
		case <-ticker.C:
			for _, run := range runs {
				if run.active && run.dirty {
					run.dirty = !run.save()
				}
			}
		}
	}
	for _, run := range runs {
		if !run.active {
			continue
		}
		if err := ctx.Err(); err != nil {
			for i := range run.result.Files {
				if run.result.Files[i].Phase == PhasePending {
					phase := PhaseFailed
					if run.plan.Incremental {
						phase = PhaseUndetermined
					}
					run.result.Files[i].Phase, run.result.Files[i].Error = phase, err.Error()
					if _, resuming := run.plan.resume[i]; !resuming {
						run.state.Files[i] = run.result.Files[i]
					}
				}
			}
		}
		run.save()
	}
	return results()
}

// pinRoot returns the identity of root, creating it first when it does not
// exist. A root that exists but is not a directory is refused.
func pinRoot(root string) (os.FileInfo, error) {
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		if err := verifiedcopy.EnsureDirectory("", root); err != nil {
			return nil, fmt.Errorf("create destination root: %w", err)
		}
		info, err = os.Stat(root)
	}
	if err != nil {
		return nil, fmt.Errorf("destination root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("destination root is not a directory: %s", root)
	}
	return info, nil
}

// EarlierSourceKept counts the source-kept Moves of earlier batches that a
// state file carries: imported, with only the deletion unfinished. Without
// Incremental, those records are what lets a later run finish them.
func EarlierSourceKept(state State) int {
	count := 0
	for _, file := range state.Files {
		if file.Index < 0 && file.Phase == PhaseSourceKept {
			count++
		}
	}
	return count
}

// carriedRecords keeps imported records of earlier batches whose source is
// not part of this one. Their Index is -1: they belong to no job here.
func carriedRecords(prior State, jobs []Job) []FileResult {
	current := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		current[job.Source] = struct{}{}
	}
	var carried []FileResult
	for _, file := range prior.Files {
		imported := file.Phase == PhaseComplete || file.Phase == PhaseSourceKept || file.Phase == PhaseAlreadyPresent
		if _, ok := current[file.Source]; ok || !imported || file.DestinationHash == "" {
			continue
		}
		file.Index = -1
		carried = append(carried, file)
	}
	return carried
}

func reserveDestinations(plan Plan, pending []int) Plan {
	plan.names = newReservations()
	if plan.Conflict != KeepBoth {
		return plan
	}
	plan.Jobs = append([]Job(nil), plan.Jobs...)
	for _, index := range pending {
		name, err := plan.names.claim(plan.Jobs[index].Destination)
		if err != nil {
			// The job keeps its planned name and fails in its worker.
			if plan.claimErrors == nil {
				plan.claimErrors = make(map[int]error)
			}
			plan.claimErrors[index] = err
			continue
		}
		plan.Jobs[index].Destination = name
	}
	return plan
}

func LoadState(path string) (State, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return State{}, err
	}
	if !info.Mode().IsRegular() {
		return State{}, errors.New("state must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	if state.Version != 1 && state.Version != StateVersion {
		return State{}, fmt.Errorf("unsupported state version %d", state.Version)
	}
	if state.HashAlgorithm != "SHA-256" {
		return State{}, fmt.Errorf("unsupported hash algorithm %q", state.HashAlgorithm)
	}
	return state, nil
}

func loadState(path string) State {
	if path == "" {
		return State{}
	}
	state, err := LoadState(path)
	if err != nil {
		return State{}
	}
	return state
}

// UpdateDestinations records paths changed by verified post-processing using
// the same atomic state-file writer as the transfer engine.
func UpdateDestinations(path string, destinations map[string]string) error {
	if path == "" || len(destinations) == 0 {
		return nil
	}
	state, err := LoadState(path)
	if err != nil {
		return err
	}
	for index := range state.Files {
		if destination, ok := destinations[state.Files[index].Destination]; ok {
			if state.Files[index].PlannedDestination == "" {
				state.Files[index].PlannedDestination = state.Files[index].Destination
			}
			state.Files[index].Destination = destination
		}
	}
	return writeState(path, &state)
}

// resumeRecord finds the state record of a job a previous run already
// imported, provided its destination still has the recorded size. It reads no
// file contents: verifyResume does that inside a worker.
func resumeRecord(state State, job Job) (FileResult, bool) {
	for _, file := range state.Files {
		matchesDestination := file.Destination == job.Destination || file.PlannedDestination == job.Destination
		imported := file.Phase == PhaseComplete || file.Phase == PhaseSourceKept || file.Phase == PhaseAlreadyPresent
		if !imported || file.Source != job.Source || !matchesDestination || file.DestinationHash == "" {
			continue
		}
		info, err := os.Lstat(file.Destination)
		if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size {
			return FileResult{}, false
		}
		return file, true
	}
	return FileResult{}, false
}

// verifyResume confirms a resume record: its destination still hashes to the
// recorded digest. Copy needs nothing more. Move also needs the source gone,
// so a source that is still there and still matches is deleted now — whether
// the earlier run was a Copy, or a Move that could not delete it.
//
// State is bookkeeping, not evidence: a damaged, edited or foreign record must
// not authorise anything. The record's two digests must agree, its
// destination must lie under the pinned DestinationRoot without a link on the
// way, and that destination must be a different file from the source.
func verifyResume(ctx context.Context, file FileResult, plan Plan, sources *sourceChecks, base Event, events chan<- Event) (FileResult, Event, bool) {
	if file.SourceHash == "" || file.SourceHash != file.DestinationHash {
		return FileResult{}, base, false
	}
	if verifiedcopy.CheckRoot(plan.DestinationRoot, plan.expectedRoot) != nil {
		return FileResult{}, base, false
	}
	check := func(phase Phase, root, path string, sources *sourceChecks) (verifiedcopy.Result, error) {
		base.Total = file.Size
		var limit throttle
		return sources.check(ctx, root, path, plan.Controller.wait, func(done int64) {
			if limit.allow(phase, done, file.Size) {
				emit(ctx, events, withProgress(base, phase, done))
			}
		})
	}
	base.Destination = file.Destination
	destination, err := check(PhaseCheckingDestination, plan.DestinationRoot, file.Destination, nil)
	if err != nil || destination.Size != file.Size || destination.Digest != file.DestinationHash {
		return FileResult{}, base, false
	}
	file.Phase, file.Error, file.Existing = PhaseComplete, "", true
	base.Phase, base.Existing, base.Bytes = PhaseComplete, true, file.Size
	base.SourceHash, base.DestinationHash = file.SourceHash, file.DestinationHash
	file.verified = destination.Source
	before, statErr := os.Lstat(file.Source)
	if plan.Operation == Move && errors.Is(statErr, os.ErrNotExist) {
		return file, base, true
	}
	if statErr != nil || !before.Mode().IsRegular() {
		return FileResult{}, base, false
	}
	source, err := check(PhaseCheckingSource, plan.SourceRoot, file.Source, sources)
	if err != nil || source.Size != file.Size || source.Digest != file.SourceHash || os.SameFile(source.Source, destination.Source) {
		return FileResult{}, base, false
	}
	if plan.Operation == Move {
		emit(ctx, events, withProgress(base, PhaseDeletingSource, file.Size))
		if err := deletePinnedSource(plan, file.Source, source.Source); err != nil {
			file.Phase, file.Error = PhaseSourceKept, err.Error()
			base.Phase, base.Err = PhaseSourceKept, err
		}
	}
	return file, base, true
}

// deletePinnedSource deletes a Move's source only while the destination root
// is still the pinned directory. A copy published before an unmount still
// exists on that disk, but a source is never deleted on the word of a root
// that cannot be seen: it is kept and reported, and a later run finishes it.
func deletePinnedSource(plan Plan, path string, read os.FileInfo) error {
	if err := verifiedcopy.CheckRoot(plan.DestinationRoot, plan.expectedRoot); err != nil {
		return fmt.Errorf("%w; source kept", err)
	}
	return deleteSource(plan.SourceRoot, path, read)
}

// deleteSource removes a Move's source only while it is still the file that was
// read: same file, size and modification time. The digest describes those
// bytes; a source that changed since holds something that was never imported.
func deleteSource(root, path string, read os.FileInfo) error {
	current, err := verifiedcopy.FileInfo(root, path)
	if err != nil {
		return fmt.Errorf("source metadata before delete: %w", err)
	}
	if !os.SameFile(current, read) || current.Size() != read.Size() || !current.ModTime().Equal(read.ModTime()) {
		return errors.New("source changed after it was copied; kept")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete source: %w", err)
	}
	return nil
}

// sourceChecks holds the digest of one file's source once a target has read
// it, so the other targets of the batch compare against it rather than read
// the card again. It belongs to one worker and one file.
type sourceChecks struct {
	root   string
	done   bool
	result verifiedcopy.Result
}

// check returns the digest of path under root, reading it only the first time.
// Reuse requires unchanged file metadata. A failed read is not kept: the
// next target tries again. Progress is reported only for an actual read,
// starting from zero.
func (s *sourceChecks) check(ctx context.Context, root, path string, wait func(context.Context) error, progress func(int64)) (verifiedcopy.Result, error) {
	if s != nil && s.done && s.root == root {
		current, err := verifiedcopy.FileInfo(root, path)
		if err != nil {
			return verifiedcopy.Result{}, err
		}
		if !verifiedcopy.SameVersion(current, s.result.Source) {
			return verifiedcopy.Result{}, errors.New("source changed since comparison")
		}
		return s.result, nil
	}
	if progress != nil {
		progress(0)
	}
	result, err := verifiedcopy.Check(ctx, root, path, wait, progress)
	if err == nil && s != nil {
		s.root, s.done, s.result = root, true, result
	}
	return result, err
}

// prepared is a target's file after every check that needs no copy. done
// means result and event are final; otherwise the file still needs copying to
// destination.
type prepared struct {
	result      FileResult
	event       Event
	done        bool
	destination string
	info        os.FileInfo
	expected    verifiedcopy.Result
}

// transferAll imports one file into every active target and hands each
// target's outcome to completed as soon as it is final. Calls of completed
// never overlap.
//
// The integrity contract — copy, read the destination back independently, and
// publish under the final name only when the two digests agree — is
// internal/verifiedcopy. What is
// left here is what belongs to Photo Import and to nothing else: the conflict
// policy, Move, the pause controller and the progress events the TUI draws.
func transferAll(ctx context.Context, index int, runs []*targetRun, completed func(int, outcome)) {
	record := func(target int, result FileResult, event Event) {
		completed(target, outcome{result: result, event: event})
	}
	steps := make([]prepared, len(runs))
	sources := &sourceChecks{}
	var copying []int
	for i, run := range runs {
		if run == nil {
			continue
		}
		steps[i] = prepare(ctx, index, run.plan, sources, run.events)
		if steps[i].done {
			record(i, steps[i].result, steps[i].event)
			continue
		}
		copying = append(copying, i)
	}
	if len(copying) == 0 {
		return
	}
	// One read serves every target. It is bound to the strictest observation
	// a target made of the source — its digest when one was read — taking the
	// identity and the digest from the same observation.
	request := verifiedcopy.ManyRequest{
		Source: runs[copying[0]].plan.Jobs[index].Source,
		// A photograph's modification time is part of what was imported, so it
		// is applied before the file is published and never after.
		PreserveModTime: true,
		// A photo library is read by more than its owner: a NAS media server,
		// another account. The 0600 the unpublished copy has would hide it.
		Mode: verifiedcopy.DefaultPhotoMode,
		// Pausing is the same wait the copy already had, now asked for between
		// blocks of both passes rather than written into each loop. A copy
		// shared by several targets waits for every one of their controllers.
		Wait: func(ctx context.Context) error {
			for _, i := range copying {
				if err := runs[i].plan.Controller.wait(ctx); err != nil {
					return err
				}
			}
			return nil
		},
	}
	limits := make([]throttle, len(copying))
	for _, i := range copying {
		step := steps[i]
		if step.expected.Digest != "" && request.ExpectedDigest == "" {
			request.ExpectedSource, request.ExpectedDigest = step.expected.Source, step.expected.Digest
		} else if step.expected.Source != nil && request.ExpectedSource == nil {
			request.ExpectedSource = step.expected.Source
		}
		request.Targets = append(request.Targets, verifiedcopy.Target{
			Destination:  step.destination,
			Root:         runs[i].plan.DestinationRoot,
			ExpectedRoot: runs[i].plan.expectedRoot,
		})
	}
	request.Progress = func(n int, phase verifiedcopy.Phase, done, total int64) {
		// Complete owns the terminal event. Published progress may arrive later
		// when another target resumes, and must not reopen a completed UI row.
		if phase == verifiedcopy.PhasePublished {
			return
		}
		if !limits[n].allow(Phase(phase), done, total) {
			return
		}
		i := copying[n]
		event := steps[i].event
		event.Total = total
		switch phase {
		case verifiedcopy.PhaseCopying:
			emit(ctx, runs[i].events, withProgress(event, PhaseCopying, done))
		case verifiedcopy.PhaseVerifying:
			emit(ctx, runs[i].events, withProgress(event, PhaseVerifying, done))
		case verifiedcopy.PhasePublishing:
			emit(ctx, runs[i].events, withProgress(event, PhasePublishing, done))
		}
	}
	copyMany := runs[copying[0]].plan.copyMany
	if copyMany == nil {
		copyMany = verifiedcopy.CopyMany
	}
	request.Complete = func(n int, copied verifiedcopy.Result, err error) {
		i := copying[n]
		result, event := finishTransfer(ctx, runs[i].plan, steps[i], copied, err, runs[i].events)
		record(i, result, event)
	}
	copyMany(ctx, request)
}

// prepare runs every per-target step of a file before its copy: conflict
// policy, resume, the incremental comparison and Keep both naming.
func prepare(ctx context.Context, index int, plan Plan, sources *sourceChecks, events chan<- Event) prepared {
	job := plan.Jobs[index]
	base := Event{Worker: plan.worker, Index: index, Source: job.Source, Destination: job.Destination}
	result := FileResult{Index: index, Source: job.Source, Destination: job.Destination}
	if index < len(plan.planned) && plan.planned[index] != job.Destination {
		result.PlannedDestination = plan.planned[index]
	}
	final := func(result FileResult, event Event) prepared {
		return prepared{result: result, event: event, done: true}
	}
	fail := func(err error) prepared {
		result.Phase, result.Error = PhaseFailed, err.Error()
		base.Phase, base.Err = PhaseFailed, err
		return final(result, base)
	}
	if err := verifiedcopy.CheckRoot(plan.DestinationRoot, plan.expectedRoot); err != nil {
		return fail(err)
	}
	if plan.Conflict == Replace {
		return fail(errors.New("replace conflict policy is not implemented safely"))
	}
	if err := plan.claimErrors[index]; err != nil {
		return fail(err)
	}
	if record, ok := plan.resume[index]; ok {
		if resumed, event, ok := verifyResume(ctx, record, plan, sources, base, events); ok {
			resumed.Index = index
			return final(resumed, event)
		}
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		// The record no longer describes what is there: import as new.
	}
	info, err := verifiedcopy.FileInfo(plan.SourceRoot, job.Source)
	if err != nil {
		return fail(fmt.Errorf("source metadata: %w", err))
	}
	if !info.Mode().IsRegular() {
		return fail(fmt.Errorf("source is not a regular file: %s", job.Source))
	}
	result.Size, result.SourceModTime, base.Total = info.Size(), info.ModTime().UnixNano(), info.Size()
	var expected verifiedcopy.Result
	if plan.Incremental {
		match, checked, warning, checkErr := plan.index.match(ctx, job, plan, sources, base, events)
		expected, result.Warning = checked, warning
		if checkErr != nil {
			step := fail(checkErr)
			step.result.Phase, step.event.Phase = PhaseUndetermined, PhaseUndetermined
			return step
		}
		if expected.Source != nil && !verifiedcopy.SameVersion(info, expected.Source) {
			return fail(errors.New("source changed before comparison"))
		}
		if expected.Source == nil {
			expected.Source = info
		}
		if match != "" {
			result.Destination, base.Destination = match, match
			result.PlannedDestination = plan.planned[index]
			result.SourceHash, result.DestinationHash = checked.Digest, checked.Digest
			result.verified = plan.index.byPath[match].info
			result.Existing, base.Existing = true, true
			result.Phase, base.Phase = PhaseAlreadyPresent, PhaseAlreadyPresent
			base.SourceHash, base.DestinationHash = checked.Digest, checked.Digest
			base.Bytes = checked.Size
			if plan.Operation == Move {
				emit(ctx, events, withProgress(base, PhaseDeletingSource, checked.Size))
				if err := deleteMatchedSource(ctx, job.Source, match, plan, checked); err != nil {
					result.Phase, result.Error = PhaseSourceKept, err.Error()
					base.Phase, base.Err = PhaseSourceKept, err
				}
			}
			return final(result, base)
		}
	}
	if _, err := os.Lstat(job.Destination); err == nil {
		if plan.Conflict == Skip {
			result.Phase, base.Phase = PhaseSkipped, PhaseSkipped
			if plan.Incremental {
				// No copy of this content exists anywhere under Destination,
				// so the name belongs to a different photo: this one is not
				// imported, which must not read as an ordinary skip.
				result.Warning = ConflictNotImported
			}
			return final(result, base)
		}
		if plan.Conflict == KeepBoth {
			if result.PlannedDestination == "" {
				result.PlannedDestination = job.Destination
			}
			names := plan.names
			if names == nil {
				names = newReservations()
			}
			name, err := names.claim(job.Destination)
			if err != nil {
				return fail(err)
			}
			job.Destination = name
			result.Destination, base.Destination = job.Destination, job.Destination
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("destination metadata: %w", err))
	}
	return prepared{result: result, event: base, destination: job.Destination, info: info, expected: expected}
}

// finishTransfer records a target's copy and, for Move, deletes the source.
func finishTransfer(ctx context.Context, plan Plan, step prepared, copied verifiedcopy.Result, err error, events chan<- Event) (FileResult, Event) {
	result, base := step.result, step.event
	if err != nil {
		result.Phase, result.Error = PhaseFailed, err.Error()
		base.Phase, base.Err = PhaseFailed, err
		return result, base
	}
	info, statErr := verifiedcopy.FileInfo(plan.DestinationRoot, result.Destination)
	if statErr != nil || info.Size() != copied.Size {
		if statErr == nil {
			statErr = errors.New("published destination size changed")
		}
		result.Phase, result.Error = PhaseFailed, statErr.Error()
		base.Phase, base.Err = PhaseFailed, statErr
		return result, base
	}
	result.verified = info
	// The two passes agreed, or Copy would not have returned: that is what the
	// one digest means, and both fields carry it so a state file written before
	// this change still reads the same way.
	result.SourceHash, result.DestinationHash = copied.Digest, copied.Digest
	result.Size, result.SourceModTime = copied.Size, copied.Source.ModTime().UnixNano()
	base.SourceHash, base.DestinationHash = copied.Digest, copied.Digest

	result.Phase, base.Phase, base.Bytes = PhaseComplete, PhaseComplete, step.info.Size()
	if plan.Operation == Move {
		emit(ctx, events, withProgress(base, PhaseDeletingSource, step.info.Size()))
		// The photo is imported whatever happens here. A source that cannot be
		// deleted, or changed after it was read, is kept and reported, not
		// turned into a failed import.
		if err := deletePinnedSource(plan, step.result.Source, copied.Source); err != nil {
			result.Phase, result.Error = PhaseSourceKept, err.Error()
			base.Phase, base.Err = PhaseSourceKept, err
		}
	}
	return result, base
}

func withProgress(event Event, phase Phase, bytes int64) Event {
	event.Phase, event.Bytes = phase, bytes
	return event
}
func emit(ctx context.Context, events chan<- Event, event Event) {
	if events == nil {
		return
	}
	select {
	case events <- event:
	case <-ctx.Done():
	}
}

func writeState(path string, state *State) error {
	before, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if _, err := LoadState(path); err != nil {
			return fmt.Errorf("preserving unrecognized state file: %w", err)
		}
	}
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// A temporary name of its own, so two imports into one Destination never
	// interleave their writes into one file before either is renamed.
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		if before == nil {
			err = verifiedcopy.Publish(temporary, path)
		} else if current, statErr := os.Lstat(path); statErr != nil {
			err = statErr
		} else if !verifiedcopy.SameVersion(before, current) {
			err = errors.New("state file changed before replacement")
		} else {
			err = os.Rename(temporary, path)
		}
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return verifiedcopy.SyncDirectory(filepath.Dir(path))
}
