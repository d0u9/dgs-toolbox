// Package overlay composes a floating view over an existing workspace.
package overlay

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Place centers foreground over background and returns exactly width by height.
func Place(background, foreground string, width, height int) string {
	frontWidth := lipgloss.Width(foreground)
	frontHeight := lipgloss.Height(foreground)
	x := max(0, (width-frontWidth)/2)
	y := max(0, (height-frontHeight)/2)

	return PlaceAt(background, foreground, x, y, width, height)
}

// PlaceAt composites foreground at workspace coordinates and clips it to the viewport.
func PlaceAt(background, foreground string, x, y, width, height int) string {
	frontWidth := lipgloss.Width(foreground)
	backLines := strings.Split(background, "\n")
	frontLines := strings.Split(foreground, "\n")
	for i := range backLines {
		backLines[i] = pad(backLines[i], width)
	}
	for len(backLines) < height {
		backLines = append(backLines, strings.Repeat(" ", width))
	}
	for i, frontLine := range frontLines {
		row := y + i
		if row >= len(backLines) || row >= height {
			break
		}
		backLine := backLines[row]
		left := ansi.Cut(backLine, 0, x)
		right := ansi.Cut(backLine, x+frontWidth, width)
		backLines[row] = pad(left, x) + pad(frontLine, frontWidth) + pad(right, width-x-frontWidth)
	}
	return strings.Join(backLines[:height], "\n")
}

func pad(value string, width int) string {
	value = ansi.Truncate(value, max(0, width), "")
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}
