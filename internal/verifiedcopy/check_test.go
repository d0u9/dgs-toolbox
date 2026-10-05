package verifiedcopy_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/verifiedcopy"
)

func TestCheckRejectsFileReplacementWhileReading(t *testing.T) {
	root := t.TempDir()
	path := write(t, filepath.Join(root, "photo.JPG"), []byte("original photo"))
	replaced := false
	_, err := verifiedcopy.Check(context.Background(), root, path, nil, func(int64) {
		if replaced {
			return
		}
		replaced = true
		if err := os.Rename(path, path+".old"); err != nil {
			t.Fatal(err)
		}
		write(t, path, []byte("original photo"))
	})
	if err == nil {
		t.Fatal("replacement counted as verified")
	}
}

func TestCheckRejectsLinksButAcceptsSelectedRootLink(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "library")
	path := write(t, filepath.Join(actual, "date", "photo.JPG"), []byte("photo"))
	alias := filepath.Join(root, "chosen-library")
	if err := os.Symlink(actual, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := verifiedcopy.Check(context.Background(), alias, filepath.Join(alias, "date", "photo.JPG"), nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{path, filepath.Join(actual, "photo-link.JPG")}, {filepath.Dir(path), filepath.Join(actual, "date-link")}} {
		if err := os.Symlink(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(actual, "photo-link.JPG"), filepath.Join(actual, "date-link", "photo.JPG")} {
		if _, err := verifiedcopy.Check(context.Background(), actual, path, nil, nil); err == nil {
			t.Fatalf("accepted link %s", path)
		}
	}
}

func TestCopyExpectedDigestRejectsChangedBytesBeforePublication(t *testing.T) {
	root := t.TempDir()
	source := write(t, filepath.Join(root, "card", "photo.JPG"), []byte("old photograph"))
	checked, err := verifiedcopy.Check(context.Background(), "", source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	write(t, source, []byte("new photograph"))
	if err := os.Chtimes(source, checked.Source.ModTime(), checked.Source.ModTime()); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "library", "photo.JPG")
	_, err = verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: dest, ExpectedSource: checked.Source, ExpectedDigest: checked.Digest})
	if err == nil || !strings.Contains(err.Error(), "digest changed") {
		t.Fatalf("copy err = %v", err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("changed source published")
	}
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("partial copy retained after expected-digest failure")
	}
}

func TestCopyExpectedIdentityRejectsReplacementBeforePublication(t *testing.T) {
	root := t.TempDir()
	source := write(t, filepath.Join(root, "card", "photo.JPG"), []byte("photograph"))
	checked, err := verifiedcopy.Check(context.Background(), "", source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, source+".old"); err != nil {
		t.Fatal(err)
	}
	write(t, source, []byte("photograph"))
	if err := os.Chtimes(source, checked.Source.ModTime(), checked.Source.ModTime()); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "library", "photo.JPG")
	_, err = verifiedcopy.Copy(context.Background(), verifiedcopy.Request{Source: source, Destination: dest, ExpectedSource: checked.Source, ExpectedDigest: checked.Digest})
	if err == nil {
		t.Fatal("source identity replacement accepted")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("replacement published")
	}
}
