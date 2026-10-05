package photo

import (
	"context"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/apps/photo/postprocess"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func awaitOrganizationPlan(t *testing.T, m importModel) postprocess.Plan {
	t.Helper()
	updated, cmd := m.runPostProcessing()
	m = updated.(importModel)
	defer m.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		msg := cmd()
		if planned, ok := msg.(postprocessPlannedMsg); ok {
			return planned.plan
		}
		updated, next := m.Update(msg)
		m, cmd = updated.(importModel), next
		if cmd == nil {
			t.Fatal("planning stopped without outcome")
		}
	}
	t.Fatal("planning did not complete")
	return postprocess.Plan{}
}

func TestOrganizationHeartbeatWhileFilesystemWorkWaits(t *testing.T) {
	m := newImportModel().(importModel)
	m.stage = postprocessStage
	m.postprocess.running = true
	m.width, m.height = 120, 28
	updated, cmd := m.startOrganizationTask(func(ctx context.Context, report func(postprocess.Progress)) tea.Msg {
		report(postprocess.Progress{Phase: "Reading capture dates", Path: "photo.JPG", Completed: 1, Total: 3})
		<-ctx.Done()
		return postprocessDoneMsg{}
	})
	m = updated.(importModel)
	defer m.Close()
	updated, cmd = m.Update(cmd())
	m = updated.(importModel)
	before := m.postprocessView()
	for _, want := range []string{"Reading capture dates", "photo.JPG", "1/3 files", "Destination 1/1"} {
		if !strings.Contains(before, want) {
			t.Fatalf("missing activity %q: %s", want, before)
		}
	}
	msg := cmd()
	if _, ok := msg.(postprocessPulseMsg); !ok {
		t.Fatalf("waiting work did not generate heartbeat: %T", msg)
	}
	updated, cmd = m.Update(msg)
	m = updated.(importModel)
	if cmd == nil || !m.postprocess.running || m.postprocess.pulse != 1 || before == m.postprocessView() {
		t.Fatal("activity froze while work waited")
	}
	if lipgloss.Width(m.postprocessView()) > m.width || lipgloss.Height(m.postprocessView()) > m.height {
		t.Fatal("progress view overflows")
	}
}

func TestOrganizationReportsMovesAndStateSaving(t *testing.T) {
	m, _ := runMultipleTUI(t, multipleImportModel(t))
	m.stage = postprocessStage
	updated, cmd := m.runPostProcessing()
	m = updated.(importModel)
	phases := map[string]bool{}
	destinations := map[int]bool{}
	for m.postprocess.running {
		msg := cmd()
		if progress, ok := msg.(postprocessProgressMsg); ok {
			phases[progress.Phase] = true
			destinations[m.postprocessTarget] = true
		}
		updated, cmd = m.Update(msg)
		m = updated.(importModel)
		if m.postprocessPrompt {
			t.Fatal("unexpected confirmation")
		}
	}
	for _, phase := range []string{"Reading capture dates", "Organizing files", "Saving import state"} {
		if !phases[phase] {
			t.Fatalf("missing progress phase %s", phase)
		}
	}
	if len(destinations) != 2 || len(m.postprocess.result.Files) != 4 {
		t.Fatal("did not organize both destinations")
	}
}
