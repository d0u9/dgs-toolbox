// Package web serves the dgs doc page. At M1 it lists the PDFs under the tree
// and shows one; it writes nothing. The design is docs/apps/doc/.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/pdflist"
	"dgs-toolbox/internal/webui"
)

//go:embed web
var webFiles embed.FS

// Settings is what the server needs from the configuration.
type Settings struct {
	Addr string
	// Root is the tree the page lists. Empty is the working directory.
	Root string
}

// SettingsFrom reads the server settings out of the configuration.
func SettingsFrom(global config.Config) Settings {
	return Settings{Addr: global.DocWebAddr(), Root: global.DocRoot()}
}

// ResolvedRoot is the folder actually listed.
func (s Settings) ResolvedRoot() string {
	if s.Root != "" {
		return s.Root
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

type fileJSON struct {
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// Handler serves the page and its API.
func Handler(settings Settings) http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	root := settings.ResolvedRoot()
	tree := os.DirFS(root)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"root": root})
	})
	mux.HandleFunc("GET /api/files", func(w http.ResponseWriter, _ *http.Request) {
		files, err := pdflist.List(tree)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]fileJSON, 0, len(files))
		for _, f := range files {
			out = append(out, fileJSON{Path: f.Path, Size: f.Size, Modified: f.Modified})
		}
		writeJSON(w, http.StatusOK, out)
	})
	// A file is served only when the listing names it, so a path cannot reach
	// outside the tree or at anything that is not a listed PDF.
	mux.HandleFunc("GET /api/file", func(w http.ResponseWriter, r *http.Request) {
		want := r.URL.Query().Get("path")
		files, err := pdflist.List(tree)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, f := range files {
			if f.Path != want {
				continue
			}
			file, err := tree.Open(f.Path)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			defer file.Close()
			seeker, ok := file.(io.ReadSeeker)
			if !ok {
				http.Error(w, "file cannot be served", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeContent(w, r, "", f.Modified, seeker)
			return
		}
		http.Error(w, "no such PDF in the tree", http.StatusNotFound)
	})
	webui.Mount(mux)
	files := http.FileServerFS(static)
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

var (
	serverOnce sync.Once
	serverURL  string
	serverErr  error
)

// Serve starts the page once per process and returns the URL to open. The
// address is always a loopback one.
func Serve(settings Settings) (string, error) {
	serverOnce.Do(func() {
		listener, err := net.Listen("tcp", settings.Addr)
		if err != nil {
			serverErr = err
			return
		}
		addr := listener.Addr().(*net.TCPAddr)
		serverURL = "http://" + net.JoinHostPort(addr.IP.String(), strconv.Itoa(addr.Port)) + "/"
		handler := Handler(settings)
		go func() {
			if err := http.Serve(listener, handler); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErr = err
			}
		}()
	})
	return serverURL, serverErr
}
