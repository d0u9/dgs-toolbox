package importer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A photo deleted from the library sits in its recycle bin. Importing the card
// again with Move must copy it back, not delete the card's only durable copy
// on the evidence of a bin that will be emptied.
func TestRecycleBinAndSnapshotCopiesAreNotEvidence(t *testing.T) {
	for _, bin := range []string{"#recycle", "#snapshot/daily", "$RECYCLE.BIN", ".Trashes/501", "@eaDir", ".Trash-1000/files"} {
		t.Run(bin, func(t *testing.T) {
			plan := incrementalPlan(t, "DCIM/IMG_0001.JPG")
			plan.Operation = Move
			writeFile(t, plan.Jobs[0].Source, "only copy of this photo")
			writeFile(t, filepath.Join(plan.DestinationRoot, filepath.FromSlash(bin), "IMG_0001.JPG"), "only copy of this photo")
			result := Run(context.Background(), plan, nil).Files[0]
			if result.Phase != PhaseComplete || result.Existing || result.Destination != plan.Jobs[0].Destination {
				t.Fatalf("result = %#v", result)
			}
			data, err := os.ReadFile(plan.Jobs[0].Destination)
			if err != nil || string(data) != "only copy of this photo" {
				t.Fatalf("photo not imported: %v", err)
			}
		})
	}
}

// Volume roots hold system folders nobody can list. They are not part of the
// library, so they must not make the inventory incomplete.
func TestUnreadableSystemFoldersDoNotBlockInventory(t *testing.T) {
	plan := incrementalPlan(t, "a.JPG")
	writeFile(t, plan.Jobs[0].Source, "photo")
	for _, name := range []string{".Trashes", ".Spotlight-V100", ".fseventsd"} {
		folder := filepath.Join(plan.DestinationRoot, name)
		writeFile(t, filepath.Join(folder, "x"), "x")
		if err := os.Chmod(folder, 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(folder, 0o755)
	}
	if result := Run(context.Background(), plan, nil).Files[0]; result.Phase != PhaseComplete {
		t.Fatalf("result = %#v", result)
	}
}

func TestStateWritesAreCoalesced(t *testing.T) {
	root := t.TempDir()
	var jobs []Job
	for i := range 40 {
		name := fmt.Sprintf("IMG_%04d.JPG", i)
		writeFile(t, filepath.Join(root, "card", name), name)
		jobs = append(jobs, Job{Source: filepath.Join(root, "card", name), Destination: filepath.Join(root, "library", name)})
	}
	writes := 0
	plan := Plan{Jobs: jobs, Operation: Copy, Conflict: Skip, Workers: 4, StatePath: filepath.Join(root, "library", ".dgs-state"), StateInterval: time.Hour}
	plan.saveState = func(path string, state *State) error {
		writes++
		return writeState(path, state)
	}
	Run(context.Background(), plan, nil)
	if writes != 2 {
		t.Fatalf("state written %d times for 40 files, want start and end only", writes)
	}
	state, err := LoadState(plan.StatePath)
	if err != nil || len(state.Files) != 40 {
		t.Fatalf("final state: %d records, %v", len(state.Files), err)
	}
	for _, file := range state.Files {
		if file.Phase != PhaseComplete {
			t.Fatalf("final state not current: %#v", file)
		}
	}
}

func TestCompletedStateIsSavedWhileNextFileIsPaused(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "card", "a.JPG"), filepath.Join(root, "card", "b.JPG")
	writeFile(t, first, "photo a")
	writeFile(t, second, "photo b")
	controller := &Controller{}
	events := make(chan Event)
	paused := make(chan struct{})
	go func() {
		for event := range events {
			if event.Index == 1 && event.Phase == PhaseCopying && event.Bytes == 0 {
				controller.Pause()
				close(paused)
			}
		}
	}()
	plan := Plan{Jobs: []Job{
		{Source: first, Destination: filepath.Join(root, "library", "a.JPG")},
		{Source: second, Destination: filepath.Join(root, "library", "b.JPG")},
	}, Operation: Copy, Conflict: KeepBoth, Workers: 1, Controller: controller,
		StatePath: filepath.Join(root, "library", ".dgs-state"), StateInterval: 100 * time.Millisecond}
	snapshots := make(chan Phase, 16)
	plan.saveState = func(path string, state *State) error {
		err := writeState(path, state)
		if err == nil {
			snapshots <- state.Files[0].Phase
		}
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { Run(ctx, plan, events); close(events); close(finished) }()
	t.Cleanup(func() {
		cancel()
		controller.Resume()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("cancelled run did not finish")
		}
	})
	select {
	case <-paused:
	case <-time.After(5 * time.Second):
		t.Fatal("second file did not pause")
	}
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
waiting:
	for {
		select {
		case phase := <-snapshots:
			if phase == PhaseComplete {
				break waiting
			}
		case <-timeout.C:
			t.Fatal("completed first file was not saved while the second file was paused")
		}
	}
	state, err := LoadState(plan.StatePath)
	if err != nil || state.Files[0].Phase != PhaseComplete || state.Files[1].Phase != PhasePending {
		t.Fatalf("paused batch state = %#v, %v", state.Files, err)
	}
	// No new transitions means there is nothing to rewrite on later ticks.
	select {
	case phase := <-snapshots:
		t.Fatalf("unchanged state was rewritten: %s", phase)
	case <-time.After(3 * plan.StateInterval):
	}
}

