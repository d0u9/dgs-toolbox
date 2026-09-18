// Package scrolllist provides a numbered, selectable viewport for reusable TUI lists.
package scrolllist

import (
	"fmt"
	"strings"

	"dgs-toolbox/internal/tui/divider"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type Item struct {
	ID    string
	Label string
	// Detail is an optional second line rendered under Label in the muted
	// style, for entries whose identity does not fit one row. When any item
	// carries a Detail every item occupies two rows, so rows stay aligned and
	// the cursor never changes height as it moves.
	Detail string
}

type Model struct {
	items      []Item
	cursor     int
	top        int
	horizontal int
	width      int
	height     int
	// divider splits the list in two at this index, labelled by dividerLabel.
	// It is a rule drawn between items rather than an item of its own, so it
	// is never selectable and never shifts the numbering.
	divider      int
	dividerLabel string
	// hideNumbers drops the line-number column. A list whose rows carry
	// their own structure — a tree, where depth is drawn into the label —
	// gets nothing from a number that counts visible rows rather than
	// items, and folding renumbers everything under the cursor. See
	// docs/scroll-lists.md#line-numbers.
	hideNumbers bool
}

var selectedRowStyle = lipgloss.NewStyle().Bold(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}).
	Background(lipgloss.AdaptiveColor{Light: "#DDF3F0", Dark: "#173F3B"})

// SelectedRowStyle is how a list marks the row under the cursor. It is exported
// so a hand-rendered list elsewhere marks its selection the same way: one
// screen should not highlight two lists differently.
func SelectedRowStyle() lipgloss.Style { return selectedRowStyle }

func New() Model { return Model{width: 20, height: 4, divider: -1} }

// SetDivider draws a labelled rule above the item at index, splitting the list
// into two runs. A negative index, or one at either end of the list, removes
// it: a rule with nothing on one side of it separates nothing.
func (m *Model) SetDivider(index int, label string) {
	if index <= 0 || index >= len(m.items) {
		m.divider = -1
		m.dividerLabel = ""
		return
	}
	m.divider, m.dividerLabel = index, label
}

// hasDivider reports whether a rule is currently drawn between two items.
func (m Model) hasDivider() bool { return m.divider > 0 && m.divider < len(m.items) }

// dividerRows is the height the rule takes from the item rows.
func (m Model) dividerRows() int {
	if m.hasDivider() {
		return 1
	}
	return 0
}

