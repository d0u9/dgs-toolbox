// Renaming a part of a GPX. Every name is held beside the file until the edit
// is saved; saving writes it where it belongs — into a GPX dgs wrote, and into
// the sidecar of a recording, which is never written. A track added here, and
// every track of a draft, carries its name into the GPX it is written to.

package gpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

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
	if _, _, err := partKey(body.Key); err != nil {
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
	at, err := locate(result, body.Key)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	// Every name is kept beside the file until the edit is saved. Saving puts
	// it where it belongs: into a GPX dgs wrote, and into the sidecar of a
	// recording, which is never written.
	file := result.cleaning
	if at.kind == 't' && at.held >= 0 {
		// A track added here is not in a GPX yet: it carries its name where it
		// is held, into the GPX it is written to.
		file.Added[at.held].Name = name
	}
	if file.Names == nil {
		file.Names = map[string]string{}
	}
	file.Names[body.Key] = name
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
