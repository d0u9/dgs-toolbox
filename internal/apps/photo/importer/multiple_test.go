package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func multiplePlans(t *testing.T) []Plan {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "card")
	writeFile(t, filepath.Join(source, "DCIM", "a.JPG"), "photo a")
	writeFile(t, filepath.Join(source, "DCIM", "b.RAW"), "photo b")
	var plans []Plan
	for _, name := range []string{"library-a", "library-b"} {
		destination := filepath.Join(root, name)
		if err := os.MkdirAll(destination, 0o755); err != nil {
			t.Fatal(err)
		}
		plans = append(plans, Plan{SourceRoot: source, DestinationRoot: destination,
			Jobs: []Job{{Source: filepath.Join(source, "DCIM", "a.JPG"), Destination: filepath.Join(destination, "DCIM", "a.JPG")},
				{Source: filepath.Join(source, "DCIM", "b.RAW"), Destination: filepath.Join(destination, "DCIM", "b.RAW")}},
			Conflict: Skip, Workers: 2, Incremental: true, StatePath: filepath.Join(destination, ".dgs-state")})
	}
	return plans
}

func TestMultipleCopiesAndIndependentIncrementalTargets(t *testing.T) {
	plans := multiplePlans(t)
	// Only the first target already has the JPG, organized under another name.
	old := filepath.Join(plans[0].DestinationRoot, "20261010", "renamed.JPG")
	writeFile(t, old, "photo a")
	result, err := RunMultiple(context.Background(), plans, Copy, MultipleHooks{})
	if err != nil || result.Targets[0].Files[0].Phase != PhaseAlreadyPresent || result.Targets[0].Files[0].Destination != old ||
		result.Targets[1].Files[0].Phase != PhaseComplete {
		t.Fatalf("result = %#v, %v", result, err)
	}
	for i, target := range result.Targets {
		for j, file := range target.Files {
			if data, err := os.ReadFile(file.Destination); err != nil || string(data) != []string{"photo a", "photo b"}[j] {
				t.Fatalf("target %d file %d: %q %v", i, j, data, err)
			}
		}
	}
	writeFile(t, filepath.Join(plans[0].SourceRoot, "DCIM", "c.JPG"), "photo c")
	for i := range plans {
		plans[i].Jobs = append(plans[i].Jobs, Job{Source: filepath.Join(plans[i].SourceRoot, "DCIM", "c.JPG"), Destination: filepath.Join(plans[i].DestinationRoot, "DCIM", "c.JPG")})
	}
	result, err = RunMultiple(context.Background(), plans, Copy, MultipleHooks{})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range result.Targets {
		if target.Files[0].Phase != PhaseAlreadyPresent || target.Files[1].Phase != PhaseAlreadyPresent || target.Files[2].Phase != PhaseComplete {
			t.Fatalf("incremental = %#v", target.Files)
		}
	}
}

