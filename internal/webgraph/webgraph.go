// Package webgraph draws a node-link graph in a local web page: a small JSON
// API and an embedded static bundle, force-directed and draggable, with
// click events a caller acts on.
//
// It knows nothing about what it is drawing. A caller supplies a function
// producing its own graph as this package's Node/Group/Edge shape, and this
// package supplies the page. The vocabulary of whatever domain is being
// drawn — nodes and instances, stops and legs, packages and imports — stays
// in the caller, so the next thing wanting a topology or dependency picture
// writes a mapping function rather than choosing a library again.
package webgraph

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sync"

	"dgs-toolbox/internal/webui"
)

//go:embed web
var webFiles embed.FS

// Group is a container drawn around the nodes that belong to it — a machine
// holding processes, a package holding files. A node naming no group is
// drawn on its own.
type Group struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Detail is a line under the label.
	Detail string `json:"detail,omitempty"`
	// Parent is the group this one sits inside, for a picture with more
	// than one level of containment — processes inside a machine, files
	// inside a package inside a module. Empty for a top-level group.
	Parent string `json:"parent,omitempty"`
	// Kind styles the box the way Node.Kind styles a shape.
	Kind string `json:"kind,omitempty"`
	// Collapse marks a box that stands for its contents when the picture is
	// drawn small: zoomed out, the page hides what is inside it, draws the
	// box itself as one shape, and replaces the lines in and out of its
	// contents with one line per pair of boxes. Zoomed in, the box opens
	// again. It is the caller saying which level of its own hierarchy is
	// the one worth seeing first — a machine, rather than every port on it.
	Collapse bool `json:"collapse,omitempty"`
}

// Node is one shape of the graph.
type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Group is the container this node sits in, or empty for one drawn on
	// its own.
	Group string `json:"group,omitempty"`
	// Kind is the caller's own classification, used to style the shape:
	// the page gives each distinct kind its own colour and puts them in a
	// legend. It is a word from the caller's vocabulary, not this package's.
	Kind string `json:"kind,omitempty"`
	// Detail is a line under the label, and Tooltip is what hovering shows.
	Detail  string `json:"detail,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
}

// Edge is one connection. It is drawn with an arrow from From to To.
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
	// Kind styles the line the way Node.Kind styles a shape.
	Kind string `json:"kind,omitempty"`
}

// Graph is a whole picture.
type Graph struct {
	// Title names what is being drawn, shown in the page's header.
	Title  string  `json:"title"`
	Groups []Group `json:"groups"`
	Nodes  []Node  `json:"nodes"`
	Edges  []Edge  `json:"edges"`
	// Legend maps a Kind to what it means, for the key beside the graph.
	// A kind with no entry is drawn and left out of the key.
	Legend map[string]string `json:"legend,omitempty"`
}

// Load produces the graph to draw. It is called on every request rather than
// once, so a page reloaded after an edit shows the edit.
type Load func() (Graph, error)

// Handler serves the page and its API.
func Handler(load Load) http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/graph", func(w http.ResponseWriter, _ *http.Request) {
		g, err := load()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(g)
	})
	webui.Mount(mux)
	mux.Handle("GET /", http.FileServerFS(static))
	return mux
}

// Server is one running page. Start returns the same server for the same
// address, so reopening the command reuses it rather than failing to bind.
type Server struct {
	once sync.Once
	url  string
	err  error
}

// Start listens on addr and serves load's graph, once per Server. It returns
// the URL to open. The server runs until the process exits.
func (s *Server) Start(addr string, load Load) (string, error) {
	s.once.Do(func() {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			s.err = err
			return
		}
		s.url = pageURL(listener.Addr().(*net.TCPAddr))
		go func() {
			err := http.Serve(listener, Handler(load))
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.err = err
			}
		}()
	})
	return s.url, s.err
}

// pageURL is the address to open. A server listening on every interface is
// shown at loopback, which is the one address the machine that started it
// can always reach.
func pageURL(addr *net.TCPAddr) string {
	ip := addr.IP
	if ip == nil || ip.IsUnspecified() {
		return fmt.Sprintf("http://127.0.0.1:%d", addr.Port)
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(ip.String(), fmt.Sprint(addr.Port)))
}
