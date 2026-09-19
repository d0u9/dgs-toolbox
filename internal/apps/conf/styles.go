// Package conf implements dgs conf: the inspect page, export, and the reports
// over a generator root's inventory.
package conf

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(
		lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"},
	)
	mutedStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"},
	)
	brokenStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"},
	)
)
