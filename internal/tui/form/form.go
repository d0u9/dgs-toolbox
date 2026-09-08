package form

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

type Kind int

// PrimaryActionKey advances from a form using its current values.
const PrimaryActionKey = "n"

const (
	Path Kind = iota
	Option
	Checkbox
	Text
	Radio
	Button
	MultiCheckbox
	Number
)

type Field struct {
	ID        string
	Kind      Kind
	Label     string
	Value     string
	Options   []string
	Selected  []string
	SelectAll bool
	Checked   bool
	Min       int
	Max       int
	Step      int
}

type Model struct {
	fields      []Field
	focus       int
	activeID    string
	original    string
	choice      int
	multiChoice int
}

var (
	accent     = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	muted      = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	focusStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedStyle = lipgloss.NewStyle().Foreground(muted)
	labelStyle = lipgloss.NewStyle().Foreground(muted).Width(16)
	rowStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent).Background(
		lipgloss.AdaptiveColor{Light: "#DDF3F0", Dark: "#173F3B"},
	)
)

func New(fields ...Field) Model {
	return Model{fields: append([]Field(nil), fields...), multiChoice: -1}
}

func (m *Model) UpdateNavigation(key string) bool {
	if len(m.fields) == 0 {
		return false
	}
	switch key {
	case "up", "k", "left", "h", "shift+tab":
		m.focus = (m.focus - 1 + len(m.fields)) % len(m.fields)
		return true
	case "down", "j", "right", "l", "tab":
		m.focus = (m.focus + 1) % len(m.fields)
		return true
	}
	return false
}

// UpdateNavigationWithin moves focus through a visible subset of fields.
func (m *Model) UpdateNavigationWithin(ids []string, key string) bool {
	delta := 0
	switch key {
	case "up", "k", "left", "h", "shift+tab":
		delta = -1
	case "down", "j", "right", "l", "tab":
		delta = 1
	default:
		return false
	}
	if len(ids) == 0 {
		return false
	}
	current := 0
	for index, id := range ids {
		if id == m.FocusedID() {
			current = index
			break
		}
	}
	m.SetFocusID(ids[(current+delta+len(ids))%len(ids)])
	return true
}

func (m Model) FocusedID() string {
	if len(m.fields) == 0 {
		return ""
	}
	return m.fields[m.focus].ID
}

func (m Model) FocusIndex() int { return m.focus }

func (m *Model) SetFocusID(id string) {
	for index := range m.fields {
		if m.fields[index].ID == id {
			m.focus = index
			if m.fields[index].Kind == MultiCheckbox {
				m.multiChoice = -1
			}
			return
		}
	}
}

func (m Model) Focused(id string) bool { return m.FocusedID() == id }

func (m Model) Value(id string) string {
	for _, field := range m.fields {
		if field.ID == id {
			return field.Value
		}
	}
	return ""
}

func (m Model) Checked(id string) bool {
	for _, field := range m.fields {
		if field.ID == id {
			return field.Checked
		}
	}
	return false
}

func (m Model) IntValue(id string) int {
	value, _ := strconv.Atoi(m.Value(id))
	return value
}

func (m Model) Values(id string) []string {
	for _, field := range m.fields {
		if field.ID == id {
			return append([]string(nil), field.Selected...)
		}
	}
	return nil
}

// SetOptions replaces a dynamic option set. When selectAll is true every new
// option is selected; otherwise existing selections are retained when valid.
func (m *Model) SetOptions(id string, options []string, selectAll bool) {
	for index := range m.fields {
		if m.fields[index].ID != id {
			continue
		}
		m.fields[index].Options = append([]string(nil), options...)
		selected := make([]string, 0, len(options))
		for _, option := range options {
			if selectAll || contains(m.fields[index].Selected, option) {
				selected = append(selected, option)
			}
		}
		m.fields[index].Selected = selected
		return
	}
}

