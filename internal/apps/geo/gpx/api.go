package gpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/desktop"
	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gcj02"
	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/stops"
	"dgs-toolbox/internal/geo/track"
)

// baseMap is a base map as the page receives it. URLs are the tile templates
// with any {s} already expanded; Overlay, when present, is drawn above the
// tiles, as road names above satellite imagery.
type baseMap struct {
	Name        string   `json:"name"`
	URLs        []string `json:"urls"`
	Overlay     []string `json:"overlay,omitempty"`
	Attribution string   `json:"attribution"`
	MaxZoom     int      `json:"maxZoom"`
	Coordinates string   `json:"coordinates"` // "wgs84" or "gcj02"
}

const (
	systemWGS84 = "wgs84"
	systemGCJ02 = "gcj02"
)

// builtInMaps are offered on every page, before the configured ones. Gaode
// and Esri need no key; Gaode draws in GCJ-02, as maps published in China must.
var builtInMaps = []baseMap{
	{
		Name:        "OpenStreetMap",
		URLs:        []string{"https://tile.openstreetmap.org/{z}/{x}/{y}.png"},
		Attribution: "© OpenStreetMap contributors",
		MaxZoom:     19,
		Coordinates: systemWGS84,
	},
	{
		Name:        "Gaode",
		URLs:        subdomains("https://webrd0{s}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}", "1", "2", "3", "4"),
		Attribution: "© AutoNavi",
		MaxZoom:     18,
		Coordinates: systemGCJ02,
	},
	{
		Name:        "Gaode Satellite",
		URLs:        subdomains("https://wprd0{s}.is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}", "1", "2", "3", "4"),
		Overlay:     subdomains("https://wprd0{s}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scl=1&style=8&ltype=11&x={x}&y={y}&z={z}", "1", "2", "3", "4"),
		Attribution: "© AutoNavi",
		MaxZoom:     18,
		Coordinates: systemGCJ02,
	},
	{
		Name:        "Esri World Imagery",
		URLs:        []string{"https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}"},
		Attribution: "Esri, Maxar, Earthstar Geographics",
		MaxZoom:     19,
		Coordinates: systemWGS84,
	},
}

// subdomains expands {s} in a template, once per subdomain.
func subdomains(template string, names ...string) []string {
	if !strings.Contains(template, "{s}") {
		return []string{template}
	}
	urls := make([]string, len(names))
	for i, name := range names {
		urls[i] = strings.ReplaceAll(template, "{s}", name)
	}
	return urls
}

type api struct {
	settings Settings
	board    *focusBoard
}

func (a api) config(w http.ResponseWriter, r *http.Request) {
	tiles := append([]baseMap{}, builtInMaps...)
	for _, tile := range a.settings.Tiles {
		tiles = append(tiles, configuredMap(tile))
	}
	writeJSON(w, map[string]any{"root": a.settings.Root, "tiles": tiles, "canReveal": isLocal(r)})
}

func configuredMap(tile config.GeoGPXTile) baseMap {
	m := baseMap{
		Name:        tile.Name,
		URLs:        subdomains(tile.URL, "a", "b", "c"),
		Attribution: tile.Attribution,
		MaxZoom:     tile.MaxZoom,
		Coordinates: tile.Coordinates,
	}
	if m.MaxZoom == 0 {
		m.MaxZoom = 19
	}
	if m.Coordinates == "" {
		m.Coordinates = systemWGS84
	}
	return m
}

type dirEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size,omitempty"`
	Modified time.Time `json:"modified,omitzero"`
}

// dir lists the folders and GPX files of one folder. Hidden entries are left
// out; everything that is not a folder or a .gpx file is too.
func (a api) dir(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = a.settings.Root
	}
	path, err := filepath.Abs(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	dirs, files := []dirEntry{}, []dirEntry{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(path, name)
		info, err := os.Stat(full) // follows symlinks, so a linked folder is a folder
		if err != nil {
			continue
		}
		switch {
		case info.IsDir():
			dirs = append(dirs, dirEntry{Name: name, Path: full})
		case strings.EqualFold(filepath.Ext(name), ".gpx"):
			files = append(files, dirEntry{Name: name, Path: full, Size: info.Size(), Modified: info.ModTime()})
		}
	}
	byName := func(entries []dirEntry) {
		sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name) })
	}
	byName(dirs)
	byName(files)
	parent := filepath.Dir(path)
	if parent == path {
		parent = ""
	}
	writeJSON(w, map[string]any{"path": path, "parent": parent, "dirs": dirs, "files": files})
}

