package capture

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestRoutePayloadWrapsAndScrollsWithoutLosingContent(t *testing.T) {
	m := loadedRoute(t, routeTestRoot(t, "alpha", "beta"))
	entry, _ := m.selectedCapture()
	for i := range m.entries {
		if m.entries[i].path == entry.path {
			m.entries[i].index.Payload = map[string]any{"nested": map[string]any{"message": strings.Repeat("中英文", 40)}, "items": []any{1, true, "last-value"}}
		}
	}
	lines := m.payloadLines(20)
	for _, line := range lines {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("payload overflows: %q", line)
		}
	}
	if !strings.Contains(strings.Join(lines, ""), "last-value") {
		t.Fatal("payload data was dropped")
	}
	m.fields.Set(routePayloadField)
	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(routeModel)
	if m.payloadOffset == 0 {
		t.Fatal("long payload did not scroll")
	}
	if !m.CapturesShellKey("esc") {
		t.Fatal("payload escape would exit Capture")
	}
	updated, _ = m.updateCaptures("j")
	m = updated.(routeModel)
	if m.payloadOffset != 0 {
		t.Fatal("new Capture inherited payload scroll")
	}
}

func TestArchiveWrappedPathsPreserveTailAndListHitTesting(t *testing.T) {
	m := loadedArchive(t, routeTestRoot(t, "alpha", "beta"), "/a/long/archive/folder/whose/final/component/is/kept", "/a/long/reject/folder/whose/final/component/is/kept")
	m.archived = append([]captureEntry(nil), m.entries...)
	m.rejected = append([]captureEntry(nil), m.entries...)
	m.rebuildItems()
	m.resizeComponents()
	width := m.columnWidths()[2] - 4
	for _, destination := range []archiveDestination{destinationArchive, destinationReject} {
		header := ansi.Strip(m.destinationHeader(destination, width))
		parts := strings.Split(header, "\n")
		for i := range parts {
			parts[i] = strings.TrimRight(parts[i], " ")
		}
		if strings.Join(parts, "") != m.folderFor(destination) {
			t.Fatalf("path was lost: %q", header)
		}
		rows := lipgloss.Height(header)
		y := 1 + rows + 2 // second Capture, after the two-row first Capture
		if destination == destinationReject {
			y += m.destinationHeight()
		}
		x := m.columnWidths()[0] + m.columnWidths()[1] + 2*columnGutter + 2
		updated, _ := m.updateMouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m = updated.(archiveModel)
		selected, ok := m.selectedIn(destination)
		if !ok || selected.name != "beta" {
			t.Fatalf("wrapped header shifted click to %q", selected.name)
		}
	}
	m.fields.Set(archiveCapturesField)
	m.entries[0].path = "/a/long/capture/folder/with/a/final-directory-name"
	m.rebuildItems()
	detail := strings.ReplaceAll(ansi.Strip(m.detail(24)), "\n", "")
	if !strings.Contains(detail, m.entries[0].path) {
		t.Fatalf("Capture path lost its tail: %s", detail)
	}
}
