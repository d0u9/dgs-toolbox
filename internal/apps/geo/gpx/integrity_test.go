package gpx

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
	"dgs-toolbox/internal/geo/stops"
)

func integrityRequest(t *testing.T, method, endpoint string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, endpoint, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	Handler(Settings{}).ServeHTTP(response, request)
	return response
}

func integrityOK(t *testing.T, method, endpoint string, body any) *httptest.ResponseRecorder {
	t.Helper()
	response := integrityRequest(t, method, endpoint, body)
	if response.Code != http.StatusOK {
		t.Fatalf("%s = %d %s", endpoint, response.Code, response.Body)
	}
	return response
}

func photoFixture(t *testing.T) (string, string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.gpx")
	data, err := hex.DecodeString("524946461a000000574542505650384c0e0000002f01400000071011fd0f4444ff03")
	if err != nil {
		t.Fatal(err)
	}
	name, err := publishPhoto(filepath.Join(filepath.Dir(path), photoDir(path)), data)
	if err != nil {
		t.Fatal(err)
	}
	wpt := gpxfile.Waypoint{Name: "A", Description: "Old", Links: []gpxfile.Link{
		{Href: "https://example.com/info", Text: "Info"},
		{Href: photoDir(path) + "/" + name, Text: "Old caption", Type: "image/webp"},
	}}
	if err := gpxfile.CreateAll(path, "Source", []gpxfile.Waypoint{wpt}, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pending.forget(path) })
	return path, name, data
}

func TestSaveAsLeavesOriginalAndSidecarUntouched(t *testing.T) {
	path := ownRenameGPX(t)
	defer pending.forget(path)
	side := []byte(`{"version":2,"segments":{"cuts":[1]}}`)
	if err := os.WriteFile(sidecar.PathFor(path), side, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := addTracks(path, "synthetic.gpx", []gpxfile.Track{{Name: "Added", Segments: []gpxfile.Segment{{Points: []gpxfile.Point{{}}}}}}); err != nil {
		t.Fatal(err)
	}
	integrityOK(t, http.MethodDelete, "/api/part", map[string]string{"path": path, "key": "t0"})
	target := filepath.Join(t.TempDir(), "export.gpx")
	integrityOK(t, http.MethodPost, "/api/save-as", map[string]string{"path": path, "target": target})
	for name, want := range map[string][]byte{path: before, sidecar.PathFor(path): side} {
		if got, err := os.ReadFile(name); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("Save as changed the original: %v", err)
		}
	}
	if pending.has(path) {
		t.Fatal("exported edits remain pending")
	}
	if undo, redo := history.counts(path); undo != 0 || redo != 0 {
		t.Fatal("exported edits remain in history")
	}
	file, err := gpxfile.Open(target)
	if err != nil || len(file.Tracks) != 1 || file.Tracks[0].Name != "Added" {
		t.Fatalf("export = %+v, %v", file, err)
	}
}

func TestPhotoCleanupFailureCannotRepeatSavedEdits(t *testing.T) {
	path := ownRenameGPX(t)
	defer pending.forget(path)
	dir := filepath.Join(filepath.Dir(path), photoDir(path))
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"aaaaaaaaaaaa.webp", ".trash"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := addTracks(path, "synthetic.gpx", []gpxfile.Track{{Name: "Added"}}); err != nil {
		t.Fatal(err)
	}
	response := integrityOK(t, http.MethodPost, "/api/save", map[string]string{"path": path})
	var result struct{ Saved, Warnings []string }
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Saved) != 1 || len(result.Warnings) != 1 || pending.has(path) {
		t.Fatalf("save = %+v, pending = %v", result, pending.has(path))
	}
	if err := os.Remove(filepath.Join(dir, ".trash")); err != nil {
		t.Fatal(err)
	}
	integrityOK(t, http.MethodPost, "/api/save", map[string]string{"path": path})
	file, err := gpxfile.Open(path)
	if err != nil || len(file.Tracks) != 2 {
		t.Fatalf("retry duplicated an addition: %+v, %v", file, err)
	}
}

func TestSavingRecordingDoesNotCleanItsPhotoFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.gpx")
	if err := os.WriteFile(path, []byte(`<gpx creator="device"><wpt lat="0" lon="0"/></gpx>`), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(filepath.Dir(path), photoDir(path))
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	photo := filepath.Join(dir, "aaaaaaaaaaaa.webp")
	if err := os.WriteFile(photo, []byte("synthetic recorded photo"), 0600); err != nil {
		t.Fatal(err)
	}
	defer pending.forget(path)
	integrityOK(t, http.MethodPut, "/api/part-name", map[string]string{"path": path, "key": "f", "name": "Named"})
	integrityOK(t, http.MethodPost, "/api/save", map[string]string{"path": path})
	if _, err := os.Stat(photo); err != nil {
		t.Fatalf("Save moved a recording's photo: %v", err)
	}
}

