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
	"syscall"
	"time"
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

func transfer(ctx context.Context, index int, plan Plan, events chan<- Event) (FileResult, Event) {
	job := plan.Jobs[index]
	base := Event{Index: index, Source: job.Source, Destination: job.Destination}
	result := FileResult{Index: index, Source: job.Source, Destination: job.Destination}
	// partial names the exclusively created .dgs-part file once it belongs to
	// this transfer, so every failure path discards it instead of leaving an
	// unverifiable remnant in the destination directory.
	var partial string
	fail := func(err error) (FileResult, Event) {
		if partial != "" {
			_ = os.Remove(partial)
		}
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
	if err := os.MkdirAll(filepath.Dir(job.Destination), 0o755); err != nil {
		return fail(fmt.Errorf("create destination directory: %w", err))
	}
	temporary := filepath.Join(filepath.Dir(job.Destination), "."+filepath.Base(job.Destination)+".dgs-part")
	if temporaryInfo, temporaryErr := os.Lstat(temporary); temporaryErr == nil {
		if !temporaryInfo.Mode().IsRegular() {
			return fail(fmt.Errorf("temporary path is not a regular file: %s", temporary))
		}
		if err := os.Remove(temporary); err != nil {
			return fail(fmt.Errorf("remove stale temporary file: %w", err))
		}
	} else if !errors.Is(temporaryErr, os.ErrNotExist) {
		return fail(fmt.Errorf("temporary metadata: %w", temporaryErr))
	}
	source, err := os.Open(job.Source)
	if err != nil {
		return fail(fmt.Errorf("open source: %w", err))
	}
	defer source.Close()
	destination, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fail(fmt.Errorf("open destination temporary file: %w", err))
	}
	partial = temporary
	sourceDigest := sha256.New()
	buffer := make([]byte, 1024*1024)
	var copied int64
	emit(ctx, events, withProgress(base, PhaseCopying, 0))
	for {
		if err := plan.Controller.wait(ctx); err != nil {
			destination.Close()
			return fail(err)
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			chunk := buffer[:read]
			if _, err := destination.Write(chunk); err != nil {
				destination.Close()
				return fail(fmt.Errorf("write destination: %w", err))
			}
			_, _ = sourceDigest.Write(chunk)
			copied += int64(read)
			emit(ctx, events, withProgress(base, PhaseCopying, copied))
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			destination.Close()
			return fail(fmt.Errorf("read source: %w", readErr))
		}
	}
	if err := destination.Sync(); err != nil {
		destination.Close()
		return fail(fmt.Errorf("sync destination: %w", err))
	}
	if err := destination.Close(); err != nil {
		return fail(fmt.Errorf("close destination: %w", err))
	}
	after, err := os.Lstat(job.Source)
	if err != nil {
		return fail(fmt.Errorf("source metadata after copy: %w", err))
	}
	if !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return fail(errors.New("source changed during copy"))
	}
	result.SourceHash = hex.EncodeToString(sourceDigest.Sum(nil))
	base.SourceHash = result.SourceHash
	emit(ctx, events, withProgress(base, PhaseVerifying, 0))
	verify, err := os.Open(temporary)
	if err != nil {
		return fail(fmt.Errorf("open destination for verification: %w", err))
	}
	destinationDigest := sha256.New()
	verified, err := copyWithControl(ctx, plan.Controller, destinationDigest, verify, buffer, func(bytes int64) {
		event := withProgress(base, PhaseVerifying, bytes)
		event.SourceHash = result.SourceHash
		emit(ctx, events, event)
	})
	closeErr := verify.Close()
	if err != nil {
		return fail(fmt.Errorf("read destination for verification: %w", err))
	}
	if closeErr != nil {
		return fail(fmt.Errorf("close destination verification: %w", closeErr))
	}
	result.DestinationHash = hex.EncodeToString(destinationDigest.Sum(nil))
	if verified != info.Size() || result.DestinationHash != result.SourceHash {
		return fail(errors.New("source and destination SHA-256 mismatch"))
	}
	// Apply source timestamps while the file still has its unpublished temporary
	// name, so the final name never denotes a file with incomplete metadata.
	if err := os.Chtimes(temporary, info.ModTime(), info.ModTime()); err != nil {
		return fail(fmt.Errorf("preserve source modification time: %w", err))
	}
	// Some network filesystems (notably SMB/NAS mounts) reject fsync on a
	// read-only descriptor even when this client created and wrote the file.
	// Reopen the verified temporary file read-write so the metadata sync uses a
	// writable handle; do not weaken the contract by ignoring permission errors.
	metadataFile, err := os.OpenFile(temporary, os.O_RDWR, 0)
	if err != nil {
		return fail(fmt.Errorf("open destination to sync metadata: %w", err))
	}
	if err := metadataFile.Sync(); err != nil {
		metadataFile.Close()
		return fail(fmt.Errorf("sync destination metadata: %w", err))
	}
	if err := metadataFile.Close(); err != nil {
		return fail(fmt.Errorf("close destination metadata: %w", err))
	}
	base.SourceHash, base.DestinationHash = result.SourceHash, result.DestinationHash
	emit(ctx, events, withProgress(base, PhasePublishing, info.Size()))
	if _, err := os.Lstat(job.Destination); err == nil {
		return fail(errors.New("destination appeared before publish"))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("destination metadata before publish: %w", err))
	}
	if err := os.Rename(temporary, job.Destination); err != nil {
		return fail(fmt.Errorf("publish destination: %w", err))
	}
	partial = ""
	if err := syncDirectory(filepath.Dir(job.Destination)); err != nil {
		return fail(fmt.Errorf("sync destination directory: %w", err))
	}
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

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, buffer []byte, progress func(int64)) (int64, error) {
	return copyWithControl(ctx, nil, dst, src, buffer, progress)
}

func copyWithControl(ctx context.Context, controller *Controller, dst io.Writer, src io.Reader, buffer []byte, progress func(int64)) (int64, error) {
	var total int64
	for {
		if err := controller.wait(ctx); err != nil {
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
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
		return err
	}
	return nil
}
