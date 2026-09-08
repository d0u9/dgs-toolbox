package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the one and only top-level Bubble Tea program.
func Run(apps []App, launch Launch) error {
	model := NewModel(apps, launch)
	if model.launchErr != "" {
		return fmt.Errorf("cannot launch TUI: %s", model.launchErr)
	}
	_, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
