package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumeReadsDigestsInWorkersWithProgress(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "src.jpg"), filepath.Join(root, "dst", "src.jpg")
	statePath := filepath.Join(root, "dst", ".dgs-state")
	writeFile(t, source, "photo")
	plan := Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: Skip, StatePath: statePath}
	if first := Run(context.Background(), plan, nil); first.Files[0].Phase != PhaseComplete {
		t.Fatal(first.Files[0].Error)
	}
	events := make(chan Event, 64)
	second := Run(context.Background(), plan, events)
	close(events)
	if second.Files[0].Phase != PhaseComplete || !second.Files[0].Existing {
		t.Fatalf("resume = %#v", second.Files[0])
	}
	var checked, finished bool
	for event := range events {
		checked = checked || event.Phase == PhaseCheckingDestination
		finished = finished || (event.Phase == PhaseComplete && event.Existing)
	}
	if !checked || !finished {
		t.Fatalf("resume events: checking=%v finished=%v", checked, finished)
	}
}

func TestInterruptedResumeKeepsStateRecord(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "src.jpg"), filepath.Join(root, "dst", "src.jpg")
	statePath := filepath.Join(root, "dst", ".dgs-state")
	writeFile(t, source, "photo")
	plan := Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: Skip, StatePath: statePath}
	if first := Run(context.Background(), plan, nil); first.Files[0].Phase != PhaseComplete {
		t.Fatal(first.Files[0].Error)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := Run(ctx, plan, nil); result.Files[0].Phase == PhaseComplete {
		t.Fatalf("cancelled resume completed: %#v", result.Files[0])
	}
	state, err := LoadState(statePath)
	if err != nil || state.Files[0].Phase != PhaseComplete || state.Files[0].DestinationHash == "" {
		t.Fatalf("state lost its evidence: %#v %v", state, err)
	}
	if third := Run(context.Background(), plan, nil); third.Files[0].Phase != PhaseComplete || !third.Files[0].Existing {
		t.Fatalf("later resume = %#v", third.Files[0])
	}
}

func TestRefutedResumeImportsAsNew(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "src.jpg"), filepath.Join(root, "dst", "src.jpg")
	statePath := filepath.Join(root, "dst", ".dgs-state")
	writeFile(t, source, "photo")
	plan := Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: KeepBoth, StatePath: statePath}
	Run(context.Background(), plan, nil)
	// Same size, other bytes: the record no longer describes the file.
	if err := os.WriteFile(destination, []byte("PHOTO"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := Run(context.Background(), plan, nil).Files[0]
	if result.Phase != PhaseComplete || result.Existing || result.Destination == destination {
		t.Fatalf("refuted resume = %#v", result)
	}
}

func TestCandidateReadFailureIsNotCached(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	writeFile(t, plan.Jobs[0].Destination, "photo")
	index, err := newContentIndex(context.Background(), plan, State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := index.byPath[plan.Jobs[0].Destination]
	if err := os.Chmod(c.path, 0); err != nil {
		t.Fatal(err)
	}
	if f, err := os.Open(c.path); err == nil {
		f.Close()
		t.Skip("filesystem permits reading mode-000 files")
	}
	if _, err := c.check(context.Background(), index, plan, Event{}, nil); err == nil {
		t.Fatal("unreadable candidate checked")
	}
	if err := os.Chmod(c.path, 0o644); err != nil {
		t.Fatal(err)
	}
	// Restoring permission changes no metadata SameVersion compares.
	if checked, err := c.check(context.Background(), index, plan, Event{}, nil); err != nil || checked.Digest == "" {
		t.Fatalf("read failure was cached: %v", err)
	}
}

func TestUndeterminedNamesTheCandidate(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	other := filepath.Join(plan.DestinationRoot, "old", "b.JPG")
	writeFile(t, other, "PHOTO")
	index, err := newContentIndex(context.Background(), plan, State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan.index, plan.planned = index, []string{plan.Jobs[0].Destination}
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	result, _ := transfer(context.Background(), 0, plan, nil)
	if result.Phase != PhaseUndetermined || !strings.Contains(result.Error, other) {
		t.Fatalf("result = %#v", result)
	}
}

func TestValidateRootsComparesIdentitiesNotSpelling(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "Card")
	writeFile(t, filepath.Join(source, "a.JPG"), "photo")
	respelled := filepath.Join(root, "card")
	if _, err := os.Stat(respelled); err != nil {
		t.Skip("filesystem is case-sensitive")
	}
	if err := ValidateRoots(source, filepath.Join(respelled, "out")); err == nil {
		t.Fatal("accepted a destination inside the source spelled in another case")
	}
	if err := ValidateRoots(filepath.Join(source), filepath.Join(root, "library")); err != nil {
		t.Fatal(err)
	}
}

func TestRunCapsWorkers(t *testing.T) {
	root := t.TempDir()
	var jobs []Job
	for i := range MaxWorkers + 4 {
		name := string(rune('a'+i)) + ".jpg"
		writeFile(t, filepath.Join(root, "src", name), name)
		jobs = append(jobs, Job{Source: filepath.Join(root, "src", name), Destination: filepath.Join(root, "dst", name)})
	}
	result := Run(context.Background(), Plan{Jobs: jobs, Operation: Copy, Conflict: Skip, Workers: 100}, nil)
	for _, file := range result.Files {
		if file.Phase != PhaseComplete {
			t.Fatal(file)
		}
	}
}
