package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

func TestMultipleResumeRejectsSourceChangedBetweenTargets(t *testing.T) {
	plans := multiplePlans(t)
	for i := range plans {
		plans[i].Jobs = plans[i].Jobs[:1]
		plans[i].Incremental = false
		plans[i].Workers = 1
	}
	first, err := RunMultiple(context.Background(), plans, Copy, MultipleHooks{})
	if err != nil || first.Files[0].Phase != PhaseComplete {
		t.Fatal("initial import failed", err)
	}
	controller := &Controller{}
	controller.Pause()
	defer controller.Resume()
	plans[1].Controller = controller
	changed := false
	var mutationErr error
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := RunMultiple(ctx, plans, Copy, MultipleHooks{Event: func(target int, event Event) {
		if target == 1 && event.Phase == PhaseCheckingDestination && event.Bytes == 0 && !changed {
			// The first target already hashed Source. The second waits before its check.
			changed = true
			mutationErr = os.WriteFile(plans[0].Jobs[0].Source, []byte("new photo content"), 0o644)
			controller.Resume()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if mutationErr != nil {
		t.Fatal(mutationErr)
	}
	if !changed {
		t.Fatal("source did not change between target checks")
	}
	if result.Files[0].Phase != PhaseFailed || result.Targets[1].Files[0].Existing {
		t.Fatalf("changed source trusted as resumed: %#v", result)
	}
	for _, plan := range plans {
		if got, err := os.ReadFile(plan.Jobs[0].Destination); err != nil || string(got) != "photo a" {
			t.Fatal("previous copy changed", err)
		}
	}
	if got, err := os.ReadFile(plans[0].Jobs[0].Source); err != nil || string(got) != "new photo content" {
		t.Fatal("changed source lost", err)
	}
}

func TestSourceChecksRevalidateCachedFile(t *testing.T) {
	for _, change := range []string{"unchanged", "rewrite", "modtime", "replace", "remove", "symlink"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "a.JPG")
			writeFile(t, source, "photo a")
			cache := &sourceChecks{}
			reads := 0
			progress := func(done int64) {
				if done == 0 {
					reads++
				}
			}
			first, err := cache.check(context.Background(), root, source, nil, progress)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "rewrite":
				writeFile(t, source, "changed photo content")
			case "modtime":
				later := first.Source.ModTime().Add(time.Hour)
				if err := os.Chtimes(source, later, later); err != nil {
					t.Fatal(err)
				}
			case "replace":
				if err := os.Rename(source, source+".old"); err != nil {
					t.Fatal(err)
				}
				writeFile(t, source, "photo a")
				if err := os.Chtimes(source, first.Source.ModTime(), first.Source.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "remove":
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(source, source+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(source+".old", source); err != nil {
					t.Fatal(err)
				}
			}
			checked, err := cache.check(context.Background(), root, source, nil, progress)
			if change == "unchanged" {
				if err != nil || checked.Digest != first.Digest || reads != 1 {
					t.Fatal("unchanged source was not shared", err, reads)
				}
			} else if err == nil {
				t.Fatal("changed source reused cached evidence")
			}
		})
	}
}

// A real first copy publishes, while the injected engine delays the other
// target. The coordinator must persist the first outcome before release.
func TestMultipleSavesPublishedTargetWhileAnotherIsBlocked(t *testing.T) {
	plans := multiplePlans(t)
	saved := make(chan State, 4)
	blocked, release := make(chan struct{}), make(chan struct{})
	for i := range plans {
		plans[i].Jobs = plans[i].Jobs[:1]
		plans[i].Incremental = false
		plans[i].Workers = 1
		plans[i].StateInterval = 20 * time.Millisecond
		plans[i].copyMany = func(ctx context.Context, request verifiedcopy.ManyRequest) (verifiedcopy.Result, []error) {
			errs := make([]error, len(request.Targets))
			var result verifiedcopy.Result
			for n, target := range request.Targets {
				if n == 1 {
					close(blocked)
					<-release
				}
				copied, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{
					Source: request.Source, Destination: target.Destination,
					ExpectedSource: request.ExpectedSource, ExpectedDigest: request.ExpectedDigest,
					PreserveModTime: request.PreserveModTime, Mode: request.Mode,
					Root: target.Root, ExpectedRoot: target.ExpectedRoot,
				})
				errs[n] = err
				if err == nil {
					result = copied
				}
				if request.Complete != nil {
					request.Complete(n, copied, err)
				}
			}
			return result, errs
		}
	}
	plans[0].saveState = func(path string, state *State) error {
		if err := writeState(path, state); err != nil {
			return err
		}
		if state.Files[0].Phase == PhaseComplete {
			snapshot := *state
			snapshot.Files = append([]FileResult(nil), state.Files...)
			select {
			case saved <- snapshot:
			default:
			}
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var result MultipleResult
	var runErr error
	go func() { result, runErr = RunMultiple(ctx, plans, Copy, MultipleHooks{}); close(done) }()
	released := false
	t.Cleanup(func() {
		cancel()
		if !released {
			close(release)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("run did not stop")
		}
	})
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("second target did not block")
	}
	select {
	case state := <-saved:
		if state.Files[0].SourceHash == "" || state.Files[0].SourceHash != state.Files[0].DestinationHash {
			t.Fatal("saved unverified outcome")
		}
		disk, err := LoadState(plans[0].StatePath)
		if err != nil || disk.Files[0].Phase != PhaseComplete {
			t.Fatal("completion missing on disk", err)
		}
	case <-time.After(time.Second):
		t.Fatal("published first target was not saved while second target blocked")
	}
	select {
	case <-done:
		t.Fatal("batch completed while second target blocked")
	default:
	}
	close(release)
	released = true
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("batch did not resume")
	}
	if runErr != nil || result.Files[0].Phase != PhaseComplete {
		t.Fatal("final batch failed", runErr, result.Files)
	}
	// With Incremental disabled and Keep both, the persisted record must prevent
	// another copy of the already published first target.
	for i := range plans {
		plans[i].copyMany = nil
		plans[i].Conflict = KeepBoth
	}
	retried, err := RunMultiple(context.Background(), plans, Copy, MultipleHooks{})
	if err != nil || !retried.Targets[0].Files[0].Existing {
		t.Fatal("retry did not resume published target", err)
	}
	entries, err := os.ReadDir(filepath.Dir(plans[0].Jobs[0].Destination))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "-1.") {
			t.Fatal("retry duplicated successful target", entry.Name())
		}
	}
}
