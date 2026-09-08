// Package stepper renders compact, stateful progress tracks.
package stepper

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

var (
	completedStyle = lipgloss.NewStyle().Bold(true)
	currentStyle   = lipgloss.NewStyle().Bold(true)
	pendingStyle   = lipgloss.NewStyle()
)

// View renders every step as completed, current, or forthcoming. A current
// value equal to len(steps) represents a fully completed track.
func View(steps []string, current, width int) string {
	parts := make([]string, len(steps))
	for i, step := range steps {
		switch {
		case i < current:
			parts[i] = completedStyle.Render("✓ " + step)
		case i == current:
			parts[i] = currentStyle.Render("● " + step)
		default:
			parts[i] = pendingStyle.Render("○ " + step)
		}
	}
	return ansi.Truncate(strings.Join(parts, " ── "), width, "…")
}
