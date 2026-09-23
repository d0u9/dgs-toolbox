// A file's parts — its tracks, routes and waypoints — as the page lists them,
// and what is done to one: deleting it, copying or moving it to another GPX,
// and putting a waypoint somewhere else.
//
// A part is keyed "t0", "r0", "w0". The key counts the file on disk, and the
// parts carried beside it — tracks added from other files, routes and
// waypoints copied into it — follow those on disk. A key therefore stays the
// same while another part is deleted, so a name or an edit keyed by it still
// names the part it was given for.
//
// Deleting and moving are edits like any other: they are held until the file
// is saved, and saving takes them into the GPX itself, which only a GPX dgs
// wrote is. A recording is never written, so only a track added to it here
// can be taken out of it again.

package gpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/compose"
	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
	"dgs-toolbox/internal/geo/stops"
)

// partKeys names each part of a file, in file order.
type partKeys struct {
	Tracks    []string
	Routes    []string
	Waypoints []string
}

// partCounts is how many parts of each kind a file holds.
type partCounts struct {
	Tracks    int
	Routes    int
	Waypoints int
}

// laid lays what is held beside a GPX over the file on disk: the parts deleted
// but not saved yet are taken out, a waypoint that was moved is at its new
// position, and the routes and waypoints copied into the file follow its own.
// It answers the file as the page sees it, the key of each of its parts, and
// what the file on disk holds.
func laid(onDisk *gpxfile.File, held sidecar.File) (*gpxfile.File, partKeys, partCounts) {
	file := *onDisk
	layout := partCounts{Tracks: len(onDisk.Tracks), Routes: len(onDisk.Routes), Waypoints: len(onDisk.Waypoints)}
	deleted := map[string]bool{}
	for _, key := range held.Deleted {
		deleted[key] = true
	}
	var keys partKeys
	file.Tracks = nil
	for i, trk := range onDisk.Tracks {
		key := fmt.Sprintf("t%d", i)
		if deleted[key] {
			continue
		}
		file.Tracks = append(file.Tracks, trk)
		keys.Tracks = append(keys.Tracks, key)
	}
	file.Routes = nil
	for i, rte := range onDisk.Routes {
		key := fmt.Sprintf("r%d", i)
		if deleted[key] {
			continue
		}
		file.Routes = append(file.Routes, rte)
		keys.Routes = append(keys.Routes, key)
	}
	for i, route := range held.Routes {
		file.Routes = append(file.Routes, route.Route())
		keys.Routes = append(keys.Routes, fmt.Sprintf("r%d", layout.Routes+i))
	}
	file.Waypoints = nil
	for i, wpt := range onDisk.Waypoints {
		key := fmt.Sprintf("w%d", i)
		if deleted[key] {
			continue
		}
		if at, ok := held.Moved[key]; ok {
			wpt.LatLon = geo.LatLon{Lat: at[1], Lon: at[0]}
		}
		file.Waypoints = append(file.Waypoints, wpt)
		keys.Waypoints = append(keys.Waypoints, key)
	}
	for i, waypoint := range held.Waypoints {
		key := fmt.Sprintf("w%d", layout.Waypoints+i)
		point := waypoint.Point()
		if at, ok := held.Moved[key]; ok {
			point.LatLon = geo.LatLon{Lat: at[1], Lon: at[0]}
		}
		file.Waypoints = append(file.Waypoints, point)
		keys.Waypoints = append(keys.Waypoints, key)
	}
	// Last, the routes and waypoints the page put in another order are put
	// there. Tracks are put in order with the tracks added here, by analyse.
	file.Routes, keys.Routes = ordered(held.Order["r"], file.Routes, keys.Routes)
	file.Waypoints, keys.Waypoints = ordered(held.Order["w"], file.Waypoints, keys.Waypoints)
	return &file, keys, layout
}

// ordered puts parts and their keys in the order wanted. A key the file no
// longer holds is passed over, and a part the order does not name keeps its
// place after the ones it does, so an order made before a part was added or
// deleted still reads.
func ordered[T any](wanted []string, parts []T, keys []string) ([]T, []string) {
	if len(wanted) == 0 || len(parts) != len(keys) {
		return parts, keys
	}
	at := make(map[string]int, len(keys))
	for i, key := range keys {
		at[key] = i
	}
	taken := make([]bool, len(parts))
	outParts, outKeys := make([]T, 0, len(parts)), make([]string, 0, len(keys))
	for _, key := range wanted {
		i, ok := at[key]
		if !ok || taken[i] {
			continue
		}
		taken[i] = true
		outParts = append(outParts, parts[i])
		outKeys = append(outKeys, keys[i])
	}
	for i := range parts {
		if taken[i] {
			continue
		}
		outParts = append(outParts, parts[i])
		outKeys = append(outKeys, keys[i])
	}
	return outParts, outKeys
}

