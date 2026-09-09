package capture

import (
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/apps/capture/mockcapture"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMockCaptureRootIsUsable keeps the generated fixture honest: every mock
// Capture must validate, and the rejected neighbours must stay rejected.
func TestMockCaptureRootIsUsable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "capture")
	if err := mockcapture.Write(root); err != nil {
		t.Fatal(err)
	}
	loaded, ok := loadCaptures(root, "index.json")().(capturesLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("loading the mock Capture root failed: %#v", loaded)
	}
	if len(loaded.captures) != 8 {
		names := make([]string, 0, len(loaded.captures))
		for _, capture := range loaded.captures {
			names = append(names, capture.name)
		}
		t.Fatalf("mock root has %d valid captures %v, want 8", len(loaded.captures), names)
	}
	for _, capture := range loaded.captures {
		if capture.index.Schema != "v1" || capture.index.ID == "" {
			t.Fatalf("capture %q did not decode: %#v", capture.name, capture.index)
		}
	}
	attachments := 0
	for _, capture := range loaded.captures {
		attachments += len(capture.files) - 1
	}
	if attachments != 5 {
		t.Fatalf("mock captures expose %d attachment files, want 5", attachments)
	}

	m := newRouteModel(root, "index.json", nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	updated, _ = updated.(routeModel).Update(loaded)
	if got := len(updated.(routeModel).entries); got != 8 {
		t.Fatalf("Route shows %d captures, want 8", got)
	}
}
