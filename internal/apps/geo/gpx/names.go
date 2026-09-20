// Renaming a part of a GPX. A file dgs wrote is renamed in the file itself;
// a recording from somewhere else is never written, so its new names are kept
// in the sidecar beside it. A track added here, and every track of a draft,
// carries its name in the sidecar until the file is saved.

package gpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/stops"
)

// renamePart sets the name of one <trk>, <rte> or <wpt>, keyed as the page
// keys them: "t0", "r0", "w0". An empty name puts back the file's own.
func (a api) renamePart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	kind, index, err := partKey(body.Key)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if strings.ContainsAny(name, "\n\r") {
		writeError(w, http.StatusBadRequest, errors.New("a name is one line"))
		return
	}
	result, err := analyse(body.Path, stops.Defaults())
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if result.sidecarErr != nil {
		writeError(w, http.StatusUnprocessableEntity, result.sidecarErr)
		return
	}
	if !partExists(result, kind, index) {
		writeError(w, http.StatusNotFound, fmt.Errorf("%s: no such part", body.Key))
		return
	}
	file := result.cleaning
	switch {
	// A track added here is not in the file yet: it is named in the sidecar
	// that holds it, and takes that name into the GPX it is written to.
	case kind == 't' && index >= result.own:
		file.Added[index-result.own].Name = name
	// A file dgs wrote may be renamed in place; nothing else is written.
	case kind == 't' && result.file.IsOurs() && !isDraft(body.Path):
		if err := gpxfile.RenameTrack(body.Path, index, name); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		delete(file.Names, body.Key)
	default:
		if file.Names == nil {
			file.Names = map[string]string{}
		}
		file.Names[body.Key] = name
	}
	if len(file.Names) == 0 {
		file.Names = nil
	}
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "name": name})
}

// partKey reads a part key: its kind ('t', 'r' or 'w') and its index.
func partKey(key string) (byte, int, error) {
	invalid := errors.New(`the part is keyed "t0", "r0" or "w0"`)
	if len(key) < 2 || (key[0] != 't' && key[0] != 'r' && key[0] != 'w') {
		return 0, 0, invalid
	}
	index, err := strconv.Atoi(key[1:])
	if err != nil || index < 0 {
		return 0, 0, invalid
	}
	return key[0], index, nil
}

func partExists(a analysis, kind byte, index int) bool {
	switch kind {
	case 't':
		return index < len(a.file.Tracks)
	case 'r':
		return index < len(a.file.Routes)
	}
	return index < len(a.file.Waypoints)
}
