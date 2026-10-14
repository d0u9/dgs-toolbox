package gpx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func TestSaveDraftReleasesHistoryOnlyOnSuccess(t *testing.T) {
	for _, saved := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed", true: "saved"}[saved], func(t *testing.T) {
			path := drafts.create("Synthetic")
			defer drafts.remove(path)
			for _, name := range []string{"One", "Two"} {
				file, _ := drafts.get(path)
				file.Names = map[string]string{fileNameKey: name}
				if err := saveSidecar(path, file); err != nil {
					t.Fatal(err)
				}
			}
			if moved, err := history.step(path, true); !moved || err != nil {
				t.Fatalf("undo = %v, %v", moved, err)
			}
			target := filepath.Join(t.TempDir(), "saved.gpx")
			if !saved {
				target = filepath.Join(t.TempDir(), "missing", "saved.gpx")
			}
			body, err := json.Marshal(map[string]string{"path": path, "target": target})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/save-as", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			Handler(Settings{}).ServeHTTP(response, request)
			if (response.Code == http.StatusOK) != saved {
				t.Fatalf("save-as = %d %s", response.Code, response.Body)
			}
			if _, exists := drafts.get(path); exists == saved {
				t.Fatalf("draft exists = %v after saved = %v", exists, saved)
			}
			want := 1
			if saved {
				want = 0
			}
			if undo, redo := history.counts(path); undo != want || redo != want {
				t.Fatalf("history = (%d, %d), want (%d, %d)", undo, redo, want, want)
			}
		})
	}
}

func TestUndoRedoUnsavedEdits(t *testing.T) {
	server := httptest.NewServer(Handler(Settings{}))
	defer server.Close()
	path := ownRenameGPX(t)
	defer pending.forget(path)
	track := func() trackJSON {
		var got trackJSON
		get(t, server, "/api/track?path="+url.QueryEscape(path), &got)
		return got
	}
	rename := func(name string) {
		if code, _ := send(t, server, http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": name}); code != http.StatusOK {
			t.Fatalf("rename %s = %d", name, code)
		}
	}
	step := func(kind string) {
		if code, _ := send(t, server, http.MethodPost, "/api/"+kind, map[string]any{"path": path}); code != http.StatusOK {
			t.Fatalf("%s = %d", kind, code)
		}
	}
	rename("One")
	rename("Two")
	if got := track(); got.Parts[0].Name != "Two" || got.Undo != 2 || got.Redo != 0 {
		t.Fatalf("after edits: %q undo %d redo %d", got.Parts[0].Name, got.Undo, got.Redo)
	}
	step("undo")
	if got := track(); got.Parts[0].Name != "One" || got.Undo != 1 || got.Redo != 1 {
		t.Fatalf("after undo: %q undo %d redo %d", got.Parts[0].Name, got.Undo, got.Redo)
	}
	step("undo")
	if got := track(); got.Parts[0].Name != "Walk" || got.Unsaved {
		t.Fatalf("back to disk: %q unsaved %v", got.Parts[0].Name, got.Unsaved)
	}
	step("redo")
	if got := track(); got.Parts[0].Name != "One" || !got.Unsaved {
		t.Fatalf("after redo: %q", got.Parts[0].Name)
	}
	rename("Three")
	if got := track(); got.Redo != 0 {
		t.Fatal("a new edit kept the redo")
	}
	step("redo") // nothing to redo is not an error
	send(t, server, http.MethodPost, "/api/save", map[string]any{"path": path})
	if got := track(); got.Undo != 0 || got.Redo != 0 || got.Parts[0].Name != "Three" {
		t.Fatalf("after save: undo %d redo %d %q", got.Undo, got.Redo, got.Parts[0].Name)
	}
}
