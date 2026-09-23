// Package web serves the two pages dgs box works through: intake, where new
// scans are described, and browse, where a Box is looked through and
// corrected. A terminal cannot show a 200 dpi receipt legibly enough to read a
// date and a total off it, so the pages own everything that involves looking
// at a scan; the TUI stays the entry point.
//
// The pages talk to a Source, which is whatever holds the scans: Engine for a
// real Box, and Sample for someone who has not said where theirs is. The pages
// say which they are looking at, because describing scans that do not exist is
// worse than an empty page.
package web

import (
	"embed"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"sync"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/webui"
)

//go:embed web
var webFiles embed.FS

// Settings is what the server needs from the configuration.
type Settings struct {
	Addr  string
	Root  string
	Inbox string
	// Currency is what an amount typed with no code is read in; empty means
	// every amount has to name its own.
	Currency string
	// Zone is the IANA name a newly entered event date defaults to.
	Zone string
	// MarkerName is the file that proves Root is a Box. Empty uses the default.
	MarkerName string
	// StateFile is the name Import keeps its resumable state under, inside the
	// inbox. Empty uses the default.
	StateFile string
	// CacheDir is where the discardable index and the pictures live. It is
	// outside the Box on purpose.
	CacheDir string
	// PreviewKeepDays is how long an unused 1600 px preview is kept. Zero keeps
	// previews indefinitely.
	PreviewKeepDays int
	// TrashKeepDays is how long the trash is worth keeping. It is advice shown
	// on the browse page; nothing is ever removed for it. Zero shows no advice.
	TrashKeepDays int
	// Workers is how many files the inbox is read with at once. Zero or one
	// reads serially.
	Workers int
	// Source holds the scans. Nil uses Sample, which is what the pages show
	// until the engine is wired up.
	Source Source
}

// SettingsFrom reads the server settings out of the configuration.
func SettingsFrom(global config.Config) Settings {
	zone := ""
	if location, err := global.BoxZone(); err == nil {
		zone = location.String()
	}
	settings := Settings{
		Addr:            global.BoxWebAddr(),
		Root:            global.BoxRoot(),
		Inbox:           global.BoxInbox(),
		Currency:        global.BoxCurrency(),
		Zone:            zone,
		MarkerName:      global.BoxMarker(),
		StateFile:       global.BoxStateFile(),
		PreviewKeepDays: global.BoxPreviewKeepDays(),
		TrashKeepDays:   global.BoxTrashKeepDays(),
		Workers:         global.BoxWorkers(),
	}
	if cacheDir, err := global.BoxCacheDir(settings.Root); err == nil {
		settings.CacheDir = cacheDir
	}
	return settings
}

// Handler serves both pages and their API.
func Handler(settings Settings) http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	if settings.Source == nil {
		settings.Source = OpenSource(settings)
	}
	api := api{settings: settings}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/config", api.config)
	mux.HandleFunc("GET /api/types", api.types)
	mux.HandleFunc("GET /api/intake", api.intake)
	mux.HandleFunc("POST /api/intake/file", api.file)
	mux.HandleFunc("GET /api/scans", api.scans)
	mux.HandleFunc("PATCH /api/scan", api.patch)
	mux.HandleFunc("PATCH /api/scans", api.patchMany)
	mux.HandleFunc("POST /api/trash", api.trash)
	mux.HandleFunc("POST /api/trash/batch", api.trashMany)
	mux.HandleFunc("GET /api/trash", api.trashSummary)
	mux.HandleFunc("GET /api/incomplete", api.incomplete)
	mux.HandleFunc("GET /api/rejected", api.rejected)
	mux.HandleFunc("GET /api/rejected/file", api.rejectedFile)
	mux.HandleFunc("POST /api/rejected/restore", api.restore)
	mux.HandleFunc("GET /api/zones", api.zones)
	mux.HandleFunc("POST /api/unfile", api.unfile)
	mux.HandleFunc("POST /api/adopt", api.adopt)
	mux.HandleFunc("POST /api/verify", api.verify)
	mux.HandleFunc("GET /api/exceptions", api.exceptions)
	mux.HandleFunc("GET /api/image", api.image)
	// The page never asks for a file by path. It asks by the digest that
	// identifies a scan, and the server looks the path up: a request carrying a
	// path is a path to escape the Box with.
	webui.Mount(mux)
	mux.Handle("GET /intake", http.RedirectHandler("/intake/", http.StatusFound))
	mux.Handle("GET /intake/", http.StripPrefix("/intake/", pageHandler(static, "intake.html")))
	mux.Handle("GET /browse", http.RedirectHandler("/browse/", http.StatusFound))
	mux.Handle("GET /browse/", http.StripPrefix("/browse/", pageHandler(static, "browse.html")))
	mux.Handle("GET /", http.RedirectHandler("/browse/", http.StatusFound))
	return mux
}

// OpenSource is the real Box when there is one to open, and the stand-in when
// there is not.
//
// Falling back rather than failing is deliberate: a person who has not yet said
// where their Box is should see the pages, with the banner that says these
// scans are made up, rather than a server that refuses to start. What must
// never happen is the other way round — a real Box quietly presented as a
// sample, or a sample quietly presented as real — and Source.Sample() is what
// the pages ask to tell them apart.
func OpenSource(settings Settings) Source {
	engine, err := NewEngine(settings)
	if err != nil {
		return NewSample(settings.Currency)
	}
	return engine
}

// pageHandler serves one page's files, answering its own root with the named
// document so a reload of /intake/ lands on the page rather than on a listing.
func pageHandler(static fs.FS, document string) http.Handler {
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || r.URL.Path == "/" {
			r = r.Clone(r.Context())
			r.URL.Path = "/" + document
		}
		files.ServeHTTP(w, r)
	})
}

var (
	serverOnce sync.Once
	serverURL  string
	serverErr  error
)

// Serve starts the pages once per process and returns the URL to open. The
// address is always a loopback one: a Box holds identity and medical
// documents, so reaching it from another machine is a design with access
// control rather than a setting.
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
