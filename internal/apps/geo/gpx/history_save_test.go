package gpx

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

// Saving a GPX dgs wrote takes the removed points out of the file, and undo
// and redo cross the save: the file on disk goes back and forth with them.
func TestUndoCrossesSaveOfOurGPX(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	ourGPX(t, path)
	original := read(t, path)
	s := newStepper(t, path)

	s.lasso(3)
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{path}})
	saved := read(t, path)
	if got := pointsOnDisk(t, path); len(got) != 2 || got[1] != 1 {
		t.Fatalf("points after save = %v", got)
	}
	if state, found, _ := gpxfile.ReadState(path); found && len(state) > 0 {
		var held sidecar.File
		_ = json.Unmarshal(state, &held)
		if len(held.Clean.Edits) != 0 {
			t.Fatalf("edits kept after save: %+v", held.Clean.Edits)
		}
	}

	if !s.step(true) {
		t.Fatal("the save was not undone")
	}
	if string(read(t, path)) != string(original) {
		t.Fatal("undoing the save did not put the file back")
	}
	if got := s.shown(); !got.Unsaved || len(got.Clean.Params.Edits) != 1 {
		t.Fatalf("after undoing the save: unsaved %v edits %d", got.Unsaved, len(got.Clean.Params.Edits))
	}
	if !s.step(true) {
		t.Fatal("the edit before the save was not undone")
	}
	if got := s.shown(); got.Unsaved {
		t.Fatal("the file still reads as edited")
	}

	s.step(false)
	s.step(false)
	if string(read(t, path)) != string(saved) {
		t.Fatal("redoing the save did not write it again")
	}
	if got := s.shown(); got.Unsaved {
		t.Fatal("the redone save left edits unsaved")
	}
}

// A file changed on disk after a save is never overwritten by undoing it.
func TestUndoOfSaveRefusesAChangedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	ourGPX(t, path)
	s := newStepper(t, path)
	s.lasso(3)
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{path}})
	changed := append(read(t, path), []byte("\n<!-- edited elsewhere -->\n")...)
	if err := os.WriteFile(path, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := s.call(http.MethodPost, "/api/undo", nil); code == http.StatusOK {
		t.Fatal("undo overwrote a file changed elsewhere")
	}
	if string(read(t, path)) != string(changed) {
		t.Fatal("the changed file was written")
	}
	if undo, _ := history.counts(path); undo != 2 {
		t.Fatalf("a refused undo moved the history: %d left", undo)
	}
}

// A recording is never written: saving puts its edits in the sidecar, the
// removed point stays in the GPX, and undoing the save takes the sidecar back.
func TestRecordingSaveKeepsPointsAndIsUndone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recorded.gpx")
	if err := os.WriteFile(path, []byte(sampleWithParts), 0o644); err != nil {
		t.Fatal(err)
	}
	original := read(t, path)
	before := pointsOnDisk(t, path)
	s := newStepper(t, path)
	s.lasso(1)
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{path}})
	if string(read(t, path)) != string(original) {
		t.Fatal("saving wrote the recording")
	}
	if got := pointsOnDisk(t, path); len(got) != len(before) || got[0] != before[0] {
		t.Fatalf("points = %v, recorded %v", got, before)
	}
	if _, err := os.Stat(sidecar.PathFor(path)); err != nil {
		t.Fatalf("no sidecar after save: %v", err)
	}
	if !s.step(true) {
		t.Fatal("the save was not undone")
	}
	if _, err := os.Stat(sidecar.PathFor(path)); !os.IsNotExist(err) {
		t.Fatalf("undoing the first save left the sidecar: %v", err)
	}
	if got := s.shown(); !got.Unsaved {
		t.Fatal("the edit is not unsaved again")
	}
	s.step(false)
	if _, err := os.Stat(sidecar.PathFor(path)); err != nil {
		t.Fatalf("redoing the save wrote no sidecar: %v", err)
	}
	if string(read(t, path)) != string(original) {
		t.Fatal("undo or redo wrote the recording")
	}
}

