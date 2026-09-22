// Package filebrowse lists a folder the way a file manager lists one, and
// names the places a file dialog offers down its side. It is the computation
// behind every open and save dialog dgs serves: plain values in, plain values
// out, so a web handler, a TUI and a test all call the same code.
//
// Nothing here reads a file's contents, writes anything, or knows about HTTP.
package filebrowse

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Entry is one row of a listing: a folder, or a file that passed the filter.
// Size is zero for a folder, and Modified is when it was last written.
type Entry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size,omitempty"`
	Modified time.Time `json:"modified,omitzero"`
}

// Listing is one folder as a dialog draws it. Parent is empty at the root of
// the filesystem, so a dialog knows there is no folder above.
type Listing struct {
	Path   string  `json:"path"`
	Parent string  `json:"parent"`
	Dirs   []Entry `json:"dirs"`
	Files  []Entry `json:"files"`
}

// Options is what a caller may vary: which files are listed, and whether the
// entries a file manager keeps out of sight are shown.
type Options struct {
	// Extensions keeps only files with one of these suffixes, compared
	// without case and with or without the leading dot. Empty lists every
	// file, as a dialog's "All files" filter does.
	Extensions []string
	// Hidden lists entries whose name starts with a dot as well.
	Hidden bool
}

// List reads one folder. A symlinked folder counts as a folder, and an entry
// that cannot be stat'ed is left out rather than failing the whole listing.
// Folders are always listed, whatever the filter: a dialog browses through
// them to reach the files it wants.
func List(dir string, options Options) (Listing, error) {
	path, err := filepath.Abs(dir)
	if err != nil {
		return Listing{}, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return Listing{}, err
	}
	wanted := suffixes(options.Extensions)
	listing := Listing{Path: path, Dirs: []Entry{}, Files: []Entry{}}
	for _, entry := range entries {
		name := entry.Name()
		if !options.Hidden && strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(path, name)
		info, err := os.Stat(full) // follows symlinks, so a linked folder is a folder
		if err != nil {
			continue
		}
		switch {
		case info.IsDir():
			listing.Dirs = append(listing.Dirs, Entry{Name: name, Path: full, Modified: info.ModTime()})
		case matches(name, wanted):
			listing.Files = append(listing.Files, Entry{Name: name, Path: full, Size: info.Size(), Modified: info.ModTime()})
		}
	}
	byName(listing.Dirs)
	byName(listing.Files)
	if parent := filepath.Dir(path); parent != path {
		listing.Parent = parent
	}
	return listing, nil
}

// suffixes normalises a filter to lower-case suffixes that start with a dot,
// so ".GPX", "gpx" and ".gpx" all mean the same filter.
func suffixes(extensions []string) []string {
	wanted := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		wanted = append(wanted, ext)
	}
	return wanted
}

func matches(name string, wanted []string) bool {
	if len(wanted) == 0 {
		return true
	}
	lower := strings.ToLower(name)
	for _, ext := range wanted {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// byName orders entries the way a reader reads them: case is ignored, so f
// sits beside F.
func byName(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

// Place is one entry of the list down a dialog's side: what it is called and
// where it is.
type Place struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// homeFolders are the folders under the home directory worth offering, in the
// order they are listed.
var homeFolders = []string{"Desktop", "Documents", "Downloads", "Pictures", "Movies"}

// Places lists the folders to offer, the caller's root first when it is not
// the home directory: Home, the folders under it a document is likely to be
// in, and, on macOS, the mounted volumes an external disk shows up as. Only
// folders that are there are offered, and a folder listed twice is listed
// once.
func Places(root string) []Place {
	list := []Place{}
	seen := map[string]bool{}
	add := func(name, path string) {
		if path == "" {
			return
		}
		full, err := filepath.Abs(path)
		if err != nil || seen[full] {
			return
		}
		if info, err := os.Stat(full); err != nil || !info.IsDir() {
			return
		}
		seen[full] = true
		list = append(list, Place{Name: name, Path: full})
	}
	home, _ := os.UserHomeDir()
	if root != "" && home != "" && filepath.Clean(root) != filepath.Clean(home) {
		add(filepath.Base(root), root)
	}
	add("Home", home)
	for _, name := range homeFolders {
		if home != "" {
			add(name, filepath.Join(home, name))
		}
	}
	for _, volume := range volumes() {
		add(filepath.Base(volume), volume)
	}
	return list
}

// volumes are the mounted disks, so an external drive can be reached without
// typing its path. Only macOS lists them; elsewhere the list is empty, and
// the home folders are all that is offered.
func volumes() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return nil
	}
	names := []string{}
	for _, entry := range entries {
		if name := entry.Name(); name[0] != '.' {
			names = append(names, filepath.Join("/Volumes", name))
		}
	}
	sort.Strings(names)
	return names
}

// A name typed into a dialog is one entry of the folder being browsed, never
// a path: a separator, an empty name, or a name that means somewhere else is
// refused rather than followed.
var (
	ErrEmptyName = errors.New("the name is empty")
	ErrPathName  = errors.New("a name cannot hold a path")
	ErrExists    = errors.New("something of that name is already there")
)

// CheckName says whether a typed name may be used inside a folder.
func CheckName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return ErrEmptyName
	case strings.ContainsAny(name, `/\`), name == "." || name == "..":
		return ErrPathName
	default:
		return nil
	}
}

// CreateFolder makes one folder inside parent and answers its path. A name
// already taken is refused: nothing on disk is replaced or merged.
func CreateFolder(parent, name string) (string, error) {
	if err := CheckName(name); err != nil {
		return "", err
	}
	dir, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	full := filepath.Join(dir, strings.TrimSpace(name))
	if _, err := os.Lstat(full); err == nil {
		return "", ErrExists
	}
	if err := os.Mkdir(full, 0o755); err != nil {
		return "", err
	}
	return full, nil
}

// Rename gives a file or folder another name in the folder it is already in,
// and answers its new path. A name already taken is refused, so renaming
// never replaces anything.
func Rename(path, name string) (string, error) {
	if err := CheckName(name); err != nil {
		return "", err
	}
	full, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(full); err != nil {
		return "", err
	}
	target := filepath.Join(filepath.Dir(full), strings.TrimSpace(name))
	if target == full {
		return full, nil
	}
	if _, err := os.Lstat(target); err == nil {
		return "", ErrExists
	}
	if err := os.Rename(full, target); err != nil {
		return "", err
	}
	return target, nil
}

// Taken says whether a folder already holds this name, so a save dialog can
// say so before the server is asked to write.
func Taken(dir, name string) (bool, error) {
	if err := CheckName(name); err != nil {
		return false, err
	}
	full := filepath.Join(dir, strings.TrimSpace(name))
	_, err := os.Lstat(full)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}
