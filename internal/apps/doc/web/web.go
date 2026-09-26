// Package web serves the dgs doc pages: browse, over the Items kept in the
// tree, and import, over a folder of PDFs opened from anywhere. The design is docs/apps/doc/.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/doc/classify"
	"dgs-toolbox/internal/doc/dates"
	"dgs-toolbox/internal/doc/expiry"
	"dgs-toolbox/internal/doc/ocr"
	"dgs-toolbox/internal/doc/pdflist"
	"dgs-toolbox/internal/doc/suggest"
	"dgs-toolbox/internal/doc/textcache"
	"dgs-toolbox/internal/doc/textread"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/webfile"
	"dgs-toolbox/internal/webui"
)

//go:embed web
var webFiles embed.FS

// Settings is what the server needs from the configuration.
type Settings struct {
	Addr string
	// Root is the tree the page lists when Trees is empty. Empty is the
	// working directory.
	Root string
	// Trees are the trees the page switches between, by name, when there is
	// more than one.
	Trees []Tree
	// ReadPage recognises one page of a PDF. Nil uses ocr.RecognizePage.
	ReadPage textread.ReadFunc
	// CacheDir keeps the text read, by digest. Empty keeps it nowhere.
	CacheDir string
	// DateOrder reads dates like 03/04/2026. Empty is dates.DefaultOrder.
	DateOrder dates.Order
	// ExpiringWithin is how long before its expiry a document counts as
	// expiring soon. Zero is expiry.DefaultSoon.
	ExpiringWithin time.Duration
}

// Tree is one tree the page opens, by the name it switches to.
type Tree struct {
	Name string `json:"name"`
	Root string `json:"root"`
}

// SettingsFrom reads the server settings out of the configuration.
func SettingsFrom(global config.Config) Settings {
	settings := Settings{Addr: global.DocWebAddr(), Root: global.DocRoot(), DateOrder: global.DocDateOrder(), ExpiringWithin: global.DocExpiringWithin()}
	if dir, err := global.DocCacheDir(); err == nil {
		settings.CacheDir = dir
	}
	if len(global.Doc.Trees) > 0 {
		for _, t := range global.DocTrees() {
			settings.Trees = append(settings.Trees, Tree{Name: t.Name, Root: t.Root})
		}
	}
	return settings
}

// ResolvedRoot is the folder actually listed: the first tree's.
func (s Settings) ResolvedRoot() string { return s.ResolvedTrees()[0].Root }

// ResolvedTrees is every tree the page opens, the first opened first: Trees,
// or the one tree Root names.
func (s Settings) ResolvedTrees() []Tree {
	if len(s.Trees) > 0 {
		return s.Trees
	}
	root := s.Root
	if root == "" {
		root = "."
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	return []Tree{{Root: root}}
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
	Root string `json:"root"`
	// Name is the tree's name in doc.trees, empty for doc.root.
	Name string `json:"name"`
	// Trees are every tree the page switches between.
	Trees     []Tree          `json:"trees"`
	Tree      bool            `json:"tree"`
	Error     string          `json:"error,omitempty"`
	Templates []tree.Template `json:"templates"`
	Items     []tree.Item     `json:"items"`
	// Expiry is each Item's expiry as its Current revision has it, by ID.
	Expiry map[string]expiry.Status `json:"expiry"`
	// SoonDays is how many days before its expiry an Item counts as
	// expiring soon, doc.expiring_within_days.
	SoonDays int `json:"soonDays"`
}

type server struct {
	root string
	name string
	// trees are every tree served, this one too, so an export can check its
	// Target against the others.
	trees     []Tree
	now       func() time.Time
	dateOrder dates.Order
	soon      time.Duration
	// store is the text cache the reader keeps, read directly to compare a
	// PDF with the Items already filed.
	store textcache.Store
	// reader reads PDFs' text through a queue and a cache: the page being
	// looked at first, pages wanted next in advance, each page once.
	reader *textread.Reader
	// pictures holds pages drawn for the preview.
	pictures *pictures
	// writing is held while an export or a merge writes, so two never overlap.
	writing *sync.Mutex
}

// Handler serves the pages and their API.
func Handler(settings Settings) http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	if settings.ExpiringWithin <= 0 {
		settings.ExpiringWithin = expiry.DefaultSoon
	}
	if settings.DateOrder == "" {
		settings.DateOrder = dates.DefaultOrder
	}
	read := settings.ReadPage
	if read == nil {
		read = ocr.RecognizePage
	}
	trees := settings.ResolvedTrees()
	shared := server{
		now: time.Now, pictures: newPictures(), dateOrder: settings.DateOrder, soon: settings.ExpiringWithin,
		writing: &sync.Mutex{}, trees: trees,
		store:  textcache.Store{Dir: settings.CacheDir},
		reader: textread.New(read, textcache.Store{Dir: settings.CacheDir}, ocr.DefaultMaxPages, textread.DefaultWorkers),
	}
	// Each tree has its own API; a request names its tree by ?tree=, and
	// one that names none is the first tree's.
	apis := map[string]http.Handler{}
	for _, t := range trees {
		s := shared
		s.root, s.name = t.Root, t.Name
		apis[t.Name] = s.api()
	}
	first := trees[0].Name
	mux := http.NewServeMux()
	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("tree")
		if name == "" {
			name = first
		}
		api, ok := apis[name]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no tree named " + name})
			return
		}
		api.ServeHTTP(w, r)
	})
	mux.Handle("GET /api/", dispatch)
	mux.Handle("POST /api/", dispatch)
	webui.Mount(mux)
	// The dialog only chooses a folder to read; nothing it reaches is changed.
	webfile.Mount(mux, webfile.Options{Root: trees[0].Root, Writable: true})
	files := http.FileServerFS(static)
	serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
	for _, page := range []string{"browse", "templates", "import", "views", "targets", "cases", "merge", "change-type", "log"} {
		mux.Handle("GET /"+page, http.RedirectHandler("/"+page+"/", http.StatusFound))
		mux.Handle("GET /"+page+"/", http.StripPrefix("/"+page+"/", pageHandler(serve, page+".html")))
	}
	mux.Handle("GET /{$}", http.RedirectHandler("/browse/", http.StatusFound))
	mux.Handle("GET /", serve)
	return mux
}

