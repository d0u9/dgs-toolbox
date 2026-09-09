package capture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func routeTestRoot(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		index := []byte(`{"schema":"v1","source":{"app":"Shortcut","workflow":"note","device":{"os":"iOS","systemVersion":"26.4.2","name":"Phone"}},"coordinates":{"altitude":0,"longitude":0,"latitude":0},"id":"` + name + `","payload":{},"createdAt":"2026-09-09T16:34:35.556+10:00"}`)
		if err := os.WriteFile(filepath.Join(path, "index.json"), index, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func loadedRoute(t *testing.T, root string, destinations []config.CaptureDestination) routeModel {
	t.Helper()
	m := newRouteModel(root, "index.json", destinations)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(routeModel)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	return updated.(routeModel)
}

func TestRouteWorkspaceShowsCapturesAndDestinations(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	m := loadedRoute(t, root, []config.CaptureDestination{{Name: "Notes", Path: "/tmp/notes"}})

	view := ansi.Strip(m.View())
	for _, want := range []string{"CAPTURES", "CAPTURE", "DESTINATIONS", "ROUTE PLAN", "alpha", "Notes"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Route workspace is missing %q:\n%s", want, view)
		}
	}
	if got := strings.Count(view, "\n") + 1; got != m.height {
		t.Fatalf("Route workspace height = %d, want %d", got, m.height)
	}
}

func TestRouteAssignsSelectedCaptureToDestination(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	destination := config.CaptureDestination{Name: "Notes", Path: "/tmp/notes"}
	m := loadedRoute(t, root, []config.CaptureDestination{destination})

	updated, _ := m.updateCaptures("enter")
	m = updated.(routeModel)
	if got := m.fields.Current(); got != routeDestinationsField {
		t.Fatalf("focus after Enter = %q, want %q", got, routeDestinationsField)
	}
	updated, _ = m.updateDestinations("enter")
	m = updated.(routeModel)

	if got := m.assignments[filepath.Join(root, "alpha")]; got != destination.Path {
		t.Fatalf("alpha assignment = %q, want %q", got, destination.Path)
	}
	if got := m.fields.Current(); got != routeCapturesField {
		t.Fatalf("focus after assigning = %q, want %q", got, routeCapturesField)
	}
	if got := m.captures.Cursor(); got != 1 {
		t.Fatalf("cursor after assigning = %d, want 1", got)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "● alpha") || !strings.Contains(view, "→ Notes") {
		t.Fatalf("routed capture is not marked in the list:\n%s", view)
	}

	m.captures.First()
	updated, _ = m.updateCaptures("u")
	m = updated.(routeModel)
	if _, ok := m.assignments[filepath.Join(root, "alpha")]; ok {
		t.Fatal("u did not clear the assignment")
	}
}

func TestCaptureSessionExposesScanAndRouteTabs(t *testing.T) {
	s := newSession()
	if want := []tui.Tab{{Label: "Scan", Active: true}, {Label: "Route"}}; s.Tabs()[0] != want[0] || s.Tabs()[1] != want[1] {
		t.Fatalf("tabs = %#v", s.Tabs())
	}
	updated, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	s = updated.(session)
	if s.active != 1 || !s.Tabs()[1].Active {
		t.Fatalf("] did not activate Route: %#v", s.Tabs())
	}
	if got := s.Status().Left; got != "ROUTE" {
		t.Fatalf("Route status left = %q", got)
	}
	updated, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	s = updated.(session)
	if s.active != 0 || !s.Tabs()[0].Active {
		t.Fatalf("[ did not return to Scan: %#v", s.Tabs())
	}
}

func TestCaptureSessionActivatesClickedTab(t *testing.T) {
	s := newSession()
	updated, _ := s.Update(tui.TabSelectedMsg{Index: 1})
	s = updated.(session)
	if s.active != 1 || !s.Tabs()[1].Active {
		t.Fatalf("clicking Route did not activate it: %#v", s.Tabs())
	}
	updated, _ = s.Update(tui.TabSelectedMsg{Index: 0})
	s = updated.(session)
	if s.active != 0 || !s.Tabs()[0].Active {
		t.Fatalf("clicking Scan did not activate it: %#v", s.Tabs())
	}
}

func TestCaptureSessionSharesOneCaptureLoad(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	s := newSessionWithSettings(root, "index.json", nil)
	updated, _ := s.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	s = updated.(session)
	updated, _ = s.Update(loadCaptures(root, "index.json")())
	s = updated.(session)

	if len(s.scan.entries) != 1 || len(s.route.entries) != 1 {
		t.Fatalf("scan entries = %d, route entries = %d, want 1 each", len(s.scan.entries), len(s.route.entries))
	}
}