func TestMultipleMoveWaitsForAllTargets(t *testing.T) {
	plans := multiplePlans(t)
	result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{Target: func(_ int, _ Result) {
		for _, job := range plans[0].Jobs {
			if _, err := os.Stat(job.Source); err != nil {
				t.Fatal("source removed before all target copies were checked", err)
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range result.Files {
		if file.Phase != PhaseComplete {
			t.Fatal(file)
		}
		if _, err := os.Stat(file.Source); !os.IsNotExist(err) {
			t.Fatal("successful multi-target Move kept source", err)
		}
	}
}

func TestMultipleMoveKeepsSourceOnTargetConflict(t *testing.T) {
	plans := multiplePlans(t)
	writeFile(t, plans[1].Jobs[0].Destination, "different photo")
	result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{})
	// Not copied everywhere is a failure, not "source kept": that phase means
	// imported everywhere with only the deletion unfinished.
	if err != nil || result.Files[0].Phase != PhaseFailed || !strings.Contains(result.Files[0].Error, ConflictNotImported) || result.Files[1].Phase != PhaseComplete {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if data, err := os.ReadFile(plans[0].Jobs[0].Source); err != nil || string(data) != "photo a" {
		t.Fatal("incomplete Move lost source", err)
	}
	if data, err := os.ReadFile(plans[0].Jobs[0].Destination); err != nil || string(data) != "photo a" {
		t.Fatal("successful target was discarded", err)
	}
}

func TestMultipleMoveFreshlyRevalidatesCopies(t *testing.T) {
	plans := multiplePlans(t)
	result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{Target: func(i int, _ Result) {
		if i == len(plans)-1 {
			writeFile(t, plans[0].Jobs[0].Destination, "damaged")
		}
	}})
	if err != nil || result.Files[0].Phase != PhaseSourceKept {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if _, err := os.Stat(plans[0].Jobs[0].Source); err != nil {
		t.Fatal("stale verification authorized deletion", err)
	}
}

func TestMultipleFailedTargetCanRetryWithoutRecopyingSuccessfulTarget(t *testing.T) {
	plans := multiplePlans(t)
	link := filepath.Join(plans[1].DestinationRoot, "DCIM")
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(plans[1].DestinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range result.Files {
		if file.Phase != PhaseFailed {
			t.Fatal("failed write authorized source deletion", file)
		}
		if _, err := os.Stat(file.Source); err != nil {
			t.Fatal(err)
		}
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatal("wrote through destination link")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	result, err = RunMultiple(context.Background(), plans, Move, MultipleHooks{})
	if err != nil {
		t.Fatal(err)
	}
	for i, file := range result.Files {
		if file.Phase != PhaseComplete || result.Targets[0].Files[i].Phase != PhaseAlreadyPresent {
			t.Fatalf("retry = %#v", result)
		}
		if _, err := os.Stat(file.Source); !os.IsNotExist(err) {
			t.Fatal("successful retry did not finish Move", err)
		}
	}
}

func TestMultipleCancellationKeepsSourcesAfterCopiesFinish(t *testing.T) {
	plans := multiplePlans(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := RunMultiple(ctx, plans, Move, MultipleHooks{Target: func(i int, _ Result) {
		if i == len(plans)-1 {
			cancel()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range result.Files {
		if file.Phase != PhaseSourceKept {
			t.Fatal(file)
		}
		if _, err := os.Stat(file.Source); err != nil {
			t.Fatal("cancellation removed source", err)
		}
	}
}

func TestMultipleKeepsSourceChangedAfterCopies(t *testing.T) {
	plans := multiplePlans(t)
	result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{Target: func(i int, _ Result) {
		if i == 0 {
			writeFile(t, plans[0].Jobs[0].Source, "PHOTO A")
		}
	}})
	if err != nil || result.Files[0].Phase != PhaseSourceKept {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if data, err := os.ReadFile(plans[0].Jobs[0].Source); err != nil || string(data) != "PHOTO A" {
		t.Fatal("changed source deleted", err)
	}
}

func TestMultipleRejectsAliasedAndNestedTargetsBeforeWrites(t *testing.T) {
	for _, mode := range []string{"duplicate", "nested", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			plans := multiplePlans(t)
			root := plans[0].DestinationRoot
			switch mode {
			case "duplicate":
				plans[1].DestinationRoot = root
			case "nested":
				plans[1].DestinationRoot = filepath.Join(root, "child")
			case "symlink":
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(plans[1].DestinationRoot); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(root, plans[1].DestinationRoot); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := RunMultiple(context.Background(), plans, Copy, MultipleHooks{}); err == nil {
				t.Fatal("unsafe targets accepted")
			}
			if _, err := os.Stat(plans[0].Jobs[0].Destination); !os.IsNotExist(err) {
				t.Fatal("validation wrote destination", err)
			}
		})
	}
}

// Every target holds a different photo of the same size, so each needs the
// source digest to decide. The card is read for it once, not once per target,
// and the copies still land in both.
func TestMultipleChecksSourceOnceForAllTargets(t *testing.T) {
	plans := multiplePlans(t)
	for _, plan := range plans {
		writeFile(t, filepath.Join(plan.DestinationRoot, "old", "x.JPG"), "photo x")
	}
	checks := make(map[int]map[int]bool)
	var copies [2]int
	result, err := RunMultiple(context.Background(), plans, Copy, MultipleHooks{Event: func(target int, event Event) {
		switch event.Phase {
		case PhaseCheckingSource:
			if checks[event.Index] == nil {
				checks[event.Index] = make(map[int]bool)
			}
			checks[event.Index][target] = true
		case PhaseComplete:
			copies[target]++
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks[0]) != 1 {
		t.Fatalf("source of a.JPG checked by targets %v", checks[0])
	}
	if copies != [2]int{2, 2} {
		t.Fatalf("copies = %v", copies)
	}
	for _, file := range result.Files {
		if file.Phase != PhaseComplete {
			t.Fatal(file)
		}
	}
}

// One target that cannot take a file does not stop the other from getting it.
func TestMultipleTargetFailureLeavesOtherCopies(t *testing.T) {
	plans := multiplePlans(t)
	writeFile(t, filepath.Join(plans[1].DestinationRoot, "DCIM"), "not a directory")
	result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{})
	if err != nil {
		t.Fatal(err)
	}
	for i, file := range result.Targets[0].Files {
		if file.Phase != PhaseComplete {
			t.Fatal(i, file)
		}
	}
	for i, file := range result.Targets[1].Files {
		if file.Phase != PhaseFailed {
			t.Fatal(i, file)
		}
	}
	for _, file := range result.Files {
		if file.Phase != PhaseFailed {
			t.Fatal(file)
		}
		if _, err := os.Stat(file.Source); err != nil {
			t.Fatal("source of a failed target removed", err)
		}
	}
}
