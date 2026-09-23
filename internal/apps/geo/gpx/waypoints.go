package gpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"dgs-toolbox/internal/geo/compose"
	"dgs-toolbox/internal/geo/gpxfile"
)

// addWaypoint adds a standalone GPX waypoint to an existing dgs-created file.
func (a api) addWaypoint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path        string  `json:"path"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Lon         float64 `json:"lon"`
		Lat         float64 `json:"lat"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if err := checkGPX(body.Path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("name a waypoint"))
		return
	}
	name, description := strings.TrimSpace(body.Name), strings.TrimSpace(body.Description)
	if math.IsNaN(body.Lat) || math.IsNaN(body.Lon) || math.Abs(body.Lat) > 90 || math.Abs(body.Lon) > 180 {
		writeError(w, http.StatusBadRequest, errors.New("waypoint coordinates are outside the globe"))
		return
	}
	// Only a GPX dgs wrote takes a waypoint: a recording is never written.
	if !isDraft(body.Path) {
		file, err := openGPX(body.Path)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		if !file.IsOurs() {
			writeError(w, http.StatusUnprocessableEntity, fmt.Errorf("%s: %w", body.Path, gpxfile.ErrNotOurs))
			return
		}
	}
	// The point is held beside the file, as every edit is, and goes into the
	// GPX when the edit is saved.
	held, _, err := loadSidecar(body.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	held.Waypoints = append(held.Waypoints, compose.Waypoint{Lon: body.Lon, Lat: body.Lat, Name: name, Description: description, At: time.Now().UnixMilli()})
	if err := saveSidecar(body.Path, held); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
