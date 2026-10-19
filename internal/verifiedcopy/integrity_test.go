package verifiedcopy

import (
	"context"
	"errors"
	"fmt"
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

func TestAllowCheckRenameLocksTheRootAndNeverReplaces(t *testing.T) {
	root := t.TempDir()
	unsupportedOp := func(string, string) error { return syscall.ENOTSUP }
	write := func(name, body string) string {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	partial := write("partial", "photo A")
	final := filepath.Join(root, "20261015", "a.JPG")
	if err := publish(partial, final, unsupportedOp, unsupportedOp); !errors.Is(err, ErrNoExclusiveRename) {
		t.Fatalf("not opted in: %v", err)
	}
	release, err := AllowCheckRename(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AllowCheckRename(root); err == nil {
		t.Fatal("a second run took the same root")
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := publish(partial, final, unsupportedOp, unsupportedOp); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(final); string(got) != "photo A" {
		t.Fatalf("published %q", got)
	}
	// An existing name is still refused, not replaced.
	other := write("other", "photo B")
	if err := publish(other, final, unsupportedOp, unsupportedOp); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing final: %v", err)
	}
	if got, _ := os.ReadFile(final); string(got) != "photo A" {
		t.Fatal("existing photo replaced")
	}
	// Outside the root, and after release, the fallback is refused again.
	outside := filepath.Join(t.TempDir(), "b.JPG")
	if err := publish(other, outside, unsupportedOp, unsupportedOp); !errors.Is(err, ErrNoExclusiveRename) {
		t.Fatalf("outside root: %v", err)
	}
	release()
	if _, err := os.Stat(filepath.Join(root, LockName)); !os.IsNotExist(err) {
		t.Fatal("lock left after release")
	}
	if err := publish(other, filepath.Join(root, "c.JPG"), unsupportedOp, unsupportedOp); !errors.Is(err, ErrNoExclusiveRename) {
		t.Fatalf("after release: %v", err)
	}
}

func TestAllowCheckRenameTakesOverOnlyADeadLocalOwner(t *testing.T) {
	host, _ := os.Hostname()
	for held, takeOver := range map[string]bool{
		host + " 2147483646\n":                      true,  // no such process on this host
		host + " " + fmt.Sprint(os.Getpid()) + "\n": false, // alive
		"other-host 2147483646\n":                   false, // cannot be checked from here
		"garbled":                                   false,
	} {
		root := t.TempDir()
		lock := filepath.Join(root, LockName)
		if err := os.WriteFile(lock, []byte(held), 0o644); err != nil {
			t.Fatal(err)
		}
		release, err := AllowCheckRename(root)
		if (err == nil) != takeOver {
			t.Fatalf("lock %q: err %v", held, err)
		}
		if err != nil {
			if got, _ := os.ReadFile(lock); string(got) != held {
				t.Fatalf("refused lock %q was changed to %q", held, got)
			}
			continue
		}
		// Release removes only its own lock, not one that replaced it.
		if err := os.WriteFile(lock, []byte("newer 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		release()
		if got, _ := os.ReadFile(lock); string(got) != "newer 1\n" {
			t.Fatal("release removed another owner's lock")
		}
	}
}

func TestCheckPublicationPassesHereAndLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	if err := CheckPublication(dir); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("probe left %d files", len(entries))
	}
}