// place is where a part of a file is held: in the GPX on disk, or beside it
// until the file is saved.
type place struct {
	kind byte // 't', 'r' or 'w'
	// shown is the part's place in the file as the page sees it.
	shown int
	// disk is its place in the GPX on disk, or -1 when it is not there yet;
	// held is its place among the tracks, routes or waypoints carried beside
	// the file, or -1 when the file itself holds it.
	disk int
	held int
}

var errNoSuchPart = errors.New("no such part; reload the page")

// locate finds the part a key names.
func locate(a analysis, key string) (place, error) {
	kind, _, err := partKey(key)
	if err != nil {
		return place{}, err
	}
	keys, onDisk := a.keys.Waypoints, a.layout.Waypoints
	switch kind {
	case 't':
		keys, onDisk = a.keys.Tracks, a.layout.Tracks
	case 'r':
		keys, onDisk = a.keys.Routes, a.layout.Routes
	}
	for i, name := range keys {
		if name != key {
			continue
		}
		found := place{kind: kind, shown: i, disk: -1, held: -1}
		if index, err := indexOf(key); err == nil && index < onDisk {
			found.disk = index
		} else if err == nil {
			found.held = index - onDisk
		}
		return found, nil
	}
	return place{}, errNoSuchPart
}

func indexOf(key string) (int, error) {
	_, index, err := partKey(key)
	return index, err
}

// dropKey takes a part carried beside a file out of the orders kept for it.
// The parts carried after it are keyed one lower from now on, so their keys in
// the order are too.
func dropKey(order map[string][]string, key string, at place, layout partCounts) map[string][]string {
	letter := string(at.kind)
	keys, ok := order[letter]
	if !ok {
		return order
	}
	onDisk := map[byte]int{'t': layout.Tracks, 'r': layout.Routes, 'w': layout.Waypoints}[at.kind]
	gone := onDisk + at.held
	var kept []string
	for _, one := range keys {
		index, err := indexOf(one)
		switch {
		case one == key || err != nil:
			continue
		case index > gone:
			one = fmt.Sprintf("%s%d", letter, index-1)
		}
		kept = append(kept, one)
	}
	order[letter] = kept
	return order
}

// withoutAdded takes the tracks added here out of the track order, once they
// are written into a file of their own.
func withoutAdded(order map[string][]string, onDisk int) map[string][]string {
	keys, ok := order["t"]
	if !ok {
		return order
	}
	var kept []string
	for _, key := range keys {
		if index, err := indexOf(key); err == nil && index < onDisk {
			kept = append(kept, key)
		}
	}
	order["t"] = kept
	return order
}

