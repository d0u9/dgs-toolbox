package gpx

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

// A track's type is held like its name, shown at once, written into a GPX
// dgs wrote when the file is saved, and kept by a cleaned save.
func TestTrackTypeHeldThenWrittenIntoOurGPX(t *testing.T) {
	path := ownRenameGPX(t)
	s := newStepper(t, path)
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Walk", "type": " hike "})
	if shown := s.shown(); shown.Parts[0].Type != "hike" || !shown.Unsaved {
		t.Fatalf("held: %q", shown.Parts[0].Type)
	}
	if parsed, _ := gpxfile.Open(path); parsed.Tracks[0].Type != "" {
		t.Fatal("written before save")
	}
	s.ok(http.MethodPost, "/api/save", nil)
	if parsed, _ := gpxfile.Open(path); parsed.Tracks[0].Type != "hike" || parsed.Tracks[0].Name != "Walk" {
		t.Fatalf("saved: %+v", parsed.Tracks[0])
	}
	// Sending the type it has holds nothing; leaving it out keeps it.
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Walk", "type": "hike"})
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Walk"})
	if s.shown().Unsaved {
		t.Fatal("an unchanged type is held")
	}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Walk", "type": ""})
	s.ok(http.MethodPost, "/api/save", nil)
	if parsed, _ := gpxfile.Open(path); parsed.Tracks[0].Type != "" {
		t.Fatalf("not taken out: %q", parsed.Tracks[0].Type)
	}
}

func TestTrackTypeRefusesBadInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trip.gpx")
	if err := gpxfile.CreateAll(path, "Trip", []gpxfile.Waypoint{{Name: "w"}}, nil, []gpxfile.Track{{Name: "Walk", Segments: []gpxfile.Segment{{Points: []gpxfile.Point{{}}}}}}); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	for _, body := range []map[string]any{
		{"path": path, "key": "w0", "name": "w", "type": "drive"},
		{"path": path, "key": "f", "name": "Trip", "type": "drive"},
		{"path": path, "key": "t0", "name": "Walk", "type": "two\nlines"},
	} {
		if code, _ := s.call(http.MethodPut, "/api/part-name", body); code != http.StatusBadRequest {
			t.Fatalf("%v = %d", body, code)
		}
	}
}

// A recording keeps a track's type in its sidecar; Save as, copying and
// writing a segment out carry it into the GPX written.
func TestTrackTypeOfRecordingFollowsItsTrack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rec.gpx")
	recorded := []byte(`<gpx version="1.1" creator="device"><trk><name>t</name><type>run</type><trkseg><trkpt lat="1" lon="1"/><trkpt lat="1.1" lon="1"/></trkseg></trk></gpx>`)
	if err := os.WriteFile(path, recorded, 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "b.gpx")
	if err := gpxfile.CreateAll(target, "", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	t.Cleanup(func() { pending.forget(target) })
	if s.shown().Parts[0].Type != "run" {
		t.Fatal("the recording's own type is not shown")
	}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "t", "type": "drive"})
	s.ok(http.MethodPost, "/api/save", nil)
	if data, _ := os.ReadFile(path); string(data) != string(recorded) {
		t.Fatal("the recording was written")
	}
	if side, _, err := sidecar.Load(path); err != nil || side.Types["t0"] != "drive" {
		t.Fatalf("sidecar: %v %v", side.Types, err)
	}
	out := filepath.Join(dir, "out.gpx")
	s.ok(http.MethodPost, "/api/save-as", map[string]any{"path": path, "target": out})
	if parsed, err := gpxfile.Open(out); err != nil || parsed.Tracks[0].Type != "drive" {
		t.Fatalf("save as: %+v %v", parsed, err)
	}
	s.ok(http.MethodPost, "/api/part/copy", map[string]any{"path": path, "key": "t0", "target": target})
	var shown trackJSON
	get(t, s.server, "/api/track?path="+url.QueryEscape(target), &shown)
	if shown.Parts[0].Type != "drive" {
		t.Fatalf("copy lost the type: %+v", shown.Parts)
	}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": target, "key": "t0", "name": "t", "type": "bike"})
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{target}})
	if parsed, err := gpxfile.Open(target); err != nil || parsed.Tracks[0].Type != "bike" {
		t.Fatalf("copied track: %+v %v", parsed, err)
	}
	pieces := filepath.Join(dir, "pieces.gpx")
	s.ok(http.MethodPost, "/api/segments/write", map[string]any{"path": path, "segments": []map[string]any{{"first": 0, "last": 1, "name": "piece"}}, "target": map[string]any{"mode": "create", "path": pieces}})
	if parsed, err := gpxfile.Open(pieces); err != nil || parsed.Tracks[0].Type != "drive" {
		t.Fatalf("segment: %+v %v", parsed, err)
	}
}

