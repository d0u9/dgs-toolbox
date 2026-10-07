package gpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/filebrowse"
	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

// renameFile changes a draft's name or a dgs-created file's name in its
// current folder. Embedded state moves with the file; pending edits keep
// their unsaved status under the new path.
func (a api) renameFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if err := checkGPX(body.Path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	name, err := gpxName(body.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var target string
	if isDraft(body.Path) {
		target, err = drafts.rename(body.Path, name)
	} else {
		var parsed *gpxfile.File
		parsed, err = openGPX(body.Path)
		if err == nil && !parsed.IsOurs() {
			err = gpxfile.ErrNotOurs
		}
		if err == nil {
			target, err = renameOwnFile(body.Path, name)
		}
	}
	if err != nil {
		status := statusFor(err)
		if errors.Is(err, filebrowse.ErrExists) {
			status = http.StatusConflict
		}
		if errors.Is(err, gpxfile.ErrNotOurs) {
			status = http.StatusForbidden
		}
		writeError(w, status, err)
		return
	}
	pending.move(body.Path, target)
	writeJSON(w, map[string]string{"path": target})
}

// gpxName adds only the GPX extension, preserving dots in the given name.
func gpxName(given string) (string, error) {
	name := strings.TrimSpace(given)
	if err := filebrowse.CheckName(name); err != nil {
		return "", err
	}
	if strings.ContainsRune(name, 0) {
		return "", errors.New("a name cannot contain a NUL character")
	}
	if strings.EqualFold(filepath.Ext(name), ".gpx") {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	if err := filebrowse.CheckName(name); err != nil {
		return "", err
	}
	return name + ".gpx", nil
}

func (s *draftStore) rename(path, name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := s.drafts[path]
	if !ok {
		return "", errNoDraft
	}
	target := path[:strings.LastIndex(path, "/")+1] + name
	if target == path {
		return path, nil
	}
	if _, exists := s.drafts[target]; exists {
		return "", filebrowse.ErrExists
	}
	s.drafts[target] = file
	delete(s.drafts, path)
	return target, nil
}

// A companion sidecar, if present, follows the GPX. Refuse occupied names
// before moving either file, and roll the GPX back if the sidecar move fails.
func renameOwnFile(path, name string) (string, error) {
	full, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	target := filepath.Join(filepath.Dir(full), name)
	if target == full {
		return full, nil
	}
	if pending.has(target) {
		return "", filebrowse.ErrExists
	}
	oldSide, newSide := sidecar.PathFor(full), sidecar.PathFor(target)
	_, sideErr := os.Lstat(oldSide)
	if sideErr != nil && !errors.Is(sideErr, os.ErrNotExist) {
		return "", sideErr
	}
	if _, err := os.Lstat(newSide); err == nil {
		return "", filebrowse.ErrExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	newPhotos := filepath.Join(filepath.Dir(target), photoDir(target))
	if _, err := os.Lstat(newPhotos); err == nil {
		return "", filebrowse.ErrExists
	}
	renamed, err := filebrowse.Rename(full, name)
	if err != nil {
		return "", err
	}
	if sideErr == nil {
		if _, err := filebrowse.Rename(oldSide, filepath.Base(newSide)); err != nil {
			_, rollbackErr := filebrowse.Rename(renamed, filepath.Base(full))
			return "", errors.Join(err, rollbackErr)
		}
	}
	if err := renamePhotos(full, renamed); err != nil {
		return "", err
	}
	return renamed, nil
}

func (s *pendingStore) move(from, to string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, to = pendingKey(from), pendingKey(to)
	if from == to {
		return
	}
	if file, ok := s.edits[from]; ok {
		s.edits[to] = file
		delete(s.edits, from)
	}
}
