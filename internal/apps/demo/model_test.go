package demo

import (
	"strings"
	"testing"

	"dgs-toolbox/internal/tui/form"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestGalleryShowsReusableComponents(t *testing.T) {
	m := newModel().(model)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m = updated.(model)
	view := m.View()
	for _, want := range []string{"COMPONENT DEMO", "Path · opens File Explorer", "Selection controls", "Text input", "Actions · Local and Page Navigation", "── ◆ ──", "System", "[ ] Show hidden", "(●) Comfortable", "Trigger demo action", "← Prev", "Next →", "Processing"} {
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
	if !m.confirming || !strings.Contains(m.View(), "RUN DEMO ACTION?") {
		t.Fatal("button did not open confirmation dialog")
	}
}

func TestAltDownNavigationFollowsDataFieldOrder(t *testing.T) {
	m := newModel().(model)
	want := []string{optionID, textID, buttonID}
	for _, wantID := range want {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}, Alt: true})
		m = updated.(model)
		if got := m.controls.FocusedID(); got != wantID {
			t.Fatalf("focused %q, want %q", got, wantID)
		}
	}
}

func TestMouseClickSwitchesGalleryDataFieldFocus(t *testing.T) {
	m := newModel().(model)
	m.width, m.height = 110, 40
	width := min(100, max(24, m.width-4))
	selectionY := 4 + lipgloss.Height(m.pathFieldView(width)) + 1
	updated, _ := m.Update(tea.MouseMsg{
		X:      (m.width-width)/2 + 2,
		Y:      selectionY + 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	m = updated.(model)
	if got := m.fields.Current(); got != "selection" {
		t.Fatalf("click focused field %q, want selection", got)
	}
	if got := m.controls.FocusedID(); got != optionID {
		t.Fatalf("click focused control %q, want %q", got, optionID)
	}
}

func TestSharedNextScreenShortcut(t *testing.T) {
	m := newModel().(model)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(form.PrimaryActionKey)})
	m = updated.(model)
	if !m.confirming {
		t.Fatal("shared next-screen shortcut did not open confirmation dialog")
	}
}
