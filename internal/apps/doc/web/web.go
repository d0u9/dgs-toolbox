// Package web serves the dgs doc page: the loose PDFs in the tree, the Items
// kept in it, and importing one into the other. The design is docs/apps/doc/.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/pdflist"
	"dgs-toolbox/internal/doc/tree"
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

type looseJSON struct {
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	// Item is the Item already keeping this PDF, when one does.
	Item string `json:"item,omitempty"`
}

type stateJSON struct {
	Root      string          `json:"root"`
	Tree      bool            `json:"tree"`
	Error     string          `json:"error,omitempty"`
	Templates []tree.Template `json:"templates"`
	Items     []tree.Item     `json:"items"`
	Loose     []looseJSON     `json:"loose"`
}

type server struct {
	root string
	fsys fs.FS
	now  func() time.Time
}

// Handler serves the page and its API.
func Handler(settings Settings) http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	root := settings.ResolvedRoot()
	s := server{root: root, fsys: os.DirFS(root), now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/file", s.file)
	mux.HandleFunc("GET /api/revision", s.revision)
	mux.HandleFunc("POST /api/import", s.importPDF)
	mux.HandleFunc("POST /api/revisions", s.addRevision)
	mux.HandleFunc("POST /api/head", s.setHead)
	mux.HandleFunc("POST /api/fields", s.setFields)
	webui.Mount(mux)
	files := http.FileServerFS(static)
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
	return mux
}

// loose is every PDF outside the items folder: what import offers.
func (s server) loose() ([]pdflist.File, error) {
	files, err := pdflist.List(s.fsys)
	if err != nil {
		return nil, err
	}
	out := files[:0]
	for _, f := range files {
		if !strings.HasPrefix(f.Path, tree.ItemsDir+"/") {
			out = append(out, f)
		}
	}
	return out, nil
}

func (s server) state(w http.ResponseWriter, _ *http.Request) {
	out := stateJSON{Root: s.root, Templates: []tree.Template{}, Items: []tree.Item{}, Loose: []looseJSON{}}
	var problems []string
	if err := tree.Require(s.root); err == nil {
		out.Tree = true
		if out.Templates, err = tree.LoadTemplates(s.root); err != nil {
			problems = append(problems, err.Error())
			out.Templates = []tree.Template{}
		}
		if out.Items, err = tree.LoadItems(s.root); err != nil {
			problems = append(problems, err.Error())
		}
		if out.Items == nil {
			out.Items = []tree.Item{}
		}
	} else if !errors.Is(err, tree.ErrNotATree) {
		problems = append(problems, err.Error())
	}
	kept := map[string]string{}
	for _, item := range out.Items {
		for _, r := range item.Revisions {
			kept[r.Digest] = item.ID
		}
	}
	files, err := s.loose()
	if err != nil {
		problems = append(problems, err.Error())
	}
	for _, f := range files {
		entry := looseJSON{Path: f.Path, Size: f.Size, Modified: f.Modified}
		if len(kept) > 0 {
			if digest, err := tree.FileDigest(filepath.Join(s.root, filepath.FromSlash(f.Path))); err == nil {
				entry.Item = kept[digest]
			}
		}
		out.Loose = append(out.Loose, entry)
	}
	out.Error = strings.Join(problems, "; ")
	writeJSON(w, http.StatusOK, out)
}

// file serves a loose PDF, only when the listing names it, so a path cannot
// reach outside the tree or at anything that is not a listed PDF.
func (s server) file(w http.ResponseWriter, r *http.Request) {
	want := r.URL.Query().Get("path")
	files, err := s.loose()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, f := range files {
		if f.Path == want {
			servePDF(w, r, filepath.Join(s.root, filepath.FromSlash(f.Path)))
			return
		}
	}
	http.Error(w, "no such PDF in the tree", http.StatusNotFound)
}

// revision serves an Item's PDF, looked up through its sidecar.
func (s server) revision(w http.ResponseWriter, r *http.Request) {
	id, digest := r.URL.Query().Get("item"), r.URL.Query().Get("digest")
	items, err := tree.LoadItems(s.root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		for _, rev := range item.Revisions {
			if rev.Digest == digest {
				servePDF(w, r, tree.PDFPath(s.root, item.ID, rev.Digest))
				return
			}
		}
	}
	http.Error(w, "no such revision", http.StatusNotFound)
}

func servePDF(w http.ResponseWriter, r *http.Request, path string) {
	file, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", info.ModTime(), file)
}

type importJSON struct {
	Path   string            `json:"path"`
	Type   string            `json:"type"`
	Fields map[string]string `json:"fields"`
}

func (s server) importPDF(w http.ResponseWriter, r *http.Request) {
	var request importJSON
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	source, ok := s.source(w, request.Path)
	if !ok {
		return
	}
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	for _, t := range templates {
		if t.Type != request.Type {
			continue
		}
		item, err := tree.Import(r.Context(), tree.ImportRequest{
			Root: s.root, Source: source, Template: t, Fields: request.Fields, Now: s.now(),
		})
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no Template for type " + request.Type})
}

// source is a listed loose PDF's path on disk. A path the listing does not
// name is refused, and the response is written.
func (s server) source(w http.ResponseWriter, path string) (string, bool) {
	files, err := s.loose()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return "", false
	}
	for _, f := range files {
		if f.Path == path {
			return filepath.Join(s.root, filepath.FromSlash(f.Path)), true
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such PDF in the tree"})
	return "", false
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return false
	}
	return true
}

func answer(w http.ResponseWriter, item tree.Item, err error) {
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, tree.ErrNoItem) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// addRevision adds a loose PDF to a document and moves HEAD to it.
func (s server) addRevision(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
		Item string `json:"item"`
	}
	if !decode(w, r, &request) {
		return
	}
	source, ok := s.source(w, request.Path)
	if !ok {
		return
	}
	item, err := tree.AddRevision(r.Context(), s.root, request.Item, source, s.now())
	answer(w, item, err)
}

func (s server) setHead(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item   string `json:"item"`
		Digest string `json:"digest"`
	}
	if !decode(w, r, &request) {
		return
	}
	item, err := tree.SetHead(s.root, request.Item, request.Digest)
	answer(w, item, err)
}

func (s server) setFields(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item   string            `json:"item"`
		Fields map[string]string `json:"fields"`
	}
	if !decode(w, r, &request) {
		return
	}
	item, _, err := tree.FindItem(s.root, request.Item)
	if err != nil {
		answer(w, item, err)
		return
	}
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		answer(w, item, err)
		return
	}
	for _, t := range templates {
		if t.Type == item.Type {
			item, err = tree.SetFields(s.root, request.Item, t, request.Fields)
			answer(w, item, err)
			return
		}
	}
	answer(w, item, errors.New("no Template for type "+item.Type))
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
