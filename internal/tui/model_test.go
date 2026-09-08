package tui

import (
	"strings"
	"testing"
	"time"

	dgsconfig "dgs-toolbox/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type stubCommand struct {
	label         string
	width         int
	height        int
	help          bool
	captureQ      bool
	capturedQ     bool
	captureCtrlC  bool
	capturedCtrlC bool
}

func (m stubCommand) Init() tea.Cmd { return nil }

func (m stubCommand) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.String() == "?" {
			m.help = !m.help
		}
		if msg.String() == "q" && m.captureQ {
			m.capturedQ = true
		}
		if msg.String() == "ctrl+c" && m.captureCtrlC {
			m.capturedCtrlC = true
		}
	}
	return m, nil
}

func (m stubCommand) View() string {
	label := m.label
	if m.help {
		label = "COMMAND HELP"
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, label)
}

func (m stubCommand) Status() Status {
	return Status{Left: "ACTIVE", Center: m.label, Right: "? Help  esc Back"}
}

func (m stubCommand) CapturesShellKey(key string) bool {
	return (m.captureQ && key == "q") || (m.captureCtrlC && key == "ctrl+c")
}

func newStub(label string) func() CommandModel {
	return func() CommandModel { return stubCommand{label: label} }
}

var testApps = []App{
	{ID: "photo", Name: "Photo", Commands: []Command{
		{ID: "import", Name: "Import", New: newStub("PHOTO IMPORT")},
		{ID: "encode", Name: "Encode", New: newStub("PHOTO ENCODE")},
	}},
	{ID: "gpx", Name: "GPX", Commands: []Command{
		{ID: "import", Name: "Import", New: newStub("GPX IMPORT")},
		{ID: "inspect", Name: "Inspect", New: newStub("GPX INSPECT")},
	}},
}

func TestDirectCommandUsesSingleBreadcrumbSegment(t *testing.T) {
	apps := []App{{
		ID: "demo", Name: "Component Demo", Direct: true,
		Commands: []Command{{ID: "demo", Name: "Component Demo", New: newStub("DEMO")}},
	}}
	m := NewModel(apps, Launch{App: "demo", Command: "demo"})
	if got := m.breadcrumb(); got != "dgs › demo" {
		t.Fatalf("breadcrumb = %q, want %q", got, "dgs › demo")
	}
}

type contextualStub struct{ stubCommand }

func (m contextualStub) CommandPath() []string { return []string{"scan"} }
func (m contextualStub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.stubCommand.Update(msg)
	m.stubCommand = updated.(stubCommand)
	return m, cmd
}

func TestCommandCanContributeBreadcrumbState(t *testing.T) {
	apps := []App{{ID: "photo", Name: "Photo", Commands: []Command{{
		ID: "import", Name: "Import", New: func() CommandModel { return contextualStub{stubCommand{label: "IMPORT"}} },
	}}}}
	m := NewModel(apps, Launch{App: "photo", Command: "import"})
	if got := m.breadcrumb(); got != "dgs › photo › import › scan" {
		t.Fatalf("breadcrumb = %q", got)
	}
}

