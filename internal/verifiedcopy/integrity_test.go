package verifiedcopy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPublishRefusesUnsafeFallback(t *testing.T) {
	for _, linkError := range []error{syscall.ENOTSUP, syscall.EPERM, os.ErrExist} {
		t.Run(linkError.Error(), func(t *testing.T) {
			dir := t.TempDir()
			partial, final := filepath.Join(dir, "partial"), filepath.Join(dir, "a.JPG")
			if err := os.WriteFile(partial, []byte("photo A"), 0600); err != nil {
				t.Fatal(err)
			}
			err := publish(partial, final, func(string, string) error { return syscall.ENOTSUP }, func(string, string) error {
				if err := os.WriteFile(final, []byte("photo B"), 0600); err != nil {
					t.Fatal(err)
				}
				return linkError
			})
			if err == nil {
				t.Fatal("unsafe publication succeeded")
			}
			for path, want := range map[string]string{partial: "photo A", final: "photo B"} {
				if got, err := os.ReadFile(path); err != nil || string(got) != want {
					t.Fatalf("photo changed: %q %v", got, err)
				}
			}
		})
	}
}

func TestNestedDirectorySyncFailurePreventsPublicationAndRetries(t *testing.T) {
	root := t.TempDir()
	source, library := filepath.Join(root, "source.JPG"), filepath.Join(root, "library")
	if err := os.WriteFile(source, []byte("synthetic photo"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(library, 0755); err != nil {
		t.Fatal(err)
	}
	identity, err := os.Stat(library)
	if err != nil {
		t.Fatal(err)
	}
	previous := syncDirectory
	t.Cleanup(func() { syncDirectory = previous })
	fail := true
	var synced []string
	syncDirectory = func(path string) error {
		synced = append(synced, path)
		if fail && path == filepath.Join(library, "DCIM") {
			return syscall.EIO
		}
		return previous(path)
	}
	final := filepath.Join(library, "DCIM", "100CAM", "a.JPG")
	request := Request{Source: source, Destination: final, Root: library, ExpectedRoot: identity}
	for range 2 {
		if _, err := Copy(context.Background(), request); !errors.Is(err, syscall.EIO) {
			t.Fatalf("sync failure = %v", err)
		}
		if _, err := os.Stat(final); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("published before parent sync", err)
		}
	}
	fail, synced = false, nil
	if _, err := Copy(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{library, filepath.Join(library, "DCIM")} {
		found := false
		for _, path := range synced {
			found = found || path == parent
		}
		if !found {
			t.Fatalf("parent %s was not synced", filepath.Base(parent))
		}
	}
}
