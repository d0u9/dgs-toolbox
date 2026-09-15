package tui

import (
	"dgs-toolbox/internal/tui/stepper"
	"slices"
	"strings"
	"testing"
	"time"

	dgsconfig "dgs-toolbox/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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
	if got := m.breadcrumbTrail(); !slices.Equal(got, []string{"demo", "dgs"}) {
		t.Fatalf("breadcrumb trail = %q, want the command then the shell", got)
	}
}

type contextualStub struct{ stubCommand }

func (m contextualStub) CommandPath() []string { return []string{"scan"} }
func (m contextualStub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.stubCommand.Update(msg)
	m.stubCommand = updated.(stubCommand)
	return m, cmd
}

type tabbedStub struct{ stubCommand }

func (m tabbedStub) Tabs() []Tab { return []Tab{{Label: "scan", Active: true}} }
func (m tabbedStub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.stubCommand.Update(msg)
	m.stubCommand = updated.(stubCommand)
	return m, cmd
}

func TestCommandCanContributeTopBarTabs(t *testing.T) {
	apps := []App{{ID: "capture", Name: "Capture", Direct: true, Commands: []Command{{
		ID: "scan", Name: "Scan", New: func() CommandModel {
			return tabbedStub{stubCommand{label: "SCAN"}}
		},
	}}}}
	m := NewModel(apps, Launch{App: "capture", Command: "scan"})
	bar := ansi.Strip(m.topBar(100))
	if !strings.HasPrefix(bar, "  SCAN   ") {
		t.Fatalf("top bar does not show the active Scan tab at its minimum width: %q", bar)
	}
	if !strings.Contains(bar, "capture "+breadcrumbSeparator+" dgs") {
		t.Fatalf("top bar is missing the Capture breadcrumb: %q", bar)
	}
}

