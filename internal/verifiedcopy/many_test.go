package verifiedcopy_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

func TestCopyManyReadsSourceOnceAndPublishesEveryTarget(t *testing.T) {
	dir := t.TempDir()
	// Larger than the pipeline, so blocks are reused while targets lag.
	data := []byte(strings.Repeat("0123456789abcdef", (verifiedcopy.PipelineDepth+3)*verifiedcopy.DefaultBufferSize/16+7))
	source := write(t, filepath.Join(dir, "card", "a.JPG"), data)
	targets := []verifiedcopy.Target{
		{Destination: filepath.Join(dir, "library", "DCIM", "a.JPG"), Root: filepath.Join(dir, "library")},
		{Destination: filepath.Join(dir, "backup", "DCIM", "a.JPG"), Root: filepath.Join(dir, "backup")},
		{Destination: filepath.Join(dir, "third", "DCIM", "a.JPG"), Root: filepath.Join(dir, "third")},
	}
	opens := 0
	defer verifiedcopy.CountSourceOpens(&opens)()
	var mu sync.Mutex
	published := make(map[int]bool)
	result, errs := verifiedcopy.CopyMany(context.Background(), verifiedcopy.ManyRequest{
		Source:  source,
		Targets: targets,
		Progress: func(target int, phase verifiedcopy.Phase, done, total int64) {
			if phase == verifiedcopy.PhasePublished {
				mu.Lock()
				published[target] = true
				mu.Unlock()
			}
		},
	})
	if opens != 1 {
		t.Fatalf("source opened %d times", opens)
	}
	sum := sha256.Sum256(data)
	if result.Digest != hex.EncodeToString(sum[:]) || result.Size != int64(len(data)) {
		t.Fatal("result does not describe the source", result)
	}
	for i, target := range targets {
		if errs[i] != nil {
			t.Fatal(i, errs[i])
		}
		got, err := os.ReadFile(target.Destination)
		if err != nil || string(got) != string(data) {
			t.Fatal(i, "copy differs", err)
		}
		if !published[i] {
			t.Fatal(i, "publication not reported")
		}
		if parts := partFiles(t, filepath.Dir(target.Destination)); len(parts) != 0 {
			t.Fatal(i, parts)
		}
	}
}

func TestCopyManyTargetFailsAlone(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "card", "a.JPG"), []byte("photo"))
	// The second target's directory is a file: it cannot be prepared.
	blocked := write(t, filepath.Join(dir, "backup", "DCIM"), []byte("not a directory"))
	existing := write(t, filepath.Join(dir, "third", "a.JPG"), []byte("other"))
	targets := []verifiedcopy.Target{
		{Destination: filepath.Join(dir, "library", "DCIM", "a.JPG")},
		{Destination: filepath.Join(blocked, "a.JPG")},
		{Destination: existing},
	}
	_, errs := verifiedcopy.CopyMany(context.Background(), verifiedcopy.ManyRequest{Source: source, Targets: targets})
	if errs[0] != nil {
		t.Fatal(errs[0])
	}
	if errs[1] == nil || errs[2] == nil {
		t.Fatal("failed targets reported success", errs)
	}
	if got, err := os.ReadFile(targets[0].Destination); err != nil || string(got) != "photo" {
		t.Fatal("surviving target not published", err)
	}
	if got, _ := os.ReadFile(existing); string(got) != "other" {
		t.Fatal("existing file replaced")
	}
}

func TestCopyManySourceFailureFailsEveryTargetAndLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	source := write(t, filepath.Join(dir, "card", "a.JPG"), []byte("photo"))
	targets := []verifiedcopy.Target{
		{Destination: filepath.Join(dir, "library", "a.JPG")},
		{Destination: filepath.Join(dir, "backup", "a.JPG")},
	}
	for name, request := range map[string]verifiedcopy.ManyRequest{
		"digest":    {Source: source, Targets: targets, ExpectedDigest: strings.Repeat("0", 64)},
		"cancelled": {Source: source, Targets: targets, Wait: func(context.Context) error { return context.Canceled }},
	} {
		t.Run(name, func(t *testing.T) {
			_, errs := verifiedcopy.CopyMany(context.Background(), request)
			for i, target := range targets {
				if errs[i] == nil {
					t.Fatal(i, "published")
				}
				if _, err := os.Lstat(target.Destination); !os.IsNotExist(err) {
					t.Fatal(i, "final name exists", err)
				}
				if parts := partFiles(t, filepath.Dir(target.Destination)); len(parts) != 0 {
					t.Fatal(i, parts)
				}
			}
		})
	}
}

// A Wait that blocks mid-copy, while writers and their progress calls are
// in flight, leaves every target able to finish once it returns.
func TestCopyManyResumesEveryTargetAfterPause(t *testing.T) {
	dir := t.TempDir()
	data := []byte(strings.Repeat("x", 3*verifiedcopy.DefaultBufferSize))
	source := write(t, filepath.Join(dir, "card", "a.JPG"), data)
	targets := []verifiedcopy.Target{
		{Destination: filepath.Join(dir, "library", "a.JPG")},
		{Destination: filepath.Join(dir, "backup", "a.JPG")},
	}
	paused, resume := make(chan struct{}), make(chan struct{})
	calls, waits := 0, 0
	done := make(chan []error)
	go func() {
		_, errs := verifiedcopy.CopyMany(context.Background(), verifiedcopy.ManyRequest{
			Source:   source,
			Targets:  targets,
			Progress: func(int, verifiedcopy.Phase, int64, int64) { calls++ },
			Wait: func(context.Context) error {
				waits++
				if waits == 2 {
					close(paused)
					<-resume
				}
				return nil
			},
		})
		done <- errs
	}()
	<-paused
	time.Sleep(50 * time.Millisecond)
	close(resume)
	errs := <-done
	for i, err := range errs {
		if err != nil {
			t.Fatal(i, err)
		}
		if got, _ := os.ReadFile(targets[i].Destination); string(got) != string(data) {
			t.Fatal(i, "copy differs")
		}
	}
	if calls == 0 {
		t.Fatal("no progress reported")
	}
}

// A target whose writes fail mid-copy loses only its partial file. The source
// keeps being read for the other target, which is published.
func TestCopyManyWriteFailureLeavesOtherTarget(t *testing.T) {
	dir := t.TempDir()
	data := []byte(strings.Repeat("y", 2*verifiedcopy.DefaultBufferSize+5))
	source := write(t, filepath.Join(dir, "card", "a.JPG"), data)
	targets := []verifiedcopy.Target{
		{Destination: filepath.Join(dir, "library", "a.JPG")},
		{Destination: filepath.Join(dir, "backup", "a.JPG")},
	}
	defer verifiedcopy.FailWritesTo(targets[1].Destination)()
	_, errs := verifiedcopy.CopyMany(context.Background(), verifiedcopy.ManyRequest{Source: source, Targets: targets})
	if errs[0] != nil {
		t.Fatal(errs[0])
	}
	if errs[1] == nil || !strings.Contains(errs[1].Error(), "write destination") {
		t.Fatal("failed write not reported", errs[1])
	}
	if got, err := os.ReadFile(targets[0].Destination); err != nil || string(got) != string(data) {
		t.Fatal("surviving target differs", err)
	}
	if _, err := os.Lstat(targets[1].Destination); !os.IsNotExist(err) {
		t.Fatal("failed target published", err)
	}
	if parts := partFiles(t, filepath.Dir(targets[1].Destination)); len(parts) != 0 {
		t.Fatal("partial left behind", parts)
	}
}
