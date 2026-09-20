package gpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dgs-toolbox/internal/geo/clean"
	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

// A draft is a new GPX not yet on disk: an empty file that tracks are added
// to, kept in this process's memory until it is saved. Its path is
// "draft:<id>/<name>.gpx", so it reads like a file everywhere else; what a
// sidecar holds for a file on disk, a draft holds here.
const draftPrefix = "draft:"

type draftStore struct {
	mu     sync.Mutex
	next   int
	drafts map[string]sidecar.File
}

// drafts are this process's drafts; they go when dgs exits.
var drafts = &draftStore{drafts: map[string]sidecar.File{}}

var errNoDraft = fmt.Errorf("the new GPX is gone — it was never saved, and dgs has restarted since: %w", os.ErrNotExist)

func isDraft(path string) bool { return strings.HasPrefix(path, draftPrefix) }

func (s *draftStore) create(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	path := fmt.Sprintf("%s%d/%s.gpx", draftPrefix, s.next, name)
	s.drafts[path] = sidecar.File{Version: sidecar.Version, Clean: clean.Defaults()}
	return path
}

func (s *draftStore) get(path string) (sidecar.File, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := s.drafts[path]
	return file, ok
}

func (s *draftStore) set(path string, file sidecar.File) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.drafts[path]; !ok {
		return errNoDraft
	}
	s.drafts[path] = file
	return nil
}

func (s *draftStore) remove(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.drafts, path)
}

// checkGPX says whether a path is a GPX the server can read: a .gpx file on
// disk, or a draft.
func checkGPX(path string) error {
	if !strings.EqualFold(filepath.Ext(path), ".gpx") {
		return errNotGPX
	}
	if isDraft(path) {
		if _, ok := drafts.get(path); !ok {
			return errNoDraft
		}
		return nil
	}
	_, err := os.Stat(path)
	return err
}

// openGPX reads a GPX file, or a draft's empty one.
func openGPX(path string) (*gpxfile.File, error) {
	if isDraft(path) {
		draft, ok := drafts.get(path)
		if !ok {
			return nil, errNoDraft
		}
		file := &gpxfile.File{Name: draftName(path)}
		for _, waypoint := range draft.Waypoints {
			file.Waypoints = append(file.Waypoints, waypoint.Point())
		}
		return file, nil
	}
	return gpxfile.Open(path)
}

// loadSidecar reads what was done to a GPX: its sidecar, or a draft's record.
func loadSidecar(path string) (sidecar.File, bool, error) {
	if isDraft(path) {
		file, ok := drafts.get(path)
		if !ok {
			return file, false, errNoDraft
		}
		return file, true, nil
	}
	if parsed, err := gpxfile.Open(path); err == nil && parsed.IsOurs() {
		data, found, err := gpxfile.ReadState(path)
		if err != nil {
			return sidecar.File{}, false, err
		}
		if found {
			var file sidecar.File
			if err := json.Unmarshal(data, &file); err != nil {
				return sidecar.File{}, false, err
			}
			if file.Version > sidecar.Version {
				return sidecar.File{}, false, fmt.Errorf("embedded state version %d is newer than this build reads", file.Version)
			}
			file.Clean = file.Clean.Normalize()
			return file, true, nil
		}
		return sidecar.File{Version: sidecar.Version, Clean: clean.Defaults()}, false, nil
	}
	return sidecar.Load(path)
}

func saveSidecar(path string, file sidecar.File) error {
	if isDraft(path) {
		return drafts.set(path, file)
	}
	parsed, err := gpxfile.Open(path)
	if err != nil || !parsed.IsOurs() {
		return sidecar.Save(path, file)
	}
	if !file.Active() {
		return gpxfile.WriteState(path, nil)
	}
	file.Version = sidecar.Version
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return gpxfile.WriteState(path, data)
}

func draftName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// discardSidecar deletes a GPX file's sidecar: its cleaning, cuts, fills and
// added tracks go, and the file reads as recorded. The GPX is not touched.
func (a api) discardSidecar(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if isDraft(body.Path) {
		writeError(w, http.StatusBadRequest, errors.New("a new GPX has no sidecar; remove it from the workspace instead"))
		return
	}
	if err := checkGPX(body.Path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if _, err := os.Stat(sidecar.PathFor(body.Path)); errors.Is(err, os.ErrNotExist) {
		if parsed, err := gpxfile.Open(body.Path); err == nil && parsed.IsOurs() {
			if err := gpxfile.WriteState(body.Path, nil); err != nil {
				writeError(w, statusFor(err), err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
			return
		}
	}
	if err := os.Remove(sidecar.PathFor(body.Path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// newDraft starts a new, empty GPX in memory.
func (a api) newDraft(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	name := strings.TrimSpace(strings.TrimSuffix(body.Name, filepath.Ext(body.Name)))
	if name == "" || strings.ContainsAny(name, `/\`) {
		writeError(w, http.StatusBadRequest, errors.New("name the new GPX, without / or \\"))
		return
	}
	writeJSON(w, map[string]any{"path": drafts.create(name)})
}
