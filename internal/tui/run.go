package tui

import (
	"fmt"

	"dgs-toolbox/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the one and only top-level Bubble Tea program.
func Run(apps []App, launch Launch) error {
	global, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load global config: %w", err)
	}
	model := NewModelWithConfig(apps, launch, global.TopBarVisibility())
	if model.launchErr != "" {
		return fmt.Errorf("cannot launch TUI: %s", model.launchErr)
	}
	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