func TestWaypointPhotosSurviveTransferAndDraftRename(t *testing.T) {
	for _, draft := range []bool{false, true} {
		for _, move := range []bool{false, true} {
			t.Run(map[bool]string{false: "file", true: "draft"}[draft]+map[bool]string{false: "/copy", true: "/move"}[move], func(t *testing.T) {
				path, name, data := photoFixture(t)
				target := ownRenameGPX(t)
				if draft {
					target = drafts.create("Target")
				}
				defer func() { pending.forget(target); drafts.remove(target) }()
				integrityOK(t, http.MethodPut, "/api/waypoint-photos", map[string]any{"path": path, "key": "w0", "description": "New", "photos": []sidecar.Photo{{Name: name, Caption: "New caption"}}})
				integrityOK(t, http.MethodPost, "/api/part/copy", map[string]any{"path": path, "key": "w0", "target": target, "move": move})
				if draft {
					response := integrityOK(t, http.MethodPut, "/api/file-name", map[string]string{"path": target, "name": "Renamed"})
					var renamed map[string]string
					if err := json.Unmarshal(response.Body.Bytes(), &renamed); err != nil {
						t.Fatal(err)
					}
					target = renamed["path"]
				}
				if move {
					if err := flush(path); err != nil {
						t.Fatal(err)
					}
				}
				check := func() {
					t.Helper()
					result, err := analyse(target, stops.Defaults())
					if err != nil {
						t.Fatal(err)
					}
					wpt := result.file.Waypoints[0]
					photos := waypointPhotos(result, "w0", wpt)
					if photos.Description != "New" || len(photos.Photos) != 1 || photos.Photos[0].Caption != "New caption" || len(wpt.Links) != 2 || wpt.Links[0].Href != "https://example.com/info" {
						t.Fatalf("transferred waypoint = %+v, photos = %+v", wpt, photos)
					}
					response := integrityOK(t, http.MethodGet, "/api/photo?"+url.Values{"path": {target}, "name": {name}}.Encode(), nil)
					if !bytes.Equal(response.Body.Bytes(), data) {
						t.Fatal("photo bytes changed during transfer")
					}
				}
				check()
				if draft {
					saved := filepath.Join(t.TempDir(), "saved.gpx")
					integrityOK(t, http.MethodPost, "/api/save-as", map[string]string{"path": target, "target": saved})
					if _, exists := drafts.photos[target]; exists {
						t.Fatal("saved draft retained photo bytes")
					}
					target = saved
				} else if err := flush(target); err != nil {
					t.Fatal(err)
				}
				check()
			})
		}
	}
}

func TestSaveAsIncludesEditedPhotosWithoutChangingSource(t *testing.T) {
	path, name, data := photoFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	integrityOK(t, http.MethodPut, "/api/waypoint-photos", map[string]any{"path": path, "key": "w0", "description": "New", "photos": []sidecar.Photo{{Name: name, Caption: "New caption"}}})
	integrityOK(t, http.MethodPut, "/api/part-name", map[string]string{"path": path, "key": "w0", "name": "Named"})
	target := filepath.Join(t.TempDir(), "export.gpx")
	integrityOK(t, http.MethodPost, "/api/save-as", map[string]string{"path": path, "target": target})
	file, err := gpxfile.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	wpt := file.Waypoints[0]
	if wpt.Name != "Named" || wpt.Description != "New" || len(wpt.Links) != 2 || wpt.Links[1].Href != "export.photos/"+name || wpt.Links[1].Text != "New caption" {
		t.Fatalf("exported waypoint = %+v", wpt)
	}
	if got, err := os.ReadFile(filepath.Join(filepath.Dir(target), "export.photos", name)); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("exported photo bytes: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, before) {
		t.Fatalf("export changed source: %v", err)
	}
}

