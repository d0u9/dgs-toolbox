// Package thumbcache stores the two thumbnail sizes on the local disk, beside
// the index and outside the Box.
//
// It follows Lightroom's arrangement — previews next to the catalogue, several
// sizes, discardable, the large ones expiring — with the top of the pyramid
// left off. Nothing here is truth: every picture is derivable from the scan's
// own bytes, so losing the whole directory costs a re-read and nothing else.
//
// The two-character shard in each path exists because a Box of several thousand
// scans would otherwise put several thousand files in one directory, which some
// filesystems handle badly and every file browser handles slowly.
package thumbcache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/thumb"
)

// The two sizes, named as the pages ask for them.
const (
	SizeGrid    = "300"
	SizePreview = "1600"
)

// MediaType is what both sizes are.
const MediaType = "image/jpeg"

// Store is one Box's pictures.
type Store struct {
	// Directory is <box.cache_dir>/<hash of root>.
	Directory string
}

// New opens the store for a Box root under cacheDir. Nothing is created until
// something is written.
func New(cacheDir, root string) Store {
	return Store{Directory: filepath.Join(cacheDir, box.CacheKey(root))}
}

// PathFor is where one picture lives.
//
// Grid thumbnails and previews are in separate trees because they expire
// differently: a grid thumbnail is a few KB and is never dropped, and a preview
// is dropped after box.preview.keep days without use. Sweeping one must never
// be able to reach the other.
func (s Store) PathFor(fullDigest, size string) (string, error) {
	return s.PathForPage(fullDigest, size, 1)
}

// PathForPage is where one page's picture lives.
//
// Page 1 is written under the name PathFor has always used, with no page in it.
// That is not tidiness: a cache written by an older build must still answer for
// the picture every grid asks for, and renaming it would silently re-read every
// scan in the Box.
func (s Store) PathForPage(fullDigest, size string, page int) (string, error) {
	short := digest.Short(fullDigest, 0)
	if len(short) < 2 {
		return "", fmt.Errorf("digest %q is too short to store a picture for", fullDigest)
	}
	var tree string
	switch size {
	case SizeGrid:
		tree = "thumbs"
	case SizePreview:
		tree = "previews"
	default:
		return "", fmt.Errorf("no such size %q", size)
	}
	if page < 1 {
		page = 1
	}
	name := digest.Short(fullDigest, 8) + "-" + size + ".jpg"
	if page > 1 {
		// Every picture of a page other than the first is a picture of the one
		// scan being looked at, whatever its size, so it lives in the tree that
		// expires. A fifty-page document would otherwise leave fifty files that
		// are never swept for one afternoon's reading.
		tree = "previews"
		name = fmt.Sprintf("%s-p%d-%s.jpg", digest.Short(fullDigest, 8), page, size)
	}
	return filepath.Join(s.Directory, tree, short[:2], name), nil
}

// Save writes both sizes for one scan's first page.
func (s Store) Save(fullDigest string, pair thumb.Pair) error {
	return s.SavePage(fullDigest, 1, pair)
}

// SavePage writes both sizes for one page of a scan.
func (s Store) SavePage(fullDigest string, page int, pair thumb.Pair) error {
	if len(pair.Grid) > 0 {
		if err := s.write(fullDigest, SizeGrid, page, pair.Grid); err != nil {
			return err
		}
	}
	if len(pair.Preview) > 0 {
		if err := s.write(fullDigest, SizePreview, page, pair.Preview); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) write(fullDigest, size string, page int, data []byte) error {
	path, err := s.PathForPage(fullDigest, size, page)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Whole file into a temporary beside it and a rename, with no sync. A lost
	// picture costs one re-read; paying for durability would buy nothing.
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

// ErrNotStored is returned when there is no picture of that size, which happens
// for a scan marked needs-render and for one whose preview has been swept.
var ErrNotStored = errors.New("no picture stored")

// Load reads one scan's first-page picture and touches it, so a preview in use
// is not swept.
func (s Store) Load(fullDigest, size string) ([]byte, error) {
	return s.LoadPage(fullDigest, size, 1)
}

// LoadPage reads one page's picture and touches it.
func (s Store) LoadPage(fullDigest, size string, page int) ([]byte, error) {
	path, err := s.PathForPage(fullDigest, size, page)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotStored, fullDigest)
		}
		return nil, err
	}
	// Used means kept. The sweep below reads this time, and a preview being
	// looked at today must not be dropped tonight.
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return data, nil
}

// Sweep drops previews that have not been used for keep days and reports how
// many went.
//
// Grid thumbnails are never swept: they are a few KB each and they are what a
// grid is made of, so dropping one only means fetching the scan again to draw
// the same picture. keep of zero sweeps nothing, which is how
// box.preview.keep = 0 means "keep previews indefinitely".
func (s Store) Sweep(keep int, now time.Time) (int, error) {
	if keep <= 0 {
		return 0, nil
	}
	previews := filepath.Join(s.Directory, "previews")
	cutoff := now.AddDate(0, 0, -keep)
	dropped := 0
	err := filepath.WalkDir(previews, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err == nil {
				dropped++
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return dropped, err
	}
	return dropped, nil
}

// Drop removes every picture stored for a scan: its grid thumbnail, its
// preview and every page drawn past the first. It reports how many files
// went. A picture that was never stored is not an error — dropping is how a
// broken picture is made to be drawn again, and nothing there means nothing to
// drop.
func (s Store) Drop(fullDigest string) (int, error) {
	short := digest.Short(fullDigest, 0)
	if len(short) < 2 {
		return 0, fmt.Errorf("digest %q is too short to hold a picture", fullDigest)
	}
	prefix := digest.Short(fullDigest, 8) + "-"
	dropped := 0
	for _, tree := range []string{"thumbs", "previews"} {
		entries, err := os.ReadDir(filepath.Join(s.Directory, tree, short[:2]))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return dropped, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".jpg") {
				continue
			}
			if err := os.Remove(filepath.Join(s.Directory, tree, short[:2], entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return dropped, err
			}
			dropped++
		}
	}
	return dropped, nil
}
