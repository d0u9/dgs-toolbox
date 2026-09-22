package webfile_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/filebrowse"
	"dgs-toolbox/internal/webfile"
)

type listing struct {
	Path   string             `json:"path"`
	Parent string             `json:"parent"`
	Dirs   []filebrowse.Entry `json:"dirs"`
	Files  []filebrowse.Entry `json:"files"`
}

func server(t *testing.T, root string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	webfile.Mount(mux, webfile.Options{Root: root})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func get(t *testing.T, s *httptest.Server, path string, into any) int {
	t.Helper()
	response, err := http.Get(s.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if into != nil {
		if err := json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return response.StatusCode
}

func folder(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "day one"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"walk.gpx", "notes.txt", ".hidden.gpx"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// A request without a path lists the root the page mounted the endpoints
// with, and the times and sizes a file manager's columns need come with it.
func TestDirDefaultsToTheRoot(t *testing.T) {
	root := folder(t)
	var got listing
	if status := get(t, server(t, root), webfile.Prefix+"dir", &got); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if got.Path != root || got.Parent != filepath.Dir(root) {
		t.Fatalf("listing = %+v", got)
	}
	if len(got.Dirs) != 1 || got.Dirs[0].Modified.IsZero() {
		t.Fatalf("dirs = %+v", got.Dirs)
	}
	if len(got.Files) != 2 { // notes.txt and walk.gpx; the hidden one is out
		t.Fatalf("files = %+v", got.Files)
	}
	for _, file := range got.Files {
		if file.Size == 0 || file.Modified.IsZero() {
			t.Fatalf("file = %+v", file)
		}
	}
}

func TestDirFiltersByExtension(t *testing.T) {
	root := folder(t)
	var got listing
	get(t, server(t, root), webfile.Prefix+"dir?path="+url.QueryEscape(root)+"&ext=.gpx", &got)
	if len(got.Files) != 1 || got.Files[0].Name != "walk.gpx" {
		t.Fatalf("files = %+v", got.Files)
	}
	// The folders stay, whatever the filter, so the dialog can step into them.
	if len(got.Dirs) != 1 {
		t.Fatalf("dirs = %+v", got.Dirs)
	}
}

func TestDirShowsHiddenWhenAsked(t *testing.T) {
	root := folder(t)
	var got listing
	get(t, server(t, root), webfile.Prefix+"dir?path="+url.QueryEscape(root)+"&ext=gpx&hidden=1", &got)
	if len(got.Files) != 2 {
		t.Fatalf("files = %+v", got.Files)
	}
}

func TestDirSaysWhyAFolderCannotBeRead(t *testing.T) {
	root := folder(t)
	missing := url.QueryEscape(filepath.Join(root, "gone"))
	if status := get(t, server(t, root), webfile.Prefix+"dir?path="+missing, nil); status != http.StatusNotFound {
		t.Fatalf("status %d, want 404", status)
	}
}

func TestPlacesLeadWithTheRoot(t *testing.T) {
	root := folder(t)
	var got struct {
		Places []filebrowse.Place `json:"places"`
	}
	if status := get(t, server(t, root), webfile.Prefix+"places", &got); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if len(got.Places) == 0 || got.Places[0].Path != root {
		t.Fatalf("places = %+v", got.Places)
	}
}

// The endpoints a save dialog needs are mounted only when the page asks for
// them, so a page that only opens files serves nothing that writes.
func writable(t *testing.T, root string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	webfile.Mount(mux, webfile.Options{Root: root, Writable: true})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func post(t *testing.T, s *httptest.Server, path string, body any, into any) int {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(s.URL+path, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if into != nil {
		if err := json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return response.StatusCode
}

func TestWriteEndpointsAreNotMountedUnlessAsked(t *testing.T) {
	s := server(t, folder(t))
	if status := post(t, s, "/ui/files/folder", map[string]string{"name": "new"}, nil); status != http.StatusNotFound {
		t.Fatalf("folder on a read-only mount = %d, want %d", status, http.StatusNotFound)
	}
	if status := get(t, s, "/ui/files/taken?name=walk.gpx", nil); status != http.StatusNotFound {
		t.Fatalf("taken on a read-only mount = %d, want %d", status, http.StatusNotFound)
	}
}

func TestTakenSaysWhatIsInTheWay(t *testing.T) {
	root := folder(t)
	s := writable(t, root)
	var answer struct {
		Taken bool   `json:"taken"`
		Kind  string `json:"kind"`
	}
	if status := get(t, s, "/ui/files/taken?name=walk.gpx", &answer); status != http.StatusOK {
		t.Fatalf("taken = %d", status)
	}
	if !answer.Taken || answer.Kind != "file" {
		t.Fatalf("taken walk.gpx = %+v, want a file", answer)
	}
	if get(t, s, "/ui/files/taken?name="+url.QueryEscape("day one"), &answer); !answer.Taken || answer.Kind != "dir" {
		t.Fatalf("taken \"day one\" = %+v, want a folder", answer)
	}
	if get(t, s, "/ui/files/taken?name=free.gpx", &answer); answer.Taken {
		t.Fatalf("taken free.gpx = %+v, want nothing there", answer)
	}
}

func TestFolderMakesOneAndRefusesANameTaken(t *testing.T) {
	root := folder(t)
	s := writable(t, root)
	var made struct {
		Path string `json:"path"`
	}
	if status := post(t, s, "/ui/files/folder", map[string]string{"name": "trip"}, &made); status != http.StatusOK {
		t.Fatalf("folder = %d", status)
	}
	if info, err := os.Stat(filepath.Join(root, "trip")); err != nil || !info.IsDir() {
		t.Fatalf("trip is not a folder: %v", err)
	}
	if made.Path != filepath.Join(root, "trip") {
		t.Fatalf("folder path = %q", made.Path)
	}
	if status := post(t, s, "/ui/files/folder", map[string]string{"name": "trip"}, nil); status != http.StatusConflict {
		t.Fatalf("second trip = %d, want %d", status, http.StatusConflict)
	}
	if status := post(t, s, "/ui/files/folder", map[string]string{"name": "a/b"}, nil); status != http.StatusBadRequest {
		t.Fatalf("a name holding a path = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestRenameMovesNothingOutOfItsFolder(t *testing.T) {
	root := folder(t)
	s := writable(t, root)
	var renamed struct {
		Path string `json:"path"`
	}
	body := map[string]string{"path": filepath.Join(root, "walk.gpx"), "name": "hike.gpx"}
	if status := post(t, s, "/ui/files/rename", body, &renamed); status != http.StatusOK {
		t.Fatalf("rename = %d", status)
	}
	if renamed.Path != filepath.Join(root, "hike.gpx") {
		t.Fatalf("rename path = %q", renamed.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "walk.gpx")); err == nil {
		t.Fatal("walk.gpx is still there")
	}
	taken := map[string]string{"path": filepath.Join(root, "hike.gpx"), "name": "notes.txt"}
	if status := post(t, s, "/ui/files/rename", taken, nil); status != http.StatusConflict {
		t.Fatalf("rename onto notes.txt = %d, want %d", status, http.StatusConflict)
	}
}
