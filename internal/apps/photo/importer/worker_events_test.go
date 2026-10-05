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
	for event := range events {
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
		}
	}
	if completed != len(jobs) || len(workers) != len(jobs) {
		t.Fatal("missing completion or progress events", <-result)
	}
}
