package verifiedcopy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

func TestCopyManyCompletesPublishedTargetBeforeOtherReadback(t *testing.T) {
	root := t.TempDir()
	source := write(t, filepath.Join(root, "card", "a.JPG"), []byte("photo"))
	targets := []verifiedcopy.Target{
		{Destination: filepath.Join(root, "library", "a.JPG")},
		{Destination: filepath.Join(root, "backup", "a.JPG")},
	}
	blocked, release := make(chan struct{}), make(chan struct{})
	restore := verifiedcopy.ReplaceReadBackOpener(func(path string) (*os.File, error) {
		if strings.HasPrefix(path, filepath.Dir(targets[1].Destination)+string(os.PathSeparator)) {
			close(blocked)
			<-release
		}
		return os.Open(path)
	})
	ctx, cancel := context.WithCancel(context.Background())
	type completion struct {
		target int
		result verifiedcopy.Result
		err    error
	}
	completed := make(chan completion, 2)
	done := make(chan []error, 1)
	go func() {
		_, errs := verifiedcopy.CopyMany(ctx, verifiedcopy.ManyRequest{
			Source: source, Targets: targets,
			Complete: func(target int, result verifiedcopy.Result, err error) { completed <- completion{target, result, err} },
		})
		done <- errs
	}()
	released := false
	t.Cleanup(func() {
		cancel()
		if !released {
			close(release)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("copy did not stop")
		}
		restore()
	})
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("second readback did not block")
	}
	select {
	case first := <-completed:
		if first.target != 0 || first.err != nil || first.result.Digest == "" {
			t.Fatal("first completion invalid", first)
		}
		got, err := os.ReadFile(targets[0].Destination)
		if err != nil || string(got) != "photo" {
			t.Fatal("completion preceded verified publication", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first target completion waited for blocked readback")
	}
	if _, err := os.Stat(targets[1].Destination); !os.IsNotExist(err) {
		t.Fatal("blocked target published", err)
	}
	close(release)
	released = true
	select {
	case second := <-completed:
		if second.target != 1 || second.err != nil {
			t.Fatal("second completion invalid", second)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second completion missing")
	}
	// Leave the final return for cleanup to receive, then ensure no extra callback.
	select {
	case extra := <-completed:
		t.Fatal("duplicate completion", extra)
	default:
	}
}

func TestCopyManyCompletesFailedTargetsExactlyOnce(t *testing.T) {
	for _, failure := range []string{"source", "destination", "digest", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			source := write(t, filepath.Join(root, "card", "a.JPG"), []byte("photo"))
			targets := []verifiedcopy.Target{{Destination: filepath.Join(root, "a", "a.JPG")}, {Destination: filepath.Join(root, "b", "a.JPG")}}
			request := verifiedcopy.ManyRequest{Source: source, Targets: targets}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch failure {
			case "source":
				request.Source += ".missing"
			case "destination":
				write(t, targets[1].Destination, []byte("existing"))
			case "digest":
				request.ExpectedDigest = strings.Repeat("0", 64)
			case "cancel":
				cancel()
			}
			counts := make([]int, len(targets))
			reported := make([]error, len(targets))
			request.Complete = func(target int, result verifiedcopy.Result, err error) {
				counts[target]++
				reported[target] = err
				if err != nil {
					if result.Source != nil || result.Digest != "" {
						t.Error("failure included success evidence")
					}
					var parts []string
					directory := filepath.Dir(targets[target].Destination)
					if _, statErr := os.Stat(directory); statErr == nil {
						parts = partFiles(t, directory)
					} else if !os.IsNotExist(statErr) {
						t.Error(statErr)
					}
					if len(parts) > 0 {
						t.Error("completion preceded partial cleanup", parts)
					}
				}
			}
			_, errs := verifiedcopy.CopyMany(ctx, request)
			for i := range targets {
				if counts[i] != 1 || reported[i] != errs[i] {
					t.Fatal("outcomes not reported exactly once", counts, reported, errs)
				}
			}
		})
	}
}
