// Package fieldset renders bordered groups with a legend embedded in the top
// edge, similar to an HTML fieldset and legend.
package fieldset

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	borderColor = lipgloss.AdaptiveColor{Light: "#A21CAF", Dark: "#F04FC1"}
	legendColor = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	borderStyle = lipgloss.NewStyle().Foreground(borderColor)
	legendStyle = lipgloss.NewStyle().Bold(true).Foreground(legendColor)
)

// View renders a fully closed rectangle of exactly width cells. The legend is
// part of the top border and content is padded inside the remaining edges.
func View(legend, content string, width int) string {
	width = max(12, width)
	innerWidth := width - 4
	legend = ansi.Truncate(strings.TrimSpace(legend), max(1, width-8), "…")
	topFill := max(1, width-lipgloss.Width(legend)-5)
	top := borderStyle.Render("╭─ ") + legendStyle.Render(legend) + borderStyle.Render(" "+strings.Repeat("─", topFill)+"╮")

	lines := strings.Split(content, "\n")
	if content == "" {
		lines = []string{""}
	}
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, top)
	for _, line := range lines {
		line = ansi.Truncate(line, innerWidth, "…")
		padding := strings.Repeat(" ", max(0, innerWidth-lipgloss.Width(line)))
		rows = append(rows, borderStyle.Render("│ ")+line+padding+borderStyle.Render(" │"))
	}
	rows = append(rows, borderStyle.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(rows, "\n")
}
