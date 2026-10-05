package gpx

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
)

// A rename before and after segment addition must retain the same file's
// pending cuts and new tracks, including a draft's contents. Saving twice
// must not duplicate additions; reading again must show the committed names.
func TestRenameAddSegmentsSaveAndReloadContract(t *testing.T) {
	for _, draft := range []bool{false, true} {
		for _, merge := range []bool{false, true} {
			t.Run(fmt.Sprintf("draft=%t/merge=%t", draft, merge), func(t *testing.T) {
				server := httptest.NewServer(Handler(Settings{}))
				defer server.Close()
				points := make([]gpxfile.Point, 5)
				for i := range points {
					points[i].LatLon = geo.LatLon{Lat: float64(i) / 10000}
				}
				original := gpxfile.Track{Name: "Original", Segments: []gpxfile.Segment{{Points: points}}}
				var path string
				if draft {
					path = drafts.create("Initial")
					if err := addTracks(path, "synthetic.gpx", []gpxfile.Track{original}); err != nil {
						t.Fatal(err)
					}
				} else {
					path = filepath.Join(t.TempDir(), "initial.gpx")
					if err := gpxfile.Create(path, "Initial", []gpxfile.Track{original}); err != nil {
						t.Fatal(err)
					}
				}
				initial := path
				defer func() { pending.drop(path); pending.drop(initial); drafts.remove(path); drafts.remove(initial) }()
				if code, result := send(t, server, http.MethodPut, "/api/segments", map[string]any{"path": path, "cuts": []int{2}}); code != http.StatusOK {
					t.Fatalf("cuts: %d %v", code, result)
				}
				rename := func(name string) {
					t.Helper()
					old := path
					code, result := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": path, "name": name})
					if code != http.StatusOK {
						t.Fatalf("rename: %d %v", code, result)
					}
					path = result["path"].(string)
					if pending.has(old) {
						t.Fatal("rename left pending state at old path")
					}
					if code := get(t, server, "/api/track?path="+url.QueryEscape(old), nil); code != http.StatusNotFound {
						t.Fatalf("old name remains readable: %d", code)
					}
				}
				rename("Before.v2")
				var shown trackJSON
				get(t, server, "/api/track?path="+url.QueryEscape(path), &shown)
				if len(shown.Pieces) != 2 {
					t.Fatalf("rename lost cuts: %+v", shown.Pieces)
				}
				wanted := make([]map[string]any, 2)
				for i, piece := range shown.Pieces {
					wanted[i] = map[string]any{"first": piece.First, "last": piece.Last, "name": fmt.Sprintf("Piece %d", i+1)}
				}
				code, result := send(t, server, http.MethodPost, "/api/segments/write", map[string]any{"path": path, "segments": wanted, "target": map[string]any{"mode": "add", "path": path, "merge": merge, "name": "Combined"}})
				if code != http.StatusOK {
					t.Fatalf("add: %d %v", code, result)
				}
				rename("After.v3.GPX")
				target := path
				if draft {
					target = filepath.Join(t.TempDir(), "saved.gpx")
					code, result = send(t, server, http.MethodPost, "/api/save-as", map[string]any{"path": path, "target": target})
				} else {
					before, err := gpxfile.Open(path)
					if err != nil || len(before.Tracks) != 1 {
						t.Fatal("addition was saved by Rename", err)
					}
					code, result = send(t, server, http.MethodPost, "/api/save", map[string]any{"path": path})
				}
				if code != http.StatusOK {
					t.Fatalf("save: %d %v", code, result)
				}
				expected := 3
				if merge {
					expected = 2
				}
				parsed, err := gpxfile.Open(target)
				if err != nil {
					t.Fatal(err)
				}
				if len(parsed.Tracks) != expected || !reflect.DeepEqual(parsed.Tracks[0], original) {
					t.Fatal("original or new tracks changed on Save")
				}
				if merge {
					if parsed.Tracks[1].Name != "Combined" || len(parsed.Tracks[1].Segments[0].Points) != 5 {
						t.Fatal("merge name or shared cut point changed")
					}
				} else {
					if parsed.Tracks[1].Name != "Piece 1" || parsed.Tracks[2].Name != "Piece 2" {
						t.Fatal("separate names changed")
					}
				}
				before, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				code, result = send(t, server, http.MethodPost, "/api/save", map[string]any{"path": target})
				if code != http.StatusOK {
					t.Fatalf("second save: %d %v", code, result)
				}
				after, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("second Save repeated an edit")
				}
				shown = trackJSON{}
				get(t, server, "/api/track?path="+url.QueryEscape(target), &shown)
				if shown.Unsaved || shown.Added != 0 || len(shown.Parts) != expected {
					t.Fatalf("reload disagrees with disk: %+v", shown.Parts)
				}
			})
		}
	}
}

func TestRevertCurrentSegmentAdditionKeepsOriginalBytes(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := ownRenameGPX(t)
	defer pending.drop(path)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var shown trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &shown)
	piece := shown.Pieces[0]
	code, result := send(t, server, http.MethodPost, "/api/segments/write", map[string]any{"path": path, "segments": []map[string]any{{"first": piece.First, "last": piece.Last, "name": "Copied"}}, "target": map[string]any{"mode": "add", "path": path}})
	if code != http.StatusOK {
		t.Fatalf("add: %d %v", code, result)
	}
	code, result = send(t, server, http.MethodDelete, "/api/pending", map[string]any{"path": path})
	if code != http.StatusOK {
		t.Fatalf("revert: %d %v", code, result)
	}
	get(t, server, "/api/track?path="+url.QueryEscape(path), &shown)
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) || shown.Unsaved || len(shown.Parts) != 1 || shown.Added != 0 {
		t.Fatal("Revert lost original data or kept the addition")
	}
}

// The API must expose the same retained anchor for the marker, popup and
// proposed cut, even when the midpoint falls in a merged stop's excursion.
func TestStopAnchorAndCandidateContractWithCleaning(t *testing.T) {
	for _, cleaned := range []bool{false, true} {
		t.Run(fmt.Sprintf("cleaned=%t", cleaned), func(t *testing.T) {
			server := httptest.NewServer(Handler(Settings{}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "stay.gpx")
			defer pending.drop(path)
			points := make([]gpxfile.Point, 805)
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for i := range points {
				points[i].Time = at.Add(time.Duration(i) * time.Second)
				if i >= 400 && i < 405 {
					points[i].Lat = 0.0006
				}
			}
			if err := gpxfile.Create(path, "Stay", []gpxfile.Track{{Segments: []gpxfile.Segment{{Points: points}}}}); err != nil {
				t.Fatal(err)
			}
			if cleaned {
				code, result := send(t, server, http.MethodPut, "/api/clean", map[string]any{"path": path, "clean": map[string]any{"stops": map[string]any{"enabled": true}}})
				if code != http.StatusOK {
					t.Fatalf("clean: %d %v", code, result)
				}
			}
			var shown trackJSON
			get(t, server, "/api/track?path="+url.QueryEscape(path), &shown)
			if len(shown.Stops) != 1 || len(shown.CutCandidates) != 1 {
				t.Fatalf("visit split: %+v", shown.Stops)
			}
			anchor := shown.Stops[0].Anchor
			if anchor >= 400 && anchor < 405 {
				t.Fatal("API anchored the stop on its excursion")
			}
			if shown.Removed[anchor] != "" || shown.CutCandidates[0] != anchor || shown.Points[anchor] != [2]float64{} {
				t.Fatal("marker, cleaning and candidate cut disagree")
			}
		})
	}
}
