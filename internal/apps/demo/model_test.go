package demo

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestGalleryShowsReusableComponents(t *testing.T) {
	m := newModel().(model)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m = updated.(model)
	view := m.View()
	for _, want := range []string{"COMPONENT DEMO", "Path · opens File Explorer", "Selection controls", "Text input", "Action", "System", "[ ] Show hidden", "(●) Comfortable", "Trigger demo action"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}
	if lipgloss.Width(view) != 110 || lipgloss.Height(view) != 30 {
		t.Fatalf("view size = %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
}

func TestGalleryControlsAreInteractive(t *testing.T) {
	m := newModel().(model)
	m.controls.SetFocusID(checkboxID)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(model)
	if !strings.Contains(m.View(), "[x] Show hidden") {
		t.Fatal("checkbox did not toggle")
	}
	m.controls.SetFocusID(buttonID)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !strings.Contains(m.View(), "Demo action triggered") {
		t.Fatal("button did not report activation")
	}
}

func TestDownNavigationFollowsVisualOrder(t *testing.T) {
	m := newModel().(model)
	want := []string{optionID, checkboxID, radioID, textID, buttonID, pathID}
	for _, wantID := range want {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
		if got := m.controls.FocusedID(); got != wantID {
			t.Fatalf("focused %q, want %q", got, wantID)
		}
	}
}
