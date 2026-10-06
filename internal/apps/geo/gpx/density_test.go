package gpx

import (
	"dgs-toolbox/internal/webui"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGPXCompactStyles(t *testing.T) {
	h := Handler(Settings{})
	for _, item := range []struct{ path, want string }{
		{"/app.css", "font-size: 10.8px"}, {"/ui/controls.css", ""}, {"/vendor/maplibre-gl.css", ""},
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", item.path, nil))
		if rr.Code != 200 || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/css") {
			t.Fatalf("%s: %d", item.path, rr.Code)
		}
		if item.want != "" && !strings.Contains(rr.Body.String(), item.want) {
			t.Fatal(item.path, "not compact")
		}
		head := httptest.NewRecorder()
		h.ServeHTTP(head, httptest.NewRequest("HEAD", item.path, nil))
		if head.Code != 200 || head.Body.Len() != 0 {
			t.Fatal("HEAD", head.Code)
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/ui/tokens.css", nil))
	original, err := io.ReadAll(mustOpenStyles(t, "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	if rr.Body.String() != webui.ScaleStylesheet(string(original), .9) {
		t.Fatal("shared tokens not scaled")
	}
	normal := httptest.NewRecorder()
	webui.Handler().ServeHTTP(normal, httptest.NewRequest(http.MethodGet, "/ui/tokens.css", nil))
	if normal.Body.String() != string(original) {
		t.Fatal("other apps changed")
	}
	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest("GET", "/missing.css", nil))
	if missing.Code != 404 {
		t.Fatal(missing.Code)
	}
}
func mustOpenStyles(t *testing.T, name string) io.ReadCloser {
	t.Helper()
	f, err := webui.Assets().Open(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
