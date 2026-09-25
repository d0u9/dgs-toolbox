// Package web serves the dgs doc pages: browse, over the Items kept in the
// tree, and import, over a folder of PDFs opened from anywhere. The design is docs/apps/doc/.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/ocr"
	"dgs-toolbox/internal/doc/pdflist"
	"dgs-toolbox/internal/doc/suggest"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/webfile"
	"dgs-toolbox/internal/webui"
)

//go:embed web
var webFiles embed.FS

// Settings is what the server needs from the configuration.
type Settings struct {
	Addr string
	// Root is the tree the page lists. Empty is the working directory.
	Root string
	// Read recognises a PDF's text. Nil uses ocr.Recognize.
	Read func(path string) (ocr.Result, error)
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

type sourceFileJSON struct {
	// Path is slash-separated and relative to the opened folder.
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
}

type server struct {
	root string
	now  func() time.Time
	// read is what recognises a PDF's text: ocr.Recognize, or a stand-in.
	read func(path string) (ocr.Result, error)
	// texts keeps what was read, by digest, for the life of the process.
	// Reading is slow and the same PDF is looked at more than once.
	texts *sync.Map
	// pictures holds pages drawn for the preview.
	pictures *pictures
}

// Handler serves the pages and their API.
func Handler(settings Settings) http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	s := server{root: settings.ResolvedRoot(), now: time.Now, read: settings.Read, texts: &sync.Map{}, pictures: newPictures()}
	if s.read == nil {
		s.read = func(path string) (ocr.Result, error) { return ocr.Recognize(path, ocr.DefaultMaxPages) }
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/source", s.sourceList)
	mux.HandleFunc("GET /api/source/file", s.sourceFile)
	mux.HandleFunc("GET /api/revision", s.revision)
	mux.HandleFunc("GET /api/text", s.text)
	mux.HandleFunc("GET /api/pages", s.pages)
	mux.HandleFunc("GET /api/page", s.page)
	mux.HandleFunc("POST /api/import", s.importPDF)
	mux.HandleFunc("POST /api/revisions", s.addRevision)
	mux.HandleFunc("POST /api/head", s.setHead)
	mux.HandleFunc("POST /api/fields", s.setFields)
	webui.Mount(mux)
	// The dialog only chooses a folder to read; nothing it reaches is changed.
	webfile.Mount(mux, webfile.Options{Root: s.root})
	files := http.FileServerFS(static)
	serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
	for _, page := range []string{"browse", "import"} {
		mux.Handle("GET /"+page, http.RedirectHandler("/"+page+"/", http.StatusFound))
		mux.Handle("GET /"+page+"/", http.StripPrefix("/"+page+"/", pageHandler(serve, page+".html")))
	}
	mux.Handle("GET /{$}", http.RedirectHandler("/browse/", http.StatusFound))
	mux.Handle("GET /", serve)
	return mux
}

// pageHandler answers a page's own root with its document, so a reload of
// /import/ lands on the page rather than on a listing.
func pageHandler(files http.Handler, document string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || r.URL.Path == "/" {
			r = r.Clone(r.Context())
			r.URL.Path = "/" + document
		}
		files.ServeHTTP(w, r)
	})
}

func (s server) state(w http.ResponseWriter, _ *http.Request) {
	out := stateJSON{Root: s.root, Templates: []tree.Template{}, Items: []tree.Item{}}
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
	out.Error = strings.Join(problems, "; ")
	writeJSON(w, http.StatusOK, out)
}

// sourceList is every PDF under an opened folder, each marked with the Item
// already keeping it. The items folder of the tree itself is left out: those
// PDFs are the tree's, not something to import.
func (s server) sourceList(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	if !filepath.IsAbs(dir) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "open a folder by its full path"})
		return
	}
	files, err := pdflist.List(os.DirFS(dir))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	items, _ := tree.LoadItems(s.root)
	kept := map[string]string{}
	for _, item := range items {
		for _, r := range item.Revisions {
			kept[r.Digest] = item.ID
		}
	}
	itemsDir := filepath.Join(s.root, tree.ItemsDir)
	out := []sourceFileJSON{}
	for _, f := range files {
		full := filepath.Join(dir, filepath.FromSlash(f.Path))
		if within(itemsDir, full) {
			continue
		}
		entry := sourceFileJSON{Path: f.Path, Size: f.Size, Modified: f.Modified}
		if len(kept) > 0 {
			if digest, err := tree.FileDigest(full); err == nil {
				entry.Item = kept[digest]
			}
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"dir": dir, "files": out})
}

