package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunCopiesVerifiesPublishesAndPersistsState(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source", "IMG_0001.JPG")
	destination := filepath.Join(root, "destination", "album", "IMG_0001.JPG")
	writeFile(t, source, strings.Repeat("camera-bytes-", 10000))
	sourceTime := time.Date(2022, time.July, 14, 9, 8, 7, 0, time.UTC)
	if err := os.Chtimes(source, sourceTime, sourceTime); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, "destination", ".dgs-state")
	result := Run(context.Background(), Plan{
		Jobs:      []Job{{Source: source, Destination: destination}},
		Operation: Copy, Conflict: Skip, Workers: 1, StatePath: statePath,
	}, nil)
	if len(result.Files) != 1 || result.Files[0].Phase != PhaseComplete {
		t.Fatalf("result = %#v", result)
	}
	sourceData, _ := os.ReadFile(source)
	destinationData, _ := os.ReadFile(destination)
	if string(sourceData) != string(destinationData) {
		t.Fatal("published destination differs from source")
	}
	destinationInfo, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !destinationInfo.ModTime().Equal(sourceTime) {
		t.Fatalf("destination modification time = %s, want %s", destinationInfo.ModTime(), sourceTime)
	}
	wantHash := sha256.Sum256(sourceData)
	if result.Files[0].SourceHash != hex.EncodeToString(wantHash[:]) || result.Files[0].DestinationHash != result.Files[0].SourceHash {
		t.Fatalf("hashes = %q %q", result.Files[0].SourceHash, result.Files[0].DestinationHash)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destination), ".IMG_0001.JPG.dgs-part")); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains after publish: %v", err)
	}
	state, err := LoadState(statePath)
	if err != nil || len(state.Files) != 1 || state.Files[0].Phase != PhaseComplete {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestRunMoveDeletesSourceOnlyAfterVerifiedPublish(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "src.jpg"), filepath.Join(root, "dst", "src.jpg")
	writeFile(t, source, "photo")
	result := Run(context.Background(), Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Move, Conflict: Skip, Workers: 1}, nil)
	if result.Files[0].Phase != PhaseComplete {
		t.Fatalf("result = %#v", result.Files[0])
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source was not deleted: %v", err)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "photo" {
		t.Fatalf("destination data=%q err=%v", data, err)
	}
}

func TestRunSkipsExistingDestinationWithoutChangingIt(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "src.jpg"), filepath.Join(root, "dst.jpg")
	writeFile(t, source, "new")
	writeFile(t, destination, "existing")
	result := Run(context.Background(), Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: Skip}, nil)
	if result.Files[0].Phase != PhaseSkipped {
		t.Fatalf("phase = %s", result.Files[0].Phase)
	}
	if data, _ := os.ReadFile(destination); string(data) != "existing" {
		t.Fatalf("destination changed: %q", data)
	}
}

func TestRunRejectsSymlinkAndUnsafeReplace(t *testing.T) {
	root := t.TempDir()
	realSource := filepath.Join(root, "real.jpg")
	linkSource := filepath.Join(root, "link.jpg")
	writeFile(t, realSource, "photo")
	if err := os.Symlink(realSource, linkSource); err != nil {
		t.Fatal(err)
	}
	result := Run(context.Background(), Plan{Jobs: []Job{{Source: linkSource, Destination: filepath.Join(root, "out.jpg")}}, Conflict: Skip}, nil)
	if result.Files[0].Phase != PhaseFailed || !strings.Contains(result.Files[0].Error, "regular file") {
		t.Fatalf("symlink result=%#v", result.Files[0])
	}
	result = Run(context.Background(), Plan{Jobs: []Job{{Source: realSource, Destination: filepath.Join(root, "out.jpg")}}, Conflict: Replace}, nil)
	if result.Files[0].Phase != PhaseFailed || !strings.Contains(result.Files[0].Error, "replace") {
		t.Fatalf("replace result=%#v", result.Files[0])
	}
}

func TestRunNeverWritesThroughTemporarySymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src.jpg")
	destination := filepath.Join(root, "dst", "photo.jpg")
	victim := filepath.Join(root, "victim")
	writeFile(t, source, "photo")
	writeFile(t, victim, "keep")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(filepath.Dir(destination), ".photo.jpg.dgs-part")
	if err := os.Symlink(victim, partial); err != nil {
		t.Fatal(err)
	}
	result := Run(context.Background(), Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: Skip}, nil)
	if result.Files[0].Phase != PhaseFailed || !strings.Contains(result.Files[0].Error, "temporary path") {
		t.Fatalf("result = %#v", result.Files[0])
	}
	if data, err := os.ReadFile(victim); err != nil || string(data) != "keep" {
		t.Fatalf("symlink target changed: %q, %v", data, err)
	}
}

