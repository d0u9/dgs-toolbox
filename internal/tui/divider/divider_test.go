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

func TestLabelledNamesTheSectionAndKeepsExactWidth(t *testing.T) {
	view := Labelled("ORGANIZED", 40)
	if !strings.Contains(view, "── ◆ ORGANIZED ─") {
		t.Fatalf("labelled divider = %q", view)
	}
	if got := lipgloss.Width(view); got != 40 {
		t.Fatalf("width = %d, want 40", got)
	}
	if got := lipgloss.Width(Labelled("ORGANIZED", 12)); got != 12 {
		t.Fatalf("narrow width = %d, want 12", got)
	}
	if Labelled("  ", 40) != Anchored(40) {
		t.Fatal("an empty label should give the plain anchored rule")
	}
}