// api is one tree's API.
func (s server) api() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/issuers", s.issuers)
	mux.HandleFunc("GET /api/source", s.sourceList)
	mux.HandleFunc("GET /api/source/file", s.sourceFile)
	mux.HandleFunc("GET /api/revision", s.revision)
	mux.HandleFunc("GET /api/text", s.text)
	mux.HandleFunc("POST /api/ahead", s.ahead)
	mux.HandleFunc("GET /api/pages", s.pages)
	mux.HandleFunc("GET /api/page", s.page)
	mux.HandleFunc("POST /api/import", s.importPDF)
	mux.HandleFunc("POST /api/revisions", s.addRevision)
	mux.HandleFunc("POST /api/attach", s.attachPDF)
	mux.HandleFunc("POST /api/head", s.setHead)
	mux.HandleFunc("POST /api/revisions/delete", s.deleteRevision)
	mux.HandleFunc("POST /api/fields", s.setFields)
	mux.HandleFunc("POST /api/change-type", s.changeType)
	mux.HandleFunc("POST /api/notes", s.setNotes)
	mux.HandleFunc("POST /api/tags", s.setTags)
	mux.HandleFunc("POST /api/revision-tags", s.setRevisionTags)
	mux.HandleFunc("POST /api/frequent", s.setFrequent)
	mux.HandleFunc("POST /api/retired", s.setRetired)
	mux.HandleFunc("POST /api/supersession", s.supersession)
	mux.HandleFunc("GET /api/supersession-candidates", s.supersessionCandidates)
	mux.HandleFunc("GET /api/history", s.historyLog)
	mux.HandleFunc("POST /api/items/delete", s.deleteItem)
	mux.HandleFunc("GET /api/templates", s.templateList)
	mux.HandleFunc("POST /api/templates", s.templateSave)
	mux.HandleFunc("POST /api/templates/delete", s.templateDelete)
	mux.HandleFunc("GET /api/views", s.viewList)
	mux.HandleFunc("POST /api/views/plan", s.viewPlan)
	mux.HandleFunc("POST /api/views", s.viewSave)
	mux.HandleFunc("POST /api/views/delete", s.viewDelete)
	mux.HandleFunc("GET /api/targets", s.targetList)
	mux.HandleFunc("POST /api/targets", s.targetSave)
	mux.HandleFunc("POST /api/export/plan", s.exportPlan)
	mux.HandleFunc("POST /api/export", s.exportRun)
	mux.HandleFunc("POST /api/merge/plan", s.mergePlan)
	mux.HandleFunc("POST /api/merge", s.mergeRun)
	mux.HandleFunc("GET /api/cases", s.caseList)
	mux.HandleFunc("POST /api/cases/change", s.caseChange)
	mux.HandleFunc("POST /api/cases/export/plan", s.caseExportPlan)
	mux.HandleFunc("POST /api/cases/export", s.caseExportRun)
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/similar", s.similar)
	mux.HandleFunc("POST /api/read-all", s.readAll)
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
	out := stateJSON{Root: s.root, Name: s.name, Trees: s.trees, Templates: []tree.Template{}, Items: []tree.Item{}}
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
	out.Expiry = map[string]expiry.Status{}
	out.SoonDays = int(s.soon / (24 * time.Hour))
	now := s.now()
	for _, item := range out.Items {
		out.Expiry[item.ID] = expiry.Of(item.CurrentFields(), now, s.soon, s.dateOrder)
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
			if rev.Ref() == digest && rev.Digest != "" {
				servePDF(w, r, tree.PDFPath(s.root, item.ID, rev.Digest))
				return
			}
		}
	}
	http.Error(w, "no such revision", http.StatusNotFound)
}

