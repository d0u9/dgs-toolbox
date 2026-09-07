package photo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestImportSetupScreen(t *testing.T) {
	model := newImportModel()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 90, Height: 28})
	model = updated.(tui.CommandModel)

	view := model.View()
	for _, text := range []string{
		"PHOTO IMPORT",
		"Source",
		"/Volumes/LEICA",
		"Destination",
		"~/Pictures/Photos",
		"Review import",
		"Review the plan before any files are copied.",
	} {
		if !strings.Contains(view, text) {
			t.Errorf("setup view does not contain %q:\n%s", text, view)
		}
	}
	if strings.Contains(view, "Archive") {
		t.Errorf("setup view still uses Archive:\n%s", view)
	}
	if got := lipgloss.Width(view); got != 90 {
		t.Errorf("view width = %d, want 90", got)
	}
	if got := lipgloss.Height(view); got != 28 {
		t.Errorf("view height = %d, want 28", got)
	}

	status := model.Status()
	if status.Left != "READY" || status.Center != "No source scanned" {
		t.Errorf("status = %#v", status)
	}
}

func TestDirectoryPickerIsAWorkspaceOverlay(t *testing.T) {
	model := newImportModel().(importModel)
	model.paths[sourceField] = t.TempDir()
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if cmd != nil {
		updated, _ = model.Update(cmd())
		model = updated.(importModel)
	}

	view := model.View()
	if got := lipgloss.Width(view); got != model.width {
		t.Fatalf("overlay width = %d, want %d", got, model.width)
	}
	if got := lipgloss.Height(view); got != model.height {
		t.Fatalf("overlay height = %d, want %d", got, model.height)
	}
	for _, want := range []string{"FILE EXPLORER · SOURCE", "╭", "╯"} {
		if !strings.Contains(view, want) {
			t.Fatalf("workspace does not contain %q in floating picker:\n%s", want, view)
		}
	}
}

func TestPathFieldOpensDirectoryPickerAndSelectsDirectory(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "selected")
	if err := os.Mkdir(want, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "not-selectable.txt"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}

	model := newImportModel().(importModel)
	model.paths[sourceField] = root
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if !model.picking || !model.CapturesShellKey("esc") {
		t.Fatal("source did not open the directory picker")
	}
	if cmd == nil {
		t.Fatal("picker did not request its initial directory listing")
	}
	updated, _ = model.Update(cmd())
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if model.picking {
		t.Fatal("Enter did not close the picker after choosing a directory")
	}
	if got := model.paths[sourceField]; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if model.Status().Center != "No source scanned" {
		t.Fatal("choosing a path changed scan state")
	}
}

func TestChooseTreeRoot(t *testing.T) {
	root := t.TempDir()
	model := newImportModel().(importModel)
	model.paths[destinationField] = root
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if model.picking {
		t.Fatal("Enter did not choose the tree root")
	}
	if got := expandHome(model.paths[destinationField]); got != root {
		t.Fatalf("destination = %q, want %q", got, root)
	}
}

func TestImportFormSupportsVimVerticalNavigation(t *testing.T) {
	model := newImportModel().(importModel)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(importModel)
	if model.focus != destinationField {
		t.Fatalf("j focused field %d, want destination", model.focus)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updated.(importModel)
	if model.focus != sourceField {
		t.Fatalf("k focused field %d, want source", model.focus)
	}
}

func TestEscapeCancelsDirectoryPicker(t *testing.T) {
	model := newImportModel().(importModel)
	original := model.paths[sourceField]
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(importModel)
	if model.picking {
		t.Fatal("Esc did not close the picker")
	}
	if got := model.paths[sourceField]; got != original {
		t.Fatalf("cancelled source = %q, want %q", got, original)
	}
}

func TestExplorerDialogHintsAreNotRepeatedInStatusBar(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "delete-me"), 0o755); err != nil {
		t.Fatal(err)
	}
	model := newImportModel().(importModel)
	model.paths[sourceField] = root
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	updated, _ = model.Update(cmd())
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(importModel)

	if status := model.Status(); status.Right != "" {
		t.Fatalf("dialog controls repeated in status bar: %q", status.Right)
	}
	if !strings.Contains(model.View(), "y confirm") {
		t.Fatal("delete confirmation is missing from the explorer operation area")
	}
	if count := strings.Count(model.View(), "n/esc cancel"); count != 1 {
		t.Fatalf("delete cancel hint appears %d times, want once", count)
	}
}
