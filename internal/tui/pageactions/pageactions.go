// Package pageactions renders consistent previous/next workflow navigation.
package pageactions

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type Direction int

const (
	None Direction = iota
	Prev
	Next
)

type Action struct{ Destination string }
type Config struct {
	Prev *Action
	Next *Action
}

var (
	accent         = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	muted          = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	panel          = lipgloss.AdaptiveColor{Light: "#DDF3F0", Dark: "#173F3B"}
	secondaryPanel = lipgloss.AdaptiveColor{Light: "#EEF2F4", Dark: "#303747"}
	primaryStyle   = lipgloss.NewStyle().Bold(true).Foreground(panel).Background(accent)
	secondaryStyle = lipgloss.NewStyle().Foreground(muted).Background(secondaryPanel)
)

type geometry struct{ start, prevWidth, nextWidth, gap int }

// View renders two-line navigation buttons after one row of breathing space.
// Solid fills distinguish page actions from bordered data fields.
func View(config Config, width int) string {
	contentWidth := max(0, width-1)
	g := resolve(config, contentWidth)
	prev, next := []string{"", ""}, []string{"", ""}
	if config.Prev != nil {
		prev = button("← Prev  esc", config.Prev.Destination, g.prevWidth, false)
	}
	if config.Next != nil {
		next = button("Next →  n", config.Next.Destination, g.nextWidth, true)
	}
	lines := []string{strings.Repeat(" ", max(0, width))}
	for row := 0; row < 2; row++ {
		line := strings.Repeat(" ", g.start)
		if g.prevWidth > 0 {
			line += prev[row]
		}
		if g.gap > 0 {
			line += strings.Repeat(" ", g.gap)
		}
		if g.nextWidth > 0 {
			line += next[row]
		}
		lines = append(lines, line+strings.Repeat(" ", max(0, width-lipgloss.Width(line))))
	}
	lines = append(lines, strings.Repeat(" ", max(0, width)))
	return strings.Join(lines, "\n")
}

func button(title, destination string, width int, primary bool) []string {
	style := secondaryStyle
	prefix := ""
	if primary {
		style = primaryStyle
		prefix = "› "
	}
	inner := max(1, width-4)
	title = ansi.Truncate(title, inner, "")
	destination = ansi.Truncate(destination, max(1, inner-lipgloss.Width(prefix)), "…")
	line := func(value string) string {
		return style.Render("  " + value + strings.Repeat(" ", max(0, inner-lipgloss.Width(value))) + "  ")
	}
	return []string{line(title), line(prefix + destination)}
}

// Hit reports the action at coordinates relative to the three-row view.
func Hit(config Config, width, x, y int) Direction {
	if y < 1 || y > 2 || x < 0 || x >= width {
		return None
	}
	g := resolve(config, max(0, width-1))
	if g.prevWidth > 0 && x >= g.start && x < g.start+g.prevWidth {
		return Prev
	}
	nextStart := g.start + g.prevWidth + g.gap
	if g.nextWidth > 0 && x >= nextStart && x < nextStart+g.nextWidth {
		return Next
	}
	return None
}

func resolve(config Config, width int) geometry {
	g := geometry{gap: 2}
	if config.Prev != nil {
		g.prevWidth = min(24, max(16, max(lipgloss.Width("← Prev  esc"), lipgloss.Width(config.Prev.Destination))+4))
	}
	if config.Next != nil {
		g.nextWidth = min(24, max(16, max(lipgloss.Width("Next →  n"), lipgloss.Width(config.Next.Destination))+4))
	}
	if g.prevWidth == 0 || g.nextWidth == 0 {
		g.gap = 0
	}
	if total := g.prevWidth + g.gap + g.nextWidth; total > width {
		available := max(0, width-g.gap)
		if g.prevWidth > 0 && g.nextWidth > 0 {
			g.prevWidth = available / 2
			g.nextWidth = available - g.prevWidth
		} else if g.prevWidth > 0 {
			g.prevWidth = width
		} else {
			g.nextWidth = width
		}
	}
	g.start = max(0, width-g.prevWidth-g.gap-g.nextWidth)
	return g
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
