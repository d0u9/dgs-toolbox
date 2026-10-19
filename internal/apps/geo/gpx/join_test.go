package gpx

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
)

func recording(t *testing.T, path, name, start, end string) []byte {
	t.Helper()
	data := []byte(`<?xml version="1.0"?><gpx version="1.1" creator="Recorder" xmlns="http://www.topografix.com/GPX/1/1"><trk><name>` + name + `</name><trkseg>` +
		`<trkpt lat="1" lon="1"><time>` + start + `</time></trkpt><trkpt lat="1.001" lon="1"><time>` + end + `</time></trkpt></trkseg></trk></gpx>`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestJoinTracksIntoDGSFileLeavesSourcesAlone(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	dir := t.TempDir()
	later, earlier, target := filepath.Join(dir, "later.gpx"), filepath.Join(dir, "earlier.gpx"), filepath.Join(dir, "target.gpx")
	laterBytes := recording(t, later, "Afternoon", "2026-01-01T13:00:00Z", "2026-01-01T14:00:00Z")
	recording(t, earlier, "Morning", "2026-01-01T09:00:00Z", "2026-01-01T10:00:00Z")
	if err := gpxfile.Create(target, "Target", nil); err != nil {
		t.Fatal(err)
	}
	defer pending.drop(target)
	parts := []map[string]string{{"path": later, "key": "t0"}, {"path": earlier, "key": "t0"}}

	if code, result := send(t, server, http.MethodPost, "/api/part/join", map[string]any{"parts": parts, "target": earlier, "name": "Day"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("join into a recording = %d %v", code, result)
	}
	if code, result := send(t, server, http.MethodPost, "/api/part/join", map[string]any{"parts": parts, "target": target, "name": " Day "}); code != http.StatusOK {
		t.Fatalf("join = %d %v", code, result)
	}
	var shown trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(target), &shown)
	tracks := 0
	for _, part := range shown.Parts {
		if part.Kind == "track" {
			tracks++
			if part.Name != "Day" || part.Segments != 2 {
				t.Fatalf("joined track = %+v", part)
			}
		}
	}
	if tracks != 1 {
		t.Fatalf("target holds %d tracks", tracks)
	}
	if data, _ := os.ReadFile(later); string(data) != string(laterBytes) {
		t.Fatal("a source recording was written")
	}
}

// The same track named twice is refused, not joined to itself.
func TestJoinRefusesTheSameTrackTwice(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "ours.gpx")
	ourGPX(t, path)
	defer pending.forget(path)
	parts := []map[string]string{{"path": path, "key": "t0"}, {"path": path, "key": "t0"}}
	if code, _ := send(t, server, http.MethodPost, "/api/part/join", map[string]any{"parts": parts, "target": path, "name": "Twice"}); code != http.StatusBadRequest {
		t.Fatalf("join of one track twice = %d", code)
	}
}
