package gpx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/geo/gpxfile"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "geo", "gpx")
}

func get(t *testing.T, server *httptest.Server, path string, into any) int {
	t.Helper()
	response, err := http.Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if into != nil {
		if err := json.Unmarshal(body, into); err != nil {
			t.Fatalf("GET %s: %v\n%s", path, err, body)
		}
	}
	return response.StatusCode
}

func TestHandlerServesPage(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	response, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "<title>") {
		t.Fatalf("GET / = %d", response.StatusCode)
	}
}

func TestConfigPutsOpenStreetMapFirst(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{Root: "/data", Tiles: []config.GeoGPXTile{{Name: "Mine", URL: "https://t/{z}/{x}/{y}.png"}}}))
	defer server.Close()
	var got struct {
		Root  string    `json:"root"`
		Tiles []baseMap `json:"tiles"`
	}
	get(t, server, "/api/config", &got)
	n := len(builtInMaps)
	if got.Root != "/data" || len(got.Tiles) != n+1 || got.Tiles[0].Name != "OpenStreetMap" {
		t.Fatalf("config = %+v", got)
	}
	mine := got.Tiles[n]
	if mine.Name != "Mine" || mine.MaxZoom != 19 || mine.Coordinates != "wgs84" || len(mine.URLs) != 1 {
		t.Fatalf("configured map = %+v", mine)
	}
	for _, tile := range got.Tiles[:n] {
		if tile.Name == "Gaode" && (tile.Coordinates != "gcj02" || len(tile.URLs) != 4 || strings.Contains(tile.URLs[0], "{s}")) {
			t.Fatalf("Gaode = %+v", tile)
		}
	}
}

func TestDirListsFoldersAndGPXFiles(t *testing.T) {
	root := testdataDir(t)
	server := httptest.NewServer(Handler(Settings{Root: root}))
	defer server.Close()
	var got struct {
		Path   string     `json:"path"`
		Parent string     `json:"parent"`
		Dirs   []dirEntry `json:"dirs"`
		Files  []dirEntry `json:"files"`
	}
	if status := get(t, server, "/api/dir", &got); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if len(got.Dirs) != 1 || got.Dirs[0].Name != "2026-09-hangzhou" || len(got.Files) != 1 || got.Files[0].Name != "planned-no-time.gpx" {
		t.Fatalf("dir = %+v", got)
	}
	if status := get(t, server, "/api/dir?path="+url.QueryEscape(filepath.Join(root, "missing")), nil); status != http.StatusNotFound {
		t.Fatalf("missing dir status %d", status)
	}
}

func TestTrackCarriesSeries(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := filepath.Join(testdataDir(t), "2026-09-hangzhou", "hangzhou-jiande-drive.gpx")
	var got trackJSON
	if status := get(t, server, "/api/track?path="+url.QueryEscape(path), &got); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	n := len(got.Points)
	if n == 0 || len(got.Distance) != n || len(got.Elevation) != n || len(got.Speed) != n || len(got.Time) != n {
		t.Fatalf("series lengths differ: %d points", n)
	}
	if got.Name != "hangzhou-jiande-drive" || got.Segments[n-1] != 1 || got.Stats.Distance < 90000 || got.Stats.Start == nil {
		t.Fatalf("track = %s, %d segments, %.0f m", got.Name, got.Segments[n-1]+1, got.Stats.Distance)
	}

	var planned trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(filepath.Join(testdataDir(t), "planned-no-time.gpx")), &planned)
	for i, speed := range planned.Speed {
		if speed != nil || planned.Time[i] != nil {
			t.Fatalf("untimed point %d has speed or time", i)
		}
	}
}

func TestTrackRefusesOtherFiles(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	if status := get(t, server, "/api/track?path=/etc/passwd", nil); status != http.StatusBadRequest {
		t.Fatalf("status %d", status)
	}
}

func TestRevealOnlyForThisMachine(t *testing.T) {
	var revealed string
	handler := Handler(Settings{Reveal: func(path string) error {
		revealed = path
		return nil
	}})
	path := filepath.Join(testdataDir(t), "planned-no-time.gpx")
	post := func(remote, body string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/reveal", strings.NewReader(body))
		request.RemoteAddr = remote
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Code
	}
	body, _ := json.Marshal(map[string]string{"path": path})
	if code := post("203.0.113.9:5000", string(body)); code != http.StatusForbidden || revealed != "" {
		t.Fatalf("remote reveal = %d, revealed %q", code, revealed)
	}
	if code := post("127.0.0.1:5000", `{"path":"/does/not/exist"}`); code != http.StatusNotFound {
		t.Fatalf("missing path = %d", code)
	}
	if code := post("127.0.0.1:5000", string(body)); code != http.StatusOK || revealed != path {
		t.Fatalf("local reveal = %d, revealed %q", code, revealed)
	}
}

