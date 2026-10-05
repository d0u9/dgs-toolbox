package photo

import (
	"context"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/photoencode"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEncodeRequiresExplicitStart(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "input.png")
	os.WriteFile(source, []byte("invalid"), 0600)
	destination := filepath.Join(root, "out")
	c := config.DefaultPhotoEncode()
	c.Source = source
	c.Destination = destination
	c.Quality = 90
	m := newEncodeModel(c)
	if m.controls.IntValue("quality") != 90 {
		t.Fatal("configured quality ignored")
	}
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = model.(encodeModel)
	if cmd == nil || m.stage != "planning" {
		t.Fatal("no plan")
	}
	model, _ = m.Update(cmd())
	m = model.(encodeModel)
	if m.stage != "plan" {
		t.Fatal(m.stage)
	}
	if _, e := os.Stat(destination); !os.IsNotExist(e) {
		t.Fatal("planning wrote files")
	}
	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = model.(encodeModel)
	if cmd == nil || m.stage != "processing" {
		t.Fatal("no processing")
	}
	model, _ = m.Update(cmd())
	m = model.(encodeModel)
	if m.stage != "results" || len(m.results) != 1 || m.results[0].Error == "" {
		t.Fatal(m.results)
	}
}

func TestEncodeMousePlanButtonAndViewportFit(t *testing.T) {
	m := newEncodeModel(config.DefaultPhotoEncode())
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = model.(encodeModel)
	view := m.View()
	if lipgloss.Width(view) != 100 || lipgloss.Height(view) != 30 {
		t.Fatalf("view dimensions: %d × %d", lipgloss.Width(view), lipgloss.Height(view))
	}
	// Click the actual rendered button title, rather than inferred geometry.
	x, y := -1, -1
	for row, line := range strings.Split(view, "\n") {
		if col := strings.Index(ansi.Strip(line), "Plan →"); col >= 0 {
			x, y = col+1, row
			break
		}
	}
	if x < 0 {
		t.Fatal("no Plan button")
	}
	model, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = model.(encodeModel)
	if cmd == nil || m.stage != "planning" {
		t.Fatalf("button did not plan: %s", m.stage)
	}
}
func TestEncodeStopIsSafeByDefault(t *testing.T) {
	m := newEncodeModel(config.DefaultPhotoEncode())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx, m.cancel, m.stage = ctx, cancel, "processing"
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(encodeModel)
	if !m.confirming || !m.confirmation.CancelChosen() {
		t.Fatal("no safe confirmation")
	}
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(encodeModel)
	if m.confirming || ctx.Err() != nil {
		t.Fatal("safe action cancelled work")
	}
	m.Close()
	if ctx.Err() == nil {
		t.Fatal("Close did not cancel")
	}
}

func TestEncodeRequiredErrorsAreLocal(t *testing.T) {
	m := newEncodeModel(config.DefaultPhotoEncode())
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = model.(encodeModel)
	model, _ = m.Update(cmd())
	m = model.(encodeModel)
	if m.stage != "parameters" || m.fieldErrors["input"] != "Required" || m.fieldErrors["output"] != "Required" {
		t.Fatalf("missing local errors: %v", m.fieldErrors)
	}
	content := ansi.Strip(m.parameterContent(90))
	if !strings.Contains(content, "! Required") {
		t.Fatal(content)
	}
}

func TestEncodeIgnoresOldSessionMessages(t *testing.T) {
	old := newEncodeModel(config.DefaultPhotoEncode())
	fresh := newEncodeModel(config.DefaultPhotoEncode())
	for _, msg := range []tea.Msg{encodePlanMsg{session: old.session}, encodeResultMsg{session: old.session}} {
		model, cmd := fresh.Update(msg)
		fresh = model.(encodeModel)
		if fresh.stage != "parameters" || cmd != nil || len(fresh.results) != 0 {
			t.Fatal("old session affected fresh workspace")
		}
	}
}
func TestEncodeCloseWaitsForWorker(t *testing.T) {
	m := newEncodeModel(config.DefaultPhotoEncode())
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.stage = "processing"
	m.jobs = []photoencode.Job{{Source: "does-not-exist", Destination: "unused"}}
	cmd := m.runNext()
	go cmd()
	m.Close()
	select {
	case <-m.finished:
	default:
		t.Fatal("Close left worker running")
	}
}

func TestEncodeOptionErrorsMarkOnlyTheirField(t *testing.T) {
	m := newEncodeModel(config.DefaultPhotoEncode())
	m.controls.SetValue("input", "/photos")
	m.controls.SetValue("output", "/exports")
	updated, _ := m.Update(encodePlanMsg{session: m.session, err: &photoencode.OptionError{Option: "max_pixels", Message: "max_pixels must be 1–40000000"}})
	m = updated.(encodeModel)
	if len(m.fieldErrors) != 0 || !strings.Contains(m.notice, "max_pixels") {
		t.Fatalf("errors = %v notice = %q", m.fieldErrors, m.notice)
	}
	updated, _ = m.Update(encodePlanMsg{session: m.session, err: &photoencode.OptionError{Option: "quality", Message: "quality must be 1–100"}})
	m = updated.(encodeModel)
	if len(m.fieldErrors) != 1 || m.fieldErrors["quality"] == "" {
		t.Fatal(m.fieldErrors)
	}
}

func TestEncodeCloseDoesNotWaitForeverForAJobThatNeverRan(t *testing.T) {
	m := newEncodeModel(config.DefaultPhotoEncode())
	_, m.cancel = context.WithCancel(context.Background())
	m.finished = make(chan struct{})
	done := make(chan struct{})
	go func() { m.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(encodeCloseWait + 2*time.Second):
		t.Fatal("Close hung")
	}
}
