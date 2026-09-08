// Package contextmenu provides a reusable, anchored right-click menu.
package contextmenu

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Item struct{ ID, Label string }

type Model struct {
	items  []Item
	cursor int
	x, y   int
	open   bool
}

var (
	border = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}).Padding(0, 1)
	active = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"})
)

func New(items ...Item) Model        { return Model{items: append([]Item(nil), items...)} }
func (m *Model) Open(x, y int)       { m.x, m.y, m.cursor, m.open = x, y, 0, true }
func (m *Model) Close()              { m.open = false }
func (m Model) IsOpen() bool         { return m.open }
func (m Model) Position() (int, int) { return m.x, m.y }
func (m Model) Width() int {
	w := 0
	for _, item := range m.items {
		w = max(w, lipgloss.Width(item.Label)+2)
	}
	return w + 4
}
func (m Model) Height() int { return len(m.items) + 2 }
func (m *Model) Move(delta int) {
	if len(m.items) > 0 {
		m.cursor = (m.cursor + delta + len(m.items)) % len(m.items)
	}
}
func (m Model) Selected() (Item, bool) {
	if !m.open || len(m.items) == 0 {
		return Item{}, false
	}
	return m.items[m.cursor], true
}
func (m *Model) Click(x, y int) (Item, bool) {
	row := y - m.y - 1
	if !m.open || x < m.x || x >= m.x+m.Width() || row < 0 || row >= len(m.items) {
		return Item{}, false
	}
	m.cursor = row
	item := m.items[row]
	m.Close()
	return item, true
}
func (m Model) View() string {
	lines := make([]string, len(m.items))
	for i, item := range m.items {
		prefix := "  "
		if i == m.cursor {
			prefix = active.Render("› ")
		}
		lines[i] = prefix + item.Label
	}
	return border.Render(strings.Join(lines, "\n"))
}
