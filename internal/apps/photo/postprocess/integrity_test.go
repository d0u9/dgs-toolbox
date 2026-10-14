package postprocess

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestOrganizeSyncsBothDirectoriesAndRetainsActualPathOnFailure(t *testing.T) {
	for _, failure := range []string{"", "date", "original"} {
		t.Run("failure="+failure, func(t *testing.T) {
			root := t.TempDir()
			data := jpeg(t, "2026:10:10 11:12:13")
			original := writePhoto(t, root, "DCIM/a.JPG", data)
			plan := NewPlan(root, []File{{Source: "card/a.JPG", Path: original}})
			final := filepath.Join(root, "20261010", "a.JPG")
			previous := syncDirectory
			t.Cleanup(func() { syncDirectory = previous })
			var synced []string
			syncDirectory = func(path string) error {
				synced = append(synced, path)
				if failure == "date" && path == filepath.Dir(final) || failure == "original" && path == filepath.Dir(original) {
					return syscall.EIO
				}
				return previous(path)
			}
			outcome := Apply(plan).Files[0]
			if outcome.Final != final {
				t.Fatal("lost actual location", outcome)
			}
			if got, err := os.ReadFile(final); err != nil || string(got) != string(data) {
				t.Fatal("photo lost", err)
			}
			if _, err := os.Stat(original); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("move did not happen", err)
			}
			if failure != "" {
				if outcome.Status != Failed || !strings.Contains(outcome.Error, "sync directory") {
					t.Fatal("sync error hidden", outcome)
				}
			} else if outcome.Status != Moved || len(synced) != 2 || synced[0] != filepath.Dir(final) || synced[1] != filepath.Dir(original) {
				t.Fatal("missing directory sync", outcome, synced)
			}
		})
	}
}
