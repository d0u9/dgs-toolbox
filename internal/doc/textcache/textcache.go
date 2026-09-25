// Package textcache keeps the text read off PDFs, by the PDF's SHA-256 and
// page, so a page is recognised once per machine rather than once per look.
// It is a cache: nothing in it is truth, a file it cannot read is a miss, and
// deleting the whole directory costs only a re-read.
package textcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"dgs-toolbox/internal/doc/ocr"
)

// Version names the layout. An older layout sits under another name and is
// simply never read; there is nothing to migrate in a cache.
const Version = "text-v1"

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Store is a cache directory.
type Store struct{ Dir string }

// entry is one page as kept: the page, and how many pages its PDF has.
type entry struct {
	Count int      `json:"count"`
	Page  ocr.Page `json:"page"`
}

func (s Store) path(digest string, page int) (string, error) {
	if s.Dir == "" {
		return "", fmt.Errorf("no cache directory")
	}
	if !digestPattern.MatchString(digest) || page < 0 {
		return "", fmt.Errorf("not a digest and page: %q %d", digest, page)
	}
	return filepath.Join(s.Dir, Version, digest[:2], digest, strconv.Itoa(page)+".json"), nil
}

// Load is one page's text and its PDF's page count, counting pages from 0.
// ok is false for anything not kept or not readable.
func (s Store) Load(digest string, page int) (text ocr.Page, count int, ok bool) {
	path, err := s.path(digest, page)
	if err != nil {
		return ocr.Page{}, 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ocr.Page{}, 0, false
	}
	var e entry
	if json.Unmarshal(data, &e) != nil || e.Count <= page {
		return ocr.Page{}, 0, false
	}
	return e.Page, e.Count, true
}

// Save keeps one page, through a temporary file and a rename so a reader
// never sees half of one.
func (s Store) Save(digest string, page, count int, text ocr.Page) error {
	path, err := s.path(digest, page)
	if err != nil {
		return err
	}
	data, err := json.Marshal(entry{Count: count, Page: text})
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
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
