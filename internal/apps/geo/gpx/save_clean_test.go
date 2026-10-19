package gpx

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

// stepper drives one file through the page's API.
type stepper struct {
	t      *testing.T
	server *httptest.Server
	path   string
}

func newStepper(t *testing.T, path string) stepper {
	server := httptest.NewServer(Handler(Settings{}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { pending.forget(path) })
	return stepper{t, server, path}
}

func (s stepper) shown() trackJSON {
	var got trackJSON
	get(s.t, s.server, "/api/track?path="+url.QueryEscape(s.path), &got)
	return got
}

func (s stepper) call(method, api string, body map[string]any) (int, map[string]any) {
	s.t.Helper()
	if body == nil {
		body = map[string]any{"path": s.path}
	}
	return send(s.t, s.server, method, api, body)
}

func (s stepper) ok(method, api string, body map[string]any) {
	s.t.Helper()
	if code, result := s.call(method, api, body); code != http.StatusOK {
		s.t.Fatalf("%s %s = %d %v", method, api, code, result)
	}
}

func (s stepper) lasso(points ...int) {
	s.t.Helper()
	s.ok(http.MethodPut, "/api/clean", map[string]any{"path": s.path, "clean": map[string]any{"edits": []map[string]any{{"kind": "lasso", "points": points}}}})
}

func (s stepper) step(back bool) bool {
	s.t.Helper()
	api := map[bool]string{true: "/api/undo", false: "/api/redo"}[back]
	code, result := s.call(http.MethodPost, api, nil)
	if code != http.StatusOK {
		s.t.Fatalf("%s = %d %v", api, code, result)
	}
	return result["moved"] == true
}

func pointsOnDisk(t *testing.T, path string) []int {
	t.Helper()
	file, err := gpxfile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var counts []int
	for _, trk := range file.Tracks {
		n := 0
		for _, seg := range trk.Segments {
			n += len(seg.Points)
		}
		counts = append(counts, n)
	}
	return counts
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A save that takes points out of the file moves the cuts and segment names
// to where their points now are.
func TestSaveOfOurGPXMovesCutsWithTheirPoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	var points []gpxfile.Point
	for i := 0; i < 5; i++ {
		points = append(points, gpxfile.Point{LatLon: geo.LatLon{Lat: 30 + float64(i)/1000, Lon: 120}})
	}
	if err := gpxfile.Create(path, "Ours", []gpxfile.Track{{Name: "Walk", Segments: []gpxfile.Segment{{Points: points}}}}); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.lasso(1)
	s.ok(http.MethodPut, "/api/segments", map[string]any{"path": path, "cuts": []int{3}, "names": []map[string]any{{"start": 3, "name": "Back"}}})
	s.ok(http.MethodPost, "/api/save", map[string]any{"path": path})
	if got := pointsOnDisk(t, path); len(got) != 1 || got[0] != 4 {
		t.Fatalf("points after save = %v", got)
	}
	got := s.shown()
	if len(got.Cuts) != 1 || got.Cuts[0] != 2 {
		t.Fatalf("cuts after save = %v", got.Cuts)
	}
	if len(got.Pieces) != 2 || got.Pieces[1].Name != "Back" {
		t.Fatalf("pieces after save = %+v", got.Pieces)
	}
	if len(got.Clean.Params.Edits) != 0 {
		t.Fatalf("edits after save = %+v", got.Clean.Params.Edits)
	}
}

// A name whose segment start cleaning removed shows nowhere; saving does not
// carry it into the file, where it would land on another segment.
func TestSaveDropsANameWhosePointIsRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	var points []gpxfile.Point
	for i := 0; i < 6; i++ {
		points = append(points, gpxfile.Point{LatLon: geo.LatLon{Lat: 30 + float64(i)/1000, Lon: 120}})
	}
	if err := gpxfile.Create(path, "Ours", []gpxfile.Track{{Name: "Walk", Segments: []gpxfile.Segment{{Points: points}}}}); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.ok(http.MethodPut, "/api/segments", map[string]any{"path": path, "cuts": []int{2, 4}, "names": []map[string]any{{"start": 2, "name": "Gone"}, {"start": 4, "name": "Kept"}}})
	s.lasso(2)
	before := s.shown()
	for _, piece := range before.Pieces {
		if piece.Name == "Gone" {
			t.Fatal("a name whose point is removed is shown")
		}
	}
	s.ok(http.MethodPost, "/api/save", nil)
	after := s.shown()
	if len(after.Pieces) != len(before.Pieces) {
		t.Fatalf("pieces %d after save, %d before", len(after.Pieces), len(before.Pieces))
	}
	for i := range after.Pieces {
		if after.Pieces[i].Name != before.Pieces[i].Name {
			t.Fatalf("piece %d named %q after save, %q before", i, after.Pieces[i].Name, before.Pieces[i].Name)
		}
	}
}

