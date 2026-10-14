package gpx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

func ownRenameGPX(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trip.gpx")
	if err := gpxfile.Create(path, "Trip", []gpxfile.Track{{Name: "Walk", Segments: []gpxfile.Segment{{Points: []gpxfile.Point{{}}}}}}); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenameDraftKeepsItsContents(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := drafts.create("Original")
	state, _ := drafts.get(path)
	state.Names = map[string]string{"t0": "Held name"}
	if err := drafts.set(path, state); err != nil {
		t.Fatal(err)
	}
	defer drafts.remove(path)
	code, result := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": path, "name": "Renamed"})
	if code != http.StatusOK {
		t.Fatalf("rename = %d %v", code, result)
	}
	target := result["path"].(string)
	defer drafts.remove(target)
	if draftName(target) != "Renamed" {
		t.Fatalf("target = %q", target)
	}
	if _, ok := drafts.get(path); ok {
		t.Fatal("old draft remains")
	}
	held, ok := drafts.get(target)
	if !ok || held.Names["t0"] != "Held name" {
		t.Fatal("draft edits lost")
	}
	var track trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(target), &track)
	if track.Name != "Renamed" || !track.Draft {
		t.Fatalf("track = %+v", track)
	}
}

func TestRenameOwnFilePreservesBytesAndPendingEdits(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := ownRenameGPX(t)
	target := filepath.Join(filepath.Dir(path), "Renamed.gpx")
	defer pending.drop(path)
	defer pending.drop(target)
	if code, _ := send(t, server, http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Edited walk"}); code != http.StatusOK {
		t.Fatalf("part-name = %d", code)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if code, result := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": path, "name": "Renamed.gpx"}); code != http.StatusOK || result["path"] != target {
		t.Fatalf("rename = %d %v", code, result)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rename changed GPX contents or saved edits")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("old file exists: %v", err)
	}
	if pending.has(path) || !pending.has(target) {
		t.Fatal("pending edits did not follow rename")
	}
	var track trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(target), &track)
	if !track.Unsaved || track.Parts[0].Name != "Edited walk" {
		t.Fatal("renamed file lost pending name")
	}
	if code, result := send(t, server, http.MethodPost, "/api/save", map[string]any{"path": target}); code != http.StatusOK {
		t.Fatalf("save = %d %v", code, result)
	}
	parsed, err := gpxfile.Open(target)
	if err != nil || parsed.Tracks[0].Name != "Edited walk" || pending.has(target) {
		t.Fatalf("saved renamed file = %+v %v", parsed, err)
	}
}

func TestRenameOwnFileMovesCompanionSidecar(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := ownRenameGPX(t)
	if err := os.WriteFile(sidecar.PathFor(path), []byte("synthetic companion"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(path), "Renamed.gpx")
	if code, result := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": path, "name": "Renamed"}); code != http.StatusOK {
		t.Fatalf("rename = %d %v", code, result)
	}
	data, err := os.ReadFile(sidecar.PathFor(target))
	if err != nil || string(data) != "synthetic companion" {
		t.Fatalf("companion = %q %v", data, err)
	}
	if _, err := os.Stat(sidecar.PathFor(path)); !os.IsNotExist(err) {
		t.Fatalf("old sidecar exists: %v", err)
	}
}

func TestRenameRejectsInvalidNamesAndOccupiedTargets(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	source := ownRenameGPX(t)
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", ".", "..", "../outside", "sub/trip", `sub\trip`} {
		if code, _ := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": source, "name": name}); code != http.StatusBadRequest {
			t.Errorf("name %q = %d", name, code)
		}
	}
	for _, companion := range []bool{false, true} {
		target := filepath.Join(filepath.Dir(source), "Occupied.gpx")
		taken := target
		if companion {
			taken = sidecar.PathFor(target)
		}
		if err := os.WriteFile(taken, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if code, _ := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": source, "name": "Occupied"}); code != http.StatusConflict {
			t.Fatalf("occupied = %d", code)
		}
		data, err := os.ReadFile(taken)
		if err != nil || string(data) != "keep" {
			t.Fatal("occupied file was replaced")
		}
		if err := os.Remove(taken); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": source, "name": "trip"}); code != http.StatusOK {
		t.Fatalf("unchanged name = %d", code)
	}
	data, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("rejected rename changed source")
	}
	foreign := filepath.Join(t.TempDir(), "recorded.gpx")
	if err := os.WriteFile(foreign, []byte(`<gpx version="1.1" creator="recorder"><trk><name>Walk</name></trk></gpx>`), 0600); err != nil {
		t.Fatal(err)
	}
	recorded, _ := os.ReadFile(foreign)
	// A recording is renamed with its bytes as recorded.
	if code, _ := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": foreign, "name": "Changed"}); code != http.StatusOK {
		t.Fatalf("recording = %d", code)
	}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(foreign), "Changed.gpx")); err != nil || !bytes.Equal(data, recorded) {
		t.Fatalf("renamed recording changed: %v", err)
	}
}

