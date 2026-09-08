package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPlaceKeepsViewportSize(t *testing.T) {
	view := Place("background", "╭──╮\n│hi│\n╰──╯", 20, 8)
	if lipgloss.Width(view) != 20 || lipgloss.Height(view) != 8 {
		t.Fatalf("overlay size = %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	if !strings.Contains(view, "│hi│") {
		t.Fatal("foreground is missing")
	}
}
