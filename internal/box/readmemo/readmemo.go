// Package readmemo remembers what reading an inbox file produced, on the local
// disk, so that opening intake again does not read every file again.
//
// Reading an inbox file means its whole bytes over the network, two digests and
// two pictures. For two hundred files on a NAS that is several seconds before
// the list can show anything, spent arriving at the answer already given last
// time. A memo keyed by the file's inbox path, size and modification time hands
// that answer back instead; a file whose triple moved is read again.
//
// It is kept apart from the Box's picture cache on purpose. Those pictures
// belong to the Box, and a scan that is rejected must not leave pictures of
// itself there. Nothing here is truth either: losing the whole directory costs
// one slow start and nothing else.
package readmemo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/scanread"
)

// version is the layout this build writes. A memo carrying any other number is
// a miss, never a migration. Version 2: pictures are drawn turned as the
// page's /Rotate says, so a version 1 picture of a turned page is wrong.
const version = 2

const (
	metaName    = "read.json"
	gridName    = "grid.jpg"
	previewName = "preview.jpg"
)

// Store is one inbox's memos.
type Store struct {
	// Directory is <box.cache_dir>/inbox/<hash of the inbox path>.
	Directory string
}

// New opens the store for an inbox under cacheDir. Nothing is created until
// something is written.
func New(cacheDir, inbox string) Store {
	return Store{Directory: filepath.Join(cacheDir, "inbox", box.CacheKey(inbox))}
}

type meta struct {
	Version  int             `json:"version"`
	Relative string          `json:"relative"`
	Size     int64           `json:"size"`
	ModTime  int64           `json:"mod_time"`
	Result   scanread.Result `json:"result"`
}

// Key is the directory name for one inbox-relative path.
func Key(relative string) string {
	sum := sha256.Sum256([]byte(relative))
	return hex.EncodeToString(sum[:])[:24]
}

// Load returns the remembered read of relative, if there is one for exactly
// this size and modification time.
func (s Store) Load(relative string, size, modTime int64) (scanread.Result, bool) {
	directory := filepath.Join(s.Directory, Key(relative))
	data, err := os.ReadFile(filepath.Join(directory, metaName))
	if err != nil {
		return scanread.Result{}, false
	}
	var found meta
	if err := json.Unmarshal(data, &found); err != nil ||
		found.Version != version || found.Relative != relative ||
		found.Size != size || found.ModTime != modTime {
		return scanread.Result{}, false
	}
	result := found.Result
	result.Thumbs.Grid, _ = os.ReadFile(filepath.Join(directory, gridName))
	result.Thumbs.Preview, _ = os.ReadFile(filepath.Join(directory, previewName))
	if !result.NeedsRender && len(result.Thumbs.Grid) == 0 {
		// The pictures went missing from under the memo. Reading again is
		// cheaper to reason about than serving a scan with no picture.
		return scanread.Result{}, false
	}
	return result, true
}

// Save remembers one read. The embedded images are not kept: they are the
// input to the digests and pictures already in the result.
func (s Store) Save(relative string, size, modTime int64, result scanread.Result) error {
	directory := filepath.Join(s.Directory, Key(relative))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	grid, preview := result.Thumbs.Grid, result.Thumbs.Preview
	result.Thumbs.Grid, result.Thumbs.Preview = nil, nil
	result.Info.Images = nil
	data, err := json.Marshal(meta{Version: version, Relative: relative, Size: size, ModTime: modTime, Result: result})
	if err != nil {
		return err
	}
	// Pictures first and the meta last, so a memo that is found has its
	// pictures. No sync: a lost memo costs one read.
	for name, content := range map[string][]byte{gridName: grid, previewName: preview} {
		path := filepath.Join(directory, name)
		if len(content) == 0 {
			_ = os.Remove(path)
			continue
		}
		if err := writeFile(path, content); err != nil {
			return err
		}
	}
	return writeFile(filepath.Join(directory, metaName), data)
}

// Keep drops every memo whose inbox path is not in present, so the store holds
// the inbox as it is rather than every file that ever passed through it.
func (s Store) Keep(present map[string]bool) error {
	keys := make(map[string]bool, len(present))
	for relative := range present {
		keys[Key(relative)] = true
	}
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && !keys[entry.Name()] {
			_ = os.RemoveAll(filepath.Join(s.Directory, entry.Name()))
		}
	}
	return nil
}

func writeFile(path string, data []byte) error {
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