// deletePart takes one track, route or waypoint out of a file. A part of a GPX
// dgs wrote is taken out of the file when the edit is saved; a part carried
// beside the file goes at once, with the edits on its points. A recording is
// never written, so only a track added here can be taken out of it.
func (a api) deletePart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Key  string `json:"key"`
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
	at, err := locate(result, body.Key)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	file := result.cleaning
	switch {
	// A track added from another file, or a route or waypoint copied in, is
	// not in the GPX yet: it goes from what is held beside it.
	case at.held >= 0:
		switch at.kind {
		case 't':
			if first, last, ok := trackRange(result, at.shown); ok {
				file.Delete(first, last)
			}
			file.Added = append(file.Added[:at.held:at.held], file.Added[at.held+1:]...)
		case 'r':
			file.Routes = append(file.Routes[:at.held:at.held], file.Routes[at.held+1:]...)
		default:
			file.Waypoints = append(file.Waypoints[:at.held:at.held], file.Waypoints[at.held+1:]...)
		}
		delete(file.Moved, body.Key)
		file.Order = dropKey(file.Order, body.Key, at, result.layout)
	// The GPX itself holds it. Only a GPX dgs wrote is written to.
	case result.file.IsOurs():
		if at.kind == 't' {
			if first, last, ok := trackRange(result, at.shown); ok {
				file.Delete(first, last)
			}
		}
		file.Deleted = append(file.Deleted, body.Key)
		delete(file.Moved, body.Key)
	default:
		writeError(w, http.StatusUnprocessableEntity, errors.New("this GPX was not written by dgs, so nothing is taken out of it; only a track added here can be taken out again"))
		return
	}
	delete(file.Names, body.Key)
	if len(file.Names) == 0 {
		file.Names = nil
	}
	if len(file.Moved) == 0 {
		file.Moved = nil
	}
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// moveWaypoint puts a waypoint somewhere else, for a pin dropped in the wrong
// place. Only a GPX dgs wrote, or a new one not saved yet, holds waypoints
// that may be moved: a recording is never written, and a waypoint of one is
// the position it was recorded at.
func (a api) moveWaypoint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string  `json:"path"`
		Key  string  `json:"key"`
		Lon  float64 `json:"lon"`
		Lat  float64 `json:"lat"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if !onGlobe(body.Lon, body.Lat) {
		writeError(w, http.StatusBadRequest, errors.New("the position is off the globe"))
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
	if at.kind != 'w' {
		writeError(w, http.StatusBadRequest, errors.New("only a waypoint is moved"))
		return
	}
	if !result.file.IsOurs() && at.held < 0 {
		writeError(w, http.StatusUnprocessableEntity, errors.New("this GPX was not written by dgs, so its waypoints stay where they were recorded"))
		return
	}
	file := result.cleaning
	// A waypoint held beside the file is moved where it is held; one the GPX
	// holds is moved when the edit is saved.
	if at.held >= 0 {
		file.Waypoints[at.held].Lon, file.Waypoints[at.held].Lat = body.Lon, body.Lat
	} else {
		if file.Moved == nil {
			file.Moved = map[string][2]float64{}
		}
		file.Moved[body.Key] = [2]float64{body.Lon, body.Lat}
	}
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// orderParts puts a file's tracks, routes or waypoints in another order. The
// order is held beside the file and taken into the GPX when it is saved, so
// only a GPX dgs wrote, or a new one not saved yet, takes one: a recording is
// never written. The edits on a track's points are kept by point index, so
// when tracks change places every index is moved with its point.
func (a api) orderParts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string   `json:"path"`
		Kind string   `json:"kind"` // "route" or "waypoint"
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	letter := ""
	switch body.Kind {
	case "track":
		letter = "t"
	case "route":
		letter = "r"
	case "waypoint":
		letter = "w"
	default:
		writeError(w, http.StatusBadRequest, errors.New("only tracks, routes and waypoints are put in order"))
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
	if !result.file.IsOurs() && !isDraft(body.Path) {
		writeError(w, http.StatusUnprocessableEntity, errors.New("this GPX was not written by dgs, so its parts keep the order they were recorded in"))
		return
	}
	shown := result.keys.Waypoints
	switch letter {
	case "t":
		shown = result.keys.Tracks
	case "r":
		shown = result.keys.Routes
	}
	if len(body.Keys) != len(shown) {
		writeError(w, http.StatusConflict, errNoSuchPart)
		return
	}
	held := map[string]bool{}
	for _, key := range shown {
		held[key] = true
	}
	for _, key := range body.Keys {
		if !held[key] {
			writeError(w, http.StatusConflict, errNoSuchPart)
			return
		}
		delete(held, key)
	}
	file := result.cleaning
	if file.Order == nil {
		file.Order = map[string][]string{}
	}
	file.Order[letter] = body.Keys
	// A track's points go with it, and so the edits on them: every point
	// index the sidecar keeps is moved to where its point now lies.
	if letter == "t" {
		shownAt := map[string]int{}
		for i, key := range shown {
			shownAt[key] = i
		}
		var runs [][2]int
		for _, key := range body.Keys {
			if first, last, ok := trackRange(result, shownAt[key]); ok {
				runs = append(runs, [2]int{first, last})
			}
		}
		file.Reorder(runs)
	}
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// copyPart copies one track, route or waypoint into another GPX of the
// workspace, and takes it out of this one when it is moved rather than copied.
// Only a GPX dgs wrote, or a new one not saved yet, is copied into: a
// recording is never written.
func (a api) copyPart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path   string `json:"path"`
		Key    string `json:"key"`
		Target string `json:"target"`
		Move   bool   `json:"move"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if samePath(body.Path, body.Target) {
		writeError(w, http.StatusBadRequest, errors.New("choose another GPX to copy into"))
		return
	}
	if err := checkGPX(body.Target); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if !isDraft(body.Target) {
		target, err := openGPX(body.Target)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		if !target.IsOurs() {
			writeError(w, http.StatusUnprocessableEntity, errors.New("that GPX was not written by dgs, so nothing is written into it; convert it to a dgs GPX first"))
			return
		}
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
	if body.Move && at.disk >= 0 && !result.file.IsOurs() {
		writeError(w, http.StatusUnprocessableEntity, errors.New("this GPX was not written by dgs, so nothing is taken out of it; copy the part instead"))
		return
	}
	held, _, err := loadSidecar(body.Target)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	now := time.Now().UnixMilli()
	name := partName(result, body.Key)
	switch at.kind {
	case 't':
		// A track is copied as the page shows it: the points cleaning kept, at
		// the positions it gave them, fills included.
		trk, ok := editedTrack(result, at.shown)
		if !ok {
			writeError(w, http.StatusUnprocessableEntity, errors.New("cleaning left this track no points to copy"))
			return
		}
		trk.Name = name
		held.Added = append(held.Added, compose.AddedFrom(trk, body.Path, now))
	case 'r':
		rte := result.file.Routes[at.shown]
		rte.Name = name
		held.Routes = append(held.Routes, compose.RouteFrom(rte, body.Path, now))
	default:
		wpt := result.file.Waypoints[at.shown]
		wpt.Name = name
		held.Waypoints = append(held.Waypoints, compose.WaypointFrom(wpt, body.Path, now))
	}
	if err := saveSidecar(body.Target, held); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if !body.Move {
		writeJSON(w, map[string]any{"ok": true, "target": body.Target})
		return
	}
	// The copy is in the target; the part goes from this file the same way
	// deleting it does.
	request, err := http.NewRequest(http.MethodDelete, "/api/part", strings.NewReader(fmt.Sprintf("{%q:%q,%q:%q}", "path", body.Path, "key", body.Key)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	recorder := &captured{header: http.Header{}}
	a.deletePart(recorder, request)
	if recorder.status >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(recorder.status)
		_, _ = w.Write(recorder.body)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "target": body.Target})
}

