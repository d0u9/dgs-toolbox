package importer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"dgs-toolbox/internal/verifiedcopy"
)

// MultipleResult records each target independently and each source's overall
// outcome. Successful target copies are kept when another target fails.
// Started counts the targets that ran: all of them, or none when the context
// was already cancelled, and then every file is failed as not started.
type MultipleResult struct {
	Targets []Result
	Files   []FileResult
	Started int
}

// MultipleHooks observe RunMultiple. Each may be nil and must return promptly;
// they are called from one goroutine at a time, never concurrently.
type MultipleHooks struct {
	// Event receives every engine event of every target. Targets run at once,
	// so events of different targets interleave; each target's stay in order.
	Event func(target int, event Event)
	// Target receives each target's result once the batch has run.
	Target func(target int, result Result)
	// File receives each source's overall outcome as it is decided.
	File func(file FileResult)
}

// ValidateTargets refuses overlap with Source or between targets before any
// write. Every target must already exist: a missing one may be the empty
// mount point of an unmounted disk, and only existing directories can be
// compared by identity, which catches aliases spelled with another case or
// Unicode normalization.
func ValidateTargets(source string, targets []string) error {
	_, err := validateTargets(source, targets)
	return err
}

// validateTargets also returns the identity each target had when checked, so
// the batch pins exactly the directories that passed.
func validateTargets(source string, targets []string) ([]os.FileInfo, error) {
	if len(targets) == 0 {
		return nil, errors.New("at least one destination is required")
	}
	var roots []string
	var infos []os.FileInfo
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("destination %s does not exist; create it first (an unmounted disk leaves an empty mount point)", target)
			}
			return nil, fmt.Errorf("destination %s: %w", target, err)
		}
		if err := ValidateRoots(source, target); err != nil {
			return nil, fmt.Errorf("destination %s: %w", target, err)
		}
		root, err := canonicalPath(target)
		if err != nil {
			return nil, err
		}
		for i, other := range roots {
			if os.SameFile(infos[i], info) || inside(root, other) || inside(other, root) || within(infos[i], root) || within(info, other) {
				return nil, fmt.Errorf("destination roots overlap or alias: %s and %s", targets[i], target)
			}
		}
		roots, infos = append(roots, root), append(infos, info)
	}
	return infos, nil
}

// SharedFilesystems lists pairs of targets on one filesystem: the same device,
// or, where the platform reports it, the same mounted source (one network
// share mounted twice). Such copies are not independent of each other.
func SharedFilesystems(targets []string) [][2]int {
	type identity struct {
		device      uint64
		hasDevice   bool
		mountedFrom string
	}
	identities := make([]identity, len(targets))
	for i, target := range targets {
		if info, err := os.Stat(target); err == nil {
			identities[i].device, identities[i].hasDevice = deviceOf(info)
		}
		identities[i].mountedFrom = mountSource(target)
	}
	var pairs [][2]int
	for i := range identities {
		for j := i + 1; j < len(identities); j++ {
			a, b := identities[i], identities[j]
			if (a.hasDevice && b.hasDevice && a.device == b.device) || (a.mountedFrom != "" && a.mountedFrom == b.mountedFrom) {
				pairs = append(pairs, [2]int{i, j})
			}
		}
	}
	return pairs
}