func TestCommandCanContributeBreadcrumbState(t *testing.T) {
	apps := []App{{ID: "photo", Name: "Photo", Commands: []Command{{
		ID: "import", Name: "Import", New: func() CommandModel { return contextualStub{stubCommand{label: "IMPORT"}} },
	}}}}
	m := NewModel(apps, Launch{App: "photo", Command: "import"})
	if got := m.breadcrumbTrail(); !slices.Equal(got, []string{"scan", "import", "photo", "dgs"}) {
		t.Fatalf("breadcrumb trail = %q, want it deepest first", got)
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
	if !strings.Contains(ansi.Strip(m.View()), "encode "+breadcrumbSeparator+" photo "+breadcrumbSeparator+" dgs") {
		t.Fatalf("active command breadcrumb missing:\n%s", m.View())
	}

	m = leaveActive(t, m)
	if m.active != nil || !strings.Contains(m.View(), "CHOOSE A COMMAND") {
		t.Fatalf("leaving did not return to the global picker:\n%s", m.View())
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
	m = leaveActive(t, m)
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
	if !strings.Contains(ansi.Strip(m.View()), "commands "+breadcrumbSeparator+" gpx "+breadcrumbSeparator+" dgs") {
		t.Fatalf("domain picker breadcrumb missing:\n%s", m.View())
	}
}

func TestDirectCommandEscapeReturnsToParentPickerThenConfirmsExit(t *testing.T) {
	m := NewModel(testApps, Launch{App: "photo", Command: "import"})
	if m.active == nil || !strings.Contains(m.View(), "PHOTO IMPORT") {
		t.Fatalf("direct command did not start:\n%s", m.View())
	}

	m = leaveActive(t, m)
	if m.active != nil {
		t.Fatal("leaving should return to the parent picker")
	}
	if !strings.Contains(ansi.Strip(m.View()), "commands "+breadcrumbSeparator+" photo "+breadcrumbSeparator+" dgs") {
		t.Fatalf("parent picker breadcrumb missing:\n%s", m.View())
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil || !m.confirmQuit || !strings.Contains(m.View(), "QUIT DGS?") {
		t.Fatal("escape from the picker should request confirmation")
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
	if view := m.View(); !strings.Contains(view, "Tab switch") || !strings.Contains(view, "› No") {
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
	breadcrumb := strings.Index(bar, "import "+breadcrumbSeparator+" photo "+breadcrumbSeparator+" dgs")
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
	for _, want := range []string{"import " + breadcrumbSeparator + " photo " + breadcrumbSeparator + " dgs  │ DISK R  4.0M/s W  5.0K/s │ NET ↑  2.0M/s ↓  3.0K/s │ CPU  17% │ 09:07:05"} {
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

// leaveActive walks the way out of a command: esc asks, and the answer is
// given the way a reader gives it. Leaving is guarded because esc is one key
// away from the esc that walks back a column inside a command.
func leaveActive(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if !m.confirmLeave {
		t.Fatal("escape did not ask before leaving the command")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model)
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

type twoTabStub struct {
	stubCommand
	active int
}

func (m twoTabStub) Tabs() []Tab {
	return []Tab{{Label: "scan", Active: m.active == 0}, {Label: "route", Active: m.active == 1}}
}

func (m twoTabStub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if selected, ok := msg.(TabSelectedMsg); ok {
		m.active = selected.Index
		return m, nil
	}
	updated, cmd := m.stubCommand.Update(msg)
	m.stubCommand = updated.(stubCommand)
	return m, cmd
}

func TestShortTabsKeepAMinimumWidth(t *testing.T) {
	apps := []App{{ID: "capture", Name: "Capture", Direct: true, Commands: []Command{{
		ID: "scan", Name: "Scan", New: func() CommandModel { return twoTabStub{stubCommand: stubCommand{label: "SCAN"}} },
	}}}}
	m := NewModel(apps, Launch{App: "capture", Command: "scan"})
	bar := ansi.Strip(m.topBar(100))
	if !strings.HasPrefix(bar, "  SCAN      ROUTE  ") {
		t.Fatalf("tabs are not padded to the minimum width and separated: %q", bar)
	}
	if got := lipgloss.Width(renderTab(topBarTabStyle, "destinations")); got != len("destinations")+2 {
		t.Fatalf("long tab width = %d, want %d", got, len("destinations")+2)
	}
	if topBarInactiveTabStyle.GetBackground() == topBarStyle.GetBackground() {
		t.Fatal("inactive tabs must be painted so their click target is visible")
	}
}

func TestClickingATabTellsTheCommandWhichTabWasSelected(t *testing.T) {
	apps := []App{{ID: "capture", Name: "Capture", Direct: true, Commands: []Command{{
		ID: "scan", Name: "Scan", New: func() CommandModel { return twoTabStub{stubCommand: stubCommand{label: "SCAN"}} },
	}}}}
	m := NewModel(apps, Launch{App: "capture", Command: "scan"})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(Model)

	bar := ansi.Strip(m.topBar(100))
	routeX := strings.Index(bar, "ROUTE")
	if routeX < 0 {
		t.Fatalf("top bar is missing the Route tab: %q", bar)
	}
	click := tea.MouseMsg{X: routeX, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	updated, _ = m.Update(click)
	m = updated.(Model)
	if got := m.active.(twoTabStub).active; got != 1 {
		t.Fatalf("clicking ROUTE selected tab %d, want 1", got)
	}

	click.X = 1
	updated, _ = m.Update(click)
	m = updated.(Model)
	if got := m.active.(twoTabStub).active; got != 0 {
		t.Fatalf("clicking SCAN selected tab %d, want 0", got)
	}
}

// Leaving a command is guarded because esc is the same key that walks back a
// column inside one: the dialog names the command, and staying leaves the
// session exactly as it was.
func TestLeavingACommandIsConfirmed(t *testing.T) {
	m := NewModel(testApps, Launch{App: "photo", Command: "import"})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil || m.active == nil {
		t.Fatal("escape left the command without asking")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "LEAVE PHOTO IMPORT?") {
		t.Fatalf("the dialog does not name the command:\n%s", view)
	}

	// Staying is the safe answer, and the one the cursor starts on.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.confirmLeave || m.active == nil {
		t.Fatal("the default answer did not keep the command")
	}
	if !strings.Contains(m.View(), "PHOTO IMPORT") {
		t.Fatalf("the command is not back on screen:\n%s", m.View())
	}

	// Escape answers the dialog rather than leaving as well.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.confirmLeave || m.active == nil {
		t.Fatal("escape in the dialog should cancel it, not leave the command")
	}
}

// TestStatusBarFitsTerminalWidthWithStyledContent renders with a real color
// profile: without escape sequences, style resets cannot add stray cells, and
// a plain-text test passes while the terminal overflows.
func TestStatusBarFitsTerminalWidthWithStyledContent(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	steps := []string{"Directories", "Parameters", "Processing", "Post-processing", "Result"}
	hints := []string{
		"n Next  r Refresh  space Preview  alt+hjkl Focus  esc Back",
		"p Pause  ↑↓ Workers  esc Back  q Quit",
		"n Next  esc Prev  q Quit",
		"r Again  ↑↓ Browse  alt+h/l Focus  q Quit",
	}
	for current := range steps {
		for _, right := range hints {
			for width := 40; width <= 240; width += 3 {
				status := Status{Left: "POST-PROCESSING · COMPLETE", Center: stepper.View(steps, current, width*56/100), Right: right}
				bar := renderStatusBar(status, width)
				if got := ansi.StringWidth(bar); got != width {
					t.Fatalf("step %d width %d: rendered %d cells: %q", current, width, got, ansi.Strip(bar))
				}
				plain := ansi.Strip(bar)
				if !strings.Contains(plain, strings.Split(right, "  ")[0]) {
					t.Fatalf("step %d width %d: most immediate hint missing: %q", current, width, plain)
				}
				if strings.HasSuffix(strings.TrimSpace(plain), "…") {
					t.Fatalf("step %d width %d: hint cut mid-word: %q", current, width, plain)
				}
			}
		}
	}
}
