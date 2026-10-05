package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunEventsKeepWorkerIdentityThroughCompletion(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "card"), filepath.Join(root, "library")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	var jobs []Job
	for _, name := range []string{"a.JPG", "b.JPG", "c.JPG", "d.JPG"} {
		path := filepath.Join(source, name)
		if err := os.WriteFile(path, []byte("synthetic photo"), 0o644); err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, Job{Source: path, Destination: filepath.Join(target, name)})
	}
	events := make(chan Event)
	result := make(chan Result, 1)
	go func() {
		result <- Run(context.Background(), Plan{Jobs: jobs, Workers: 2, SourceRoot: source, DestinationRoot: target}, events)
		close(events)
	}()
	workers := make(map[int]int)
	completed := 0
	final := make(map[int]bool)
	for event := range events {
		if final[event.Index] {
			t.Errorf("event after terminal completion: index=%d phase=%s", event.Index, event.Phase)
		}
		if event.Worker < 1 || event.Worker > 2 {
			t.Errorf("invalid worker: %+v", event)
			continue
		}
		if previous, ok := workers[event.Index]; ok && previous != event.Worker {
			t.Errorf("file switched workers: %+v", event)
		}
		workers[event.Index] = event.Worker
		if event.Phase == PhaseComplete {
			completed++
			final[event.Index] = true
		}
	}
	if completed != len(jobs) || len(workers) != len(jobs) {
		t.Fatal("missing completion or progress events", <-result)
	}
}

// A copy announces publishing after its readback and before its completion.
func TestRunAnnouncesPublishingBeforeCompletion(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "card", "a.JPG"), filepath.Join(root, "library", "a.JPG")
	writeFile(t, source, "photo")
	events := make(chan Event, 64)
	Run(context.Background(), Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: Skip}, events)
	close(events)
	var phases []Phase
	for event := range events {
		phases = append(phases, event.Phase)
	}
	publishing, complete := -1, -1
	for i, phase := range phases {
		switch phase {
		case PhasePublishing:
			if publishing < 0 {
				publishing = i
			}
		case PhaseComplete:
			complete = i
		}
	}
	if publishing < 0 || complete < 0 || publishing > complete {
		t.Fatalf("phases = %v", phases)
	}
	for _, phase := range phases[publishing+1 : complete] {
		if phase != PhasePublishing {
			t.Fatalf("phase %s between publishing and completion: %v", phase, phases)
		}
	}
}