// A fill becomes ordinary points of the file when it is saved.
func TestSaveWritesFillsAsPoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	ourGPX(t, path)
	s := newStepper(t, path)
	s.ok(http.MethodPost, "/api/fill", map[string]any{"path": path, "first": 0, "last": 1, "profile": "foot", "route": [][]float64{{120.13, 30.13}, {120.16, 30.16}}})
	s.ok(http.MethodPost, "/api/save", nil)
	file, err := gpxfile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	one := file.Tracks[0].Segments[0].Points
	if len(one) != 4 || one[1].Source == "" || one[2].Source == "" || one[3].Source != "" {
		t.Fatalf("track One after save = %+v", one)
	}
	if got := s.shown(); len(got.Fills) != 0 {
		t.Fatalf("fills kept after save: %+v", got.Fills)
	}
}

// Parts deleted, copied in and put in order are saved together with the
// cleaning on the points: each track lands where the page laid it.
func TestSaveAppliesPartsAndCleaningTogether(t *testing.T) {
	folder := t.TempDir()
	path, recording := filepath.Join(folder, "ours.gpx"), filepath.Join(folder, "recorded.gpx")
	ourGPX(t, path)
	if err := os.WriteFile(recording, []byte(sampleWithParts), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.ok(http.MethodPost, "/api/part/copy", map[string]any{"path": recording, "key": "t0", "target": path})
	s.ok(http.MethodDelete, "/api/part", map[string]any{"path": path, "key": "t0"})
	s.ok(http.MethodPut, "/api/part-order", map[string]any{"path": path, "kind": "track", "keys": []string{"t2", "t1"}})
	if got := partNames(s.shown(), "track"); len(got) != 2 || got[0] != "Morning" || got[1] != "Two" {
		t.Fatalf("laid tracks = %v", got)
	}
	s.lasso(1) // the middle point of Morning, laid first
	s.ok(http.MethodPost, "/api/save", nil)
	file, err := gpxfile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Tracks) != 2 || file.Tracks[0].Name != "Morning" || file.Tracks[1].Name != "Two" {
		t.Fatalf("tracks in the file = %+v", file.Tracks)
	}
	if got := pointsOnDisk(t, path); got[0] != 2 || got[1] != 2 {
		t.Fatalf("points in the file = %v", got)
	}
	if len(file.Routes) != 1 || len(file.Waypoints) != 1 {
		t.Fatalf("routes %d waypoints %d after save", len(file.Routes), len(file.Waypoints))
	}
	if got := s.shown(); got.Unsaved || len(got.Clean.Params.Edits) != 0 {
		t.Fatalf("after save: unsaved %v edits %d", got.Unsaved, len(got.Clean.Params.Edits))
	}
}

// Saving a recording writes only its sidecar: the points it removed stay in
// the GPX, byte for byte.
func TestSavingARecordingKeepsItsPoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recorded.gpx")
	if err := os.WriteFile(path, []byte(sampleWithParts), 0o644); err != nil {
		t.Fatal(err)
	}
	original := read(t, path)
	s := newStepper(t, path)
	s.lasso(1)
	s.ok(http.MethodPost, "/api/save", nil)
	if string(read(t, path)) != string(original) {
		t.Fatal("saving wrote the recording")
	}
	if _, err := os.Stat(sidecar.PathFor(path)); err != nil {
		t.Fatalf("no sidecar after save: %v", err)
	}
	if got := s.shown(); len(got.Clean.Params.Edits) != 1 {
		t.Fatalf("the sidecar lost the edit: %d", len(got.Clean.Params.Edits))
	}
}
