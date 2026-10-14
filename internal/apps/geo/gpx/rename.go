package gpx

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/filebrowse"
	"dgs-toolbox/internal/geo/sidecar"
)

// renameFile changes a draft's name, or a file's name in its current folder,
// whoever wrote it. The sidecar and photos move with the file; a recording's
// bytes are not touched. Pending edits keep their unsaved status under the
// new path.
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
		// A recording may be renamed too: its bytes stay as recorded, and the
		// digest taken before is checked against the file under its new name.
		// A GPX dgs wrote is ours: its photo links are rewritten for the new name.
		parsed, openErr := openGPX(body.Path)
		recording := openErr != nil || !parsed.IsOurs()
		var before [sha256.Size]byte
		before, err = fileDigest(body.Path)
		if err == nil {
			target, err = renameOwnFile(body.Path, name)
		}
		if err == nil && recording {
			var after [sha256.Size]byte
			if after, err = fileDigest(target); err == nil && after != before {
				err = fmt.Errorf("%s: contents changed while it was renamed", target)
			}
		}
	}
	if err != nil {
		status := statusFor(err)
		if errors.Is(err, filebrowse.ErrExists) {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}
	pending.move(body.Path, target)
	history.move(body.Path, target)
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
	if photos, ok := s.photos[path]; ok {
		s.photos[target] = photos
		delete(s.photos, path)
	}
	return target, nil
}

// A companion sidecar, if present, follows the GPX. Refuse occupied names
// before moving either file, and roll the GPX back if the sidecar move fails.
func fileDigest(path string) ([sha256.Size]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}

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

// namedGPX says whether a path is a .gpx file with a name before the
// extension: ".gpx" alone would be a hidden file listed with no name.
func namedGPX(path string) bool {
	ext := filepath.Ext(path)
	return strings.EqualFold(ext, ".gpx") && strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), ext)) != ""
}
