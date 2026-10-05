package verifiedcopy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedRootNeverRecreatesOrPublishesIntoReplacement(t *testing.T) {
	for _, when := range []string{"before copy", "before publication"} {
		t.Run(when, func(t *testing.T) {
			base := t.TempDir()
			root, source := filepath.Join(base, "library"), filepath.Join(base, "source.jpg")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, []byte("photo"), 0o644); err != nil {
				t.Fatal(err)
			}
			identity, err := os.Stat(root)
			if err != nil {
				t.Fatal(err)
			}
			replaced := false
			replace := func() {
				if replaced {
					return
				}
				replaced = true
				if err := os.Rename(root, root+"-detached"); err != nil {
					t.Fatal(err)
				}
				if when == "before publication" {
					if err := os.Mkdir(root, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			if when == "before copy" {
				replace()
			}
			_, err = Copy(context.Background(), Request{Source: source, Destination: filepath.Join(root, "nested", "photo.jpg"),
				Root: root, ExpectedRoot: identity, Progress: func(phase Phase, done, total int64) {
					if when == "before publication" && phase == PhaseVerifying && done == total {
						replace()
					}
				}})
			if err == nil {
				t.Fatal("lost root accepted")
			}
			if when == "before copy" {
				if _, err := os.Stat(root); !os.IsNotExist(err) {
					t.Fatal("pinned root recreated", err)
				}
			} else if entries, _ := os.ReadDir(root); len(entries) != 0 {
				t.Fatal("replacement root received a file", entries)
			}
			if _, err := os.Stat(filepath.Join(root+"-detached", "nested", "photo.jpg")); !os.IsNotExist(err) {
				t.Fatal("copy published after losing root identity", err)
			}
		})
	}
}
