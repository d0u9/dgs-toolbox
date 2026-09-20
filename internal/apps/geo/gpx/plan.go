package gpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/clean"
	"dgs-toolbox/internal/geo/compose"
	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

// routeLeg asks the router for the road between two places of a route being
// planned, in WGS-84. Only the two positions are sent.
func (a api) routeLeg(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Profile string     `json:"profile"`
		From    [2]float64 `json:"from"`
		To      [2]float64 `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if _, ok := a.wayIDs()[body.Profile]; !ok {
		writeError(w, http.StatusBadRequest, errUnknownWay)
		return
	}
	if err := checkRoute([][2]float64{body.From, body.To}); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("a waypoint is off the globe"))
		return
	}
	from := geo.LatLon{Lat: body.From[1], Lon: body.From[0]}
	to := geo.LatLon{Lat: body.To[1], Lon: body.To[0]}
	route, err := a.route(r.Context(), body.Profile, from, to)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	points := make([][2]float64, len(route.Points))
	for i, p := range route.Points {
		points[i] = [2]float64{p.Lon, p.Lat}
	}
	writeJSON(w, map[string]any{"route": points, "distance": route.Distance, "duration": route.Duration})
}

// saveRoute writes a planned route into a new GPX — one <trk> of its legs,
// and a <rte> of its waypoints when asked — and keeps the plan in the new
// file's sidecar, so it opens to be changed again. No file is replaced.
func (a api) saveRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Target   string       `json:"target"`
		Name     string       `json:"name"`
		Plan     compose.Plan `json:"plan"`
		WriteRte bool         `json:"writeRte"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	target, err := filepath.Abs(strings.TrimSpace(body.Target))
	if err != nil || !strings.EqualFold(filepath.Ext(target), ".gpx") {
		writeError(w, http.StatusBadRequest, errors.New("the route must be saved as a .gpx file"))
		return
	}
	if err := body.Plan.Check(a.wayIDs()); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := os.Stat(filepath.Dir(target)); err != nil {
		writeError(w, statusFor(err), fmt.Errorf("folder %s: %w", filepath.Dir(target), err))
		return
	}
	if _, err := os.Stat(sidecar.PathFor(target)); err == nil {
		writeError(w, http.StatusConflict, fmt.Errorf("%s: %w", sidecar.PathFor(target), gpxfile.ErrExists))
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
	}
	var routes []gpxfile.Route
	if body.WriteRte {
		routes = []gpxfile.Route{body.Plan.Route(name)}
	}
	if err := gpxfile.CreateWith(target, name, routes, []gpxfile.Track{body.Plan.Track(name)}); err != nil {
		if errors.Is(err, gpxfile.ErrExists) {
			writeError(w, http.StatusConflict, err)
		} else {
			writeError(w, statusFor(err), err)
		}
		return
	}
	plan := body.Plan
	if err := saveSidecar(target, sidecar.File{Version: sidecar.Version, Clean: clean.Defaults(), Plan: &plan}); err != nil {
		os.Remove(target)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "path": target})
}
