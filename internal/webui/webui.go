// Package webui holds what every page dgs serves has in common: the design
// tokens of docs/web.md, the page frame, and the controls. It is assets and
// nothing else — no app knowledge, no API — so any command that serves a
// page mounts the same files.
//
// The files are embedded, so the one dgs binary carries them.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var assetFiles embed.FS

// Prefix is the path every shared file is served under. A page links
// "/ui/tokens.css" and the rest by the same name.
const Prefix = "/ui/"

// Assets is the shared files, rooted so a name is "tokens.css" rather than
// "assets/tokens.css".
func Assets() fs.FS {
	sub, err := fs.Sub(assetFiles, "assets")
	if err != nil {
		// The directory is embedded above, so this cannot fail at runtime.
		panic(err)
	}
	return sub
}

// Handler serves the shared files under Prefix.
func Handler() http.Handler {
	return http.StripPrefix(Prefix, http.FileServerFS(Assets()))
}

// Mount registers the shared files on a page's own mux.
func Mount(mux *http.ServeMux) {
	mux.Handle("GET "+Prefix, Handler())
}
