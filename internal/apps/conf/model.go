// Package conf implements dgs conf export's tree: discovering a generator
// root's inventory and letting the reader check the targets to render.
// Exporting is a later milestone of docs/apps/conf/export.md; this package
// is the page's selection surface.
package conf

import (
	"fmt"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/target"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(
		lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"},
	)
	mutedStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"},
	)
	brokenStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"},
	)
)

// Model is dgs conf export's tree: nodes (and unmanaged users) holding
// instances, with the tri-state checklist behaviour
// docs/apps/cred/vault.md#the-flow's recipient checklist already uses.
type Model struct {
	rootPath   string
	secretsDir string
	loadErr    error

	invRoot     *inventory.Root
	manifests   map[string]confgen.Manifest
	serviceDirs map[string]string
	exports     map[string]confgen.Export
	exportDirs  map[string]string
	derived     *derive.Model
	nodes       []*nodeGroup
	rows        []row
	checked     map[string]bool
	list        scrolllist.Model
	preview     *previewState

	width, height int
	pendingG      bool
}

// New loads rootPath and builds the tree. An empty rootPath, or one that
// fails to load, is not fatal: the page says so and waits, since where the
// generator root is configured (conf.root) is the reader's own decision.
// secretsDir is conf.secrets, read only when a preview needs an instance's
// own secrets or a principal's.
func newModel(rootPath, secretsDir string) Model {
	m := Model{rootPath: rootPath, secretsDir: secretsDir, checked: map[string]bool{}, list: scrolllist.New()}
	if rootPath == "" {
		return m
	}

	l, err := load(rootPath)
	if err != nil {
		m.loadErr = err
		return m
	}
	m.invRoot = l.inv
	m.manifests = l.manifests
	m.serviceDirs = l.serviceDirs
	m.exports = l.exports
	m.exportDirs = l.exportDirs
	m.derived = l.derived

	m.nodes = buildTree(target.List(l.inv, l.derived))
	m.refresh()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if len(m.rows) == 0 {
			return m, nil
		}
		if m.preview != nil {
			m.handlePreviewKey(msg.String())
			return m, nil
		}
		m.handleKey(msg.String())
		return m, nil
	}
	return m, nil
}

// CapturesShellKey keeps Esc closing the preview instead of leaving the
// command for the picker, the same way an open dialog elsewhere in the
// toolbox holds onto it.
func (m Model) CapturesShellKey(key string) bool {
	return m.preview != nil && key == "esc"
}

func (m *Model) handleKey(key string) {
	if key != "g" {
		m.pendingG = false
	}
	switch key {
	case "up", "k":
		m.list.Move(-1)
	case "down", "j":
		m.list.Move(1)
	case "home":
		m.list.First()
	case "end", "G":
		m.list.Last()
	case "g":
		if m.pendingG {
			m.list.First()
			m.pendingG = false
		} else {
			m.pendingG = true
		}
	case " ", "space", "x":
		m.toggle()
	case "enter":
		m.openPreview()
	case "right", "l":
		if node := m.expandable(); node != nil {
			*node = true
			m.refresh()
		}
	case "o":
		if node := m.expandable(); node != nil {
			*node = !*node
			m.refresh()
		}
	case "left", "h":
		if node := m.expandable(); node != nil && *node {
			*node = false
			m.refresh()
		} else {
			m.toParent()
		}
	case "w":
		for _, n := range m.nodes {
			n.expanded = false
		}
		m.refresh()
		m.list.First()
	}
}

// expandable returns the expanded flag of the row under the cursor, when it
// is a node; nil otherwise.
func (m Model) expandable() *bool {
	item, ok := m.list.Selected()
	if !ok {
		return nil
	}
	for i := range m.rows {
		if m.rows[i].id == item.ID && m.rows[i].node != nil {
			return &m.rows[i].node.expanded
		}
	}
	return nil
}

// toParent moves the cursor from an instance row to its node row.
func (m *Model) toParent() {
	item, ok := m.list.Selected()
	if !ok {
		return
	}
	// The row's own id names its parent by convention (see refresh):
	// "inst:<node>/<instance>".
	if len(item.ID) > 5 && item.ID[:5] == "inst:" {
		rest := item.ID[5:]
		if i := lastSlash(rest); i >= 0 {
			m.list.SelectID("node:" + rest[:i])
		}
	}
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// toggle checks or unchecks every key the row under the cursor stands for.
func (m *Model) toggle() {
	item, ok := m.list.Selected()
	if !ok {
		return
	}
	for _, r := range m.rows {
		if r.id != item.ID || r.checkbox == "" {
			continue
		}
		all := r.checkbox == "[x]"
		for _, k := range r.keys {
			m.checked[k] = !all
		}
		m.refresh()
		return
	}
}

func (m Model) View() string {
	if m.preview != nil {
		return m.previewView()
	}
	if m.rootPath == "" {
		return m.centered(titleStyle.Render("dgs conf export") + "\n\n" +
			mutedStyle.Render("no generator root configured — set conf.root"))
	}
	if m.loadErr != nil {
		return m.centered(titleStyle.Render("dgs conf export") + "\n\n" +
			brokenStyle.Render(m.loadErr.Error()))
	}
	if len(m.nodes) == 0 {
		return m.centered(titleStyle.Render("dgs conf export") + "\n\n" +
			mutedStyle.Render("no node or unmanaged user holds a target"))
	}
	list := m.list
	list.SetSize(m.width, max(1, m.height))
	return list.View(true, titleStyle, mutedStyle)
}

func (m Model) centered(content string) string {
	return lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, content)
}

func (m Model) Status() tui.Status {
	if m.preview != nil {
		if m.preview.err != nil {
			return tui.Status{Left: "PREVIEW · ERROR", Center: m.preview.target, Right: "esc Close"}
		}
		shown := min(len(m.preview.lines), m.preview.scroll+max(1, m.height-2))
		return tui.Status{
			Left:   "PREVIEW",
			Center: fmt.Sprintf("%s · line %d–%d of %d", m.preview.target, m.preview.scroll+1, shown, len(m.preview.lines)),
			Right:  "↑↓ Scroll  esc Close",
		}
	}
	if m.rootPath == "" || m.loadErr != nil {
		return tui.Status{Left: "EXPORT", Center: "no generator root", Right: "esc Back  q Quit"}
	}
	total, selected := m.counts()
	return tui.Status{
		Left:   "EXPORT",
		Center: fmt.Sprintf("%d/%d targets checked", selected, total),
		Right:  "↑↓ Move  space Check  ↵ Preview  ←/→ Fold  esc Back",
	}
}

func (m Model) counts() (total, selected int) {
	for _, n := range m.nodes {
		for _, k := range checkableKeys(n) {
			total++
			if m.checked[k] {
				selected++
			}
		}
	}
	return total, selected
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// renderer is the export page's own rendering, with nothing of the page in
// it — the same one `dgs conf export` uses from the command line.
func (m Model) renderer() renderer {
	return renderer{
		l: loaded{
			inv:         m.invRoot,
			manifests:   m.manifests,
			serviceDirs: m.serviceDirs,
			exports:     m.exports,
			exportDirs:  m.exportDirs,
			derived:     m.derived,
		},
		rootPath:   m.rootPath,
		secretsDir: m.secretsDir,
	}
}
