package divider

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestAnchoredHasLeadingAnchorAndExactWidth(t *testing.T) {
	view := Anchored(40)
	if !strings.Contains(view, "── ◆ ──") {
		t.Fatalf("divider does not use the anchored pattern: %q", view)
	}
	if got := lipgloss.Width(view); got != 40 {
		t.Fatalf("width = %d, want 40", got)
	}
}
