package importer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

func TestIncrementalRejectsUnreadableDestinationSubtreeBeforeWrites(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	blocked := filepath.Join(plan.DestinationRoot, ".hidden-library")
	writeFile(t, filepath.Join(blocked, "old.JPG"), "photo")
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(blocked, 0755)
	if _, err := os.ReadDir(blocked); err == nil {
		t.Skip("filesystem permits reading mode-000 directories")
	}
	result := Run(context.Background(), plan, nil)
	if result.Files[0].Phase != PhaseUndetermined {
		t.Fatal(result.Files[0])
	}
	if _, err := os.Stat(plan.StatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("incomplete inventory wrote state")
	}
	if _, err := os.Stat(plan.Jobs[0].Destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("incomplete inventory published")
	}
}

func TestMatchedMoveRevalidationKeepsChangedSourceOrDestination(t *testing.T) {
	for _, changed := range []string{"source", "destination"} {
		t.Run(changed, func(t *testing.T) {
			plan := incrementalPlan(t, "a.JPG")
			writeFile(t, plan.Jobs[0].Source, "photo")
			writeFile(t, plan.Jobs[0].Destination, "photo")
			checked, err := verifiedcopy.Check(context.Background(), plan.SourceRoot, plan.Jobs[0].Source, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			path := plan.Jobs[0].Source
			if changed == "destination" {
				path = plan.Jobs[0].Destination
			}
			writeFile(t, path, "other")
			stamp := checked.Source.ModTime().Add(time.Second)
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			if err := deleteMatchedSource(context.Background(), plan.Jobs[0].Source, plan.Jobs[0].Destination, plan, checked); err == nil {
				t.Fatal("changed evidence authorized deletion")
			}
			if _, err := os.Stat(plan.Jobs[0].Source); err != nil {
				t.Fatal("changed evidence deleted source")
			}
		})
	}
}

func TestIncrementalRejectsCandidatePathSubstitution(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	folder := filepath.Join(plan.DestinationRoot, "date")
	writeFile(t, filepath.Join(folder, "a.JPG"), "photo")
	index, err := newContentIndex(context.Background(), plan, State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(folder, folder+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(folder+"-old", folder); err != nil {
		t.Fatal(err)
	}
	plan.index = index
	plan.planned = []string{plan.Jobs[0].Destination}
	if result, _ := transfer(context.Background(), 0, plan, nil); result.Phase != PhaseUndetermined {
		t.Fatal(result)
	}
}

func TestIncrementalStateWriteFailurePreservesExistingEvidence(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	writeFile(t, filepath.Join(plan.DestinationRoot, "date", "a.JPG"), "photo")
	plan.StatePath = filepath.Join(plan.DestinationRoot, "invalid-state")
	if err := os.MkdirAll(plan.StatePath, 0755); err != nil {
		t.Fatal(err)
	}
	result := Run(context.Background(), plan, nil)
	if result.StateError == nil || result.Files[0].Phase != PhaseAlreadyPresent {
		t.Fatalf("result = %#v", result)
	}
}

func incrementalPlan(t *testing.T, names ...string) Plan {
	t.Helper()
	root := t.TempDir()
	plan := Plan{SourceRoot: filepath.Join(root, "card"), DestinationRoot: filepath.Join(root, "library"), Incremental: true, Operation: Copy, Conflict: Skip, Workers: 4}
	plan.StatePath = filepath.Join(plan.DestinationRoot, ".dgs-state")
	for _, name := range names {
		plan.Jobs = append(plan.Jobs, Job{Source: filepath.Join(plan.SourceRoot, name), Destination: filepath.Join(plan.DestinationRoot, name)})
	}
	return plan
}

func TestIncrementalFindsOrganizedAndRenamedCopiesWithoutState(t *testing.T) {
	plan := incrementalPlan(t, "DCIM/old.JPG", "DCIM/new.JPG")
	writeFile(t, plan.Jobs[0].Source, "old photo bytes")
	writeFile(t, plan.Jobs[1].Source, "new camera photograph")
	old := filepath.Join(plan.DestinationRoot, "2026", "10", "03", "renamed-image.bin")
	writeFile(t, old, "old photo bytes")
	before, _ := os.Stat(old)
	result := Run(context.Background(), plan, nil)
	if result.Files[0].Phase != PhaseAlreadyPresent || !result.Files[0].Existing || result.Files[0].Destination != old {
		t.Fatalf("old result = %#v", result.Files[0])
	}
	if result.Files[1].Phase != PhaseComplete || result.Files[1].Existing {
		t.Fatalf("new result = %#v", result.Files[1])
	}
	if _, err := os.Stat(plan.Jobs[0].Destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old copy published: %v", err)
	}
	after, _ := os.Stat(old)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("old file metadata changed")
	}
	if err := os.Remove(plan.StatePath); err != nil {
		t.Fatal(err)
	}
	// A different card mount path must not depend on the deleted state file.
	nextRoot := filepath.Join(filepath.Dir(plan.SourceRoot), "card-again")
	if err := os.Rename(plan.SourceRoot, nextRoot); err != nil {
		t.Fatal(err)
	}
	for i := range plan.Jobs {
		rel, _ := filepath.Rel(plan.SourceRoot, plan.Jobs[i].Source)
		plan.Jobs[i].Source = filepath.Join(nextRoot, rel)
	}
	plan.SourceRoot = nextRoot
	next := Run(context.Background(), plan, nil)
	for _, file := range next.Files {
		if file.Phase != PhaseAlreadyPresent {
			t.Fatalf("repeated result = %#v", file)
		}
	}
}

func TestIncrementalSameNameSizeAndTimeStillChecksBytes(t *testing.T) {
	for _, policy := range []ConflictPolicy{Skip, KeepBoth} {
		t.Run(string(policy), func(t *testing.T) {
			plan := incrementalPlan(t, "IMG_0001.JPG")
			plan.Conflict = policy
			plan.Operation = Move
			writeFile(t, plan.Jobs[0].Source, "new")
			writeFile(t, plan.Jobs[0].Destination, "old")
			stamp := time.Date(2026, 10, 3, 1, 2, 3, 0, time.UTC)
			for _, path := range []string{plan.Jobs[0].Source, plan.Jobs[0].Destination} {
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			result := Run(context.Background(), plan, nil).Files[0]
			if result.Existing || result.SourceHash == result.DestinationHash && result.Phase == PhaseAlreadyPresent {
				t.Fatalf("false match: %#v", result)
			}
			if policy == Skip {
				if result.Phase != PhaseSkipped {
					t.Fatal(result)
				}
				if _, err := os.Stat(plan.Jobs[0].Source); err != nil {
					t.Fatal("conflict deleted source")
				}
			} else if result.Phase != PhaseComplete || result.Destination == plan.Jobs[0].Destination {
				t.Fatal(result)
			}
			if data, _ := os.ReadFile(plan.Jobs[0].Destination); string(data) != "old" {
				t.Fatal("conflict overwritten")
			}
		})
	}
}

func TestIncrementalInventoryDoesNotCoalesceNewSourceFiles(t *testing.T) {
	plan := incrementalPlan(t, "a/same.JPG", "b/same.JPG")
	for _, job := range plan.Jobs {
		writeFile(t, job.Source, "identical bytes")
	}
	for _, file := range Run(context.Background(), plan, nil).Files {
		if file.Phase != PhaseComplete || file.Existing {
			t.Fatalf("new source coalesced: %#v", file)
		}
		if _, err := os.Stat(file.Destination); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIncrementalCandidateDigestSharedByWorkers(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG", "b.JPG", "c.JPG", "d.JPG")
	for _, job := range plan.Jobs {
		writeFile(t, job.Source, "identical photograph")
	}
	writeFile(t, filepath.Join(plan.DestinationRoot, "20261003", "old.JPG"), "identical photograph")
	events := make(chan Event, 100)
	result := Run(context.Background(), plan, events)
	close(events)
	checks := 0
	for event := range events {
		if event.Phase == PhaseCheckingDestination && event.Bytes == 0 {
			checks++
		}
	}
	if checks != 1 {
		t.Fatalf("destination hashed %d times", checks)
	}
	for _, file := range result.Files {
		if file.Phase != PhaseAlreadyPresent {
			t.Fatal(file)
		}
	}
}

func TestIncrementalNoCandidatesUsesSinglePassCopy(t *testing.T) {
	plan := incrementalPlan(t, "video.MP4")
	writeFile(t, plan.Jobs[0].Source, "synthetic video bytes")
	events := make(chan Event, 100)
	result := Run(context.Background(), plan, events)
	close(events)
	for event := range events {
		if event.Phase == PhaseCheckingSource || event.Phase == PhaseCheckingDestination {
			t.Fatal("prehashed without candidates")
		}
	}
	if result.Files[0].Phase != PhaseComplete {
		t.Fatal(result.Files[0])
	}
}

func TestIncrementalMoveOverExistingCopy(t *testing.T) {
	plan := incrementalPlan(t, "old.JPG")
	plan.Operation = Move
	writeFile(t, plan.Jobs[0].Source, "photo")
	old := filepath.Join(plan.DestinationRoot, "20261003", "old.JPG")
	writeFile(t, old, "photo")
	result := Run(context.Background(), plan, nil).Files[0]
	if result.Phase != PhaseAlreadyPresent || !result.Existing {
		t.Fatal(result)
	}
	if _, err := os.Stat(plan.Jobs[0].Source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source remains: %v", err)
	}
	if data, _ := os.ReadFile(old); string(data) != "photo" {
		t.Fatal("old copy changed")
	}
}

func TestIncrementalRejectsOverlappingAndAliasedRoots(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	writeFile(t, filepath.Join(source, "a.JPG"), "photo")
	for _, dest := range []string{source, filepath.Join(source, "out"), root} {
		if err := ValidateRoots(source, dest); err == nil {
			t.Fatalf("accepted overlap %s", dest)
		}
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRoots(source, alias); err == nil {
		t.Fatal("accepted root alias")
	}
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	if err := os.MkdirAll(plan.DestinationRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(plan.Jobs[0].Source, plan.Jobs[0].Destination); err != nil {
		t.Fatal(err)
	}
	if result := Run(context.Background(), plan, nil).Files[0]; result.Phase != PhaseUndetermined {
		t.Fatalf("accepted hardlink: %#v", result)
	}
}

func TestIncrementalUnreadableCandidateDoesNotBecomeNew(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	writeFile(t, plan.Jobs[0].Destination, "photo")
	index, err := newContentIndex(context.Background(), plan, State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan.index = index
	plan.planned = []string{plan.Jobs[0].Destination}
	if err := os.Remove(plan.Jobs[0].Destination); err != nil {
		t.Fatal(err)
	}
	result, _ := transfer(context.Background(), 0, plan, nil)
	if result.Phase != PhaseUndetermined {
		t.Fatalf("missing candidate = %#v", result)
	}
	if _, err := os.Stat(plan.Jobs[0].Destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("undetermined comparison published")
	}
}

func TestIncrementalPositiveMatchCanSurviveAnotherCandidateError(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	writeFile(t, plan.Jobs[0].Destination, "photo")
	good := filepath.Join(plan.DestinationRoot, "20261003", "a.JPG")
	writeFile(t, good, "photo")
	index, err := newContentIndex(context.Background(), plan, State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan.index = index
	plan.planned = []string{plan.Jobs[0].Destination}
	if err := os.Remove(plan.Jobs[0].Destination); err != nil {
		t.Fatal(err)
	}
	result, _ := transfer(context.Background(), 0, plan, nil)
	if result.Phase != PhaseAlreadyPresent || result.Destination != good {
		t.Fatal(result)
	}
}

func TestIncrementalIgnoresStateAndPartArtifacts(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	plan.StatePath = filepath.Join(plan.DestinationRoot, ".custom-state")
	for _, name := range []string{".custom-state", "..custom-state.1.tmp", ".a.JPG.1.dgs-part", ".dgs-state"} {
		writeFile(t, filepath.Join(plan.DestinationRoot, name), "photo")
	}
	if result := Run(context.Background(), plan, nil).Files[0]; result.Phase != PhaseComplete {
		t.Fatal(result)
	}
}

func TestIncrementalRebuildsAfterInvalidStateAndLostCopy(t *testing.T) {
	for _, state := range []string{"invalid", `{"version":99,"hash_algorithm":"SHA-256"}`} {
		t.Run(state, func(t *testing.T) {
			plan := incrementalPlan(t, "a.JPG")
			writeFile(t, plan.Jobs[0].Source, "photo")
			first := Run(context.Background(), plan, nil).Files[0]
			if first.Phase != PhaseComplete {
				t.Fatal(first)
			}
			writeFile(t, plan.StatePath, state)
			if next := Run(context.Background(), plan, nil).Files[0]; next.Phase != PhaseAlreadyPresent {
				t.Fatal(next)
			}
			if err := os.Remove(first.Destination); err != nil {
				t.Fatal(err)
			}
			if next := Run(context.Background(), plan, nil).Files[0]; next.Phase != PhaseComplete {
				t.Fatal(next)
			}
		})
	}
}

func TestIncrementalReportsRecordedDestinationDigestMismatch(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	Run(context.Background(), plan, nil)
	writeFile(t, plan.Jobs[0].Destination, "other")
	result := Run(context.Background(), plan, nil).Files[0]
	if result.Phase != PhaseSkipped || result.Warning == "" {
		t.Fatal(result)
	}
}

func TestLoadVersionOneStateRemainsSupported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data, err := json.Marshal(State{Version: 1, HashAlgorithm: "SHA-256"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalPausedRunCancelsWithoutPublishing(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	plan.Controller = &Controller{}
	plan.Controller.Pause()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() { done <- Run(ctx, plan, nil) }()
	select {
	case <-done:
		t.Fatal("paused run finished")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case result := <-done:
		if result.Files[0].Phase != PhaseUndetermined || !strings.Contains(result.Files[0].Error, "canceled") {
			t.Fatal(result.Files[0])
		}
	case <-time.After(time.Second):
		t.Fatal("paused run did not cancel")
	}
	if _, err := os.Stat(plan.Jobs[0].Destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancellation published a file")
	}
}
