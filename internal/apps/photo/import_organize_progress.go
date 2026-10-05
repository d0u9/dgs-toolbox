package photo

import (
	"context"
	"time"

	"dgs-toolbox/internal/apps/photo/postprocess"
	tea "github.com/charmbracelet/bubbletea"
)

// Work and activity reports stay outside the Tea update loop. The heartbeat
// continues even when a filesystem call takes longer than a progress step.
func (m importModel) startOrganizationTask(task func(context.Context, func(postprocess.Progress)) tea.Msg) (tea.Model, tea.Cmd) {
	if m.postprocessCancel != nil {
		m.postprocessCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan tea.Msg, 32)
	m.postprocessCancel = cancel
	m.postprocessUpdates = updates
	m.postprocess.progress = postprocess.Progress{Phase: "Preparing organization"}
	m.postprocess.pulse = 0
	go func() {
		defer close(updates)
		report := func(progress postprocess.Progress) {
			select {
			case updates <- postprocessProgressMsg(progress):
			default:
			}
		}
		done := task(ctx, report)
		select {
		case updates <- done:
		case <-ctx.Done():
		}
	}()
	return m, waitPostprocessUpdate(updates)
}

func waitPostprocessUpdate(updates <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		timer := time.NewTimer(120 * time.Millisecond)
		defer timer.Stop()
		select {
		case message := <-updates:
			return message
		case <-timer.C:
			return postprocessPulseMsg{}
		}
	}
}
