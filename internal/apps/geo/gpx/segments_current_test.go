package gpx

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
)

func TestAddSegmentToCurrentGPXAndSaveAsTrack(t *testing.T) {
	for _, draft := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "draft"}[draft], func(t *testing.T) {
			server := httptest.NewServer(Handler(Settings{}))
			defer server.Close()
			tracks := []gpxfile.Track{{Name: "Original", Segments: []gpxfile.Segment{{Points: []gpxfile.Point{{LatLon: geo.LatLon{}}, {LatLon: geo.LatLon{Lat: 0.0001}}, {LatLon: geo.LatLon{Lat: 0.0002}}}}}}}
			var path string
			var before []byte
			if draft {
				path = drafts.create("Current")
				defer drafts.remove(path)
				if err := addTracks(path, "synthetic.gpx", tracks); err != nil {
					t.Fatal(err)
				}
			} else {
				path = filepath.Join(t.TempDir(), "current.gpx")
				if err := gpxfile.Create(path, "Current", tracks); err != nil {
					t.Fatal(err)
				}
				var err error
				before, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			defer pending.drop(path)
			if code, result := send(t, server, http.MethodPut, "/api/segments", map[string]any{"path": path, "cuts": []int{1}}); code != http.StatusOK {
				t.Fatalf("cut = %d %v", code, result)
			}
			var shown trackJSON
			get(t, server, "/api/track?path="+url.QueryEscape(path), &shown)
			piece := shown.Pieces[0]
			request := map[string]any{"path": path, "segments": []map[string]any{{"first": piece.First, "last": piece.Last, "name": "Selected segment"}}, "target": map[string]any{"mode": "add", "path": path}}
			if code, result := send(t, server, http.MethodPost, "/api/segments/write", request); code != http.StatusOK {
				t.Fatalf("add to current = %d %v", code, result)
			}
			get(t, server, "/api/track?path="+url.QueryEscape(path), &shown)
			if len(shown.Parts) != 2 || shown.Parts[1].Name != "Selected segment" {
				t.Fatalf("parts = %+v", shown.Parts)
			}
			target := path
			if draft {
				target = filepath.Join(t.TempDir(), "saved.gpx")
				if code, result := send(t, server, http.MethodPost, "/api/save-as", map[string]any{"path": path, "target": target}); code != http.StatusOK {
					t.Fatalf("save draft = %d %v", code, result)
				}
			} else {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, before) {
					t.Fatal("adding wrote the GPX before Save")
				}
				if !shown.Unsaved {
					t.Fatal("addition did not remain unsaved")
				}
				if code, result := send(t, server, http.MethodPost, "/api/save", map[string]any{"path": path}); code != http.StatusOK {
					t.Fatalf("save = %d %v", code, result)
				}
			}
			parsed, err := gpxfile.Open(target)
			if err != nil {
				t.Fatal(err)
			}
			if len(parsed.Tracks) != 2 || parsed.Tracks[0].Name != "Original" || parsed.Tracks[1].Name != "Selected segment" {
				t.Fatalf("saved tracks = %+v", parsed.Tracks)
			}
			if len(parsed.Tracks[0].Segments[0].Points) != 3 || len(parsed.Tracks[1].Segments[0].Points) != piece.Last-piece.First+1 {
				t.Fatal("original or selected point count changed")
			}
		})
	}
}

func TestRecordingCannotAddASegmentToItself(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "recorded.gpx")
	data := []byte(`<gpx version="1.1" creator="recorder"><trk><trkseg><trkpt lat="0" lon="0"/><trkpt lat="0.0001" lon="0"/></trkseg></trk></gpx>`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if code, result := send(t, server, http.MethodPost, "/api/segments/write", map[string]any{"path": path, "segments": []map[string]any{{"first": 0, "last": 1, "name": "Copy"}}, "target": map[string]any{"mode": "add", "path": path}}); code != http.StatusBadRequest {
		t.Fatalf("self-add recording = %d %v", code, result)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, data) || pending.has(path) {
		t.Fatal("recording or its edits changed")
	}
}