func (m *Model) SetValues(id string, values []string) {
	for index := range m.fields {
		if m.fields[index].ID == id {
			m.fields[index].Selected = append([]string(nil), values...)
			return
		}
	}
}

func (m *Model) SetValue(id, value string) {
	for index := range m.fields {
		if m.fields[index].ID == id {
			m.fields[index].Value = value
			return
		}
	}
}

func (m Model) CapturesText() bool {
	field, ok := m.focusedField()
	return ok && m.activeID == field.ID && field.Kind == Text
}

func (m Model) IsActive() bool { return m.activeID != "" }

// HandleInteraction applies keys belonging to the focused control. It returns
// true when the key was consumed and form navigation must not run.
func (m *Model) HandleInteraction(key string) bool {
	field, ok := m.focusedField()
	if !ok {
		return false
	}
	if m.activeID != "" {
		return m.handleActive(field, key)
	}
	switch field.Kind {
	case Checkbox:
		if key == " " || key == "enter" {
			m.fields[m.focus].Checked = !field.Checked
			return true
		}
	case Option:
		if key == "enter" || key == " " {
			m.begin(field)
			return true
		}
	case Text:
		if key == "enter" {
			m.begin(field)
			return true
		}
	case Radio:
		if key == "left" || key == "h" {
			m.moveChoice(-1, field.Options)
			return true
		}
		if key == "right" || key == "l" || key == "enter" || key == " " {
			m.moveChoice(1, field.Options)
			return true
		}
	case MultiCheckbox:
		if key == "right" || key == "l" || key == "enter" {
			m.beginMulti(field)
			return true
		}
	case Number:
		if key == "left" || key == "h" || key == "-" {
			m.adjustNumber(-1, field)
			return true
		}
		if key == "right" || key == "l" || key == "+" || key == "=" || key == "enter" {
			m.adjustNumber(1, field)
			return true
		}
	}
	return false
}

func (m *Model) begin(field Field) {
	m.activeID = field.ID
	m.original = field.Value
	m.choice = optionIndex(field.Options, field.Value)
}

func (m *Model) beginMulti(field Field) {
	m.activeID = field.ID
	if field.SelectAll {
		m.multiChoice = -1
	} else {
		m.multiChoice = 0
	}
}

func (m *Model) handleActive(field Field, key string) bool {
	if field.Kind == MultiCheckbox {
		minimum := 0
		if field.SelectAll {
			minimum = -1
		}
		maximum := max(minimum, len(field.Options)-1)
		switch key {
		case "esc", "left", "h":
			m.activeID = ""
		case "up", "k":
			m.multiChoice = max(minimum, m.multiChoice-1)
		case "down", "j":
			m.multiChoice = min(maximum, m.multiChoice+1)
		case " ", "enter":
			if m.multiChoice == -1 {
				m.selectAll(field.ID)
			} else if m.multiChoice < len(field.Options) {
				m.toggleSelected(field.ID, field.Options[m.multiChoice])
			}
		}
		return true
	}
	switch key {
	case "esc":
		m.fields[m.focus].Value = m.original
		m.activeID = ""
		return true
	case "enter":
		if field.Kind == Option && len(field.Options) > 0 {
			m.fields[m.focus].Value = field.Options[m.choice]
		}
		m.activeID = ""
		return true
	}
	if field.Kind == Option {
		if len(field.Options) == 0 {
			m.activeID = ""
			return true
		}
		switch key {
		case "up", "k", "left", "h", "shift+tab":
			m.choice = (m.choice - 1 + len(field.Options)) % len(field.Options)
		case "down", "j", "right", "l", "tab":
			m.choice = (m.choice + 1) % len(field.Options)
		}
		return true
	}
	if field.Kind != Text {
		return true
	}
	switch key {
	case "backspace":
		value := []rune(field.Value)
		if len(value) > 0 {
			m.fields[m.focus].Value = string(value[:len(value)-1])
		}
		return true
	case "space", " ":
		m.fields[m.focus].Value += " "
		return true
	}
	if utf8.RuneCountInString(key) == 1 {
		m.fields[m.focus].Value += key
		return true
	}
	return true
}

