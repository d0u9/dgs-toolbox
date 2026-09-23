package gpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"dgs-toolbox/internal/geo/sidecar"
)

// Editing a GPX changes nothing on disk until it is saved. What was done to a
// file — cleaning, cuts, fills, names, added tracks, a route's plan — is held
// here, in this process's memory, over what the file's sidecar says. Reading a
// file reads what is held here when there is something; saving writes it to
// the sidecar, or into the GPX itself when dgs wrote that GPX; reverting drops
// it and the file reads as it is on disk again.
//
// The cost is stated where the user can see it: unsaved edits live in one dgs
// process. They go when dgs stops, and the page asks before it is reloaded or
// closed while it holds any.
//
// Two writes are not held back, because each is a file the user asked for by
// name: the sidecar of a file just written by *Save as…* or by saving a
// planned route, which belongs to the new file and not to an edit of an open
// one. A track renamed in a GPX dgs wrote is likewise written into that file
// at once — see renamePart, which names the file itself rather than a sidecar.
type pendingStore struct {
	mu    sync.Mutex
	edits map[string]sidecar.File
}

var pending = &pendingStore{edits: map[string]sidecar.File{}}

// key is what a path is held under: the same file reached by two spellings is
// one entry.
func pendingKey(path string) string {
	if isDraft(path) {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func (s *pendingStore) get(path string) (sidecar.File, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := s.edits[pendingKey(path)]
	return file, ok
}

func (s *pendingStore) set(path string, file sidecar.File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edits[pendingKey(path)] = file
}

func (s *pendingStore) drop(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.edits, pendingKey(path))
}

func (s *pendingStore) has(path string) bool {
	_, ok := s.get(path)
	return ok
}

// paths are the files holding unsaved edits, in a steady order.
func (s *pendingStore) paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	paths := make([]string, 0, len(s.edits))
	for path := range s.edits {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// flush writes one file's unsaved edits where they belong and forgets them.
// A file with nothing unsaved is not written.
func flush(path string) error {
	file, ok := pending.get(path)
	if !ok {
		return nil
	}
	if err := writeSidecarNow(path, file); err != nil {
		return err
	}
	pending.drop(path)
	return nil
}

// saveFiles writes the unsaved edits of the paths given, or of every file
// holding any when none is given.
func (a api) saveFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	paths := body.Paths
	if body.Path != "" {
		paths = append(paths, body.Path)
	}
	if len(paths) == 0 {
		paths = pending.paths()
	}
	saved := []string{}
	for _, path := range paths {
		if !pending.has(path) {
			continue
		}
		if err := flush(path); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		saved = append(saved, path)
	}
	writeJSON(w, map[string]any{"ok": true, "saved": saved})
}

// revertFiles drops unsaved edits: the files read as they are on disk again.
// A draft holds no sidecar of its own on disk, so there is nothing to go back
// to and it is refused.
func (a api) revertFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	paths := body.Paths
	if body.Path != "" {
		paths = append(paths, body.Path)
	}
	if len(paths) == 0 {
		paths = pending.paths()
	}
	for _, path := range paths {
		if isDraft(path) {
			writeError(w, http.StatusBadRequest, errors.New("a new GPX has nothing on disk to go back to; remove it from the workspace instead"))
			return
		}
		pending.drop(path)
	}
	writeJSON(w, map[string]any{"ok": true})
}

// unsaved lists the files holding edits that are not on disk, so the page
// shows the same marks again after it is reloaded.
func (a api) unsaved(w http.ResponseWriter, _ *http.Request) {
	paths := pending.paths()
	here := make([]string, 0, len(paths))
	for _, path := range paths {
		if isDraft(path) {
			here = append(here, path)
			continue
		}
		if _, err := os.Stat(path); err == nil {
			here = append(here, path)
		}
	}
	writeJSON(w, map[string]any{"paths": here})
}