func TestFailedPhotoTransferKeepsSourceAndExistingDestination(t *testing.T) {
	path, name, _ := photoFixture(t)
	target := ownRenameGPX(t)
	defer pending.forget(target)
	dir := filepath.Join(filepath.Dir(target), photoDir(target))
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	occupied := filepath.Join(dir, name)
	if err := os.WriteFile(occupied, []byte("synthetic occupied name"), 0600); err != nil {
		t.Fatal(err)
	}
	response := integrityRequest(t, http.MethodPost, "/api/part/copy", map[string]any{"path": path, "key": "w0", "target": target, "move": true})
	if response.Code == http.StatusOK || pending.has(path) || pending.has(target) {
		t.Fatalf("failed move = %d, pending source %v target %v", response.Code, pending.has(path), pending.has(target))
	}
	if got, err := os.ReadFile(occupied); err != nil || string(got) != "synthetic occupied name" {
		t.Fatalf("occupied photo changed: %v", err)
	}
}

func TestDeletingAddedPartRemapsEveryMetadataMap(t *testing.T) {
	path := drafts.create("Synthetic")
	defer drafts.remove(path)
	for _, name := range []string{"A", "B", "C"} {
		integrityOK(t, http.MethodPost, "/api/waypoint", map[string]any{"path": path, "name": name, "lat": 0, "lon": 0})
	}
	integrityOK(t, http.MethodPut, "/api/part-name", map[string]string{"path": path, "key": "w1", "name": "Renamed B"})
	held, _ := drafts.get(path)
	held.Names["f"], held.Names["r1"] = "File", "Other route"
	held.Photos = map[string]sidecar.Photos{"w1": {Description: "B's description"}}
	held.Moved = map[string][2]float64{"w1": {1, 2}}
	held.Order = map[string][]string{"w": {"w2", "w1", "w0"}}
	if err := saveSidecar(path, held); err != nil {
		t.Fatal(err)
	}
	integrityOK(t, http.MethodDelete, "/api/part", map[string]string{"path": path, "key": "w0"})
	result, err := analyse(path, stops.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if partName(result, "w0") != "Renamed B" || partName(result, "w1") != "C" || result.cleaning.Names["f"] != "File" || result.cleaning.Names["r1"] != "Other route" {
		t.Fatalf("wrong names after deletion: %+v", result.cleaning.Names)
	}
	if result.cleaning.Photos["w0"].Description != "B's description" || result.cleaning.Moved["w0"] != [2]float64{1, 2} {
		t.Fatal("photos or position moved onto the wrong waypoint")
	}
	if got := result.cleaning.Order["w"]; len(got) != 2 || got[0] != "w1" || got[1] != "w0" {
		t.Fatalf("order = %v", got)
	}
}

func TestInvalidCoordinateAndJSONEncodingReturnErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.gpx")
	if err := os.WriteFile(path, []byte(`<gpx creator="device"><trk><trkseg><trkpt lat="NaN" lon="0"/></trkseg></trk></gpx>`), 0600); err != nil {
		t.Fatal(err)
	}
	response := integrityRequest(t, http.MethodGet, "/api/track?path="+url.QueryEscape(path), nil)
	if response.Code == http.StatusOK || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("invalid GPX = %d %s", response.Code, response.Body)
	}
	response = httptest.NewRecorder()
	writeJSON(response, map[string]float64{"invalid": math.NaN()})
	if response.Code != http.StatusInternalServerError || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("invalid JSON = %d %s", response.Code, response.Body)
	}
	response = integrityRequest(t, http.MethodGet, "/api/photo?"+url.Values{"path": {path}, "name": {"../outside.webp"}}.Encode(), nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid photo name = %d", response.Code)
	}
}

func TestDiscardDraftReleasesDataPhotosAndHistoryOnly(t *testing.T) {
	path, _, data := photoFixture(t)
	draft := drafts.create("Synthetic")
	defer drafts.remove(draft)
	integrityOK(t, http.MethodPost, "/api/part/copy", map[string]any{"path": path, "key": "w0", "target": draft})
	if len(drafts.photos[draft]) == 0 || len(data) == 0 {
		t.Fatal("fixture has no draft photo")
	}
	if undo, _ := history.counts(draft); undo == 0 {
		t.Fatal("fixture has no history")
	}
	integrityOK(t, http.MethodDelete, "/api/draft", map[string]string{"path": draft})
	if _, exists := drafts.get(draft); exists || len(drafts.photos[draft]) != 0 {
		t.Fatal("discarded draft retains data")
	}
	if undo, redo := history.counts(draft); undo != 0 || redo != 0 {
		t.Fatal("discarded draft retains history")
	}
	if response := integrityRequest(t, http.MethodDelete, "/api/draft", map[string]string{"path": path}); response.Code != http.StatusBadRequest {
		t.Fatalf("discarding a real file = %d", response.Code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("discard deleted a real file")
	}
}
