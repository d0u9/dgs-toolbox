// Package scrolllist provides a numbered, selectable viewport for reusable TUI lists.
package scrolllist

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type Item struct {
	ID    string
	Label string
}

type Model struct {
	items      []Item
	cursor     int
	top        int
	horizontal int
	width      int
	height     int
}

var selectedRowStyle = lipgloss.NewStyle().Bold(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}).
	Background(lipgloss.AdaptiveColor{Light: "#DDF3F0", Dark: "#173F3B"})

func New() Model { return Model{width: 20, height: 4} }

func (m *Model) SetItems(items []Item) {
	m.items = append([]Item(nil), items...)
	m.cursor = min(m.cursor, max(0, len(m.items)-1))
	m.clamp()
}

func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(1, width), max(1, height)
	m.clamp()
}

func (m Model) Cursor() int     { return m.cursor }
func (m Model) Top() int        { return m.top }
func (m Model) Horizontal() int { return m.horizontal }
func (m Model) Selected() (Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return Item{}, false
	}
	return m.items[m.cursor], true
}

// SelectID focuses the first item with the requested stable identifier.
func (m *Model) SelectID(id string) bool {
	for index, item := range m.items {
		if item.ID == id {
			m.cursor = index
			m.ensureVisible()
			return true
		}
	}
	return false
}

func (m *Model) Move(delta int) {
	if len(m.items) == 0 {
		return
	}
	m.cursor = min(len(m.items)-1, max(0, m.cursor+delta))
	m.ensureVisible()
}

func (m *Model) First() { m.cursor, m.top = 0, 0 }
func (m *Model) Last() {
	m.cursor = max(0, len(m.items)-1)
	m.ensureVisible()
}

func (m *Model) Pan(delta int) {
	maxPan := 0
	labelWidth := max(1, m.width-m.numberWidth()-3)
	for _, item := range m.items {
		maxPan = max(maxPan, lipgloss.Width(item.Label)-labelWidth)
	}
	m.horizontal = min(maxPan, max(0, m.horizontal+delta))
}

// Scroll changes the viewport and keeps selection inside the visible rows.
func (m *Model) Scroll(delta int) {
	maxTop := max(0, len(m.items)-m.height)
	m.top = min(maxTop, max(0, m.top+delta))
	if len(m.items) > 0 {
		m.cursor = min(max(m.cursor, m.top), min(len(m.items)-1, m.top+m.height-1))
	}
}

// SelectRow selects a zero-based row within the visible viewport.
func (m *Model) SelectRow(row int) bool {
	index := m.top + row
	if row < 0 || row >= m.height || index >= len(m.items) {
		return false
	}
	m.cursor = index
	return true
}

func (m Model) View(focused bool, _ lipgloss.Style, muted lipgloss.Style) string {
	if len(m.items) == 0 {
		return muted.Render("· No items")
	}
	numberWidth := m.numberWidth()
	labelWidth := max(1, m.width-numberWidth-3)
	end := min(len(m.items), m.top+m.height)
	lines := make([]string, 0, m.height)
	for index := m.top; index < end; index++ {
		marker := "  "
		selected := focused && index == m.cursor
		if selected {
			marker = "› "
		}
		numberText := fmt.Sprintf("%*d", numberWidth, index+1)
		number := muted.Render(numberText)
		if selected {
			number = numberText
		}
		label := ansi.Cut(m.items[index].Label, m.horizontal, m.horizontal+labelWidth)
		row := marker + number + " " + label
		if index == m.top && m.top > 0 {
			row = overflowMarkedRow(row, m.width, "↑ more")
		}
		if index == end-1 && end < len(m.items) {
			row = overflowMarkedRow(row, m.width, "↓ more")
		}
		if selected {
			row = selectedRowStyle.Width(m.width).Render(row)
		}
		lines = append(lines, row)
	}
	for len(lines) < m.height {
		lines = append(lines, strings.Repeat(" ", m.width))
	}
	return strings.Join(lines, "\n")
}

func overflowMarkedRow(row string, width int, marker string) string {
	return ansi.Truncate(row, max(1, width-lipgloss.Width(marker)-1), "…") + " " + marker
}

func (m Model) numberWidth() int { return len(fmt.Sprintf("%d", max(1, len(m.items)))) }
func (m *Model) ensureVisible() {
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+m.height {
		m.top = m.cursor - m.height + 1
	}
	m.clamp()
}
func (m *Model) clamp() { m.top = min(max(0, len(m.items)-m.height), max(0, m.top)) }
