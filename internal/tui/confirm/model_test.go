package confirm

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDefaultsToNegativeActionAndSupportsTab(t *testing.T) {
	dialog := New(Config{Title: "Leave?", Message: "Work will stop."})
	if !dialog.CancelChosen() {
		t.Fatal("negative action should be selected by default")
	}
	dialog, decision := dialog.Update("enter")
	if decision != Cancelled {
		t.Fatalf("default enter decision = %v, want Cancelled", decision)
	}
	dialog, _ = dialog.Update("tab")
	if dialog.CancelChosen() {
		t.Fatal("tab did not select affirmative action")
	}
	_, decision = dialog.Update("enter")
	if decision != Confirmed {
		t.Fatalf("affirmative enter decision = %v, want Confirmed", decision)
	}
}

func TestEscAlwaysCancelsAndViewIsBounded(t *testing.T) {
	dialog := New(Config{Title: "Exit?", Message: "Processing will stop."})
	dialog, _ = dialog.Update("tab")
	_, decision := dialog.Update("esc")
	if decision != Cancelled {
		t.Fatalf("esc decision = %v, want Cancelled", decision)
	}
	view := dialog.View(50)
	if lipgloss.Width(view) > 50 || !strings.Contains(view, "Tab switch") || !strings.Contains(view, "[ No ]") {
		t.Fatalf("unexpected dialog:\n%s", view)
	}
}
