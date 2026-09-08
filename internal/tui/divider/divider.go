// Package divider renders separators used between semantic sections.
package divider

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	lineColor   = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	anchorColor = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	lineStyle   = lipgloss.NewStyle().Foreground(lineColor)
	anchorStyle = lipgloss.NewStyle().Bold(true).Foreground(anchorColor)
)

// Anchored renders a leading short rule, a diamond anchor, and a trailing rule
// at exactly width terminal cells. Callers own the surrounding vertical space.
func Anchored(width int) string {
	width = max(5, width)
	return lineStyle.Render("──") +
		anchorStyle.Render(" ◆ ") +
		lineStyle.Render(strings.Repeat("─", width-5))
}