// trackJSON is a track as the page draws it: parallel arrays, one entry per
// point, so a long track stays compact. Missing values are null.
type trackJSON struct {
	Path      string       `json:"path"`
	Name      string       `json:"name"`
	Points    [][2]float64 `json:"points"` // [lon, lat], the order map libraries take
	Segments  []int        `json:"segments"`
	Distance  []float64    `json:"distance"`  // metres from the start
	Elevation []*float64   `json:"elevation"` // metres
	Speed     []*float64   `json:"speed"`     // metres per second
	Time      []*int64     `json:"time"`      // Unix milliseconds
	Stats     statsJSON    `json:"stats"`
	Stops     []stopJSON   `json:"stops"`
	// Parts are what the file holds, for listing and showing one at a time:
	// each <trk> (its points are first..last of the arrays above), then each
	// <rte> and <wpt>.
	Parts []partJSON `json:"parts"`
	// Coordinates is the system positions are drawn in: "wgs84", or "gcj02"
	// when the request asked for positions to lay over a GCJ-02 map. Only
	// points, stop centres and bounds change; distances and speeds do not.
	Coordinates string `json:"coordinates"`
	// StopParams are the stay-point thresholds the stops were found with.
	StopParams stopParamsJSON `json:"stopParams"`
}

// partJSON is one track, route or waypoint of a file. Key names it for the
// page: "t0", "r0", "w0".
type partJSON struct {
	Key         string        `json:"key"`
	Kind        string        `json:"kind"` // "track", "route" or "waypoint"
	Name        string        `json:"name"`
	First       int           `json:"first"` // a track's first point index
	Last        int           `json:"last"`
	Segments    int           `json:"segments,omitempty"`
	Points      [][2]float64  `json:"points,omitempty"` // a route's path, a waypoint's position
	Distance    float64       `json:"distance,omitempty"`
	Description string        `json:"description,omitempty"`
	Elevation   *float64      `json:"elevation,omitempty"`
	Time        *int64        `json:"time,omitempty"`
	Bounds      [2][2]float64 `json:"bounds"`
}

type stopJSON struct {
	First     int        `json:"first"` // index of the first point in the stop
	Last      int        `json:"last"`
	Center    [2]float64 `json:"center"`  // [lon, lat]
	Arrival   int64      `json:"arrival"` // Unix milliseconds
	Departure int64      `json:"departure"`
	Duration  float64    `json:"duration"` // seconds
}

type stopParamsJSON struct {
	Distance float64 `json:"distance"` // metres
	Duration float64 `json:"duration"` // seconds
}

type statsJSON struct {
	Distance float64       `json:"distance"`
	Duration float64       `json:"duration"` // seconds
	Ascent   float64       `json:"ascent"`
	Descent  float64       `json:"descent"`
	Start    *time.Time    `json:"start"`
	End      *time.Time    `json:"end"`
	Bounds   [2][2]float64 `json:"bounds"`     // [[west, south], [east, north]] of every point
	View     [2][2]float64 `json:"viewBounds"` // the same for the bulk of the points; frame views with it
	// InChina says some point lies where GCJ-02 maps are offset from WGS-84.
	InChina bool `json:"inChina"`
}

