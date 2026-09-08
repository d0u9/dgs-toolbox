package fieldset

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestViewIsClosedAndHasExactWidth(t *testing.T) {
	view := View("Transfer", "Radio     (●) Copy   ( ) Move", 48)
	lines := strings.Split(view, "\n")
	if !strings.HasPrefix(lines[0], "╭─ ") || !strings.HasSuffix(lines[0], "╮") {
		t.Fatalf("top border is not closed: %q", lines[0])
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") || !strings.HasSuffix(lines[len(lines)-1], "╯") {
		t.Fatalf("bottom border is not closed: %q", lines[len(lines)-1])
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got != 48 {
			t.Fatalf("line width = %d, want 48: %q", got, line)
		}
	}
}