// writeStayGPX writes a track that walks, stays ten minutes, and walks on.
func writeStayGPX(t *testing.T) string {
	t.Helper()
	var points strings.Builder
	start := time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	lat := 30.0
	add := func(second int) {
		fmt.Fprintf(&points, `<trkpt lat="%.7f" lon="120"><time>%s</time></trkpt>`, lat, start.Add(time.Duration(second)*time.Second).Format(time.RFC3339))
	}
	second := 0
	for ; second < 120; second += 5 {
		lat += 7.5 / 111195
		add(second)
	}
	for ; second < 720; second += 5 {
		add(second)
	}
	for ; second < 840; second += 5 {
		lat += 7.5 / 111195
		add(second)
	}
	path := filepath.Join(t.TempDir(), "stay.gpx")
	body := `<gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1"><trk><trkseg>` + points.String() + `</trkseg></trk></gpx>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTrackCarriesStops(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := writeStayGPX(t)
	var got trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &got)
	if len(got.Stops) != 1 || got.Stops[0].Duration < 600 || got.StopParams.Distance != 50 || got.StopParams.Duration != 300 {
		t.Fatalf("stops = %+v with %+v", got.Stops, got.StopParams)
	}
	var strict trackJSON
	get(t, server, "/api/track?stopDuration=1800&path="+url.QueryEscape(path), &strict)
	if len(strict.Stops) != 0 || strict.StopParams.Duration != 1800 {
		t.Fatalf("30 min stops = %+v", strict.Stops)
	}
	var shifted trackJSON
	get(t, server, "/api/track?coordinates=gcj02&path="+url.QueryEscape(path), &shifted)
	if shifted.Coordinates != "gcj02" || shifted.Points[0] == got.Points[0] || shifted.Stops[0].Center == got.Stops[0].Center || shifted.Distance[10] != got.Distance[10] {
		t.Fatalf("gcj02 track did not move its positions only")
	}
	if status := get(t, server, "/api/track?coordinates=bd09&path="+url.QueryEscape(path), nil); status != http.StatusBadRequest {
		t.Fatalf("bd09 status %d", status)
	}
	if status := get(t, server, "/api/track?stopDistance=-1&path="+url.QueryEscape(path), nil); status != http.StatusBadRequest {
		t.Fatalf("negative distance status %d", status)
	}
}

func TestFocusReachesTheBoard(t *testing.T) {
	board := newFocusBoard()
	server := httptest.NewServer(Handler(Settings{focus: board}))
	defer server.Close()
	_, changed := board.current()
	path := writeStayGPX(t)
	body, _ := json.Marshal(map[string]any{"path": path, "stopDuration": 300})
	response, err := http.Post(server.URL+"/api/focus", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	select {
	case <-changed:
	default:
		t.Fatal("focus did not signal a change")
	}
	summary, _ := board.current()
	if response.StatusCode != http.StatusOK || summary == nil || summary.Name != "stay" || summary.Stops != 1 || summary.Duration != 835*time.Second {
		t.Fatalf("status %d, summary %+v", response.StatusCode, summary)
	}
	response, _ = http.Post(server.URL+"/api/focus", "application/json", strings.NewReader(`{"path":""}`))
	response.Body.Close()
	if summary, _ := board.current(); summary != nil {
		t.Fatalf("cleared focus is %+v", summary)
	}
}

func TestTrackListsParts(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := filepath.Join(testdataDir(t), "2026-09-hangzhou", "day-out-tracks-route-waypoints.gpx")
	var got trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &got)
	kinds := []string{}
	for _, part := range got.Parts {
		kinds = append(kinds, part.Kind+":"+part.Name)
	}
	want := "track:Morning walk,track:Afternoon ride,route:Planned loop,waypoint:Hotel,waypoint:Leifeng Pagoda"
	if strings.Join(kinds, ",") != want {
		t.Fatalf("parts = %v", kinds)
	}
	ride := got.Parts[1]
	if ride.First != 60 || ride.Last != 139 || ride.Segments != 2 || ride.Distance <= 0 || got.Parts[2].Distance <= 0 {
		t.Fatalf("ride = %+v", ride)
	}
	if got.Parts[3].Description != "Night one" || got.Parts[3].Elevation == nil || !got.Stats.InChina {
		t.Fatalf("hotel = %+v", got.Parts[3])
	}
}

func TestCleanIsSavedAndApplied(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := writeStayGPX(t)
	put := func(body string) int {
		request, _ := http.NewRequest(http.MethodPut, server.URL+"/api/clean", strings.NewReader(body))
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	var before trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &before)
	if before.Clean.Sidecar != "" || before.Clean.Params.Spikes.MaxSpeed != 100 || len(before.Removed) != len(before.Points) {
		t.Fatalf("uncleaned = %+v", before.Clean)
	}

	quoted, _ := json.Marshal(path)
	body := `{"path":` + string(quoted) + `,"clean":{"stops":{"enabled":true},"smooth":{"enabled":true},"edits":[{"kind":"lasso","points":[0,1],"join":true}]}}`
	if code := put(body); code != http.StatusOK {
		t.Fatalf("PUT = %d", code)
	}
	var after trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &after)
	c := after.Clean
	if c.Sidecar == "" || c.Counts.Manual != 2 || c.Counts.Stop == 0 || c.Counts.Moved == 0 || after.Removed[0] != "manual" || len(after.Original) != len(after.Points) {
		t.Fatalf("cleaned = %+v", c)
	}
	if after.Speed[0] != nil || after.Distance[1] != 0 || len(after.Stops) != 1 || after.Removed[after.Stops[0].First] != "" {
		t.Fatalf("removed points or stops wrong: speed %v distance %v stops %+v", after.Speed[0], after.Distance[1], after.Stops)
	}

	rangeBody := `{"path":` + string(quoted) + `,"clean":{"ranges":[{"first":20,"last":29},{"first":40,"last":49,"join":true}]}}`
	if code := put(rangeBody); code != http.StatusOK {
		t.Fatalf("PUT ranges = %d", code)
	}
	var ranged trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &ranged)
	if ranged.Clean.Counts.Manual != 20 || ranged.Segments[19] == ranged.Segments[30] || ranged.Segments[39] != ranged.Segments[50] {
		t.Fatalf("ranges: manual %d, segments %d/%d %d/%d", ranged.Clean.Counts.Manual, ranged.Segments[19], ranged.Segments[30], ranged.Segments[39], ranged.Segments[50])
	}
	if ranged.Distance[30] != ranged.Distance[19] {
		t.Fatalf("a broken range added distance: %v → %v", ranged.Distance[19], ranged.Distance[30])
	}
	if code := put(`{"path":` + string(quoted) + `,"clean":{"ranges":[{"first":5,"last":2}]}}`); code != http.StatusBadRequest {
		t.Fatalf("backwards range = %d", code)
	}
	if code := put(`{"path":` + string(quoted) + `,"clean":{"spikes":{"maxSpeed":-1}}}`); code != http.StatusBadRequest {
		t.Fatalf("negative speed = %d", code)
	}
	if code := put(`{"path":` + string(quoted) + `,"clean":{}}`); code != http.StatusOK {
		t.Fatalf("clearing = %d", code)
	}
	if _, err := os.Stat(path + ".dgs.json"); !os.IsNotExist(err) {
		t.Fatalf("sidecar left after clearing: %v", err)
	}
}

func send(t *testing.T, server *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	data, _ := json.Marshal(body)
	request, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	_ = json.NewDecoder(response.Body).Decode(&result)
	return response.StatusCode, result
}

func TestSegmentsAreCutNamedAndWritten(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := writeStayGPX(t)
	var track trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &track)
	if len(track.Pieces) != 1 || len(track.CutCandidates) != 1 || len(track.Cuts) != 0 {
		t.Fatalf("uncut: pieces %v candidates %v", track.Pieces, track.CutCandidates)
	}
	cut := track.CutCandidates[0]
	if code, _ := send(t, server, http.MethodPut, "/api/segments", map[string]any{
		"path": path, "cuts": []int{cut}, "names": []map[string]any{{"start": cut, "name": " After "}, {"start": 0, "name": " "}},
	}); code != http.StatusOK {
		t.Fatalf("PUT segments = %d", code)
	}
	get(t, server, "/api/track?path="+url.QueryEscape(path), &track)
	pieces := track.Pieces
	if len(pieces) != 2 || pieces[0].Last != cut || pieces[1].First != cut || pieces[1].Name != "After" || pieces[0].Name != "" || track.Cuts[0] != cut {
		t.Fatalf("cut: %+v", pieces)
	}

	dir := t.TempDir()
	created := filepath.Join(dir, "days.gpx")
	write := func(mode, target string, which ...segmentJSON) (int, map[string]any) {
		list := []map[string]any{}
		for _, s := range which {
			list = append(list, map[string]any{"first": s.First, "last": s.Last, "name": "Day " + fmt.Sprint(s.First)})
		}
		return send(t, server, http.MethodPost, "/api/segments/write", map[string]any{
			"path": path, "segments": list, "target": map[string]any{"mode": mode, "path": target},
		})
	}
	if code, result := write("create", created, pieces...); code != http.StatusOK || result["tracks"] != 2.0 {
		t.Fatalf("create = %d %v", code, result)
	}
	if code, _ := write("create", created, pieces[0]); code != http.StatusConflict {
		t.Fatalf("create over existing = %d", code)
	}
	if code, _ := write("append", created, pieces[1]); code != http.StatusOK {
		t.Fatalf("append = %d", code)
	}
	file, err := gpxfile.Open(created)
	if err != nil || len(file.Tracks) != 3 || file.Tracks[2].Name != "Day "+fmt.Sprint(cut) {
		t.Fatalf("written file = %+v, %v", file, err)
	}
	if n := len(file.Tracks[0].Segments[0].Points); n != cut+1 {
		t.Fatalf("first track has %d points, want %d", n, cut+1)
	}
	if code, _ := write("append", path, pieces[0]); code != http.StatusBadRequest {
		t.Fatalf("append to source = %d", code)
	}
	if code, _ := write("create", created+"x", segmentJSON{First: 1, Last: 2}); code != http.StatusBadRequest && code != http.StatusConflict {
		t.Fatalf("unknown segment = %d", code)
	}
}
