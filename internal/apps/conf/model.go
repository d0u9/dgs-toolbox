// Package conf implements dgs conf export's tree: discovering a generator
// root and letting the reader check the targets to render. Rendering and
// exporting are later milestones of docs/apps/conf/export.md; this package
// is the page's selection surface.
package conf

import (
	"fmt"

	"dgs-toolbox/internal/conf/confgen"
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

// Model is dgs conf export's tree: services holding roles holding instances,
// with the tri-state checklist behaviour
// docs/apps/cred/vault.md#the-flow's recipient checklist already uses.
type Model struct {
	rootPath string
	loadErr  error

	services []*serviceNode
	rows     []row
	checked  map[string]bool
	list     scrolllist.Model

	width, height int
	pendingG      bool
}

// New loads rootPath and builds the tree. An empty rootPath, or one that
// fails to load, is not fatal: the page says so and waits, since where the
// generator root is configured (conf.root) is the reader's own decision.
func newModel(rootPath string) Model {
	m := Model{rootPath: rootPath, checked: map[string]bool{}, list: scrolllist.New()}
	if rootPath == "" {
		return m
	}
	root, err := confgen.Load(rootPath)
	if err != nil {
		m.loadErr = err
		return m
	}
	m.services = buildTree(root)
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
		m.handleKey(msg.String())
		return m, nil
	}
	return m, nil
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
		for _, s := range m.services {
			s.expanded = false
			for _, r := range s.roles {
				r.expanded = false
			}
		}
		m.refresh()
		m.list.First()
	}
}

// expandable returns the expanded flag of the row under the cursor, when it
// is a service or a role; nil otherwise.
func (m Model) expandable() *bool {
	item, ok := m.list.Selected()
	if !ok {
		return nil
	}
	for i := range m.rows {
		if m.rows[i].id == item.ID {
			switch {
			case m.rows[i].role != nil:
				return &m.rows[i].role.expanded
			case m.rows[i].service != nil:
				return &m.rows[i].service.expanded
			}
		}
	}
	return nil
}

// toParent moves the cursor to the row's parent: a role to its service, an
// instance to its role.
func (m *Model) toParent() {
	item, ok := m.list.Selected()
	if !ok {
		return
	}
	// The row's own id names its parent by convention (see refresh):
	// "role:<service>/<role>" or "inst:<service>/<role>/<instance>".
	switch {
	case len(item.ID) > 5 && item.ID[:5] == "inst:":
		target := item.ID[5:]
		if i := lastSlash(target); i >= 0 {
			m.list.SelectID("role:" + target[:i])
		}
	case len(item.ID) > 5 && item.ID[:5] == "role:":
		rest := item.ID[5:]
		if i := firstSlash(rest); i >= 0 {
			m.list.SelectID("svc:" + rest[:i])
		}
	}
}

func firstSlash(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return i
		}
	}
	return -1
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
	if m.rootPath == "" {
		return m.centered(titleStyle.Render("dgs conf export") + "\n\n" +
			mutedStyle.Render("no generator root configured — set conf.root"))
	}
	if m.loadErr != nil {
		return m.centered(titleStyle.Render("dgs conf export") + "\n\n" +
			brokenStyle.Render(m.loadErr.Error()))
	}
	if len(m.services) == 0 {
		return m.centered(titleStyle.Render("dgs conf export") + "\n\n" +
			mutedStyle.Render("no service holds a "+confgen.ManifestFilename))
	}
	list := m.list
	list.SetSize(m.width, max(1, m.height))
	return list.View(true, titleStyle, mutedStyle)
}

func (m Model) centered(content string) string {
	return lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, content)
}

func (m Model) Status() tui.Status {
	if m.rootPath == "" || m.loadErr != nil {
		return tui.Status{Left: "EXPORT", Center: "no generator root", Right: "esc Back  q Quit"}
	}
	total, selected := m.counts()
	return tui.Status{
		Left:   "EXPORT",
		Center: fmt.Sprintf("%d/%d targets checked", selected, total),
		Right:  "↑↓ Move  space Check  ←/→ Fold  esc Back",
	}
}

func (m Model) counts() (total, selected int) {
	for _, s := range m.services {
		for _, k := range checkableKeys(s) {
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
