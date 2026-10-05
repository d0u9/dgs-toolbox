// Package pagecache keeps the pictures of PDF pages, by the PDF's SHA-256,
// page and size, so a page is drawn once per machine rather than once per
// look. It is a cache: nothing in it is truth, a file it cannot read is a
// miss, and deleting the whole directory costs only a redraw.
package pagecache

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// Version names the layout and the drawing. An older one sits under another
// name and is simply never read; there is nothing to migrate in a cache.
// pages-v2 draws a filled form's fields, which pages-v1 left empty.
const Version = "pages-v2"

var (
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	sizePattern   = regexp.MustCompile(`^[a-z]+$`)
)

// Store is a cache directory. With no Dir it keeps nothing.
type Store struct{ Dir string }

func (s Store) path(digest string, page int, size string) (string, error) {
	if s.Dir == "" {
		return "", fmt.Errorf("no cache directory")
	}
	if !digestPattern.MatchString(digest) || page < 1 || !sizePattern.MatchString(size) {
		return "", fmt.Errorf("not a digest, page and size: %q %d %q", digest, page, size)
	}
	return filepath.Join(s.Dir, Version, digest[:2], digest, strconv.Itoa(page)+"-"+size+".jpg"), nil
}

// Load is one page's JPEG at a size, counting pages from 1. ok is false for
// anything not kept or not readable.
func (s Store) Load(digest string, page int, size string) (jpeg []byte, ok bool) {
	path, err := s.path(digest, page, size)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// Save keeps one page's JPEG, through a temporary file and a rename so a
// reader never sees half of one.
func (s Store) Save(digest string, page int, size string, jpeg []byte) error {
	path, err := s.path(digest, page, size)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".page-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(jpeg); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
