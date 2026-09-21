package confirm

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	if lipgloss.Width(view) > 50 || !strings.Contains(view, "Tab switch") || !strings.Contains(view, " No ") {
		t.Fatalf("unexpected dialog:\n%s", view)
	}
}

func TestButtonsAreRightAlignedWithSafeActionOnLeft(t *testing.T) {
	dialog := New(Config{Title: "Exit?", Message: "Leave now?"})
	lines := strings.Split(ansi.Strip(dialog.View(50)), "\n")
	var buttons string
	for _, line := range lines {
		if strings.Contains(line, "› No") {
			buttons = line
			break
		}
	}
	if buttons == "" || strings.Index(buttons, "No") > strings.Index(buttons, "Yes") {
		t.Fatalf("buttons are not ordered No then Yes: %q", buttons)
	}
	if !strings.HasSuffix(strings.TrimRight(buttons, " │"), "Yes") {
		t.Fatalf("buttons are not right aligned: %q", buttons)
	}
}

func TestViewSizeFitsTheTerminal(t *testing.T) {
	dialog := New(Config{
		Title:        "DELETE",
		Message:      strings.Repeat("a long message that wraps over several rows. ", 8),
		Detail:       strings.Repeat("a long detail that wraps too. ", 8),
		ConfirmLabel: "Delete",
		CancelLabel:  "Keep",
	})
	if height := lipgloss.Height(dialog.View(60)); height < 12 {
		t.Fatalf("unbounded height %d", height)
	}
	for _, room := range []int{10, 14, 20} {
		view := dialog.ViewSize(60, room)
		if height := lipgloss.Height(view); height > room {
			t.Errorf("height %d in %d rows", height, room)
		}
		lines := strings.Split(ansi.Strip(view), "\n")
		last := lines[len(lines)-1]
		if !strings.Contains(last, "╰") {
			t.Errorf("no bottom border in %d rows: %q", room, last)
		}
		if !strings.Contains(ansi.Strip(view), "Delete") || !strings.Contains(ansi.Strip(view), "Keep") {
			t.Errorf("buttons cut in %d rows:\n%s", room, ansi.Strip(view))
		}
		if !strings.Contains(ansi.Strip(view), "…") {
			t.Errorf("no cut marker in %d rows", room)
		}
	}
}
