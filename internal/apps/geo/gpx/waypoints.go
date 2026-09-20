package gpx

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"dgs-toolbox/internal/geo"
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
	// A new GPX is not on disk yet, so its waypoints stay in the draft until
	// it is saved; a file this program wrote takes the point into the GPX.
	if isDraft(body.Path) {
		draft, ok := drafts.get(body.Path)
		if !ok {
			writeError(w, statusFor(errNoDraft), errNoDraft)
			return
		}
		if math.IsNaN(body.Lat) || math.IsNaN(body.Lon) || math.Abs(body.Lat) > 90 || math.Abs(body.Lon) > 180 {
			writeError(w, http.StatusBadRequest, errors.New("waypoint coordinates are outside the globe"))
			return
		}
		draft.Waypoints = append(draft.Waypoints, compose.Waypoint{Lon: body.Lon, Lat: body.Lat, Name: name, Description: description, At: time.Now().UnixMilli()})
		if err := drafts.set(body.Path, draft); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	point := gpxfile.Waypoint{Point: gpxfile.Point{LatLon: geo.LatLon{Lat: body.Lat, Lon: body.Lon}}, Name: name, Description: description}
	if err := gpxfile.AddWaypoint(body.Path, point); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