func within(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sourcePath is the file a request names: a PDF under an opened folder, by a
// relative path that cannot climb out of it, and not hidden or linked. Only
// what pdflist would list is accepted.
func sourcePath(dir, path string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", errors.New("open a folder by its full path")
	}
	if !fs.ValidPath(path) || path == "." || !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return "", errors.New("not a PDF in the opened folder")
	}
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, ".") {
			return "", errors.New("not a PDF in the opened folder")
		}
	}
	full := filepath.Join(dir, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	return full, nil
}

// sourceFile serves a PDF under an opened folder for preview.
func (s server) sourceFile(w http.ResponseWriter, r *http.Request) {
	full, err := sourcePath(r.URL.Query().Get("dir"), r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	servePDF(w, r, full)
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

type textJSON struct {
	Available   bool                             `json:"available"`
	Pages       []ocr.Page                       `json:"pages"`
	Suggestions map[string]map[string]suggestion `json:"suggestions"`
	Error       string                           `json:"error,omitempty"`
}

// suggestion is a field's value from the text, and the line it was read from
// so the page can show where; Page and Line count from 0, and Page is -1
// when the value spans lines.
type suggestion struct {
	Value string `json:"value"`
	Page  int    `json:"page"`
	Line  int    `json:"line"`
}

// text is a PDF's recognised text, and what each Template's patterns find in
// it. The PDF is named as a preview names it: a file under an opened folder
// (dir, path) or a revision of an Item (item, digest).
func (s server) text(w http.ResponseWriter, r *http.Request) {
	path, err := s.pdfPath(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	out := textJSON{Available: ocr.Available(), Pages: []ocr.Page{}, Suggestions: map[string]map[string]suggestion{}}
	digest, err := tree.FileDigest(path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	result, cached := s.texts.Load(digest)
	if !cached {
		read, err := s.read(path)
		if err != nil {
			out.Error = err.Error()
			writeJSON(w, http.StatusOK, out)
			return
		}
		s.texts.Store(digest, read)
		result = read
	}
	out.Available = true
	read := result.(ocr.Result)
	out.Pages = read.Pages
	if templates, err := tree.LoadTemplates(s.root); err == nil {
		for kind, found := range suggest.All(templates, read.Text()) {
			out.Suggestions[kind] = map[string]suggestion{}
			for key, m := range found {
				at := suggestion{Value: m.Value, Page: -1, Line: -1}
				page, line, ok := read.Locate(m.Start)
				endPage, endLine, endOK := read.Locate(m.End - 1)
				if ok && endOK && page == endPage && line == endLine {
					at.Page, at.Line = page, line
				}
				out.Suggestions[kind][key] = at
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// pdfPath is the PDF a query names, as a preview names it: a file under an
// opened folder (dir, path) or a revision of an Item (item, digest).
func (s server) pdfPath(query url.Values) (string, error) {
	if query.Get("item") == "" {
		return sourcePath(query.Get("dir"), query.Get("path"))
	}
	item, _, err := tree.FindItem(s.root, query.Get("item"))
	if err != nil {
		return "", err
	}
	for _, rev := range item.Revisions {
		if rev.Digest == query.Get("digest") {
			return tree.PDFPath(s.root, item.ID, rev.Digest), nil
		}
	}
	return "", errors.New("no such revision")
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
	Dir    string            `json:"dir"`
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
	source, ok := s.source(w, request.Dir, request.Path)
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

// source is the PDF an import names, or false with the response written.
func (s server) source(w http.ResponseWriter, dir, path string) (string, bool) {
	full, err := sourcePath(dir, path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return "", false
	}
	return full, true
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
		Dir  string `json:"dir"`
		Path string `json:"path"`
		Item string `json:"item"`
	}
	if !decode(w, r, &request) {
		return
	}
	source, ok := s.source(w, request.Dir, request.Path)
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
