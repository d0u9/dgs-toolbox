package verifiedcopy_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/verifiedcopy"
)

func TestCopyAnnouncesPublishingAfterVerificationBeforePublication(t *testing.T) {
	for _, data := range []string{"", "synthetic photo"} {
		name := "photo"
		if data == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			source := write(t, filepath.Join(root, "card", "a.JPG"), []byte(data))
			destination := filepath.Join(root, "library", "a.JPG")
			publishing, published := 0, 0
			verifying := false
			_, err := verifiedcopy.Copy(context.Background(), verifiedcopy.Request{
				Source: source, Destination: destination, PreserveModTime: true, Mode: verifiedcopy.DefaultPhotoMode,
				Progress: func(phase verifiedcopy.Phase, done, total int64) {
					switch phase {
					case verifiedcopy.PhaseVerifying:
						verifying = true
						if publishing > 0 {
							t.Error("verification progress followed publishing")
						}
					case verifiedcopy.PhasePublishing:
						publishing++
						if !verifying || done != total {
							t.Error("publishing preceded readback")
						}
						if _, err := os.Stat(destination); !os.IsNotExist(err) {
							t.Error("publishing announced after final publication", err)
						}
						entries, err := os.ReadDir(filepath.Dir(destination))
						if err != nil {
							t.Error(err)
							return
						}
						if len(entries) != 1 {
							t.Error("temporary copy missing")
							return
						}
						bytes, err := os.ReadFile(filepath.Join(filepath.Dir(destination), entries[0].Name()))
						if err != nil || string(bytes) != data {
							t.Error("unverified temporary content", err)
						}
					case verifiedcopy.PhasePublished:
						published++
						if publishing != 1 {
							t.Error("final publication preceded publishing")
						}
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if publishing != 1 || published != 1 {
				t.Fatal("missing or duplicate publishing phases", publishing, published)
			}
		})
	}
}

// The publishing callback is still before the commit; a cancellation there
// must leave the verified temporary file unpublished and remove it.
func TestCopyCancelledAtPublishingLeavesFinalAbsent(t *testing.T) {
	root := t.TempDir()
	source := write(t, filepath.Join(root, "card", "a.JPG"), []byte("photo"))
	destination := filepath.Join(root, "library", "a.JPG")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	publishing := false
	_, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{
		Source: source, Destination: destination,
		Progress: func(phase verifiedcopy.Phase, _, _ int64) {
			if phase == verifiedcopy.PhasePublishing {
				publishing = true
				cancel()
			}
		},
	})
	if !publishing {
		t.Fatal("publishing phase missing")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation before commit ignored", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("cancelled copy published", err)
	}
	if parts := partFiles(t, filepath.Dir(destination)); len(parts) != 0 {
		t.Fatal("cancelled copy left partials", parts)
	}
}
