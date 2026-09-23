// Package importer implements integrity-verified photo transfers independently
// from the Bubble Tea UI.
package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

const StateVersion = 1

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
	PhasePending        Phase = "pending"
	PhaseCopying        Phase = "copying"
	PhaseVerifying      Phase = "verifying"
	PhasePublishing     Phase = "publishing"
	PhaseDeletingSource Phase = "deleting-source"
	PhaseComplete       Phase = "complete"
	PhaseSkipped        Phase = "skipped"
	PhaseFailed         Phase = "failed"
)

type Job struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type Plan struct {
	Jobs       []Job
	Operation  Operation
	Conflict   ConflictPolicy
	Workers    int
	StatePath  string
	Controller *Controller
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
	Index           int
	Phase           Phase
	Source          string
	Destination     string
	Bytes           int64
	Total           int64
	SourceHash      string
	DestinationHash string
	Err             error
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
	plan = reserveDestinations(plan)
	workers := plan.Workers
	if workers < 1 {
		workers = 1
	}
	if workers > len(plan.Jobs) && len(plan.Jobs) > 0 {
		workers = len(plan.Jobs)
	}
	result := Result{Files: make([]FileResult, len(plan.Jobs))}
	state := State{Version: StateVersion, HashAlgorithm: "SHA-256", Operation: plan.Operation, Files: make([]FileResult, len(plan.Jobs))}
	pendingIndices := make([]int, 0, len(plan.Jobs))
	prior := loadState(plan.StatePath)
	for i, job := range plan.Jobs {
		pending := FileResult{Index: i, Source: job.Source, Destination: job.Destination, Phase: PhasePending}
		result.Files[i], state.Files[i] = pending, pending
		if resumed, ok := resumable(ctx, prior, job, plan.Operation); ok {
			resumed.Index = i
			result.Files[i], state.Files[i] = resumed, resumed
			continue
		}
		pendingIndices = append(pendingIndices, i)
	}
	if plan.StatePath != "" {
		if err := writeState(plan.StatePath, &state); err != nil {
			result.StateError = fmt.Errorf("write state: %w", err)
		}
	}

	jobs := make(chan int)
	outcomes := make(chan outcome)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				fileResult, finalEvent := transfer(ctx, index, plan, events)
				outcomes <- outcome{result: fileResult, event: finalEvent}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, index := range pendingIndices {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { group.Wait(); close(outcomes) }()

	for outcome := range outcomes {
		result.Files[outcome.result.Index] = outcome.result
		state.Files[outcome.result.Index] = outcome.result
		if plan.StatePath != "" {
			// A file that reached the destination under its final name has already
			// satisfied the integrity contract. Failing to record that is a
			// bookkeeping failure, so keep the transfer outcome intact and report
			// the state error once for the batch.
			if err := writeState(plan.StatePath, &state); err != nil && result.StateError == nil {
				result.StateError = fmt.Errorf("write state: %w", err)
			}
		}
		emit(ctx, events, outcome.event)
	}
	return result
}

func reserveDestinations(plan Plan) Plan {
	if plan.Conflict != KeepBoth {
		return plan
	}
	plan.Jobs = append([]Job(nil), plan.Jobs...)
	reserved := make(map[string]struct{}, len(plan.Jobs))
	for index := range plan.Jobs {
		destination := plan.Jobs[index].Destination
		if _, exists := reserved[destination]; exists || pathExists(destination) {
			destination = availableUnreservedName(destination, reserved)
			plan.Jobs[index].Destination = destination
		}
		reserved[destination] = struct{}{}
	}
	return plan
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func availableUnreservedName(path string, reserved map[string]struct{}) string {
	ext := filepath.Ext(path)
	name := filepath.Base(path)
	stem := name[:len(name)-len(ext)]
	for index := 2; ; index++ {
		candidate := filepath.Join(filepath.Dir(path), fmt.Sprintf("%s-%d%s", stem, index, ext))
		if _, exists := reserved[candidate]; !exists && !pathExists(candidate) {
			return candidate
		}
	}
}

func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	if state.Version != StateVersion {
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

func resumable(ctx context.Context, state State, job Job, operation Operation) (FileResult, bool) {
	for _, file := range state.Files {
		matchesDestination := file.Destination == job.Destination || file.PlannedDestination == job.Destination
		if file.Phase != PhaseComplete || file.Source != job.Source || !matchesDestination || file.DestinationHash == "" {
			continue
		}
		destinationHash, destinationSize, err := hashFile(ctx, file.Destination)
		if err != nil || destinationSize != file.Size || destinationHash != file.DestinationHash {
			return FileResult{}, false
		}
		sourceHash, sourceSize, sourceErr := hashFile(ctx, file.Source)
		if operation == Move && errors.Is(sourceErr, os.ErrNotExist) {
			return file, true
		}
		if sourceErr != nil || sourceSize != file.Size || sourceHash != file.SourceHash {
			return FileResult{}, false
		}
		return file, true
	}
	return FileResult{}, false
}

func hashFile(ctx context.Context, path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	digest := sha256.New()
	written, err := copyWithContext(ctx, digest, file, make([]byte, 1024*1024), func(int64) {})
	if err != nil {
		return "", written, err
	}
	return hex.EncodeToString(digest.Sum(nil)), written, nil
}

// transfer copies one file into the destination tree.
//
// The integrity contract — copy, read the destination back independently, and
// publish under the final name only when the two digests agree — is
// internal/verifiedcopy, which dgs box uses for the same guarantee. What is
// left here is what belongs to Photo Import and to nothing else: the conflict
// policy, Move, the pause controller and the progress events the TUI draws.
func transfer(ctx context.Context, index int, plan Plan, events chan<- Event) (FileResult, Event) {
	job := plan.Jobs[index]
	base := Event{Index: index, Source: job.Source, Destination: job.Destination}
	result := FileResult{Index: index, Source: job.Source, Destination: job.Destination}
	fail := func(err error) (FileResult, Event) {
		result.Phase, result.Error = PhaseFailed, err.Error()
		base.Phase, base.Err = PhaseFailed, err
		return result, base
	}
	if plan.Conflict == Replace {
		return fail(errors.New("replace conflict policy is not implemented safely"))
	}
	info, err := os.Lstat(job.Source)
	if err != nil {
		return fail(fmt.Errorf("source metadata: %w", err))
	}
	if !info.Mode().IsRegular() {
		return fail(fmt.Errorf("source is not a regular file: %s", job.Source))
	}
	result.Size, result.SourceModTime, base.Total = info.Size(), info.ModTime().UnixNano(), info.Size()
	if _, err := os.Lstat(job.Destination); err == nil {
		if plan.Conflict == Skip {
			result.Phase, base.Phase = PhaseSkipped, PhaseSkipped
			return result, base
		}
		if plan.Conflict == KeepBoth {
			job.Destination = availableName(job.Destination)
			result.Destination, base.Destination = job.Destination, job.Destination
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("destination metadata: %w", err))
	}

	copied, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{
		Source:      job.Source,
		Destination: job.Destination,
		// A photograph's modification time is part of what was imported, so it
		// is applied before the file is published and never after.
		PreserveModTime: true,
		Progress: func(phase verifiedcopy.Phase, done, total int64) {
			event := base
			event.Total = total
			switch phase {
			case verifiedcopy.PhaseCopying:
				emit(ctx, events, withProgress(event, PhaseCopying, done))
			case verifiedcopy.PhaseVerifying:
				emit(ctx, events, withProgress(event, PhaseVerifying, done))
			case verifiedcopy.PhasePublished:
				emit(ctx, events, withProgress(event, PhasePublishing, done))
			}
		},
		// Pausing is the same wait the copy already had, now asked for between
		// blocks of both passes rather than written into each loop.
		Wait: func(ctx context.Context) error { return plan.Controller.wait(ctx) },
	})
	if err != nil {
		return fail(err)
	}
	// The two passes agreed, or Copy would not have returned: that is what the
	// one digest means, and both fields carry it so a state file written before
	// this change still reads the same way.
	result.SourceHash, result.DestinationHash = copied.Digest, copied.Digest
	base.SourceHash, base.DestinationHash = copied.Digest, copied.Digest

	if plan.Operation == Move {
		emit(ctx, events, withProgress(base, PhaseDeletingSource, info.Size()))
		if err := os.Remove(job.Source); err != nil {
			return fail(fmt.Errorf("delete source: %w", err))
		}
	}
	result.Phase, base.Phase, base.Bytes = PhaseComplete, PhaseComplete, info.Size()
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

// copyWithContext reads a whole file through dst, stopping when the context
// is cancelled. It is what the resume check hashes with; the transfer itself
// hashes inside internal/verifiedcopy.
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, buffer []byte, progress func(int64)) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := src.Read(buffer)
		if read > 0 {
			written, err := dst.Write(buffer[:read])
			total += int64(written)
			if err != nil {
				return total, err
			}
			if written != read {
				return total, io.ErrShortWrite
			}
			progress(total)
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

func availableName(path string) string {
	ext, name := filepath.Ext(path), filepath.Base(path)
	stem := name[:len(name)-len(ext)]
	for index := 2; ; index++ {
		candidate := filepath.Join(filepath.Dir(path), fmt.Sprintf("%s-%d%s", stem, index, ext))
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func writeState(path string, state *State) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return verifiedcopy.SyncDirectory(filepath.Dir(path))
}