func (m *Model) SetItems(items []Item) {
	m.items = append([]Item(nil), items...)
	m.cursor = min(m.cursor, max(0, len(m.items)-1))
	if m.divider >= len(m.items) {
		m.divider, m.dividerLabel = -1, ""
	}
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

// rowsPerItem is 2 once any item carries a Detail line, and 1 otherwise.
func (m Model) rowsPerItem() int {
	for _, item := range m.items {
		if item.Detail != "" {
			return 2
		}
	}
	return 1
}

// visibleItems is how many items fit in the current height. The rule costs one
// row whenever the list carries one, whether or not it is currently scrolled
// into view, so the count does not change as the viewport moves.
func (m Model) visibleItems() int {
	return max(1, (m.height-m.dividerRows())/m.rowsPerItem())
}

func (m *Model) Pan(delta int) {
	maxPan := 0
	labelWidth := max(1, m.width-m.LabelOffset())
	for _, item := range m.items {
		maxPan = max(maxPan, lipgloss.Width(item.Label)-labelWidth)
		maxPan = max(maxPan, lipgloss.Width(item.Detail)-labelWidth)
	}
	m.horizontal = min(maxPan, max(0, m.horizontal+delta))
}

// Scroll changes the viewport and keeps selection inside the visible rows.
func (m *Model) Scroll(delta int) {
	maxTop := max(0, len(m.items)-m.visibleItems())
	m.top = min(maxTop, max(0, m.top+delta))
	if len(m.items) > 0 {
		m.cursor = min(max(m.cursor, m.top), min(len(m.items)-1, m.top+m.visibleItems()-1))
	}
}

// SelectRow selects the item occupying a zero-based row of the visible
// viewport. With two-line items either row selects that item.
func (m *Model) SelectRow(row int) bool {
	if row < 0 || row >= m.height {
		return false
	}
	if m.hasDivider() && m.divider > m.top {
		offset := (m.divider - m.top) * m.rowsPerItem()
		if row == offset {
			// The rule itself is not selectable.
			return false
		}
		if row > offset {
			row--
		}
	}
	index := m.top + row/m.rowsPerItem()
	if index >= len(m.items) {
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
	labelWidth := max(1, m.width-m.LabelOffset())
	end := min(len(m.items), m.top+m.visibleItems())
	lines := make([]string, 0, m.height)
	detailIndent := strings.Repeat(" ", min(m.width, m.LabelOffset()))
	for index := m.top; index < end; index++ {
		if m.hasDivider() && index == m.divider && index > m.top {
			lines = append(lines, dividerRow(m.dividerLabel, m.width))
		}
		marker := "  "
		selected := focused && index == m.cursor
		if selected {
			marker = "› "
		}
		label := ansi.Cut(m.items[index].Label, m.horizontal, m.horizontal+labelWidth)
		row := marker + label
		if !m.hideNumbers {
			numberText := fmt.Sprintf("%*d", numberWidth, index+1)
			number := muted.Render(numberText)
			if selected {
				number = numberText
			}
			row = marker + number + " " + label
		}
		if index == m.top && m.top > 0 {
			row = overflowMarkedRow(row, m.width, "↑ more")
		}
		clipped := index == end-1 && end < len(m.items)
		if clipped && m.rowsPerItem() == 1 {
			row = overflowMarkedRow(row, m.width, "↓ more")
		}
		if selected {
			row = selectedRowStyle.Width(m.width).Render(row)
		}
		lines = append(lines, row)
		if m.rowsPerItem() == 1 {
			continue
		}
		detail := detailIndent + ansi.Cut(m.items[index].Detail, m.horizontal, m.horizontal+labelWidth)
		// In two-row mode the detail is the item's last rendered row, so the
		// clipping marker belongs there rather than on the label above it.
		if clipped {
			detail = overflowMarkedRow(detail, m.width, "↓ more")
		}
		if selected {
			detail = selectedRowStyle.Width(m.width).Render(detail)
		} else {
			detail = muted.Render(detail)
		}
		lines = append(lines, detail)
	}
	for len(lines) < m.height {
		lines = append(lines, strings.Repeat(" ", m.width))
	}
	return strings.Join(lines, "\n")
}

// dividerRow draws the shared anchored rule, labelled so the run below it is
// named rather than merely separated. It is the same divider used between
// sections elsewhere in the TUI, so a split list reads as one familiar thing.
func dividerRow(label string, width int) string {
	return divider.Labelled(label, width)
}

func overflowMarkedRow(row string, width int, marker string) string {
	return ansi.Truncate(row, max(1, width-lipgloss.Width(marker)-1), "…") + " " + marker
}

// LabelOffset is how many cells precede an item's label: the cursor marker,
// the number, and the space after it. A caller that wants to know which part of
// a row was clicked needs it, because the numbering width depends on the list.
func (m Model) LabelOffset() int {
	if m.hideNumbers {
		return 2 // the cursor marker alone.
	}
	return 3 + m.numberWidth()
}

func (m Model) numberWidth() int {
	if m.hideNumbers {
		return 0
	}
	return len(fmt.Sprintf("%d", max(1, len(m.items))))
}

// HideNumbers drops the line-number column, for a list whose rows already
// carry their own structure. It is off by default: a plain collection is
// easier to talk about with numbers than without.
func (m *Model) HideNumbers(hide bool) { m.hideNumbers = hide }
func (m *Model) ensureVisible() {
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+m.visibleItems() {
		m.top = m.cursor - m.visibleItems() + 1
	}
	m.clamp()
}
func (m *Model) clamp() { m.top = min(max(0, len(m.items)-m.visibleItems()), max(0, m.top)) }

// ItemAt is the item at index, or false past the end.
func (m Model) ItemAt(index int) (Item, bool) {
	if index < 0 || index >= len(m.items) {
		return Item{}, false
	}
	return m.items[index], true
}
