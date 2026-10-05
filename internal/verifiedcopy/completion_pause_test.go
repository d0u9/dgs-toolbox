package verifiedcopy_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

// Publication finishes after another target has entered a paused Wait. Wait
// owns the progress mutex, but the durable result must still be delivered.
func TestCopyManyReportsCompleteWhileOtherReadbackPaused(t *testing.T) {
	for _, cancelPaused := range []bool{false, true} {
		name := "resume"
		if cancelPaused {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			source := write(t, filepath.Join(root, "card", "a.JPG"), []byte("photo"))
			targets := []verifiedcopy.Target{
				{Destination: filepath.Join(root, "library", "a.JPG")},
				{Destination: filepath.Join(root, "backup", "a.JPG")},
			}
			syncing, finishSync := make(chan struct{}), make(chan struct{})
			paused, resume := make(chan struct{}), make(chan struct{})
			restoreSync := verifiedcopy.ReplacePublishDirectorySync(func(path string) error {
				if path == filepath.Dir(targets[0].Destination) {
					close(syncing)
					<-finishSync
				}
				return verifiedcopy.SyncDirectory(path)
			})
			restoreOpen := verifiedcopy.ReplaceReadBackOpener(func(path string) (*os.File, error) {
				if strings.HasPrefix(path, filepath.Dir(targets[1].Destination)+string(os.PathSeparator)) {
					<-syncing
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
			done := make(chan struct{})
			var once sync.Once
			var errs []error
			publications := make([]int, len(targets))
			go func() {
				_, errs = verifiedcopy.CopyMany(ctx, verifiedcopy.ManyRequest{
					Source: source, Targets: targets,
					Progress: func(target int, phase verifiedcopy.Phase, _, _ int64) {
						if phase == verifiedcopy.PhasePublished {
							publications[target]++
						}
					},
					Wait: func(ctx context.Context) error {
						select {
						case <-syncing:
							once.Do(func() { close(paused) })
							select {
							case <-resume:
								return ctx.Err()
							case <-ctx.Done():
								return ctx.Err()
							}
						default:
							return ctx.Err()
						}
					},
					Complete: func(target int, result verifiedcopy.Result, err error) { completed <- completion{target, result, err} },
				})
				close(done)
			}()
			syncReleased, resumed := false, false
			t.Cleanup(func() {
				cancel()
				if !syncReleased {
					close(finishSync)
				}
				if !resumed {
					close(resume)
				}
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("copy did not stop")
				}
				restoreOpen()
				restoreSync()
			})
			select {
			case <-paused:
			case <-time.After(5 * time.Second):
				t.Fatal("other readback did not pause")
			}
			close(finishSync)
			syncReleased = true
			select {
			case first := <-completed:
				if first.target != 0 || first.err != nil || first.result.Digest == "" {
					t.Fatal("invalid completed publication", first)
				}
				got, err := os.ReadFile(targets[0].Destination)
				if err != nil || string(got) != "photo" {
					t.Fatal("completion preceded verified publication", err)
				}
			case <-time.After(time.Second):
				t.Fatal("published target completion waited for another target's paused Wait")
			}
			select {
			case <-done:
				t.Fatal("batch ended with readback still paused")
			default:
			}
			if cancelPaused {
				cancel()
			} else {
				close(resume)
				resumed = true
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("copy did not finish")
			}
			if errs[0] != nil {
				t.Fatal("completed publication was lost", errs[0])
			}
			if cancelPaused {
				if !errors.Is(errs[1], context.Canceled) {
					t.Fatal("paused target did not cancel", errs[1])
				}
			} else if errs[1] != nil {
				t.Fatal("paused target did not resume", errs[1])
			}
			select {
			case second := <-completed:
				if second.target != 1 || second.err != errs[1] {
					t.Fatal("invalid second completion", second)
				}
			default:
				t.Fatal("second completion missing")
			}
			select {
			case extra := <-completed:
				t.Fatal("duplicate completion", extra)
			default:
			}
			if publications[0] != 1 || (!cancelPaused && publications[1] != 1) || (cancelPaused && publications[1] != 0) {
				t.Fatal("publication progress missing or duplicated", publications)
			}
		})
	}
}