func (a api) track(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	params, err := stopParams(query.Get("stopDistance"), query.Get("stopDuration"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	coordinates := query.Get("coordinates")
	if coordinates == "" {
		coordinates = systemWGS84
	}
	if coordinates != systemWGS84 && coordinates != systemGCJ02 {
		writeError(w, http.StatusBadRequest, errors.New("coordinates must be systemWGS84 or gcj02"))
		return
	}
	analysis, err := analyse(query.Get("path"), params)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	response := trackResponse(analysis)
	if coordinates == systemGCJ02 {
		toGCJ02(&response)
	}
	writeJSON(w, response)
}

// stopParams reads the stay-point thresholds of a request: metres and seconds,
// each falling back to its default when absent.
func stopParams(distance, duration string) (stops.Params, error) {
	params := stops.Defaults()
	if distance != "" {
		value, err := strconv.ParseFloat(distance, 64)
		if err != nil || !(value > 0) || math.IsInf(value, 0) {
			return params, errors.New("stopDistance must be a positive number of metres")
		}
		params.Distance = value
	}
	if duration != "" {
		value, err := strconv.ParseFloat(duration, 64)
		if err != nil || !(value > 0) || value > 1e9 {
			return params, errors.New("stopDuration must be a positive number of seconds")
		}
		params.Duration = time.Duration(value * float64(time.Second))
	}
	return params, nil
}

// analysis is one GPX file with everything the page and the TUI show of it.
type analysis struct {
	path      string
	name      string
	file      *gpxfile.File
	line      track.Line
	distances []float64
	stats     track.Stats
	stops     []stops.Stop
	params    stops.Params
}

var errNotGPX = errors.New("not a .gpx file")

func analyse(path string, params stops.Params) (analysis, error) {
	if !strings.EqualFold(filepath.Ext(path), ".gpx") {
		return analysis{}, errNotGPX
	}
	file, err := gpxfile.Open(path)
	if err != nil {
		return analysis{}, err
	}
	line := track.FromGPX(file)
	distances := track.Distances(line)
	return analysis{
		path: path,
		// The file name is the one the reader chose; the name inside is often a
		// recorder's timestamp.
		name:      strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		file:      file,
		line:      line,
		distances: distances,
		stats:     track.Summarise(line, distances),
		stops:     stops.Detect(line, params),
		params:    params,
	}, nil
}

func trackResponse(a analysis) trackJSON {
	line, distances, stats := a.line, a.distances, a.stats
	speeds := track.Speeds(line, distances, track.DefaultSpeedWindow)

	response := trackJSON{
		Path:      a.path,
		Name:      a.name,
		Points:    make([][2]float64, len(line)),
		Segments:  make([]int, len(line)),
		Distance:  distances,
		Elevation: make([]*float64, len(line)),
		Speed:     make([]*float64, len(line)),
		Time:      make([]*int64, len(line)),
		Stats: statsJSON{
			Distance: stats.Distance,
			Duration: stats.Duration.Seconds(),
			Ascent:   stats.Ascent,
			Descent:  stats.Descent,
		},
		Coordinates: systemWGS84,
		Stops:       make([]stopJSON, len(a.stops)),
		StopParams:  stopParamsJSON{Distance: a.params.Distance, Duration: a.params.Duration.Seconds()},
	}
	for i, stop := range a.stops {
		response.Stops[i] = stopJSON{
			First:     stop.First,
			Last:      stop.Last,
			Center:    [2]float64{stop.Center.Lon, stop.Center.Lat},
			Arrival:   stop.Arrival.UnixMilli(),
			Departure: stop.Departure.UnixMilli(),
			Duration:  stop.Duration().Seconds(),
		}
	}
	for i, sample := range line {
		response.Stats.InChina = response.Stats.InChina || gcj02.InChina(sample.LatLon)
		response.Points[i] = [2]float64{sample.Lon, sample.Lat}
		response.Segments[i] = sample.Segment
		if sample.HasElevation {
			response.Elevation[i] = &line[i].Elevation
		}
		if !math.IsNaN(speeds[i]) {
			response.Speed[i] = &speeds[i]
		}
		if !sample.Time.IsZero() {
			millis := sample.Time.UnixMilli()
			response.Time[i] = &millis
		}
	}
	if !stats.Start.IsZero() {
		response.Stats.Start, response.Stats.End = &stats.Start, &stats.End
	}
	bounds, view := stats.Bounds, geo.Empty()
	if len(line) > 0 {
		view = track.CoreBounds(line, track.DefaultCoreTrim)
	}
	response.Parts, bounds, view = parts(a, bounds, view)
	for _, part := range response.Parts {
		for _, point := range part.Points {
			response.Stats.InChina = response.Stats.InChina || gcj02.InChina(geo.LatLon{Lat: point[1], Lon: point[0]})
		}
	}
	if !bounds.IsEmpty() {
		response.Stats.Bounds = lonLatBox(bounds)
		response.Stats.View = lonLatBox(view)
	}
	return response
}

// parts lists a file's tracks, routes and waypoints, growing the file's boxes
// to hold the routes and waypoints too.
func parts(a analysis, bounds, view geo.Bounds) ([]partJSON, geo.Bounds, geo.Bounds) {
	list := []partJSON{}
	first := 0
	for i, trk := range a.file.Tracks {
		part := partJSON{Key: fmt.Sprintf("t%d", i), Kind: "track", Name: orNumbered(trk.Name, "Track", i)}
		box, count := geo.Empty(), 0
		for _, seg := range trk.Segments {
			if len(seg.Points) > 0 {
				part.Segments++
			}
			for _, pt := range seg.Points {
				box = box.Extend(pt.LatLon)
				count++
			}
		}
		if count == 0 {
			continue
		}
		part.First, part.Last = first, first+count-1
		part.Distance = a.distances[part.Last] - a.distances[part.First]
		part.Bounds = lonLatBox(box)
		first += count
		list = append(list, part)
	}
	for i, rte := range a.file.Routes {
		part := partJSON{Key: fmt.Sprintf("r%d", i), Kind: "route", Name: orNumbered(rte.Name, "Route", i)}
		path, box := make([]geo.LatLon, len(rte.Points)), geo.Empty()
		for j, pt := range rte.Points {
			path[j] = pt.LatLon
			box = box.Extend(pt.LatLon)
			part.Points = append(part.Points, [2]float64{pt.Lon, pt.Lat})
		}
		if len(path) == 0 {
			continue
		}
		part.Distance = geo.PathLength(path)
		part.Bounds = lonLatBox(box)
		bounds, view = extend(bounds, box), extend(view, box)
		list = append(list, part)
	}
	for i, wpt := range a.file.Waypoints {
		part := partJSON{
			Key:         fmt.Sprintf("w%d", i),
			Kind:        "waypoint",
			Name:        orNumbered(wpt.Name, "Waypoint", i),
			Description: wpt.Description,
			Points:      [][2]float64{{wpt.Lon, wpt.Lat}},
		}
		if wpt.HasElevation {
			elevation := wpt.Elevation
			part.Elevation = &elevation
		}
		if !wpt.Time.IsZero() {
			millis := wpt.Time.UnixMilli()
			part.Time = &millis
		}
		box := geo.Empty().Extend(wpt.LatLon)
		part.Bounds = lonLatBox(box)
		bounds, view = extend(bounds, box), extend(view, box)
		list = append(list, part)
	}
	return list, bounds, view
}

func orNumbered(name, kind string, index int) string {
	if name != "" {
		return name
	}
	return fmt.Sprintf("%s %d", kind, index+1)
}

func extend(b, by geo.Bounds) geo.Bounds {
	if by.IsEmpty() {
		return b
	}
	return b.Extend(by.Min).Extend(by.Max)
}

// toGCJ02 moves a track's drawn positions onto GCJ-02 maps.
func toGCJ02(response *trackJSON) {
	convert := func(lonLat [2]float64) [2]float64 {
		p := gcj02.FromWGS84(geo.LatLon{Lat: lonLat[1], Lon: lonLat[0]})
		return [2]float64{p.Lon, p.Lat}
	}
	for i, point := range response.Points {
		response.Points[i] = convert(point)
	}
	for i := range response.Stops {
		response.Stops[i].Center = convert(response.Stops[i].Center)
	}
	for i := range response.Parts {
		part := &response.Parts[i]
		for j, point := range part.Points {
			part.Points[j] = convert(point)
		}
		part.Bounds[0], part.Bounds[1] = convert(part.Bounds[0]), convert(part.Bounds[1])
	}
	for _, box := range []*[2][2]float64{&response.Stats.Bounds, &response.Stats.View} {
		box[0], box[1] = convert(box[0]), convert(box[1])
	}
	response.Coordinates = systemGCJ02
}

// reveal shows a file or folder in this machine's file manager. Only a browser
// on this machine may ask: from anywhere else it would open windows on a
// screen the asker cannot see.
func (a api) reveal(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		writeError(w, http.StatusForbidden, errors.New("only a browser on this machine can open its file manager"))
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, errors.New("path is required"))
		return
	}
	if _, err := os.Stat(body.Path); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	reveal := a.settings.Reveal
	if reveal == nil {
		reveal = desktop.Reveal
	}
	if err := reveal(body.Path); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// isLocal reports whether a request comes from this machine: from loopback, or
// from one of this machine's own addresses, as when the page is opened at the
// LAN URL on the machine serving it.
func isLocal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if network, ok := addr.(*net.IPNet); ok && network.IP.Equal(ip) {
			return true
		}
	}
	return false
}

func lonLatBox(b geo.Bounds) [2][2]float64 {
	return [2][2]float64{{b.Min.Lon, b.Min.Lat}, {b.Max.Lon, b.Max.Lat}}
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, errNotGPX):
		return http.StatusBadRequest
	case errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, os.ErrPermission):
		return http.StatusForbidden
	}
	return http.StatusUnprocessableEntity
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