func (m *Model) moveChoice(delta int, options []string) {
	if len(options) == 0 {
		return
	}
	index := optionIndex(options, m.fields[m.focus].Value)
	m.fields[m.focus].Value = options[(index+delta+len(options))%len(options)]
}

func optionIndex(options []string, value string) int {
	for index, option := range options {
		if option == value {
			return index
		}
	}
	return 0
}

func (m Model) focusedField() (Field, bool) {
	if m.focus < 0 || m.focus >= len(m.fields) {
		return Field{}, false
	}
	return m.fields[m.focus], true
}

func (m Model) View(ids []string) string {
	return m.ViewFocused(ids, true)
}

// ViewFocused hides internal focus when the containing data field is inactive.
func (m Model) ViewFocused(ids []string, focusVisible bool) string {
	return m.ViewFocusedWidth(ids, focusVisible, 0)
}

// ViewFocusedWidth highlights the complete focused row up to width cells.
func (m Model) ViewFocusedWidth(ids []string, focusVisible bool, width int) string {
	rows := make([]string, 0, len(ids))
	for _, id := range ids {
		for _, field := range m.fields {
			if field.ID == id {
				rows = append(rows, m.render(field, focusVisible && m.Focused(id), width))
				break
			}
		}
	}
	return strings.Join(rows, "\n")
}

func (m Model) render(field Field, focused bool, width int) string {
	marker := "  "
	style := lipgloss.NewStyle()
	if focused {
		marker = "› "
		style = focusStyle
	}
	var value string
	switch field.Kind {
	case Option:
		value = field.Value + "  ▾"
		if m.activeID == field.ID {
			rows := []string{marker + labelStyle.Render(field.Label) + style.Render(value)}
			for index, option := range field.Options {
				choiceMarker := "    "
				if index == m.choice {
					choiceMarker = focusStyle.Render("  › ")
				}
				rows = append(rows, choiceMarker+style.Render(option))
			}
			rows[0] = highlightRow(rows[0], focused, width)
			return strings.Join(rows, "\n")
		}
	case Checkbox:
		mark := " "
		if field.Checked {
			mark = "x"
		}
		value = "[" + mark + "] " + field.Label
		return highlightRow(marker+style.Render(value), focused, width)
	case Radio:
		choices := make([]string, len(field.Options))
		for index, option := range field.Options {
			mark := " "
			if option == field.Value {
				mark = "●"
			}
			choices[index] = "(" + mark + ") " + option
		}
		value = strings.Join(choices, "   ")
	case MultiCheckbox:
		rows := make([]string, 0, len(field.Options)+1)
		if field.SelectAll {
			row := marker + labelStyle.Render(field.Label) + "[ All ]"
			rows = append(rows, highlightRow(row, focused && m.multiChoice == -1, width))
		}
		for index, option := range field.Options {
			mark := " "
			if contains(field.Selected, option) {
				mark = "x"
			}
			prefix := strings.Repeat(" ", 18)
			if index == 0 && !field.SelectAll {
				prefix = marker + labelStyle.Render(field.Label)
			}
			rows = append(rows, highlightRow(prefix+"["+mark+"] "+option, focused && index == m.multiChoice, width))
		}
		if len(rows) == 0 {
			return highlightRow(marker+labelStyle.Render(field.Label)+mutedStyle.Render("No values"), focused, width)
		}
		return strings.Join(rows, "\n")
	case Button:
		return highlightRow(marker+style.Render("[ "+field.Label+" ]"), focused, width)
	case Number:
		value = "[ - ]  " + field.Value + "  [ + ]"
	default:
		value = field.Value
		if field.Kind == Text && m.activeID == field.ID {
			value += focusStyle.Render("█")
		}
	}
	return highlightRow(marker+labelStyle.Render(field.Label)+style.Render(value), focused, width)
}