func TestGlobalPickerStartsLeafAndReturns(t *testing.T) {
	m := NewModel(testApps, Launch{})
	if m.active != nil || len(m.choices()) != 4 {
		t.Fatalf("global picker state = active %T, %d choices", m.active, len(m.choices()))
	}
	if !strings.Contains(m.View(), "CHOOSE A COMMAND") {
		t.Fatalf("global picker missing from view:\n%s", m.View())
	}

	m = update(t, m, "down")
	m = update(t, m, "enter")
	if m.active == nil || !strings.Contains(m.View(), "PHOTO ENCODE") {
		t.Fatalf("selected command did not replace picker:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "dgs › photo › encode") {
		t.Fatalf("active command breadcrumb missing:\n%s", m.View())
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil || m.active != nil || !strings.Contains(m.View(), "CHOOSE A COMMAND") {
		t.Fatalf("escape did not return to global picker:\n%s", m.View())
	}
}

func TestCommandPickerSupportsVimVerticalNavigation(t *testing.T) {
	m := NewModel(testApps, Launch{})
	m = update(t, m, "j")
	if m.selected != 1 {
		t.Fatalf("j selected index %d, want 1", m.selected)
	}
	m = update(t, m, "k")
	if m.selected != 0 {
		t.Fatalf("k selected index %d, want 0", m.selected)
	}
}

func TestSelectingCommandCreatesFreshModel(t *testing.T) {
	created := 0
	apps := []App{{ID: "photo", Name: "Photo", Commands: []Command{{
		ID: "import", Name: "Import", New: func() CommandModel {
			created++
			return stubCommand{label: "PHOTO IMPORT"}
		},
	}}}}
	m := NewModel(apps, Launch{App: "photo"})
	m = update(t, m, "enter")
	m = update(t, m, "esc")
	m = update(t, m, "enter")
	if created != 2 {
		t.Fatalf("command factory called %d times, want 2", created)
	}
}

func TestDomainPickerIsScoped(t *testing.T) {
	m := NewModel(testApps, Launch{App: "gpx"})
	choices := m.choices()
	if len(choices) != 2 {
		t.Fatalf("domain picker choices = %d, want 2", len(choices))
	}
	for _, choice := range choices {
		if choice.appIndex != 1 {
			t.Fatalf("domain picker contains app index %d, want 1", choice.appIndex)
		}
	}
	if !strings.Contains(m.View(), "dgs › gpx › commands") {
		t.Fatalf("domain picker breadcrumb missing:\n%s", m.View())
	}
}

func TestDirectCommandEscapeReturnsToParentPickerThenConfirmsExit(t *testing.T) {
	m := NewModel(testApps, Launch{App: "photo", Command: "import"})
	if m.active == nil || !strings.Contains(m.View(), "PHOTO IMPORT") {
		t.Fatalf("direct command did not start:\n%s", m.View())
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil || m.active != nil {
		t.Fatal("first escape should return to the parent picker")
	}
	if !strings.Contains(m.View(), "dgs › photo › commands") {
		t.Fatalf("parent picker breadcrumb missing:\n%s", m.View())
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil || !m.confirmQuit || !strings.Contains(m.View(), "QUIT DGS?") {
		t.Fatal("second escape should request confirmation from the picker")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("confirmation message type = %T, want tea.QuitMsg", cmd())
	}
}

func TestQuitConfirmationCanBeCancelledWithoutDiscardingCommand(t *testing.T) {
	m := NewModel(testApps, Launch{App: "gpx", Command: "inspect"})
	m = update(t, m, "q")
	if !m.confirmQuit || m.active == nil {
		t.Fatal("q should request confirmation and preserve the active command")
	}
	m = update(t, m, "enter")
	if m.confirmQuit || m.active == nil || !strings.Contains(m.View(), "GPX INSPECT") {
		t.Fatalf("cancel should resume the active command:\n%s", m.View())
	}
}

func TestCommandQuitRequestUsesSharedConfirmation(t *testing.T) {
	m := NewModel(testApps, Launch{App: "gpx", Command: "inspect"})
	updated, cmd := m.Update(RequestQuitMsg{})
	m = updated.(Model)
	if cmd != nil || !m.confirmQuit || !m.quitDialog.CancelChosen() {
		t.Fatalf("request quit: cmd=%v confirm=%v cancel-focused=%v", cmd != nil, m.confirmQuit, m.quitDialog.CancelChosen())
	}
	if view := m.View(); !strings.Contains(view, "Tab switch") || !strings.Contains(view, "[ No ]") {
		t.Fatalf("shared quit dialog missing:\n%s", view)
	}
}

func TestActiveTextEditorCanCaptureQFromShell(t *testing.T) {
	apps := []App{{ID: "photo", Name: "Photo", Commands: []Command{{
		ID: "import", Name: "Import", New: func() CommandModel {
			return stubCommand{label: "PHOTO IMPORT", captureQ: true}
		},
	}}}}
	m := NewModel(apps, Launch{App: "photo", Command: "import"})
	m = update(t, m, "q")
	active := m.active.(stubCommand)
	if m.confirmQuit || !active.capturedQ {
		t.Fatal("captured q should reach the editor without opening quit confirmation")
	}
}

func TestActiveWorkCanCaptureCtrlCFromShell(t *testing.T) {
	apps := []App{{ID: "photo", Name: "Photo", Commands: []Command{{
		ID: "import", Name: "Import", New: func() CommandModel {
			return stubCommand{label: "PHOTO IMPORT", captureCtrlC: true}
		},
	}}}}
	m := NewModel(apps, Launch{App: "photo", Command: "import"})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	active := m.active.(stubCommand)
	if cmd != nil || !active.capturedCtrlC {
		t.Fatal("captured ctrl+c should reach active work without quitting")
	}
}

func TestHelpIsContextual(t *testing.T) {
	m := NewModel(testApps, Launch{})
	m = update(t, m, "?")
	if !strings.Contains(m.View(), "PICKER HELP") {
		t.Fatalf("picker help missing:\n%s", m.View())
	}

	m = update(t, m, "?")
	m = update(t, m, "enter")
	m = update(t, m, "?")
	if !strings.Contains(m.View(), "COMMAND HELP") {
		t.Fatalf("command help missing:\n%s", m.View())
	}
}

func TestViewFillsTerminalAndBarsStayOneRow(t *testing.T) {
	m := NewModel(testApps, Launch{})
	m.now = time.Date(2026, time.September, 7, 9, 7, 5, 0, time.Local)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	view := m.View()
	if got := lipgloss.Width(view); got != 100 {
		t.Fatalf("view width = %d, want 100", got)
	}
	if got := lipgloss.Height(view); got != 30 {
		t.Fatalf("view height = %d, want 30", got)
	}
	if !strings.Contains(view, "09:07:05") {
		t.Fatalf("top bar does not contain current time:\n%s", view)
	}
	if got := lipgloss.Height(m.topBar(100)); got != 1 {
		t.Fatalf("top bar height = %d, want 1", got)
	}
	if got := lipgloss.Height(m.statusBar(100)); got != 1 {
		t.Fatalf("status bar height = %d, want 1", got)
	}
}

func TestTopBarPlacesTabLeftAndBreadcrumbBeforeClockRight(t *testing.T) {
	m := NewModel(testApps, Launch{App: "photo", Command: "import"})
	m.now = time.Date(2026, time.September, 9, 9, 7, 5, 0, time.Local)
	bar := ansi.Strip(m.topBar(100))
	if !strings.HasPrefix(bar, " PHOTO IMPORT ") {
		t.Fatalf("top bar tab is not left-aligned: %q", bar)
	}
	breadcrumb := strings.Index(bar, "dgs › photo › import")
	clock := strings.Index(bar, "09:07:05")
	if breadcrumb < 0 || clock < 0 || breadcrumb >= clock {
		t.Fatalf("top bar right metadata order is wrong: %q", bar)
	}
}

func TestTopBarShowsLiveSystemRates(t *testing.T) {
	m := NewModel(testApps, Launch{App: "photo", Command: "import"})
	m.now = time.Date(2026, time.September, 9, 9, 7, 5, 0, time.Local)
	m.metrics = shellMetrics{ready: true, cpuPercent: 17, networkUp: 2 * 1024 * 1024, networkDown: 3 * 1024, diskRead: 4 * 1024 * 1024, diskWrite: 5 * 1024}
	bar := ansi.Strip(m.topBar(180))
	for _, want := range []string{"dgs › photo › import │ DISK R  4.0M/s W  5.0K/s │ NET ↑  2.0M/s ↓  3.0K/s │ CPU  17% │ 09:07:05"} {
		if !strings.Contains(bar, want) {
			t.Fatalf("top bar missing %q: %q", want, bar)
		}
	}
}

func TestTopBarMetricCellsKeepStableWidth(t *testing.T) {
	m := NewModel(testApps, Launch{App: "photo", Command: "import"})
	m.metrics = shellMetrics{ready: true, cpuPercent: 1, networkUp: 1, networkDown: 1, diskRead: 1, diskWrite: 1}
	first := ansi.Strip(m.topBarMetadata(180))
	m.metrics = shellMetrics{ready: true, cpuPercent: 100, networkUp: 999 * 1024 * 1024, networkDown: 88 * 1024, diskRead: 7 * 1024 * 1024 * 1024, diskWrite: 66 * 1024 * 1024}
	second := ansi.Strip(m.topBarMetadata(180))
	if lipgloss.Width(first) != lipgloss.Width(second) {
		t.Fatalf("metric metadata width moved from %d to %d:\n%s\n%s", lipgloss.Width(first), lipgloss.Width(second), first, second)
	}
}

func TestTopBarVisibilityCanDisableIndividualGlobalCells(t *testing.T) {
	m := NewModelWithConfig(testApps, Launch{App: "photo", Command: "import"}, dgsconfig.TopBarVisibility{
		Disk: true, Network: false, CPU: true, Time: false,
	})
	m.metrics = shellMetrics{ready: true, cpuPercent: 12, diskRead: 1024, diskWrite: 2048}
	bar := ansi.Strip(m.topBar(140))
	if !strings.Contains(bar, "DISK R") || !strings.Contains(bar, "CPU  12%") {
		t.Fatalf("enabled cells missing: %q", bar)
	}
	if strings.Contains(bar, "NET") || strings.Contains(bar, ":") {
		t.Fatalf("disabled cells remain visible: %q", bar)
	}
}

func TestMetricsCalculateRatesFromCounterDeltas(t *testing.T) {
	var metrics shellMetrics
	start := time.Date(2026, time.September, 9, 9, 0, 0, 0, time.UTC)
	metrics.update(metricCounters{at: start, cpuTotal: 100, cpuIdle: 80, networkRead: 1000, networkWrite: 2000, diskRead: 3000, diskWrite: 4000})
	metrics.update(metricCounters{at: start.Add(2 * time.Second), cpuTotal: 200, cpuIdle: 120, networkRead: 5000, networkWrite: 8000, diskRead: 11000, diskWrite: 14000})
	if !metrics.ready || metrics.cpuPercent != 60 || metrics.networkDown != 2000 || metrics.networkUp != 3000 || metrics.diskRead != 4000 || metrics.diskWrite != 5000 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestBarsNeverWrapInNarrowTerminal(t *testing.T) {
	m := NewModel(testApps, Launch{App: "photo", Command: "encode"})
	for _, bar := range []string{m.topBar(18), m.statusBar(18)} {
		if got := lipgloss.Width(bar); got != 18 {
			t.Fatalf("bar width = %d, want 18: %q", got, bar)
		}
		if got := lipgloss.Height(bar); got != 1 {
			t.Fatalf("bar height = %d, want 1: %q", got, bar)
		}
	}
}

func TestStatusCenterUsesWholeBarMidpoint(t *testing.T) {
	width := 100
	center := "● Processing"
	bar := renderStatusBar(Status{
		Left:   "PROCESSING · PAUSED",
		Center: center,
		Right:  "p Resume  esc Back  q Quit",
	}, width)
	plain := ansi.Strip(bar)
	start := strings.Index(plain, center)
	if start < 0 {
		t.Fatalf("center content missing: %q", plain)
	}
	contentMidpoint := start + lipgloss.Width(center)/2
	if delta := contentMidpoint - width/2; delta < -1 || delta > 1 {
		t.Fatalf("center midpoint = %d, whole bar midpoint = %d: %q", contentMidpoint, width/2, plain)
	}
}

func update(t *testing.T, m Model, key string) Model {
	t.Helper()
	keyMap := map[string]tea.KeyType{
		"up":    tea.KeyUp,
		"down":  tea.KeyDown,
		"enter": tea.KeyEnter,
		"esc":   tea.KeyEsc,
	}
	keyType, special := keyMap[key]
	msg := tea.KeyMsg{Type: keyType}
	if !special {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	updated, _ := m.Update(msg)
	result, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want Model", updated)
	}
	return result
}