type textJSON struct {
	Available bool `json:"available"`
	// Count is the PDF's pages, and MaxPages how many of them are read.
	Count    int      `json:"count"`
	MaxPages int      `json:"maxPages"`
	Page     ocr.Page `json:"page"`
	// Types ranks the Templates by how much the first page looks like the
	// first pages of the Items filed under each, and Type is the top one when
	// it passes the threshold. Only on page 0.
	Types []classify.Score `json:"types,omitempty"`
	Type  string           `json:"type,omitempty"`
	// Suggestions are what each Template's patterns find on this page.
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

// text is one page of a PDF's text, counting from 0, and what each Template's
// patterns find on it. The PDF is named as a preview names it.
func (s server) text(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	path, err := s.pdfPath(query)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	n, _ := strconv.Atoi(query.Get("page"))
	digest, err := tree.FileDigest(path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	out := textJSON{Available: true, MaxPages: s.reader.MaxPages(), Page: ocr.Page{Lines: []ocr.Line{}},
		Suggestions: map[string]map[string]suggestion{}}
	page, count, err := s.reader.Page(r.Context(), path, digest, n)
	if err != nil {
		out.Available = !errors.Is(err, ocr.ErrUnavailable)
		out.Error = err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.Page, out.Count = page, count
	if templates, err := tree.LoadTemplates(s.root); err == nil {
		read := ocr.Result{Pages: []ocr.Page{page}}
		for kind, found := range suggest.All(templates, read.Text(), s.dateOrder) {
			out.Suggestions[kind] = map[string]suggestion{}
			for key, m := range found {
				at := suggestion{Value: m.Value, Page: -1, Line: -1}
				_, line, ok := read.Locate(m.Start)
				_, endLine, endOK := read.Locate(m.End - 1)
				if ok && endOK && line == endLine {
					at.Page, at.Line = n, line
				}
				out.Suggestions[kind][key] = at
			}
		}
		if n == 0 {
			out.Types = s.rank(templates, digest, read.Text())
			out.Type, _ = classify.Best(out.Types, classify.DefaultThreshold)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ahead reads PDFs in an opened folder in advance, in the order given: the
// ones the reader is likely to pick next. It answers at once.
// rank compares a first page with the first pages of the Items filed under
// each Template, as far as their text has been read and kept. The PDF itself,
// when it is already filed, is left out: it would only find itself.
func (s server) rank(templates []tree.Template, digest, text string) []classify.Score {
	items, err := tree.LoadItems(s.root)
	if err != nil {
		return nil
	}
	known := map[string]bool{}
	for _, t := range templates {
		known[t.Type] = true
	}
	var samples []classify.Sample
	for _, item := range items {
		head := item.CurrentDigest()
		if head == "" {
			continue
		}
		if !known[item.Type] || head == digest {
			continue
		}
		if page, _, ok := s.store.Load(head, 0); ok {
			samples = append(samples, classify.Sample{Type: item.Type, Text: ocr.Result{Pages: []ocr.Page{page}}.Text()})
		}
	}
	return classify.Rank(samples, text)
}

func (s server) ahead(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Dir   string   `json:"dir"`
		Paths []string `json:"paths"`
	}
	if !decode(w, r, &request) {
		return
	}
	var paths []string
	for _, p := range request.Paths {
		if full, err := sourcePath(request.Dir, p); err == nil {
			paths = append(paths, full)
		}
	}
	// Digesting a large scan takes a moment; nobody waits for it.
	go func() {
		for _, path := range paths {
			if digest, err := tree.FileDigest(path); err == nil {
				s.reader.Ahead(path, digest)
			}
		}
	}()
	writeJSON(w, http.StatusOK, map[string]int{"queued": len(paths)})
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
		if rev.Ref() == query.Get("digest") && rev.Digest != "" {
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
	Replaces string            `json:"replaces"`
	NoPDF    bool              `json:"no_pdf"`
	Dir      string            `json:"dir"`
	Path     string            `json:"path"`
	Type     string            `json:"type"`
	Fields   map[string]string `json:"fields"`
	Notes    string            `json:"notes"`
	Tags     []string          `json:"tags"`
}

func (s server) importPDF(w http.ResponseWriter, r *http.Request) {
	var request importJSON
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	source := ""
	if !request.NoPDF {
		var ok bool
		source, ok = s.source(w, request.Dir, request.Path)
		if !ok {
			return
		}
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
		if request.Replaces != "" {
			if err := tree.ValidateSupersession(s.root, request.Replaces, request.Type, request.Fields); err != nil {
				answer(w, tree.Item{}, err)
				return
			}
		}
		req := tree.ImportRequest{
			Root: s.root, Source: source, Template: t, Fields: request.Fields, Notes: request.Notes, Tags: request.Tags, Now: s.now(),
		}
		var item tree.Item
		if request.NoPDF {
			item, err = tree.CreateWithoutPDF(req)
		} else {
			item, err = tree.Import(r.Context(), req)
		}
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if request.Replaces != "" {
			if _, err := tree.Supersede(s.root, request.Replaces, item.ID, s.now()); err != nil {
				writeJSON(w, http.StatusOK, map[string]any{"item": item, "warning": "New visa saved, but replacement failed: " + err.Error()})
				return
			}
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

// addRevision adds a loose PDF to a document, with its per_revision fields,
// and moves HEAD to it.
func (s server) addRevision(w http.ResponseWriter, r *http.Request) {
	var request struct {
		NoPDF  bool              `json:"no_pdf"`
		Dir    string            `json:"dir"`
		Path   string            `json:"path"`
		Item   string            `json:"item"`
		Fields map[string]string `json:"fields"`
		Notes  *string           `json:"notes"`
		Tags   *[]string         `json:"tags"`
		// RevisionTags are the new revision's own.
		RevisionTags []string `json:"revision_tags"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	source := ""
	if !request.NoPDF {
		var ok bool
		source, ok = s.source(w, request.Dir, request.Path)
		if !ok {
			return
		}
	}
	t, err := s.templateOfItem(request.Item)
	if err != nil {
		answer(w, tree.Item{}, err)
		return
	}
	metadata := tree.RevisionMetadata{Notes: request.Notes, Tags: request.Tags, RevisionTags: request.RevisionTags}
	var item tree.Item
	if request.NoPDF {
		item, err = tree.AddWithoutPDF(s.root, request.Item, t, request.Fields, metadata, s.now())
	} else {
		item, err = tree.AddRevisionWithMetadata(r.Context(), s.root, request.Item, source, t, request.Fields, metadata, s.now())
	}
	answer(w, item, err)
}

// templateOfItem is the Template of the Item with id.
func (s server) templateOfItem(id string) (tree.Template, error) {
	item, _, err := tree.FindItem(s.root, id)
	if err != nil {
		return tree.Template{}, err
	}
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		return tree.Template{}, err
	}
	for _, t := range templates {
		if t.Type == item.Type {
			return t, nil
		}
	}
	return tree.Template{}, errors.New("no Template for type " + item.Type)
}

func (s server) setHead(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item   string `json:"item"`
		Digest string `json:"digest"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := tree.SetHead(s.root, request.Item, request.Digest, s.now())
	answer(w, item, err)
}

func (s server) deleteRevision(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item   string `json:"item"`
		Digest string `json:"digest"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, to, err := tree.TrashRevision(s.root, request.Item, request.Digest, s.now())
	if err != nil {
		answer(w, tree.Item{}, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item, "trash": to})
}

func (s server) setFields(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item   string            `json:"item"`
		Digest string            `json:"digest"`
		Fields map[string]string `json:"fields"`
	}
	if !decode(w, r, &request) {
		return
	}
	t, err := s.templateOfItem(request.Item)
	if item, _, loadErr := tree.FindItem(s.root, request.Item); loadErr == nil {
		if rev, ok := item.Revision(request.Digest); ok && rev.Type != "" {
			templates, loadErr := tree.LoadTemplates(s.root)
			if loadErr != nil {
				err = loadErr
			} else {
				found := false
				for _, candidate := range templates {
					if candidate.Type == rev.Type {
						t = candidate
						found = true
						break
					}
				}
				if !found {
					err = fmt.Errorf("no Template for historical type %s", rev.Type)
				}
			}
		}
	}
	if err != nil {
		answer(w, tree.Item{}, err)
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := tree.SetFields(s.root, request.Item, request.Digest, t, request.Fields, s.now())
	answer(w, item, err)
}

func (s server) changeType(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item      string                       `json:"item"`
		Type      string                       `json:"type"`
		Fields    map[string]string            `json:"fields"`
		Revisions map[string]map[string]string `json:"revisions"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		answer(w, tree.Item{}, err)
		return
	}
	for _, target := range templates {
		if target.Type == request.Type {
			item, err := tree.ChangeType(s.root, request.Item, target, request.Fields, request.Revisions, s.now())
			answer(w, item, err)
			return
		}
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no Template for type " + request.Type})
}

func (s server) setNotes(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item  string `json:"item"`
		Notes string `json:"notes"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := tree.SetNotes(s.root, request.Item, request.Notes, s.now())
	answer(w, item, err)
}

func (s server) setTags(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item string   `json:"item"`
		Tags []string `json:"tags"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := tree.SetTags(s.root, request.Item, request.Tags, s.now())
	answer(w, item, err)
}

func (s server) setRevisionTags(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item   string   `json:"item"`
		Digest string   `json:"digest"`
		Tags   []string `json:"tags"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := tree.SetRevisionTags(s.root, request.Item, request.Digest, request.Tags, s.now())
	answer(w, item, err)
}

func (s server) setFrequent(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item     string `json:"item"`
		Frequent bool   `json:"frequent"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := tree.SetFrequent(s.root, request.Item, request.Frequent, s.now())
	answer(w, item, err)
}

func (s server) setRetired(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item    string `json:"item"`
		Retired bool   `json:"retired"`
		Reason  string `json:"reason"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	item, err := tree.SetRetired(s.root, request.Item, request.Retired, request.Reason, s.now())
	answer(w, item, err)
}

// historyLog is what was done to the tree's Items, newest first: every Item's,
// or with ?item= one Item's.
func (s server) historyLog(w http.ResponseWriter, r *http.Request) {
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	items, err := tree.LoadItems(s.root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if id := r.URL.Query().Get("item"); id != "" {
		items = slices.DeleteFunc(items, func(item tree.Item) bool { return item.ID != id })
	}
	entries := tree.Log(items)
	if entries == nil {
		entries = []tree.LogEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// deleteItem moves an Item's folder into the tree's trash. The page asks
// first; nothing is erased.
func (s server) deleteItem(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item string `json:"item"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	to, err := tree.Trash(s.root, request.Item, s.now())
	if err != nil {
		answer(w, tree.Item{}, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"trash": to})
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

func (s server) attachPDF(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item   string
		Digest string
		Dir    string
		Path   string
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	source, ok := s.source(w, request.Dir, request.Path)
	if !ok {
		return
	}
	item, err := tree.AttachPDF(r.Context(), s.root, request.Item, request.Digest, source, s.now())
	answer(w, item, err)
}

func (s server) supersession(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Item        string `json:"item"`
		Replacement string `json:"replacement"`
	}
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	var item tree.Item
	var err error
	if request.Replacement == "" {
		item, err = tree.UndoSupersession(s.root, request.Item, s.now())
	} else {
		item, err = tree.Supersede(s.root, request.Item, request.Replacement, s.now())
	}
	answer(w, item, err)
}

func (s server) supersessionCandidates(w http.ResponseWriter, r *http.Request) {
	items, err := tree.LoadItems(s.root)
	if err != nil {
		answer(w, tree.Item{}, err)
		return
	}
	result := []tree.Item{}
	fields := map[string]string{"owner": r.URL.Query().Get("owner"), "country": r.URL.Query().Get("country")}
	for _, it := range items {
		if it.ID != r.URL.Query().Get("exclude") && tree.ValidateSupersession(s.root, it.ID, "visa", fields) == nil {
			result = append(result, it)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// issuers returns suggestions only for the requested type and country.
func (s server) issuers(w http.ResponseWriter, r *http.Request) {
	items, err := tree.LoadItems(s.root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tree.FieldValues(items, r.URL.Query().Get("type"), r.URL.Query().Get("country"), "issuer"))
}
