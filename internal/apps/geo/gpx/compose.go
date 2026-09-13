package gpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/clean"
	"dgs-toolbox/internal/geo/compose"
	"dgs-toolbox/internal/geo/gcj02"
	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/osrm"
	"dgs-toolbox/internal/geo/sidecar"
	"dgs-toolbox/internal/geo/stops"
)

// Router finds a route along the road between two points for a profile.
type Router func(ctx context.Context, profile string, from, to geo.LatLon) (osrm.Route, error)

func (a api) router() Router {
	if a.settings.Router != nil {
		return a.settings.Router
	}
	return osrm.Default().Route
}

// fills lists a track's fills with the distance each route covers.
func fills(a analysis, distances []float64) []fillJSON {
	list := make([]fillJSON, len(a.cleaning.Fills))
	for i, fill := range a.cleaning.Fills {
		list[i] = fillJSON{First: fill.First, Last: fill.Last, Points: len(fill.Route), Profile: fill.Profile, At: fill.At}
		first, last := a.keptAt(fill.First), a.keptAt(fill.Last)
		if last < len(distances) && first < len(distances) {
			list[i].Distance = distances[last] - distances[first]
		}
	}
	return list
}

func addedTracks(added []compose.Added) []gpxfile.Track {
	tracks := make([]gpxfile.Track, len(added))
	for i, track := range added {
		tracks[i] = track.Track()
	}
	return tracks
}

type fillRequest struct {
	Path        string       `json:"path"`
	First       int          `json:"first"`
	Last        int          `json:"last"`
	Profile     string       `json:"profile"`
	Coordinates string       `json:"coordinates"`
	Route       [][2]float64 `json:"route"`
}

// fillEnds checks that a fill can go between two points of a track: both kept
// and recorded, in order, and not across another fill.
func fillEnds(a analysis, first, last int) error {
	n := len(a.source)
	if first < 0 || last >= n || last <= first {
		return errors.New("choose two points of the track, the start before the end")
	}
	for _, i := range []int{first, last} {
		if a.result.Removed[i] != clean.Kept || (a.result.Filled != nil && a.result.Filled[i]) {
			return fmt.Errorf("point %d is not a recorded point the track keeps", i)
		}
	}
	for _, fill := range a.cleaning.Fills {
		if first < fill.Last && last > fill.First {
			return errors.New("the stretch crosses a fill already there; remove that one first")
		}
	}
	return nil
}

// routeFill asks the router for the road between two points of a track. It
// sends only those two positions, as cleaning left them.
func (a api) routeFill(w http.ResponseWriter, r *http.Request) {
	var body fillRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if _, ok := a.wayIDs()[body.Profile]; !ok {
		writeError(w, http.StatusBadRequest, errUnknownWay)
		return
	}
	result, err := analyse(body.Path, stops.Defaults())
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if err := fillEnds(result, body.First, body.Last); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	from, to := result.result.Positions[body.First], result.result.Positions[body.Last]
	route, err := a.route(r.Context(), body.Profile, from, to)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	points, display := make([][2]float64, len(route.Points)), make([][2]float64, len(route.Points))
	for i, p := range route.Points {
		points[i] = [2]float64{p.Lon, p.Lat}
		shown := p
		if body.Coordinates == systemGCJ02 {
			shown = gcj02.FromWGS84(p)
		}
		display[i] = [2]float64{shown.Lon, shown.Lat}
	}
	writeJSON(w, map[string]any{"route": points, "display": display, "distance": route.Distance, "duration": route.Duration})
}

