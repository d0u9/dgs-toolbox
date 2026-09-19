package webgraph

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testGraph() Graph {
	return Graph{
		Title:  "two machines",
		Groups: []Group{{ID: "srv", Label: "srv", Detail: "a server"}},
		Nodes: []Node{
			{ID: "ss-srv", Label: "ss-srv", Group: "srv", Kind: "server", Detail: "shadowsocks / server"},
			{ID: "laptop-sfo", Label: "laptop-sfo", Kind: "client"},
		},
		Edges:  []Edge{{From: "laptop-sfo", To: "ss-srv", Label: "sfo", Kind: "entry"}},
		Legend: map[string]string{"server": "listens", "client": "dials"},
	}
}

// TestHandler_ServesTheGraphAndThePage covers the two things a caller gets:
// its own graph as JSON, and a page to draw it with. The page is embedded,
// so a binary copied to another machine carries it.
func TestHandler_ServesTheGraphAndThePage(t *testing.T) {
	h := Handler(func() (Graph, error) { return testGraph(), nil })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got Graph
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v\n%s", err, rec.Body)
	}
	if got.Title != "two machines" || len(got.Nodes) != 2 || len(got.Edges) != 1 || len(got.Groups) != 1 {
		t.Fatalf("graph = %+v, want the one the loader returned", got)
	}

	for _, path := range []string{"/", "/app.js", "/app.css", "/vendor/cytoscape.min.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want the embedded bundle to hold it", path, rec.Code)
		}
	}
}

// TestHandler_LoadIsCalledPerRequest covers a page reloaded after an edit:
// the graph is built when it is asked for, not once at startup, so what the
// reader sees is what is on disk now.
func TestHandler_LoadIsCalledPerRequest(t *testing.T) {
	calls := 0
	h := Handler(func() (Graph, error) {
		calls++
		return testGraph(), nil
	})
	for i := 0; i < 3; i++ {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	}
	if calls != 3 {
		t.Fatalf("load called %d times, want one per request", calls)
	}
}

// TestHandler_LoadFailureIsJSON covers an inventory that will not load. The
// page shows the reason rather than an empty picture, which it can only do
// if the reason arrives in the shape it parses.
func TestHandler_LoadFailureIsJSON(t *testing.T) {
	h := Handler(func() (Graph, error) { return Graph{}, errors.New("nodes/bad.yaml: not YAML") })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v\n%s", err, rec.Body)
	}
	if !strings.Contains(body["error"], "nodes/bad.yaml") {
		t.Fatalf("body = %v, want the loader's own reason", body)
	}
}

// TestServer_StartsOnce covers reopening the command: the second call gets
// the running server's URL rather than failing to bind a port twice.
func TestServer_StartsOnce(t *testing.T) {
	var s Server
	load := func() (Graph, error) { return testGraph(), nil }

	first, err := s.Start("127.0.0.1:0", load)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	second, err := s.Start("127.0.0.1:0", load)
	if err != nil {
		t.Fatalf("Start again: %v", err)
	}
	if first != second || first == "" {
		t.Fatalf("URLs = %q and %q, want one server reused", first, second)
	}
	if !strings.HasPrefix(first, "http://127.0.0.1:") {
		t.Fatalf("URL = %q, want loopback", first)
	}
}
