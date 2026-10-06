package gpx

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"dgs-toolbox/internal/webui"
)

const uiScale = 0.9

// Every stylesheet used by GPX, including the shared library and map controls,
// receives the same density. Other apps keep the shared library's normal size.
func compactStyles(next http.Handler, assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != "GET" && r.Method != "HEAD") || !strings.HasSuffix(r.URL.Path, ".css") {
			next.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		files := assets
		if strings.HasPrefix(name, "ui/") {
			files = webui.Assets()
			name = strings.TrimPrefix(name, "ui/")
		}
		source, err := fs.ReadFile(files, name)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		http.ServeContent(w, r, name, time.Time{}, strings.NewReader(webui.ScaleStylesheet(string(source), uiScale)))
	})
}
