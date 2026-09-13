package gpx

import (
	"embed"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"sync"
)

//go:embed web
var webFiles embed.FS

// Handler serves the GPX web page and its API.
func Handler() http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.Handle("GET /", http.FileServerFS(static))
	return mux
}

var (
	serverOnce sync.Once
	serverURL  string
	serverErr  error
)

// Serve starts the web server on addr once per process and returns its URL.
// Reopening the command reuses the running server; it stops when the toolbox
// exits.
func Serve(addr string) (string, error) {
	serverOnce.Do(func() {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			serverErr = err
			return
		}
		serverURL = pageURL(listener.Addr().(*net.TCPAddr))
		go func() {
			err := http.Serve(listener, Handler())
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