// saveFill records a route between two points of a track, inserted after the
// first; every later index in the sidecar moves along.
func (a api) saveFill(w http.ResponseWriter, r *http.Request) {
	var body fillRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if _, ok := a.wayIDs()[body.Profile]; !ok {
		writeError(w, http.StatusBadRequest, errUnknownWay)
		return
	}
	if err := checkRoute(body.Route); err != nil {
		writeError(w, http.StatusBadRequest, err)
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
	if err := fillEnds(result, body.First, body.Last); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	file := result.cleaning
	file.Insert(body.First, len(body.Route))
	file.Fills = append(file.Fills, compose.Fill{
		First:   body.First,
		Last:    body.Last + len(body.Route),
		Profile: body.Profile,
		Route:   body.Route,
		At:      time.Now().UnixMilli(),
	})
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func checkRoute(route [][2]float64) error {
	if len(route) < 2 || len(route) > 100000 {
		return errors.New("the route has no points")
	}
	for _, p := range route {
		if !(p[1] >= -90 && p[1] <= 90 && p[0] >= -180 && p[0] <= 180) || math.IsNaN(p[0]) {
			return errors.New("the route holds a position off the globe")
		}
	}
	return nil
}

// removeFill takes a fill out: its route's points go, and the recorded points
// it replaced come back.
func (a api) removeFill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string `json:"path"`
		Index int    `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if err := checkGPX(body.Path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	file, _, err := loadSidecar(body.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if body.Index < 0 || body.Index >= len(file.Fills) {
		writeError(w, http.StatusConflict, errors.New("no such fill; reload the page"))
		return
	}
	first, last := file.Fills[body.Index].Inserted()
	file.Delete(first, last) // the fill goes with its points
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// addTracks records tracks, taken from another file, in a GPX file's sidecar.
// The GPX itself is not written: it is saved, with them, into a new file.
func addTracks(target, from string, tracks []gpxfile.Track) error {
	if err := checkGPX(target); err != nil {
		return err
	}
	file, _, err := loadSidecar(target)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, trk := range tracks {
		file.Added = append(file.Added, compose.AddedFrom(trk, from, now))
	}
	return saveSidecar(target, file)
}

// removeAdded takes one added track out of a GPX file, with every edit,
// cut and fill on its points.
func (a api) removeAdded(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string `json:"path"`
		Index int    `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
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
	file := result.cleaning
	if body.Index < 0 || body.Index >= len(file.Added) {
		writeError(w, http.StatusConflict, errors.New("no such added track; reload the page"))
		return
	}
	if first, last, ok := trackRange(result, result.own+body.Index); ok {
		file.Delete(first, last)
	}
	file.Added = append(file.Added[:body.Index:body.Index], file.Added[body.Index+1:]...)
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// trackRange is the composed indices of one track's points.
func trackRange(a analysis, index int) (first, last int, ok bool) {
	first, last = -1, -1
	for k, sample := range a.source {
		if sample.Track == index {
			if first < 0 {
				first = k
			}
			last = k
		}
	}
	return first, last, first >= 0
}

// saveAs writes a GPX file with the tracks added to it into a new file: the
// file as it is, byte for byte, with the added tracks after its own. The new
// file's sidecar takes over the cleaning, cuts and fills; the original loses
// the added tracks and whatever was done to them, and is not written.
func (a api) saveAs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path   string `json:"path"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	target, err := filepath.Abs(strings.TrimSpace(body.Target))
	if err != nil || !strings.EqualFold(filepath.Ext(target), ".gpx") {
		writeError(w, http.StatusBadRequest, errors.New("the new file must be a .gpx file"))
		return
	}
	source := body.Path
	if !isDraft(source) {
		source, _ = filepath.Abs(body.Path)
	}
	if sameFile(source, target) {
		writeError(w, http.StatusBadRequest, errors.New("a changed GPX is saved as a new file; choose another name"))
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
	if _, err := os.Stat(sidecar.PathFor(target)); err == nil {
		writeError(w, http.StatusConflict, fmt.Errorf("%s: %w", sidecar.PathFor(target), gpxfile.ErrExists))
		return
	}
	// A draft is written as a new file holding its tracks; a file on disk is
	// copied byte for byte and its added tracks appended.
	var data []byte
	if isDraft(body.Path) {
		var encoded bytes.Buffer
		encoded.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
		encoded.WriteString(`<gpx version="1.1" creator="` + gpxfile.Creator + `" xmlns="http://www.topografix.com/GPX/1/1">` + "\n")
		encoded.WriteString("</gpx>\n")
		data = encoded.Bytes()
	} else if data, err = os.ReadFile(body.Path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if err := writeNew(target, data); err != nil {
		if errors.Is(err, gpxfile.ErrExists) {
			writeError(w, http.StatusConflict, err)
		} else {
			writeError(w, statusFor(err), err)
		}
		return
	}
	file := result.cleaning
	if len(file.Added) > 0 {
		if err := gpxfile.Append(target, addedTracks(file.Added)); err != nil {
			os.Remove(target)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	saved := file
	saved.Added = nil
	if err := sidecar.Save(target, saved); err != nil {
		os.Remove(target)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// A saved draft is done with; the original goes back to its own tracks,
	// as the added ones come last.
	if isDraft(body.Path) {
		drafts.remove(body.Path)
	} else if len(file.Added) > 0 {
		if first, _, ok := trackRange(result, result.own); ok {
			file.Delete(first, len(result.source)-1)
		}
		file.Added = nil
		if err := saveSidecar(body.Path, file); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "path": target})
}

// writeNew writes data into a file that must not exist yet.
func writeNew(path string, data []byte) error {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return fmt.Errorf("folder %s: %w", filepath.Dir(path), err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s: %w", path, gpxfile.ErrExists)
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	return file.Close()
}
