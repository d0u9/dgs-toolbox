package capture

import (
	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// session owns the Capture command-level tabs. Scan, Route and Archive are
// sibling sessions sharing one Capture root; the shell renders the tab bar from
// Tabs(). They are in the order a Capture passes through them: it is looked at,
// organized, and then put away.
type session struct {
	scan    model
	route   routeModel
	archive archiveModel
	active  int
}

// sessionCount is how many tabs Capture has, kept in one place so a fourth one
// is added by naming it rather than by finding every 3.
const sessionCount = 3

func newSession() session {
	return newSessionWithSettings("", "index.json", "", "", organizer.Set{}, organizer.DefaultSettings())
}

func newSessionWithSettings(root, indexFile, archiveRoot, rejectRoot string, recipes organizer.Set, settings organizer.Settings) session {
	scan := newModelWithSettings(root, indexFile)
	return session{
		scan:    scan,
		route:   newRouteModelWithSettings(scan.root, scan.indexFile, recipes, settings),
		archive: newArchiveModel(scan.root, scan.indexFile, archiveRoot, rejectRoot),
	}
}

// Init starts the shared Capture load and lets Archive read the two folders it
// files into, which are its own rather than part of that shared load.
func (s session) Init() tea.Cmd { return tea.Batch(s.scan.Init(), s.archive.Init()) }

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
		if msg.Index >= 0 && msg.Index < sessionCount {
			s.active = msg.Index
		}
		return s, nil
	case tea.KeyMsg:
		if s.switchesTab(msg.String()) {
			step := 1
			if msg.String() == "[" {
				step = sessionCount - 1
			}
			s.active = (s.active + step) % sessionCount
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
	switch s.active {
	case 1:
		return !s.route.rootControl.Picking()
	case 2:
		return !s.archive.rootControl.Picking()
	}
	return !s.scan.rootControl.Picking()
}

func (s session) activeModel() tea.Model {
	switch s.active {
	case 1:
		return s.route
	case 2:
		return s.archive
	}
	return s.scan
}

func (s session) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	scan, scanCmd := s.scan.Update(msg)
	s.scan = scan.(model)
	route, routeCmd := s.route.Update(msg)
	s.route = route.(routeModel)
	archive, archiveCmd := s.archive.Update(msg)
	s.archive = archive.(archiveModel)
	return s, tea.Batch(scanCmd, routeCmd, archiveCmd)
}

func (s session) updateActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch s.active {
	case 1:
		updated, cmd := s.route.Update(msg)
		s.route = updated.(routeModel)
		return s, cmd
	case 2:
		updated, cmd := s.archive.Update(msg)
		s.archive = updated.(archiveModel)
		return s, cmd
	}
	updated, cmd := s.scan.Update(msg)
	s.scan = updated.(model)
	return s, cmd
}

func (s session) View() string { return s.activeModel().View() }

func (s session) Status() tui.Status {
	if provider, ok := s.activeModel().(interface{ Status() tui.Status }); ok {
		return provider.Status()
	}
	return tui.Status{}
}

func (s session) Tabs() []tui.Tab {
	return []tui.Tab{
		{Label: "Scan", Active: s.active == 0},
		{Label: "Route", Active: s.active == 1},
		{Label: "Archive", Active: s.active == 2},
	}
}

func (s session) CapturesShellKey(key string) bool {
	if capturer, ok := s.activeModel().(tui.ShellKeyCapturer); ok {
		return capturer.CapturesShellKey(key)
	}
	return false
}
