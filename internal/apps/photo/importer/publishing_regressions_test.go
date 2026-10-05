package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/verifiedcopy"
)

// Reaching the expected byte count is not proof of a matching readback.
// Failed and cancelled checks must never advertise the publishing phase.
func TestRunDoesNotAnnouncePublishingBeforeReadbackPasses(t *testing.T) {
	for _, failure := range []string{"mismatch", "open", "empty-open", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			source, destination := filepath.Join(root, "card", "a.JPG"), filepath.Join(root, "library", "a.JPG")
			data := "photo"
			if failure == "empty-open" {
				data = ""
			}
			writeFile(t, source, data)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			plan := Plan{Jobs: []Job{{Source: source, Destination: destination}}, Operation: Copy, Conflict: Skip}
			var injectedErr error
			injected := false
			plan.copyMany = func(ctx context.Context, request verifiedcopy.ManyRequest) (verifiedcopy.Result, []error) {
				progress := request.Progress
				request.Progress = func(target int, phase verifiedcopy.Phase, done, total int64) {
					if phase == verifiedcopy.PhaseVerifying && !injected {
						if failure == "cancel" && done >= total {
							injected = true
							cancel()
						} else if failure != "cancel" && done == 0 {
							injected = true
							entries, err := os.ReadDir(filepath.Dir(destination))
							injectedErr = err
							for _, entry := range entries {
								if strings.HasSuffix(entry.Name(), verifiedcopy.PartSuffix) {
									path := filepath.Join(filepath.Dir(destination), entry.Name())
									if failure == "mismatch" {
										injectedErr = os.WriteFile(path, []byte("other"), 0o600)
									} else {
										injectedErr = os.Remove(path)
									}
								}
							}
						}
					}
					progress(target, phase, done, total)
				}
				return verifiedcopy.CopyMany(ctx, request)
			}
			events := make(chan Event, 64)
			result := Run(ctx, plan, events)
			close(events)
			if injectedErr != nil {
				t.Fatal(injectedErr)
			}
			if !injected {
				t.Fatal("failure was not injected")
			}
			file := result.Files[0]
			if file.Phase != PhaseFailed {
				t.Fatal("failed readback accepted", file.Phase)
			}
			if failure == "mismatch" && !strings.Contains(file.Error, "mismatch") {
				t.Fatal("expected hash mismatch", file.Error)
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatal("failed check published a file", err)
			}
			var phases []Phase
			premature := false
			for event := range events {
				phases = append(phases, event.Phase)
				premature = premature || event.Phase == PhasePublishing
			}
			if premature {
				t.Fatalf("readback failure advertised publishing: %v", phases)
			}
		})
	}
}