// RunMultiple imports into every target at once: one bounded pool over the
// files, and each file read once from Source for all the targets that need a
// copy. All plans must describe the same sources in the same order. The first
// plan's Workers and StateInterval apply to the batch; each plan's Controller
// pauses its own target, and a shared copy waits for all of them. Transfers always Copy;
// Move deletes a source only after fresh checks of every target copy agree
// and the copies are distinct files.
func RunMultiple(ctx context.Context, plans []Plan, operation Operation, hooks MultipleHooks) (MultipleResult, error) {
	var result MultipleResult
	if operation != Copy && operation != Move {
		return result, errors.New("operation must be copy or move")
	}
	if len(plans) == 0 {
		return result, errors.New("at least one destination is required")
	}
	roots := make([]string, len(plans))
	for i, plan := range plans {
		roots[i] = plan.DestinationRoot
		if plan.SourceRoot != plans[0].SourceRoot || len(plan.Jobs) != len(plans[0].Jobs) {
			return result, errors.New("destination plans must share the same sources")
		}
		if plan.Conflict != Skip && plan.Conflict != KeepBoth {
			return result, errors.New("conflict policy must be skip or keep-both")
		}
		for j, job := range plan.Jobs {
			if job.Source != plans[0].Jobs[j].Source {
				return result, errors.New("destination plans must share the same sources")
			}
		}
	}
	identities, err := validateTargets(plans[0].SourceRoot, roots)
	if err != nil {
		return result, err
	}
	// Retain the identities validation saw for the entire batch, including
	// queued targets and the final Move checks. Never substitute a recreated
	// mount, nor one swapped in between validation and pinning.
	plans = append([]Plan(nil), plans...)
	for i := range plans {
		plans[i].expectedRoot = identities[i]
	}
	if err := ctx.Err(); err != nil {
		for _, plan := range plans {
			result.Targets = append(result.Targets, notStarted(plan, err))
		}
	} else {
		for i := range plans {
			plans[i].Operation = Copy
		}
		result.Targets = runObserved(ctx, plans, hooks.Event)
		result.Started = len(plans)
		if hooks.Target != nil {
			for i, target := range result.Targets {
				hooks.Target(i, target)
			}
		}
	}
	result.Files = make([]FileResult, len(plans[0].Jobs))
	var removable []int
	for index, job := range plans[0].Jobs {
		file := aggregate(index, job, plans, result.Targets)
		result.Files[index] = file
		if operation == Move && file.Phase == PhaseComplete {
			removable = append(removable, index)
		} else if hooks.File != nil {
			hooks.File(file)
		}
	}
	if operation == Move {
		removeSources(ctx, plans, result, removable, hooks.File)
	}
	return result, nil
}

// runObserved runs every target at once, forwarding their events to observe
// from one goroutine.
func runObserved(ctx context.Context, plans []Plan, observe func(int, Event)) []Result {
	channels := make([]chan<- Event, len(plans))
	if observe == nil {
		return runTargets(ctx, plans, channels)
	}
	type tagged struct {
		target int
		event  Event
	}
	merged := make(chan tagged)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for next := range merged {
			observe(next.target, next.event)
		}
	}()
	var forwarders sync.WaitGroup
	owned := make([]chan Event, len(plans))
	for i := range plans {
		owned[i] = make(chan Event)
		channels[i] = owned[i]
		forwarders.Add(1)
		go func() {
			defer forwarders.Done()
			for event := range owned[i] {
				merged <- tagged{target: i, event: event}
			}
		}()
	}
	results := runTargets(ctx, plans, channels)
	for _, channel := range owned {
		close(channel)
	}
	forwarders.Wait()
	close(merged)
	<-done
	return results
}

func notStarted(plan Plan, err error) Result {
	result := Result{Files: make([]FileResult, len(plan.Jobs))}
	for i, job := range plan.Jobs {
		result.Files[i] = FileResult{Index: i, Source: job.Source, Destination: job.Destination, Phase: PhaseFailed, Error: "not started: " + err.Error()}
	}
	return result
}