func TestDraftNamesKeepDotsAndRejectInvalidNames(t *testing.T) {
	handler := Handler(Settings{})
	for _, name := range []string{"Trip.v2", "Trip.v2.GPX", " Trip.v2 "} {
		body, err := json.Marshal(map[string]string{"name": name})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/draft", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatal(response.Body)
		}
		var result struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		defer drafts.remove(result.Path)
		if draftName(result.Path) != "Trip.v2" {
			t.Fatalf("name truncated: %s", draftName(result.Path))
		}
	}
	for _, name := range []string{"", ".", "..", "../trip", "trip\x00"} {
		body, err := json.Marshal(map[string]string{"name": name})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/draft", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid draft name accepted: %q", name)
		}
	}
}

func TestNamedGPXRejectsBareExtension(t *testing.T) {
	for path, want := range map[string]bool{
		"/data/a.gpx": true, "/data/A.GPX": true, "/data/.gpx": false, "/data/ .gpx": false, "/data/a.txt": false,
	} {
		if got := namedGPX(path); got != want {
			t.Errorf("namedGPX(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestFileNameIsHeldThenWrittenAndSurvivesFileRename(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := ownRenameGPX(t)
	target := filepath.Join(filepath.Dir(path), "Other file.gpx")
	defer pending.drop(path)
	defer pending.drop(target)
	if code, _ := send(t, server, http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "Hike"}); code != http.StatusOK {
		t.Fatalf("part-name = %d", code)
	}
	var track trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &track)
	if track.Name != "Hike" || !track.Unsaved || !track.Renamed {
		t.Fatalf("held name: %q unsaved %v", track.Name, track.Unsaved)
	}
	if parsed, _ := gpxfile.Open(path); parsed.Name == "Hike" {
		t.Fatal("name written before save")
	}
	if code, result := send(t, server, http.MethodPost, "/api/save", map[string]any{"path": path}); code != http.StatusOK {
		t.Fatalf("save = %d %v", code, result)
	}
	if parsed, err := gpxfile.Open(path); err != nil || parsed.Name != "Hike" {
		t.Fatalf("saved name = %+v %v", parsed, err)
	}
	if code, _ := send(t, server, http.MethodPut, "/api/file-name", map[string]any{"path": path, "name": "Other file"}); code != http.StatusOK {
		t.Fatalf("file-name = %d", code)
	}
	get(t, server, "/api/track?path="+url.QueryEscape(target), &track)
	if track.Name != "Hike" {
		t.Fatalf("name after file rename = %q", track.Name)
	}
}

func TestRestoreNameOfRecording(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "rec.gpx")
	if err := os.WriteFile(path, []byte(`<gpx version="1.1" creator="device"><metadata><name>Recorded</name></metadata><trk><trkseg><trkpt lat="1" lon="1"/></trkseg></trk></gpx>`), 0o644); err != nil {
		t.Fatal(err)
	}
	defer pending.drop(path)
	send(t, server, http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "Given"})
	send(t, server, http.MethodPost, "/api/save", map[string]any{"path": path})
	var track trackJSON
	get(t, server, "/api/track?path="+url.QueryEscape(path), &track)
	if track.Name != "Given" || !track.Renamed {
		t.Fatalf("given: %q %v", track.Name, track.Renamed)
	}
	if code, r := send(t, server, http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": ""}); code != http.StatusOK {
		t.Fatalf("restore = %d %v", code, r)
	}
	track = trackJSON{}
	get(t, server, "/api/track?path="+url.QueryEscape(path), &track)
	if track.Name != "Recorded" || track.Renamed || !track.Unsaved {
		t.Fatalf("restored: %q renamed %v unsaved %v", track.Name, track.Renamed, track.Unsaved)
	}
}