func typedTracks(types ...string) []gpxfile.Track {
	var tracks []gpxfile.Track
	for i, kind := range types {
		points := make([]gpxfile.Point, 4)
		for k := range points {
			points[k].Lat, points[k].Lon = float64(i)+float64(k)/1000, 1
		}
		tracks = append(tracks, gpxfile.Track{Name: "T" + kind, Type: kind, Segments: []gpxfile.Segment{{Points: points}}})
	}
	return tracks
}

func trackTypes(t *testing.T, path string) []string {
	t.Helper()
	parsed, err := gpxfile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, trk := range parsed.Tracks {
		types = append(types, trk.Type)
	}
	return types
}

// A cleaned save rewrites the tracks and keeps their types, own and held;
// undo takes a saved type back.
func TestTrackTypeKeptByCleanedSaveAndUndo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	if err := gpxfile.Create(path, "Trip", typedTracks("run", "")); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t1", "name": "T", "type": "bike"})
	s.lasso(1)
	s.ok(http.MethodPost, "/api/save", nil)
	if got := pointsOnDisk(t, path); !reflect.DeepEqual(got, []int{3, 4}) {
		t.Fatalf("not cleaned: %v", got)
	}
	if got := trackTypes(t, path); !reflect.DeepEqual(got, []string{"run", "bike"}) {
		t.Fatalf("types %v", got)
	}
	s.step(true)
	if got := trackTypes(t, path); !reflect.DeepEqual(got, []string{"run", ""}) {
		t.Fatalf("after undo: %v", got)
	}
}

// Deleting a track held beside the file moves the later tracks' keys down,
// their held types with them.
func TestTrackTypeOfHeldTrackFollowsDelete(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "a.gpx"), filepath.Join(dir, "b.gpx")
	if err := gpxfile.Create(source, "A", typedTracks("drive", "hike")); err != nil {
		t.Fatal(err)
	}
	if err := gpxfile.CreateAll(target, "B", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, source)
	t.Cleanup(func() { pending.forget(target) })
	for _, key := range []string{"t0", "t1"} {
		s.ok(http.MethodPost, "/api/part/copy", map[string]any{"path": source, "key": key, "target": target})
	}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": target, "key": "t1", "name": "Thike", "type": "bike"})
	s.ok(http.MethodDelete, "/api/part", map[string]any{"path": target, "key": "t0"})
	var shown trackJSON
	get(t, s.server, "/api/track?path="+url.QueryEscape(target), &shown)
	if len(shown.Parts) != 1 || shown.Parts[0].Type != "bike" {
		t.Fatalf("parts %+v", shown.Parts)
	}
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{target}})
	if got := trackTypes(t, target); !reflect.DeepEqual(got, []string{"bike"}) {
		t.Fatalf("saved %v", got)
	}
}

// Segments merged into one track keep the type they share, and none when
// they come from tracks of different types.
func TestMergedSegmentsKeepASharedType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trip.gpx")
	if err := gpxfile.Create(path, "Trip", typedTracks("drive", "hike")); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.ok(http.MethodPut, "/api/segments", map[string]any{"path": path, "cuts": []int{2}})
	pieces := s.shown().Pieces
	if len(pieces) != 3 {
		t.Fatalf("pieces %+v", pieces)
	}
	for _, c := range []struct {
		picked []int
		want   string
	}{{[]int{0, 1}, "drive"}, {[]int{1, 2}, ""}} {
		var wanted []map[string]any
		for _, i := range c.picked {
			wanted = append(wanted, map[string]any{"first": pieces[i].First, "last": pieces[i].Last, "name": "p"})
		}
		out := filepath.Join(dir, fmt.Sprintf("merged-%d.gpx", c.picked[0]))
		s.ok(http.MethodPost, "/api/segments/write", map[string]any{"path": path, "segments": wanted, "target": map[string]any{"mode": "create", "path": out, "merge": true, "name": "m"}})
		if got := trackTypes(t, out); !reflect.DeepEqual(got, []string{c.want}) {
			t.Fatalf("%v: %v", c.picked, got)
		}
	}
}
