// Package divider renders separators used between semantic sections.
package divider

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// Labelled renders the anchored rule with a label after the anchor, naming the
// section it introduces rather than merely separating it. The label is clipped
// when the width cannot hold it; an empty label gives a plain Anchored rule.
func Labelled(label string, width int) string {
	width = max(5, width)
	label = strings.TrimSpace(label)
	if label == "" {
		return Anchored(width)
	}
	// "── ◆ " and one trailing space around the label, then at least one cell
	// of trailing rule.
	label = ansi.Truncate(label, max(1, width-7), "…")
	head := lineStyle.Render("──") + anchorStyle.Render(" ◆ ") + lineStyle.Render(label+" ")
	return head + lineStyle.Render(strings.Repeat("─", max(1, width-lipgloss.Width(head))))
}
