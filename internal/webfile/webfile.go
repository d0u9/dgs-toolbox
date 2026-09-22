// Package webfile serves what the shared file dialog needs: one folder's
// listing, the places the dialog offers down its side, and — for a page that
// saves — whether a name is already taken, a new folder, and a rename. It is
// the only server side that dialog has, so every page that opens or saves a
// file mounts these endpoints instead of writing its own.
//
// The computation is filebrowse's; this package is the HTTP skin over it.
package webfile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/filebrowse"
	"dgs-toolbox/internal/webui"
)

// Prefix is the path both endpoints live under, inside the shared /ui/ space
// the dialog's script and stylesheet are served from.
const Prefix = webui.Prefix + "files/"

// Options is what a page tells the dialog's server side.
type Options struct {
	// Root is the folder a request without a path lists, and the folder the
	// places list leads with. Empty means the process's working directory.
	Root string
	// Writable mounts the three endpoints a save dialog needs — taken,
	// folder and rename. A page that only opens files leaves it off, and
	// nothing the dialog can reach changes anything on disk.
	Writable bool
}

// Mount registers the dialog's endpoints on a page's own mux:
//
//	GET  /ui/files/dir?path=&ext=.gpx&ext=.xml&hidden=1
//	GET  /ui/files/places
//	GET  /ui/files/taken?path=&name=      (Writable)
//	POST /ui/files/folder  {parent,name}  (Writable)
//	POST /ui/files/rename  {path,name}    (Writable)
//
// Repeating ext narrows the listing to those suffixes; leaving it off lists
// every file, as the dialog's "All files" filter does.
func Mount(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET "+Prefix+"dir", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		path := query.Get("path")
		if path == "" {
			path = options.Root
		}
		listing, err := filebrowse.List(path, filebrowse.Options{
			Extensions: query["ext"],
			Hidden:     truthy(query.Get("hidden")),
		})
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, listing)
	})
	mux.HandleFunc("GET "+Prefix+"places", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"places": filebrowse.Places(options.Root)})
	})
	if !options.Writable {
		return
	}
	// taken answers before anything is written, so a save dialog says "there
	// is already a file of that name" itself rather than after the fact. A
	// folder of that name is reported as one: a save never replaces a folder.
	mux.HandleFunc("GET "+Prefix+"taken", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		dir := query.Get("path")
		if dir == "" {
			dir = options.Root
		}
		name := query.Get("name")
		taken, err := filebrowse.Taken(dir, name)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		kind := ""
		if taken {
			kind = "file"
			if info, err := os.Lstat(filepath.Join(dir, strings.TrimSpace(name))); err == nil && info.IsDir() {
				kind = "dir"
			}
		}
		writeJSON(w, map[string]any{"taken": taken, "kind": kind})
	})
	mux.HandleFunc("POST "+Prefix+"folder", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Parent, Name string }
		if !readJSON(w, r, &body) {
			return
		}
		if body.Parent == "" {
			body.Parent = options.Root
		}
		path, err := filebrowse.CreateFolder(body.Parent, body.Name)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, map[string]any{"path": path})
	})
	mux.HandleFunc("POST "+Prefix+"rename", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Path, Name string }
		if !readJSON(w, r, &body) {
			return
		}
		path, err := filebrowse.Rename(body.Path, body.Name)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, map[string]any{"path": path})
	})
}

// readJSON decodes a request body, and says whether the handler may go on.
func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func truthy(value string) bool {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// statusFor answers with the reason the work could not be done, so the
// dialog can say "no such folder" rather than "server error".
func statusFor(err error) int {
	switch {
	case errors.Is(err, filebrowse.ErrExists):
		return http.StatusConflict
	case errors.Is(err, filebrowse.ErrEmptyName), errors.Is(err, filebrowse.ErrPathName):
		return http.StatusBadRequest
	case errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, fs.ErrPermission):
		return http.StatusForbidden
	case errors.Is(err, os.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