func TestRunResumesVerifiedFileWithoutRewritingDestination(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "src.jpg"), filepath.Join(root, "dst", "src.jpg")
	statePath := filepath.Join(root, "dst", ".dgs-state")
	writeFile(t, source, "photo")
	plan := Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: Skip, StatePath: statePath}
	first := Run(context.Background(), plan, nil)
	if first.Files[0].Phase != PhaseComplete {
		t.Fatal(first.Files[0].Error)
	}
	info, _ := os.Stat(destination)
	second := Run(context.Background(), plan, nil)
	after, _ := os.Stat(destination)
	if second.Files[0].Phase != PhaseComplete || !after.ModTime().Equal(info.ModTime()) {
		t.Fatalf("resume rewrote destination: %#v", second.Files[0])
	}
}

func TestRunResumesFileRelocatedByPostProcessing(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src.jpg")
	planned := filepath.Join(root, "dst", "src.jpg")
	relocated := filepath.Join(root, "dst", "20261010", "src.jpg")
	statePath := filepath.Join(root, "dst", ".dgs-state")
	writeFile(t, source, "photo")
	plan := Plan{Jobs: []Job{{Source: source, Destination: planned}}, Operation: Copy, Conflict: Skip, StatePath: statePath}
	first := Run(context.Background(), plan, nil)
	if first.Files[0].Phase != PhaseComplete {
		t.Fatal(first.Files[0].Error)
	}
	if err := os.MkdirAll(filepath.Dir(relocated), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(planned, relocated); err != nil {
		t.Fatal(err)
	}
	if err := UpdateDestinations(statePath, map[string]string{planned: relocated}); err != nil {
		t.Fatal(err)
	}
	second := Run(context.Background(), plan, nil)
	if second.Files[0].Phase != PhaseComplete || second.Files[0].Destination != relocated {
		t.Fatalf("relocated resume = %#v", second.Files[0])
	}
	if _, err := os.Stat(planned); !os.IsNotExist(err) {
		t.Fatalf("planned destination was recreated: %v", err)
	}
}

func TestRunReservesDistinctKeepBothNamesAcrossWorkers(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "card-a", "same.jpg")
	second := filepath.Join(root, "card-b", "same.jpg")
	destination := filepath.Join(root, "dst", "same.jpg")
	writeFile(t, first, "first")
	writeFile(t, second, "second")
	writeFile(t, destination, "existing")
	result := Run(context.Background(), Plan{
		Jobs:      []Job{{Source: first, Destination: destination}, {Source: second, Destination: destination}},
		Operation: Copy, Conflict: KeepBoth, Workers: 2,
	}, nil)
	if result.Files[0].Phase != PhaseComplete || result.Files[1].Phase != PhaseComplete {
		t.Fatalf("result = %#v", result)
	}
	if result.Files[0].Destination == result.Files[1].Destination {
		t.Fatalf("workers published the same destination %q", result.Files[0].Destination)
	}
}

func TestRunDiscardsPartialFileOnFailure(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src", "IMG_0001.JPG")
	destination := filepath.Join(root, "dst", "IMG_0001.JPG")
	writeFile(t, source, strings.Repeat("camera-bytes-", 200000))
	// Cancelling mid-copy exercises a failure path that runs after the .dgs-part
	// file has been created exclusively.
	ctx, cancel := context.WithCancel(context.Background())
	controller := &Controller{}
	controller.Pause()
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
		controller.Resume()
	}()
	result := Run(ctx, Plan{
		Jobs:      []Job{{Source: source, Destination: destination}},
		Operation: Copy, Conflict: Skip, Workers: 1, Controller: controller,
	}, nil)
	if result.Files[0].Phase != PhaseFailed {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled transfer published a destination: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".dgs-part") {
			t.Fatalf("failed transfer left a partial file %q", entry.Name())
		}
	}
}

func TestRunKeepsPublishedFilesCompleteWhenStateCannotBeWritten(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src", "IMG_0001.JPG")
	destination := filepath.Join(root, "dst", "IMG_0001.JPG")
	writeFile(t, source, "camera-bytes")
	// A directory at the state path makes every write fail without affecting
	// the transfer itself.
	statePath := filepath.Join(root, "dst", ".dgs-state")
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}
	result := Run(context.Background(), Plan{
		Jobs:      []Job{{Source: source, Destination: destination}},
		Operation: Copy, Conflict: Skip, Workers: 1, StatePath: statePath,
	}, nil)
	if result.StateError == nil {
		t.Fatal("unwritable state file was not reported")
	}
	if result.Files[0].Phase != PhaseComplete || result.Files[0].Error != "" {
		t.Fatalf("bookkeeping failure changed the transfer outcome: %#v", result.Files[0])
	}
	published, err := os.ReadFile(destination)
	if err != nil || string(published) != "camera-bytes" {
		t.Fatalf("destination = %q, err = %v", published, err)
	}
}

func TestControllerPauseWaitsUntilResume(t *testing.T) {
	controller := &Controller{}
	controller.Pause()
	var completed atomic.Bool
	done := make(chan struct{})
	go func() {
		_ = controller.wait(context.Background())
		completed.Store(true)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("paused controller returned before resume")
	case <-time.After(20 * time.Millisecond):
	}
	controller.Resume()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("controller did not resume")
	}
	if !completed.Load() {
		t.Fatal("wait did not complete")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
