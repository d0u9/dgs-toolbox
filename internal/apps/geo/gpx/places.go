// The folders the save dialog lists down its side, as Finder and Explorer do:
// home and the folders under it a GPX is likely to be in, the folder the
// browser opens at, and, on macOS, the mounted volumes an external disk shows
// up as. Only folders that are there are offered.

package gpx

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// place is one entry of that list: what it is called and where it is.
type place struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// homeFolders are the folders under the home directory worth offering, in the
// order they are listed.
var homeFolders = []string{"Desktop", "Documents", "Downloads", "Pictures", "Movies"}

// places lists the folders to offer, the browser's root first when it is not
// the home directory. A folder listed twice is listed once.
func places(root string) []place {
	list := []place{}
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
		list = append(list, place{Name: name, Path: full})
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