// captured holds what a handler answered, so one handler can do another's work
// and report what it said.
type captured struct {
	header http.Header
	status int
	body   []byte
}

func (c *captured) Header() http.Header { return c.header }
func (c *captured) Write(data []byte) (int, error) {
	c.body = append(c.body, data...)
	return len(data), nil
}
func (c *captured) WriteHeader(status int) { c.status = status }

// partName is the name the page shows for a part.
func partName(a analysis, key string) string {
	at, err := locate(a, key)
	if err != nil {
		return ""
	}
	switch at.kind {
	case 't':
		return named(a, key, a.file.Tracks[at.shown].Name, "Track", nameIndex(key))
	case 'r':
		return named(a, key, a.file.Routes[at.shown].Name, "Route", nameIndex(key))
	}
	return named(a, key, a.file.Waypoints[at.shown].Name, "Waypoint", nameIndex(key))
}

func nameIndex(key string) int {
	index, err := indexOf(key)
	if err != nil {
		return 0
	}
	return index
}

// samePath says whether two paths name the same file. A draft is named by the
// path it was given; a file on disk by where it is.
func samePath(a, b string) bool {
	if a == b {
		return true
	}
	if isDraft(a) || isDraft(b) {
		return false
	}
	left, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	right, err := filepath.Abs(b)
	return err == nil && left == right
}