func TestStateKeepsEarlierBatches(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "library", ".dgs-state")
	batch := func(card string) Plan {
		source := filepath.Join(root, card, "IMG_0001.JPG")
		writeFile(t, source, card+" photo")
		return Plan{Jobs: []Job{{Source: source, Destination: filepath.Join(root, "library", card, "IMG_0001.JPG")}}, Operation: Copy, Conflict: Skip, StatePath: statePath}
	}
	a, b := batch("card-a"), batch("card-b")
	Run(context.Background(), a, nil)
	Run(context.Background(), b, nil)
	state, err := LoadState(statePath)
	if err != nil || len(state.Files) != 2 {
		t.Fatalf("state = %#v, %v", state.Files, err)
	}
	if again := Run(context.Background(), a, nil).Files[0]; again.Phase != PhaseComplete || !again.Existing {
		t.Fatalf("first card lost its resume record: %#v", again)
	}
}

func TestIncrementalNameConflictIsReportedNotImported(t *testing.T) {
	plan := incrementalPlan(t, "DCIM/IMG_0001.JPG")
	writeFile(t, plan.Jobs[0].Source, "new photo after the counter reset")
	writeFile(t, plan.Jobs[0].Destination, "an older, different photo")
	result := Run(context.Background(), plan, nil).Files[0]
	if result.Phase != PhaseSkipped || result.Warning != ConflictNotImported {
		t.Fatalf("result = %#v", result)
	}
	plan.Incremental = false
	if result := Run(context.Background(), plan, nil).Files[0]; result.Phase != PhaseSkipped || result.Warning != "" {
		t.Fatalf("a filename-only skip claims to know content: %#v", result)
	}
}

func TestOverlappingRootsAreRefusedInEveryMode(t *testing.T) {
	root := t.TempDir()
	card := filepath.Join(root, "card")
	source := filepath.Join(card, "IMG_0001.JPG")
	writeFile(t, source, "photo")
	for _, incremental := range []bool{false, true} {
		plan := Plan{Jobs: []Job{{Source: source, Destination: source}}, Operation: Move, Conflict: KeepBoth, SourceRoot: card, DestinationRoot: card, Incremental: incremental}
		if result := Run(context.Background(), plan, nil).Files[0]; result.Phase == PhaseComplete || result.Phase == PhaseAlreadyPresent {
			t.Fatalf("incremental=%v: %#v", incremental, result)
		}
		entries, _ := os.ReadDir(card)
		if len(entries) != 1 || entries[0].Name() != "IMG_0001.JPG" {
			t.Fatalf("card changed: %v", entries)
		}
	}
}

func TestProgressEventsAreThrottled(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "card", "big.JPG"), filepath.Join(root, "library", "big.JPG")
	writeFile(t, source, strings.Repeat("x", 16<<20))
	events := make(chan Event, 1024)
	Run(context.Background(), Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: Skip}, events)
	close(events)
	progress, finished := 0, false
	for event := range events {
		switch event.Phase {
		case PhaseCopying, PhaseVerifying, PhasePublishing:
			progress++
		case PhaseComplete:
			finished = true
		}
	}
	// Unthrottled, 16 MiB in 1 MiB chunks is 34 events; each phase's start and
	// end always pass, and a local copy takes far less than the interval.
	if !finished || progress < 4 || progress > 12 {
		t.Fatalf("%d progress events, finished=%v", progress, finished)
	}
}

func TestInventoryReportsProgress(t *testing.T) {
	plan := incrementalPlan(t, "new.JPG")
	writeFile(t, plan.Jobs[0].Source, "new photo")
	for i := range 3 {
		writeFile(t, filepath.Join(plan.DestinationRoot, "2026", fmt.Sprintf("old-%d.JPG", i)), "old")
	}
	events := make(chan Event, 256)
	Run(context.Background(), plan, events)
	close(events)
	last := int64(-1)
	for event := range events {
		if event.Phase == PhaseInventory {
			if event.Index != -1 {
				t.Fatalf("inventory event for job %d", event.Index)
			}
			last = event.Bytes
		}
	}
	if last != 3 {
		t.Fatalf("inventory reported %d files, want 3", last)
	}
}

func TestKeepBothNamesAreSharedAcrossClaims(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "IMG_0001.JPG")
	names := newReservations()
	if first, second := names.claim(path), names.claim(path); first != path || second != filepath.Join(root, "IMG_0001-2.JPG") {
		t.Fatalf("claims %s %s", first, second)
	}
	writeFile(t, filepath.Join(root, "IMG_0001-3.JPG"), "on disk")
	if third := names.claim(path); third != filepath.Join(root, "IMG_0001-4.JPG") {
		t.Fatalf("third claim %s", third)
	}
}

func TestRefutedResumeUnderKeepBothAvoidsReservedNames(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "library", ".dgs-state")
	a, b := filepath.Join(root, "card", "a", "IMG_0001.JPG"), filepath.Join(root, "card", "b", "IMG_0001.JPG")
	writeFile(t, a, "photo a")
	writeFile(t, b, "photo b")
	shared := filepath.Join(root, "library", "IMG_0001.JPG")
	plan := Plan{Jobs: []Job{{Source: a, Destination: shared}}, Operation: Copy, Conflict: KeepBoth, StatePath: statePath}
	Run(context.Background(), plan, nil)
	// Same size, other bytes: a's record is refuted, while b claims a name.
	if err := os.WriteFile(shared, []byte("PHOTO A"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan.Jobs = append(plan.Jobs, Job{Source: b, Destination: shared})
	plan.Workers = 2
	result := Run(context.Background(), plan, nil)
	seen := map[string]bool{}
	for _, file := range result.Files {
		if file.Phase != PhaseComplete || seen[file.Destination] {
			t.Fatalf("results = %#v", result.Files)
		}
		seen[file.Destination] = true
	}
	if _, err := os.Stat(shared); errors.Is(err, os.ErrNotExist) {
		t.Fatal("existing file removed")
	}
}
