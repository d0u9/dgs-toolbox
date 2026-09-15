// Package confirm provides a reusable modal for interrupting a consequential action.
package confirm

import (
	"strings"

	"dgs-toolbox/internal/tui/pageactions"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type Decision int

const (
	NoDecision Decision = iota
	Confirmed
	Cancelled
)

// Config supplies the command-specific language for a confirmation dialog.
type Config struct {
	Title        string
	Message      string
	Detail       string
	ConfirmLabel string
	CancelLabel  string
}

// Model starts on the safe, negative action.
type Model struct {
	config       Config
	cancelChosen bool
}

var (
	accent = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	panel  = lipgloss.AdaptiveColor{Light: "#EAF4F4", Dark: "#243447"}
	muted  = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}

	frameStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Background(panel)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	noteStyle  = lipgloss.NewStyle().Foreground(muted)
)

func New(config Config) Model {
	if config.ConfirmLabel == "" {
		config.ConfirmLabel = "Yes"
	}
	if config.CancelLabel == "" {
		config.CancelLabel = "No"
	}
	return Model{config: config, cancelChosen: true}
}

// Update accepts conventional confirmation keys. Esc always selects the safe
// cancellation outcome, even if the affirmative button currently has focus.
func (m Model) Update(key string) (Model, Decision) {
	switch key {
	case "tab", "shift+tab", "left", "h", "right", "l":
		m.cancelChosen = !m.cancelChosen
	case "enter":
		if m.cancelChosen {
			return m, Cancelled
		}
		return m, Confirmed
	case "esc":
		return m, Cancelled
	}
	return m, NoDecision
}

func (m Model) CancelChosen() bool { return m.cancelChosen }

// View renders an intentionally compact dialog. The focus cue lives on the
// selected button, leaving the key hint quiet and readable.
func (m Model) View(width int) string {
	outerWidth := max(24, min(66, width-4))
	innerWidth := max(1, outerWidth-4)
	confirm := pageactions.Inline(m.config.ConfirmLabel, !m.cancelChosen)
	cancel := pageactions.Inline(m.config.CancelLabel, m.cancelChosen)
	// The message and detail wrap rather than truncate: what a dialog asks
	// about is exactly what must not be cut off.
	content := []string{
		titleStyle.Render(ansi.Truncate(m.config.Title, innerWidth, "…")),
		"",
		ansi.Wrap(m.config.Message, innerWidth, ""),
	}
	if m.config.Detail != "" {
		content = append(content, noteStyle.Render(ansi.Wrap(m.config.Detail, innerWidth, "")))
	}
	content = append(content,
		"",
		noteStyle.Render(strings.Repeat("─", innerWidth)),
		pageactions.Footer(innerWidth, "Tab switch · Enter select · Esc continue", cancel, confirm),
	)
	return frameStyle.Width(outerWidth - 2).Render(strings.Join(content, "\n"))
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