func highlightRow(row string, focused bool, width int) string {
	if !focused {
		return row
	}
	if width > 0 {
		return rowStyle.Width(width).Render(row)
	}
	return rowStyle.Render(row)
}

// Click handles a primary click relative to a rendered form. It returns the
// clicked field ID so its owner can perform Path and Button actions.
func (m *Model) Click(ids []string, x, y int) (string, bool) {
	if x < 0 || y < 0 {
		return "", false
	}
	row := 0
	for _, id := range ids {
		fieldIndex := m.index(id)
		if fieldIndex < 0 {
			continue
		}
		field := m.fields[fieldIndex]
		height := 1
		if m.activeID == id && field.Kind == Option {
			height += len(field.Options)
		}
		if field.Kind == MultiCheckbox {
			height = max(1, len(field.Options)+boolInt(field.SelectAll))
		}
		if y < row || y >= row+height {
			row += height
			continue
		}
		if m.activeID != "" && m.activeID != id {
			activeIndex := m.index(m.activeID)
			if activeIndex >= 0 && m.fields[activeIndex].Kind == Option {
				m.fields[activeIndex].Value = m.original
			}
			m.activeID = ""
		}
		m.focus = fieldIndex
		if field.Kind == Option && m.activeID == id && y > row {
			choice := y - row - 1
			if choice < len(field.Options) {
				m.fields[fieldIndex].Value = field.Options[choice]
				m.activeID = ""
				return id, true
			}
		}
		switch field.Kind {
		case Checkbox:
			m.fields[fieldIndex].Checked = !field.Checked
		case Option, Text:
			m.begin(field)
		case Radio:
			start := 18
			for _, option := range field.Options {
				end := start + lipgloss.Width(option) + 4
				if x >= start && x < end {
					m.fields[fieldIndex].Value = option
					break
				}
				start = end + 3
			}
		case MultiCheckbox:
			optionIndex := y - row
			if field.SelectAll && optionIndex == 0 {
				m.multiChoice = -1
				m.activeID = id
				m.selectAll(id)
				break
			}
			optionIndex -= boolInt(field.SelectAll)
			if optionIndex < len(field.Options) {
				m.multiChoice = optionIndex
				m.activeID = id
				m.toggleSelected(id, field.Options[optionIndex])
			}
		case Number:
			if x >= 18 && x < 23 {
				m.adjustNumber(-1, field)
			} else if x >= 26 {
				m.adjustNumber(1, field)
			}
		}
		return id, true
	}
	return "", false
}

func (m Model) index(id string) int {
	for index := range m.fields {
		if m.fields[index].ID == id {
			return index
		}
	}
	return -1
}

func (m *Model) toggleSelected(id, option string) {
	index := m.index(id)
	if index < 0 {
		return
	}
	values := m.fields[index].Selected
	for valueIndex, value := range values {
		if value == option {
			m.fields[index].Selected = append(values[:valueIndex], values[valueIndex+1:]...)
			return
		}
	}
	m.fields[index].Selected = append(values, option)
}

func (m *Model) selectAll(id string) {
	index := m.index(id)
	if index < 0 {
		return
	}
	m.fields[index].Selected = append([]string(nil), m.fields[index].Options...)
}

func (m *Model) adjustNumber(direction int, field Field) {
	index := m.index(field.ID)
	if index < 0 {
		return
	}
	value, err := strconv.Atoi(field.Value)
	if err != nil {
		value = field.Min
	}
	step := field.Step
	if step < 1 {
		step = 1
	}
	minimum, maximum := field.Min, field.Max
	if maximum < minimum {
		maximum = minimum
	}
	value = min(maximum, max(minimum, value+direction*step))
	m.fields[index].Value = strconv.Itoa(value)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
