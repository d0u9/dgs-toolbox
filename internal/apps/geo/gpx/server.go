package gpx

import (
	"embed"
	"errors"
	"io/fs"
	"net"
	"net/http"
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
	// focus receives the page's focused track; nil uses the process's board.
	focus *focusBoard
}

// SettingsFrom reads the server settings out of the configuration.
func SettingsFrom(global config.Config) Settings {
	return Settings{Addr: global.GeoGPXAddr(), Root: global.GeoGPXRoot(), Tiles: global.Geo.GPX.Tiles}
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
	mux.Handle("GET /", http.FileServerFS(static))
	return mux
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