func onGlobe(lon, lat float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// writeParts takes what was done to the parts of a GPX dgs wrote into the file
// itself: the names given here, the waypoints put somewhere else, the parts
// deleted, and the tracks, routes and waypoints copied into it. It answers
// what is left to keep beside the file — the cleaning, the cuts and the fills,
// whose point indices the file's own order keeps.
//
// The order matters. Names and moves are written while the file still holds
// every part a key counts; the deletions follow, from the last part to the
// first, so an earlier index still names what it named; the parts copied in
// are appended after the file's own; and last every kind is put in the order
// the page gave. The point indices kept beside the file already count the
// tracks in that order, so once the file holds them so, no index moves.
func writeParts(path string, onDisk *gpxfile.File, held sidecar.File) (sidecar.File, error) {
	if len(held.Names) == 0 && len(held.Moved) == 0 && len(held.Deleted) == 0 && len(held.Order) == 0 &&
		len(held.Added) == 0 && len(held.Routes) == 0 && len(held.Waypoints) == 0 {
		return held, nil
	}
	layout := partCounts{Tracks: len(onDisk.Tracks), Routes: len(onDisk.Routes), Waypoints: len(onDisk.Waypoints)}
	counts := map[byte]int{'t': layout.Tracks, 'r': layout.Routes, 'w': layout.Waypoints}
	parts := map[byte]gpxfile.Part{'t': gpxfile.PartTrack, 'r': gpxfile.PartRoute, 'w': gpxfile.PartWaypoint}
	deleted := map[string]bool{}
	for _, key := range held.Deleted {
		deleted[key] = true
	}
	// The names of the parts the file already holds.
	for key, name := range held.Names {
		kind, index, err := partKey(key)
		if err != nil || index >= counts[kind] || deleted[key] {
			continue
		}
		if err := gpxfile.RenamePart(path, parts[kind], index, name); err != nil {
			return held, err
		}
	}
	for key, at := range held.Moved {
		kind, index, err := partKey(key)
		if err != nil || kind != 'w' || index >= counts['w'] || deleted[key] {
			continue
		}
		if err := gpxfile.MoveWaypoint(path, index, geo.LatLon{Lat: at[1], Lon: at[0]}); err != nil {
			return held, err
		}
	}
	// The deletions, last part first.
	for _, kind := range []byte{'t', 'r', 'w'} {
		var indices []int
		for key := range deleted {
			if at, index, err := partKey(key); err == nil && at == kind && index < counts[kind] {
				indices = append(indices, index)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(indices)))
		for _, index := range indices {
			if err := gpxfile.RemovePart(path, parts[kind], index); err != nil {
				return held, err
			}
		}
	}
	// The parts copied in, in the order they were laid over the file, each
	// under the name the page gave it.
	named := func(kind byte, index int, name string) string {
		if given, ok := held.Names[fmt.Sprintf("%c%d", kind, counts[kind]+index)]; ok {
			return given
		}
		return name
	}
	for i, route := range held.Routes {
		written := route.Route()
		written.Name = named('r', i, written.Name)
		if err := gpxfile.AddRoute(path, written); err != nil {
			return held, err
		}
	}
	for i, waypoint := range held.Waypoints {
		written := waypoint.Point()
		written.Name = named('w', i, written.Name)
		if err := gpxfile.AddWaypoint(path, written); err != nil {
			return held, err
		}
	}
	tracks := addedTracks(held.Added)
	for i := range tracks {
		tracks[i].Name = named('t', i, tracks[i].Name)
	}
	if len(tracks) > 0 {
		if err := gpxfile.Append(path, tracks); err != nil {
			return held, err
		}
	}
	// The file now holds every part, in its own order: the parts it had, then
	// the ones copied in. What the page put in another order is put there in
	// the file itself, so the order is in the GPX and nothing keeps it beside.
	for _, kind := range []struct {
		letter byte
		part   gpxfile.Part
		onDisk int
		copied int
	}{
		{'t', gpxfile.PartTrack, layout.Tracks, len(held.Added)},
		{'r', gpxfile.PartRoute, layout.Routes, len(held.Routes)},
		{'w', gpxfile.PartWaypoint, layout.Waypoints, len(held.Waypoints)},
	} {
		wanted := held.Order[string(kind.letter)]
		if len(wanted) == 0 {
			continue
		}
		// The keys of the parts the file holds now, in the file's own order.
		var inFile []string
		for i := 0; i < kind.onDisk+kind.copied; i++ {
			key := fmt.Sprintf("%c%d", kind.letter, i)
			if i < kind.onDisk && deleted[key] {
				continue
			}
			inFile = append(inFile, key)
		}
		at := map[string]int{}
		for i, key := range inFile {
			at[key] = i
		}
		order, taken := make([]int, 0, len(inFile)), make([]bool, len(inFile))
		for _, key := range wanted {
			if i, ok := at[key]; ok && !taken[i] {
				taken[i] = true
				order = append(order, i)
			}
		}
		for i := range inFile {
			if !taken[i] {
				order = append(order, i)
			}
		}
		if err := gpxfile.ReorderParts(path, kind.part, order); err != nil {
			return held, err
		}
	}
	// Everything above is in the file now, under the names the page gave it,
	// so nothing of it is kept beside the file.
	held.Names, held.Moved, held.Deleted, held.Order = nil, nil, nil, nil
	held.Added, held.Routes, held.Waypoints = nil, nil, nil
	return held, nil
}
