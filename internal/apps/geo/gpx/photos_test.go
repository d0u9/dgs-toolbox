package gpx

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
)

func TestWaypointPhotosAreSavedIntoTheGPXAndFollowARename(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	folder := t.TempDir()
	path := filepath.Join(folder, "trip.gpx")
	ourGPX(t, path)

	// Two 2×2 lossless WebPs, one black and one white, made with cwebp.
	samples := map[string]string{
		"black": "524946461a000000574542505650384c0e0000002f01400000071011fd0f4444ff03",
		"white": "524946461e000000574542505650384c110000002f0140000007d0fffef7bfff8188e87f0000",
	}
	upload := func(sample string) string {
		data, err := hex.DecodeString(samples[sample])
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Post(server.URL+"/api/photo?path="+url.QueryEscape(path), "image/webp", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var answer map[string]string
		_ = json.NewDecoder(response.Body).Decode(&answer)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("upload: %d %v", response.StatusCode, answer)
		}
		return answer["name"]
	}
	kept, dropped := upload("black"), upload("white")
	if kept == dropped || upload("black") != kept {
		t.Fatalf("names %q %q: the same picture is one file, another is another", kept, dropped)
	}
	if code, body := send(t, server, http.MethodPut, "/api/waypoint-photos", map[string]any{
		"path": path, "key": "w0", "description": "Windy", "photos": []map[string]string{{"name": kept, "caption": "North"}},
	}); code != http.StatusOK {
		t.Fatalf("set: %d %v", code, body)
	}
	if code, _ := send(t, server, http.MethodPost, "/api/save", map[string]any{"path": path}); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	file, err := gpxfile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := gpxfile.Link{Href: "trip.photos/" + kept, Text: "North", Type: "image/webp"}
	if wpt := file.Waypoints[0]; wpt.Description != "Windy" || len(wpt.Links) != 1 || wpt.Links[0] != want {
		t.Fatalf("waypoint = %+v", wpt)
	}
	if _, err := os.Stat(filepath.Join(folder, "trip.photos", ".trash", dropped)); err != nil {
		t.Fatalf("an unlinked photo goes to the trash: %v", err)
	}
	if response, _ := http.Get(server.URL + "/api/photo?path=" + url.QueryEscape(path) + "&name=" + dropped); response.StatusCode != http.StatusNotFound {
		t.Fatalf("an unlinked photo is served: %d", response.StatusCode)
	}

	code, body := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": path, "name": "walk"})
	if code != http.StatusOK {
		t.Fatalf("rename: %d %v", code, body)
	}
	renamed := body["path"].(string)
	if file, err = gpxfile.Open(renamed); err != nil || file.Waypoints[0].Links[0].Href != "walk.photos/"+kept {
		t.Fatalf("links after rename: %+v %v", file, err)
	}
	if response, _ := http.Get(server.URL + "/api/photo?path=" + url.QueryEscape(renamed) + "&name=" + kept); response.StatusCode != http.StatusOK {
		t.Fatalf("photo after rename: %d", response.StatusCode)
	}
}
