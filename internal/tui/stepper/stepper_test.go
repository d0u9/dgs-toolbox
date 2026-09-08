package stepper

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestViewRendersAThreeStateTrack(t *testing.T) {
	view := View([]string{"Copy", "Verify", "Publish"}, 1, 80)
	for _, want := range []string{"✓ Copy", "● Verify", "○ Publish", "──"} {
		if !strings.Contains(view, want) {
			t.Errorf("track missing %q: %q", want, view)
		}
	}
}

func TestViewSupportsCompletedTrackAndWidth(t *testing.T) {
	view := View([]string{"Copy", "Verify"}, 2, 10)
	if !strings.Contains(view, "✓ Copy") || strings.Contains(view, "●") {
		t.Fatalf("completed track = %q", view)
	}
	if lipgloss.Width(view) > 10 {
		t.Fatalf("track width = %d, want <= 10", lipgloss.Width(view))
	}
}
