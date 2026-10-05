package importer

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

// runWithin fails the test instead of hanging when Run does not return.
func runWithin(t *testing.T, plan Plan) Result {
	t.Helper()
	done := make(chan Result, 1)
	go func() { done <- Run(context.Background(), plan, nil) }()
	select {
	case result := <-done:
		return result
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
		return Result{}
	}
}

func TestKeepBothFailsWhenANameCannotBeChecked(t *testing.T) {
	for _, mode := range []string{"file-in-path", "no-permission"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source, destination := filepath.Join(root, "card"), filepath.Join(root, "library")
			writeFile(t, filepath.Join(source, "DCIM", "a.JPG"), "photo a")
			switch mode {
			case "file-in-path":
				// The source folder's name is taken by a file: every name below
				// it answers ENOTDIR, which is neither free nor taken.
				writeFile(t, filepath.Join(destination, "DCIM"), "not a folder")
			case "no-permission":
				if os.Geteuid() == 0 {
					t.Skip("root ignores directory permissions")
				}
				locked := filepath.Join(destination, "DCIM")
				if err := os.MkdirAll(locked, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(locked, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
			}
			plan := Plan{SourceRoot: source, DestinationRoot: destination, Conflict: KeepBoth, Workers: 1, Operation: Move,
				Jobs: []Job{{Source: filepath.Join(source, "DCIM", "a.JPG"), Destination: filepath.Join(destination, "DCIM", "a.JPG")}}}
			result := runWithin(t, plan)
			if result.Files[0].Phase != PhaseFailed || !strings.Contains(result.Files[0].Error, "keep both name") {
				t.Fatalf("result = %#v", result.Files[0])
			}
			if _, err := os.Stat(plan.Jobs[0].Source); err != nil {
				t.Fatal("failed Move lost its source", err)
			}
		})
	}
}

func TestKeepBothNameTooLongIsAnError(t *testing.T) {
	root := t.TempDir()
	// The name itself fits; every "-N" variant exceeds the 255-byte limit.
	path := filepath.Join(root, strings.Repeat("x", 251)+".JPG")
	writeFile(t, path, "taken")
	name, err := newReservations().claim(path)
	if err == nil {
		t.Fatalf("claimed %s", name)
	}
}

func TestKeepBothGivesUpAfterBoundedTries(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "IMG.JPG")
	names := newReservations()
	// Reserved names count as taken without touching the disk.
	names.names[path] = struct{}{}
	for index := 2; index <= MaxKeepBothNames; index++ {
		names.names[filepath.Join(root, "IMG-"+strconv.Itoa(index)+".JPG")] = struct{}{}
	}
	if name, err := names.claim(path); err == nil {
		t.Fatalf("claimed %s", name)
	}
}

func TestEmptySourceMatchesOnlyItsOwnName(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "card"), filepath.Join(root, "library")
	writeFile(t, filepath.Join(source, "DCIM", "IMG_0001.JPG"), "")
	placeholder := filepath.Join(destination, "notes", "placeholder")
	writeFile(t, placeholder, "")
	job := Job{Source: filepath.Join(source, "DCIM", "IMG_0001.JPG"), Destination: filepath.Join(destination, "DCIM", "IMG_0001.JPG")}
	plan := Plan{SourceRoot: source, DestinationRoot: destination, Conflict: Skip, Workers: 1, Operation: Move, Incremental: true,
		StatePath: filepath.Join(destination, ".dgs-state"), Jobs: []Job{job}}
	result := runWithin(t, plan)
	// An unrelated empty file is no copy of it: it is imported under its name.
	if file := result.Files[0]; file.Phase != PhaseComplete || file.Destination != job.Destination || file.Existing {
		t.Fatalf("result = %#v", file)
	}
	if info, err := os.Stat(job.Destination); err != nil || info.Size() != 0 {
		t.Fatal("empty file not imported", err)
	}
	// Run again from a fresh copy of the card: its own name now holds it.
	writeFile(t, job.Source, "")
	result = runWithin(t, plan)
	if file := result.Files[0]; file.Phase != PhaseAlreadyPresent || file.Destination != job.Destination {
		t.Fatalf("second run = %#v", file)
	}
	if _, err := os.Stat(placeholder); err != nil {
		t.Fatal(err)
	}
}

func TestMatchedSourceIsKeptWhenTheCopyIsItsHardLink(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "card"), filepath.Join(root, "library")
	path := filepath.Join(source, "a.JPG")
	writeFile(t, path, "photo a")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(destination, "a.JPG")
	if err := os.Link(path, link); err != nil {
		t.Skip("hard links unavailable:", err)
	}
	identity, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	read, err := verifiedcopy.Check(context.Background(), source, path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{SourceRoot: source, DestinationRoot: destination, expectedRoot: identity}
	if err := deleteMatchedSource(context.Background(), path, link, plan, read); err == nil {
		t.Fatal("source deleted on the evidence of its own hard link")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("source lost", err)
	}
}

func TestEarlierSourceKeptCountsOnlyCarriedRecords(t *testing.T) {
	state := State{Files: []FileResult{
		{Index: 0, Phase: PhaseSourceKept},
		{Index: -1, Phase: PhaseSourceKept},
		{Index: -1, Phase: PhaseComplete},
	}}
	if got := EarlierSourceKept(state); got != 1 {
		t.Fatalf("EarlierSourceKept = %d", got)
	}
}
