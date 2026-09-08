package pageactions

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestViewNamesNavigationDestinationsAndAlignsRight(t *testing.T) {
	view := View(Config{
		Prev: &Action{Destination: "Directories"},
		Next: &Action{Destination: "Processing"},
	}, 50)
	for _, want := range []string{"← Prev", "Directories", "Next →", "Processing"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q: %q", want, view)
		}
	}
	if lipgloss.Width(view) != 50 || lipgloss.Height(view) != 4 {
		t.Fatalf("view size = %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	if Hit(Config{Prev: &Action{Destination: "Directories"}, Next: &Action{Destination: "Processing"}}, 50, 48, 1) != Next {
		t.Fatal("right action was not clickable")
	}
	if Hit(Config{Next: &Action{Destination: "Processing"}}, 50, 49, 1) != None {
		t.Fatal("reserved right inset should not be clickable")
	}
}