// Renaming a file whose history holds a save forgets that history: the saved
// bytes name the old photo folder.
func TestRenameForgetsAHistoryHoldingASave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	ourGPX(t, path)
	s := newStepper(t, path)
	s.lasso(3)
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{path}})
	code, result := s.call(http.MethodPut, "/api/file-name", map[string]any{"path": path, "name": "moved"})
	if code != http.StatusOK {
		t.Fatalf("rename = %d %v", code, result)
	}
	moved := result["path"].(string)
	defer pending.forget(moved)
	if undo, redo := history.counts(moved); undo != 0 || redo != 0 {
		t.Fatalf("history after rename = %d, %d", undo, redo)
	}
}

// Undoing a save that sent a photo to the trash takes the photo back: the
// file put back links it again.
func TestUndoOfSaveTakesPhotosOutOfTheTrash(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "trip.gpx")
	ourGPX(t, path)
	s := newStepper(t, path)
	black, _ := hex.DecodeString("524946461a000000574542505650384c0e0000002f01400000071011fd0f4444ff03")
	response, err := http.Post(s.server.URL+"/api/photo?path="+url.QueryEscape(path), "image/webp", bytes.NewReader(black))
	if err != nil {
		t.Fatal(err)
	}
	var answer map[string]string
	_ = json.NewDecoder(response.Body).Decode(&answer)
	response.Body.Close()
	name := answer["name"]
	setPhotos := func(photos []map[string]string) {
		s.ok(http.MethodPut, "/api/waypoint-photos", map[string]any{"path": path, "key": "w0", "photos": photos})
		s.ok(http.MethodPost, "/api/save", map[string]any{"path": path})
	}
	setPhotos([]map[string]string{{"name": name}})
	setPhotos([]map[string]string{})
	inFolder, inTrash := filepath.Join(folder, "trip.photos", name), filepath.Join(folder, "trip.photos", ".trash", name)
	if _, err := os.Stat(inTrash); err != nil {
		t.Fatalf("the unlinked photo is not in the trash: %v", err)
	}
	s.step(true)
	if _, err := os.Stat(inFolder); err != nil {
		t.Fatalf("undoing the save left the photo in the trash: %v", err)
	}
	if file, err := gpxfile.Open(path); err != nil || len(file.Waypoints[0].Links) != 1 {
		t.Fatalf("the file put back does not link the photo: %v", err)
	}
	// Redoing the save unlinks the photo again, and it goes back to the trash.
	s.step(false)
	if _, err := os.Stat(inTrash); err != nil {
		t.Fatalf("redoing the save left the photo out of the trash: %v", err)
	}
	if _, err := os.Stat(inFolder); !os.IsNotExist(err) {
		t.Fatalf("the unlinked photo stayed in the folder: %v", err)
	}
}

// Two saves of a recording are undone and redone through its sidecar, byte
// for byte, so each step finds the sidecar the step before left.
func TestTwoSavesOfARecordingAreUndoneAndRedone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recorded.gpx")
	if err := os.WriteFile(path, []byte(sampleWithParts), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	side := sidecar.PathFor(path)
	s.lasso(1)
	s.ok(http.MethodPost, "/api/save", nil)
	first := read(t, side)
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Renamed"})
	s.ok(http.MethodPost, "/api/save", nil)
	second := read(t, side)
	for i := 0; i < 4; i++ {
		if !s.step(true) {
			t.Fatalf("undo %d did not move", i+1)
		}
	}
	if _, err := os.Stat(side); !os.IsNotExist(err) {
		t.Fatalf("the sidecar is still there after undoing everything: %v", err)
	}
	s.step(false)
	s.step(false)
	if string(read(t, side)) != string(first) {
		t.Fatal("redoing the first save did not give its sidecar back")
	}
	s.step(false)
	s.step(false)
	if string(read(t, side)) != string(second) {
		t.Fatal("redoing the second save did not give its sidecar back")
	}
}

// Closing a file forgets its undo history, saves included; the file on disk
// stays as saved.
func TestCloseForgetsHistoryWithSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	ourGPX(t, path)
	s := newStepper(t, path)
	s.lasso(3)
	s.ok(http.MethodPost, "/api/save", nil)
	saved := read(t, path)
	if undo, _ := history.counts(path); undo != 2 {
		t.Fatalf("history before close = %d", undo)
	}
	s.ok(http.MethodPost, "/api/close", map[string]any{"paths": []string{path}})
	if undo, redo := history.counts(path); undo != 0 || redo != 0 {
		t.Fatalf("history after close = %d, %d", undo, redo)
	}
	if string(read(t, path)) != string(saved) {
		t.Fatal("closing changed the file")
	}
	if s.step(true) {
		t.Fatal("a closed file's save was undone")
	}
}

