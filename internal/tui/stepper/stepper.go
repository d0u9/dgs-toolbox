// Package stepper renders compact workflow progress for the shared status bar.
package stepper

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

var currentStyle = lipgloss.NewStyle().Bold(true).Underline(true)

func View(steps []string, current, width int) string {
	parts := make([]string, len(steps))
	for i, step := range steps {
		if i == current {
			step = currentStyle.Render(step)
		}
		parts[i] = step
	}
	return ansi.Truncate(strings.Join(parts, "  ›  "), width, "…")
}
