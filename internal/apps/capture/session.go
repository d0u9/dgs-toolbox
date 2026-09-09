package capture

import (
	"dgs-toolbox/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// session owns the Capture command-level tabs. Scan and Route are sibling
// sessions sharing one Capture root; the shell renders the tab bar from Tabs().
type session struct {
	scan   model
	route  routeModel
	active int
}

func newSession() session {
	return newSessionWithSettings("", "index.json")
}

func newSessionWithSettings(root, indexFile string) session {
	scan := newModelWithSettings(root, indexFile)
	return session{scan: scan, route: newRouteModel(scan.root, scan.indexFile)}
}

func (s session) Init() tea.Cmd { return s.scan.Init() }

func (s session) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return s.broadcast(msg)
	case capturesLoadedMsg:
		return s.broadcast(msg)
	case rootChangedMsg:
		updated, cmd := s.broadcast(msg)
		return updated, tea.Batch(cmd, loadCaptures(msg.root, s.scan.indexFile))
	case tui.TabSelectedMsg:
		if msg.Index >= 0 && msg.Index < 2 {
			s.active = msg.Index
		}
		return s, nil
	case tea.KeyMsg:
		if s.switchesTab(msg.String()) {
			s.active = (s.active + 1) % 2
			return s, nil
		}
	}
	return s.updateActive(msg)
}

// switchesTab reports whether a key moves between Capture sessions. Keys are
// ignored while the active session owns them, so overlays and text input keep
// their meaning.
func (s session) switchesTab(key string) bool {
	switch key {
	case "[", "]":
	default:
		return false
	}
	if capturer, ok := s.activeModel().(tui.ShellKeyCapturer); ok && capturer.CapturesShellKey(key) {
		return false
	}
	if s.active == 1 {
		return !s.route.rootControl.Picking()
	}
	return !s.scan.rootControl.Picking()
}

func (s session) activeModel() tea.Model {
	if s.active == 1 {
		return s.route
	}
	return s.scan
}

func (s session) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	scan, scanCmd := s.scan.Update(msg)
	s.scan = scan.(model)
	route, routeCmd := s.route.Update(msg)
	s.route = route.(routeModel)
	return s, tea.Batch(scanCmd, routeCmd)
}

func (s session) updateActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.active == 1 {
		updated, cmd := s.route.Update(msg)
		s.route = updated.(routeModel)
		return s, cmd
	}
	updated, cmd := s.scan.Update(msg)
	s.scan = updated.(model)
	return s, cmd
}

func (s session) View() string {
	if s.active == 1 {
		return s.route.View()
	}
	return s.scan.View()
}

func (s session) Status() tui.Status {
	if s.active == 1 {
		return s.route.Status()
	}
	return s.scan.Status()
}

func (s session) Tabs() []tui.Tab {
	return []tui.Tab{
		{Label: "Scan", Active: s.active == 0},
		{Label: "Route", Active: s.active == 1},
	}
}

func (s session) CapturesShellKey(key string) bool {
	if capturer, ok := s.activeModel().(tui.ShellKeyCapturer); ok {
		return capturer.CapturesShellKey(key)
	}
	return false
}