// A save whose undo cannot be written leaves the history as it was, so the
// undo can be tried again.
func TestFailedUndoOfSaveKeepsTheHistory(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "trip")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, "ours.gpx")
	ourGPX(t, path)
	original := read(t, path)
	s := newStepper(t, path)
	s.lasso(3)
	s.ok(http.MethodPost, "/api/save", nil)
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(folder, 0o755)
	if code, _ := s.call(http.MethodPost, "/api/undo", nil); code == http.StatusOK {
		t.Skip("this filesystem writes into a read-only folder")
	}
	if undo, redo := history.counts(path); undo != 2 || redo != 0 {
		t.Fatalf("a failed undo moved the history: %d, %d", undo, redo)
	}
	if err := os.Chmod(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if !s.step(true) || string(read(t, path)) != string(original) {
		t.Fatal("the undo did not work once the folder was writable")
	}
}

// Closing a file that another page has edited since keeps those edits and
// their history.
func TestCloseKeepsEditsHeldFromElsewhere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	ourGPX(t, path)
	s := newStepper(t, path)
	s.lasso(3)
	s.ok(http.MethodPost, "/api/close", map[string]any{"paths": []string{path}})
	if !pending.has(path) {
		t.Fatal("closing dropped unsaved edits")
	}
	if undo, _ := history.counts(path); undo != 1 {
		t.Fatalf("closing dropped the history of unsaved edits: %d", undo)
	}
}

// A photo that only the held edits link comes out of the trash too, and a
// name that is not a photo this program named is never moved.
func TestUntrashLinkedFollowsHeldEdits(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "trip.gpx")
	ourGPX(t, path)
	defer pending.forget(path)
	trash := filepath.Join(folder, "trip.photos", ".trash")
	if err := os.MkdirAll(trash, 0o755); err != nil {
		t.Fatal(err)
	}
	held, odd := "0123456789ab.webp", "notes.txt"
	for _, name := range []string{held, odd} {
		if err := os.WriteFile(filepath.Join(trash, name), []byte("synthetic"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pending.set(path, sidecar.File{Version: sidecar.Version, Photos: map[string]sidecar.Photos{
		"w0": {Photos: []sidecar.Photo{{Name: held}, {Name: odd}}},
	}})
	if err := untrashLinked(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, "trip.photos", held)); err != nil {
		t.Fatalf("the photo the held edits link stayed in the trash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(trash, odd)); err != nil {
		t.Fatalf("a file that is not a photo was moved: %v", err)
	}
}

// A photo that only an earlier edit linked goes to the trash when a later
// save unlinks it; undoing back to that edit takes it out again.
func TestUndoToAnEditLinkingATrashedPhoto(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "trip.gpx")
	ourGPX(t, path)
	s := newStepper(t, path)
	black, _ := hex.DecodeString("524946461a000000574542505650384c0e0000002f01400000071011fd0f4444ff03")
	response, err := http.Post(s.server.URL+"/api/photo?path="+url.QueryEscape(path), "image/webp", bytes.NewReader(black))
	if err != nil {
		t.Fatal(err)
	}
	var answer map[string]string
	_ = json.NewDecoder(response.Body).Decode(&answer)
	response.Body.Close()
	name := answer["name"]
	link := func(photos []map[string]string) {
		s.ok(http.MethodPut, "/api/waypoint-photos", map[string]any{"path": path, "key": "w0", "photos": photos})
	}
	link([]map[string]string{{"name": name}}) // linked, never saved
	link([]map[string]string{})               // unlinked again
	s.ok(http.MethodPost, "/api/save", nil)
	inFolder, inTrash := filepath.Join(folder, "trip.photos", name), filepath.Join(folder, "trip.photos", ".trash", name)
	if _, err := os.Stat(inTrash); err != nil {
		t.Fatalf("the save did not trash the unlinked photo: %v", err)
	}
	s.step(true) // the save
	s.step(true) // the unlinking: the photo is linked again
	if _, err := os.Stat(inFolder); err != nil {
		t.Fatalf("the photo the edit links again stayed in the trash: %v", err)
	}
	if code := get(t, s.server, "/api/photo?path="+url.QueryEscape(path)+"&name="+name, nil); code != http.StatusOK {
		t.Fatalf("the photo is not served: %d", code)
	}
}