// aggregate decides one source's outcome across targets. A source is complete
// only when every target holds a verified copy of the same content, and the
// copies are distinct files: hard links of one another are one copy.
func aggregate(index int, job Job, plans []Plan, targets []Result) FileResult {
	first := targets[0].Files[index]
	file := FileResult{Index: index, Source: job.Source, Phase: PhaseComplete, Size: first.Size, SourceHash: first.SourceHash}
	fail := func(message string) FileResult {
		file.Phase, file.Error = PhaseFailed, message
		return file
	}
	infos := make([]os.FileInfo, len(targets))
	for target, imported := range targets {
		copy := imported.Files[index]
		root := plans[target].DestinationRoot
		if err := verifiedcopy.CheckRoot(root, plans[target].expectedRoot); err != nil {
			return fail(err.Error())
		}
		if copy.Phase != PhaseComplete && copy.Phase != PhaseAlreadyPresent {
			reason := copy.Error
			if copy.Warning != "" {
				reason = copy.Warning
			}
			if reason == "" {
				reason = string(copy.Phase)
			}
			return fail(fmt.Sprintf("not imported into %s: %s", root, reason))
		}
		if copy.SourceHash == "" || copy.SourceHash != first.SourceHash || copy.DestinationHash != first.SourceHash || copy.Size != first.Size {
			return fail(fmt.Sprintf("%s holds other content than %s: Source changed between targets", root, plans[0].DestinationRoot))
		}
		info, err := verifiedcopy.FileInfo(root, copy.Destination)
		if err != nil {
			return fail(fmt.Sprintf("copy in %s: %v", root, err))
		}
		if !verifiedcopy.SameVersion(info, copy.verified) {
			return fail(fmt.Sprintf("copy in %s changed after verification", root))
		}
		for other := range target {
			if os.SameFile(infos[other], info) {
				return fail(fmt.Sprintf("copies in %s and %s are the same file (hard link); only one copy exists", plans[other].DestinationRoot, root))
			}
		}
		infos[target] = info
	}
	return file
}

// removeSources finishes Move for verified sources, with the plans' worker
// count. Each source is read again and every copy checked afresh and found
// distinct before the unchanged source is deleted.
func removeSources(ctx context.Context, plans []Plan, result MultipleResult, indices []int, report func(FileResult)) {
	workers := min(max(plans[0].Workers, 1), MaxWorkers)
	queue := make(chan int)
	var mu sync.Mutex
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range queue {
				file := result.Files[index]
				if message := removeAfterMultipleChecks(ctx, plans, result.Targets, index, file); message != "" {
					file.Phase, file.Error = PhaseSourceKept, message
				}
				mu.Lock()
				result.Files[index] = file
				if report != nil {
					report(file)
				}
				mu.Unlock()
			}
		}()
	}
	for _, index := range indices {
		queue <- index
	}
	close(queue)
	group.Wait()
}

func removeAfterMultipleChecks(ctx context.Context, plans []Plan, targets []Result, index int, expected FileResult) string {
	if err := ctx.Err(); err != nil {
		return err.Error()
	}
	source, err := verifiedcopy.Check(ctx, plans[0].SourceRoot, expected.Source, nil, nil)
	if err != nil || source.Size != expected.Size || source.Digest != expected.SourceHash {
		return "source changed or could not be revalidated; source kept"
	}
	copies := make([]os.FileInfo, len(targets))
	for i, target := range targets {
		if err := verifiedcopy.CheckRoot(plans[i].DestinationRoot, plans[i].expectedRoot); err != nil {
			return err.Error()
		}
		copy, err := verifiedcopy.Check(ctx, plans[i].DestinationRoot, target.Files[index].Destination, nil, nil)
		if err != nil || copy.Size != source.Size || copy.Digest != source.Digest || os.SameFile(source.Source, copy.Source) {
			return fmt.Sprintf("destination %s changed or could not be revalidated; source kept", plans[i].DestinationRoot)
		}
		for other := range i {
			if os.SameFile(copies[other], copy.Source) {
				return fmt.Sprintf("copies in %s and %s are the same file; source kept", plans[other].DestinationRoot, plans[i].DestinationRoot)
			}
		}
		copies[i] = copy.Source
	}
	if err := ctx.Err(); err != nil {
		return err.Error()
	}
	for _, plan := range plans {
		if err := verifiedcopy.CheckRoot(plan.DestinationRoot, plan.expectedRoot); err != nil {
			return err.Error()
		}
	}
	if err := deleteSource(plans[0].SourceRoot, expected.Source, source.Source); err != nil {
		return err.Error()
	}
	return ""
}
