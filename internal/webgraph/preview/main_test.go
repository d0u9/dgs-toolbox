package preview

import (
	"net/http"
	"os"
	"testing"

	"dgs-toolbox/internal/webgraph"
)

// TestPreview serves a small graph for looking at the page by hand. It runs
// only when DGS_PREVIEW is set, so the suite stays quiet.
func TestPreview(t *testing.T) {
	if os.Getenv("DGS_PREVIEW") == "" {
		t.Skip("set DGS_PREVIEW to serve the page")
	}
	load := func() (webgraph.Graph, error) {
		return webgraph.Graph{
			Title:  "Preview",
			Groups: []webgraph.Group{{ID: "g1", Label: "machine", Detail: "host"}},
			Nodes: []webgraph.Node{
				{ID: "a", Label: "alpha", Kind: "file", Group: "g1"},
				{ID: "b", Label: "beta", Kind: "process", Group: "g1"},
				{ID: "c", Label: "gamma", Kind: "file"},
			},
			Edges: []webgraph.Edge{{From: "a", To: "b", Label: "reads"}, {From: "b", To: "c"}},
		}, nil
	}
	_ = http.ListenAndServe("127.0.0.1:8791", webgraph.Handler(load))
}
