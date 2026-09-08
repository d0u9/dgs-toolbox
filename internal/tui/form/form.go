package form

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

type Kind int

const (
	Path Kind = iota
	Option
	Checkbox
	Text
	Radio
	Button
)

type Field struct {
	ID      string
	Kind    Kind
	Label   string
	Value   string
	Options []string
	Checked bool
}

type Model struct {
	fields   []Field
	focus    int
	activeID string
	original string
	choice   int
}

var (
	accent     = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	muted      = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	focusStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	labelStyle = lipgloss.NewStyle().Foreground(muted).Width(16)
)

func New(fields ...Field) Model {
	return Model{fields: append([]Field(nil), fields...)}
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
		if key == "enter" {
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
	}
	return false
}

func (m *Model) begin(field Field) {
	m.activeID = field.ID
	m.original = field.Value
	m.choice = optionIndex(field.Options, field.Value)
}

func (m *Model) handleActive(field Field, key string) bool {
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
	rows := make([]string, 0, len(ids))
	for _, id := range ids {
		for _, field := range m.fields {
			if field.ID == id {
				rows = append(rows, m.render(field, m.Focused(id)))
				break
			}
		}
	}
	return strings.Join(rows, "\n")
}

func (m Model) render(field Field, focused bool) string {
	marker := "  "
	style := lipgloss.NewStyle()
	if focused {
		marker = focusStyle.Render("› ")
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
			return strings.Join(rows, "\n")
		}
	case Checkbox:
		mark := " "
		if field.Checked {
			mark = "x"
		}
		value = "[" + mark + "] " + field.Label
		return marker + style.Render(value)
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
	case Button:
		return marker + style.Render("[ "+field.Label+" ]")
	default:
		value = field.Value
		if field.Kind == Text && m.activeID == field.ID {
			value += focusStyle.Render("█")
		}
	}
	return marker + labelStyle.Render(field.Label) + style.Render(value)
}
