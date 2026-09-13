package gpx

import (
	"embed"
	"errors"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"dgs-toolbox/internal/config"
)

//go:embed web
var webFiles embed.FS

// Settings is what the server needs from the configuration.
type Settings struct {
	Addr  string
	Root  string
	Tiles []config.GeoGPXTile
	// Reveal shows a path in the file manager; nil uses desktop.Reveal.
	Reveal func(path string) error
	// Router fills a stretch along the road; nil uses the public OSRM services.
	Router Router
	// AmapKey, when set, offers Amap's routing beside OSRM's. Amap, when set,
	// routes Amap's ways instead of the service, as tests do.
	AmapKey string
	Amap    Router
	// DEM is the elevation tiles contour lines are drawn from.
	DEM config.GeoGPXDEM
	// tilesErr is why the configured base maps could not be read.
	tilesErr error
	// focus receives the page's focused track; nil uses the process's board.
	focus *focusBoard
}

// SettingsFrom reads the server settings out of the configuration. A tiles
// file that cannot be read keeps the server from starting, saying why.
func SettingsFrom(global config.Config) Settings {
	tiles, err := global.GeoGPXTiles()
	return Settings{Addr: global.GeoGPXAddr(), Root: global.GeoGPXRoot(), Tiles: tiles, AmapKey: global.Geo.GPX.AmapKey, DEM: global.GeoGPXDEMSource(), tilesErr: err}
}

// Handler serves the GPX web page and its API.
func Handler(settings Settings) http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	board := settings.focus
	if board == nil {
		board = focused
	}
	api := api{settings: settings, board: board}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/config", api.config)
	mux.HandleFunc("GET /api/dir", api.dir)
	mux.HandleFunc("GET /api/track", api.track)
	mux.HandleFunc("POST /api/reveal", api.reveal)
	mux.HandleFunc("POST /api/focus", api.focus)
	mux.HandleFunc("PUT /api/clean", api.saveClean)
	mux.HandleFunc("PUT /api/segments", api.saveSegments)
	mux.HandleFunc("POST /api/segments/write", api.writeSegments)
	mux.HandleFunc("POST /api/fill/route", api.routeFill)
	mux.HandleFunc("POST /api/fill", api.saveFill)
	mux.HandleFunc("DELETE /api/fill", api.removeFill)
	mux.HandleFunc("DELETE /api/added", api.removeAdded)
	mux.HandleFunc("POST /api/save-as", api.saveAs)
	mux.HandleFunc("DELETE /api/sidecar", api.discardSidecar)
	mux.HandleFunc("POST /api/draft", api.newDraft)
	mux.HandleFunc("POST /api/route/leg", api.routeLeg)
	mux.HandleFunc("POST /api/route/save", api.saveRoute)
	mux.Handle("GET /", http.FileServerFS(static))
	return sameOrigin(mux)
}

// sameOrigin refuses a request that changes something unless the page itself
// sent it. Another site open in the same browser can post to this server — it
// listens on a known local port — but only a text/plain or form body, and with
// its own Origin: requiring JSON forces a preflight the server never grants.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mediaType != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, errors.New("send JSON"))
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
					writeError(w, http.StatusForbidden, errors.New("requests from another site are refused"))
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

var (
	serverOnce sync.Once
	serverURL  string
	serverErr  error
)

// Serve starts the web server once per process and returns its URL. Reopening
// the command reuses the running server; it stops when the toolbox exits.
func Serve(settings Settings) (string, error) {
	serverOnce.Do(func() {
		if settings.tilesErr != nil {
			serverErr = settings.tilesErr
			return
		}
		listener, err := net.Listen("tcp", settings.Addr)
		if err != nil {
			serverErr = err
			return
		}
		serverURL = pageURL(listener.Addr().(*net.TCPAddr))
		go func() {
			err := http.Serve(listener, Handler(settings))
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErr = err
			}
		}()
	})
	return serverURL, serverErr
}

// pageURL is the address to open the page at. A server listening on every
// interface is shown at this machine's LAN address, which other hosts can use.
func pageURL(addr *net.TCPAddr) string {
	ip := addr.IP
	if ip.IsUnspecified() {
		ip = lanIP()
	}
	return "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(addr.Port)) + "/"
}

func lanIP() net.IP {
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if network, ok := addr.(*net.IPNet); ok && !network.IP.IsLoopback() && network.IP.To4() != nil && !network.IP.IsLinkLocalUnicast() {
				return network.IP
			}
		}
	}
	return net.IPv4(127, 0, 0, 1)
}
