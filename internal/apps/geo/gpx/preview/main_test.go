package preview

import (
	"net/http"
	"os"
	"testing"

	"dgs-toolbox/internal/apps/geo/gpx"
)

// TestPreview serves the GPX page over a chosen folder for looking at it by
// hand. It runs only when DGS_PREVIEW is set, so the suite stays quiet.
func TestPreview(t *testing.T) {
	if os.Getenv("DGS_PREVIEW") == "" {
		t.Skip("set DGS_PREVIEW to serve the page")
	}
	root := os.Getenv("DGS_PREVIEW_ROOT")
	if root == "" {
		root, _ = os.UserHomeDir()
	}
	_ = http.ListenAndServe("127.0.0.1:8792", gpx.Handler(gpx.Settings{Root: root}))
}
