package gpx

import (
	"bytes"
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

func TestSegmentAdditionSeparateAndMergedPersistence(t *testing.T) {
	for _, draft := range []bool{false, true} {
		for _, tc := range []struct {
			name          string
			merge         bool
			picked        []int
			ranges        [][2]int
			recordedBreak bool
		}{
			{"separate", false, []int{0, 1}, [][2]int{{0, 2}, {2, 4}}, false},
			{"merge adjacent reversed selection", true, []int{1, 0}, [][2]int{{0, 4}}, false},
			{"merge with unselected gap", true, []int{2, 0}, [][2]int{{0, 2}, {4, 6}}, false},
			{"merge preserves recorded break", true, []int{1, 0}, [][2]int{{0, 2}, {3, 4}}, true},
		} {
			t.Run(map[bool]string{false: "saved", true: "draft"}[draft]+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewServer(Handler(Settings{}))
				defer server.Close()
				points := make([]gpxfile.Point, 9)
				for i := range points {
					points[i] = gpxfile.Point{LatLon: geo.LatLon{Lat: float64(i) / 10000}, Time: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC), Elevation: float64(i), HasElevation: true, HDOP: 1.2, HasHDOP: true, Satellites: 8, HasSatellites: true, Source: "synthetic-source"}
				}
				original := gpxfile.Track{Name: "Original", Segments: []gpxfile.Segment{{Points: points}}}
				if tc.recordedBreak {
					original.Segments = []gpxfile.Segment{{Points: points[:3]}, {Points: points[3:]}}
				}
				var path string
				var before []byte
				if draft {
					path = drafts.create("Current")
					defer drafts.remove(path)
					if err := addTracks(path, "synthetic.gpx", []gpxfile.Track{original}); err != nil {
						t.Fatal(err)
					}
				} else {
					path = filepath.Join(t.TempDir(), "current.gpx")
					if err := gpxfile.Create(path, "Current", []gpxfile.Track{original}); err != nil {
						t.Fatal(err)
					}
					var err error
					before, err = os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
				}
				defer pending.drop(path)
				if code, result := send(t, server, http.MethodPut, "/api/segments", map[string]any{"path": path, "cuts": []int{2, 4, 6}}); code != http.StatusOK {
					t.Fatalf("cuts = %d %v", code, result)
				}
				var shown trackJSON
				get(t, server, "/api/track?path="+url.QueryEscape(path), &shown)
				wanted := []map[string]any{}
				for _, i := range tc.picked {
					piece := shown.Pieces[i]
					wanted = append(wanted, map[string]any{"first": piece.First, "last": piece.Last, "name": "Piece " + string(rune('A'+i))})
				}
				code, result := send(t, server, http.MethodPost, "/api/segments/write", map[string]any{"path": path, "segments": wanted, "target": map[string]any{"mode": "add", "path": path, "merge": tc.merge, "name": "  Combined  "}})
				count := len(wanted)
				if tc.merge {
					count = 1
				}
				if code != http.StatusOK || int(result["tracks"].(float64)) != count {
					t.Fatalf("add = %d %v", code, result)
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
						t.Fatal("addition wrote before Save")
					}
					if code, result := send(t, server, http.MethodPost, "/api/save", map[string]any{"path": path}); code != http.StatusOK {
						t.Fatalf("save = %d %v", code, result)
					}
				}
				parsed, err := gpxfile.Open(target)
				if err != nil {
					t.Fatal(err)
				}
				if len(parsed.Tracks) != 1+count || !reflect.DeepEqual(parsed.Tracks[0], original) {
					t.Fatal("original track changed or wrong output track count")
				}
				for i, span := range tc.ranges {
					trackIndex, segmentIndex := i+1, 0
					if tc.merge {
						trackIndex, segmentIndex = 1, i
					}
					trk := parsed.Tracks[trackIndex]
					name := "Combined"
					if !tc.merge {
						name = wanted[i]["name"].(string)
					}
					if trk.Name != name {
						t.Fatalf("track name = %q, want %q", trk.Name, name)
					}
					if !reflect.DeepEqual(trk.Segments[segmentIndex].Points, points[span[0]:span[1]+1]) {
						t.Fatal("selected points, order or metadata changed")
					}
				}
				if tc.merge && len(parsed.Tracks[1].Segments) != len(tc.ranges) {
					t.Fatal("lost or invented track segment breaks")
				}
			})
		}
	}
}

func TestMergedSegmentAdditionRequiresNameAndValidRanges(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := ownRenameGPX(t)
	defer pending.drop(path)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                string
		first, last, status int
	}{
		{"", 0, 0, http.StatusBadRequest}, {"   ", 0, 0, http.StatusBadRequest}, {"Merged", 0, 99, http.StatusConflict},
	} {
		if code, result := send(t, server, http.MethodPost, "/api/segments/write", map[string]any{"path": path, "segments": []map[string]any{{"first": tc.first, "last": tc.last}}, "target": map[string]any{"mode": "add", "path": path, "merge": true, "name": tc.name}}); code != tc.status {
			t.Fatalf("invalid merge = %d %v", code, result)
		}
		if pending.has(path) {
			t.Fatal("rejected merge added pending tracks")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected merge changed file")
	}
}
