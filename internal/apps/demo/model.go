package demo

import (
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/divider"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/overlay"
	"dgs-toolbox/internal/tui/pageactions"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	pathID     = "path"
	optionID   = "option"
	checkboxID = "checkbox"
	textID     = "text"
	radioID    = "radio"
	buttonID   = "button"
)

var (
	accent     = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	muted      = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	panel      = lipgloss.AdaptiveColor{Light: "#EAF4F4", Dark: "#243447"}
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedStyle = lipgloss.NewStyle().Foreground(muted)
	modalStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Background(panel)
)

type model struct {
	controls     form.Model
	fields       datafield.Navigator
	picker       fileexplorer.Model
	picking      bool
	confirming   bool
	confirmation confirm.Model
	width        int
	height       int
	event        string
}

func newModel() tui.CommandModel {
	path, err := filepath.Abs(filepath.Join("testdata", "demo"))
	if err != nil {
		path = "."
	}
	return model{
		controls: form.New(
			form.Field{ID: pathID, Kind: form.Path, Label: "Directory", Value: path},
			form.Field{ID: optionID, Kind: form.Option, Label: "Theme", Value: "System", Options: []string{"System", "Light", "Dark"}},
			form.Field{ID: checkboxID, Kind: form.Checkbox, Label: "Show hidden", Checked: false},
			form.Field{ID: textID, Kind: form.Text, Label: "Label", Value: "Editable text"},
			form.Field{ID: radioID, Kind: form.Radio, Label: "Density", Value: "Comfortable", Options: []string{"Compact", "Comfortable"}},
			form.Field{ID: buttonID, Kind: form.Button, Label: "Trigger demo action"},
		),
		fields: datafield.New(
			datafield.Field{ID: "path", Row: 0, Col: 0},
			datafield.Field{ID: "selection", Row: 1, Col: 0},
			datafield.Field{ID: "text", Row: 2, Col: 0},
			datafield.Field{ID: "action", Row: 3, Col: 0},
		),
		width:  80,
		height: 22,
		event:  "No component action yet",
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.picking {
			m.sizePicker()
		}
		return m, nil
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		if m.picking {
			var cmd tea.Cmd
			m.picker, _, cmd = m.picker.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.picking {
		width, height := m.modalSize()
		msg.X -= (m.width-width)/2 + 1
		msg.Y -= (m.height-height)/2 + 4
		var cmd tea.Cmd
		var selected string
		m.picker, selected, cmd = m.picker.Update(msg)
		if selected != "" {
			m.controls.SetValue(pathID, selected)
			m.picking = false
		}
		return m, cmd
	}
	if m.confirming {
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	width := min(tui.DefaultContentWidth, max(24, m.width-4))
	x, y := max(0, (m.width-width)/2), 4
	type placedField struct {
		id, view string
		y        int
	}
	placed := make([]placedField, 0, 4)
	for _, field := range []struct {
		id   string
		view string
	}{
		{"path", m.pathFieldView(width)},
		{"selection", m.selectionFieldView(width)},
		{"text", m.textFieldView(width)},
		{"action", m.actionFieldView(width)},
	} {
		height := lipgloss.Height(field.view)
		placed = append(placed, placedField{id: field.id, view: field.view, y: y})
		m.fields.SetBounds(field.id, datafield.Bounds{X: x, Y: y, Width: width, Height: height})
		y += height + 1
	}
	if m.fields.FocusAt(msg.X, msg.Y) {
		m.syncControlFocus()
		for _, field := range placed {
			if field.id != m.fields.Current() {
				continue
			}
			id, used := m.controls.Click(m.activeControlIDs(), msg.X-x-2, msg.Y-field.y-1)
			if !used {
				break
			}
			switch id {
			case pathID:
				return m.openPicker()
			case buttonID:
				m.openConfirmation()
			}
			break
		}
	}
	return m, nil
}

func (m *model) syncControlFocus() {
	switch m.fields.Current() {
	case "path":
		m.controls.SetFocusID(pathID)
	case "selection":
		m.controls.SetFocusID(optionID)
	case "text":
		m.controls.SetFocusID(textID)
	case "action":
		m.controls.SetFocusID(buttonID)
	}
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.picking {
		if key == "esc" && !m.picker.HasDialog() {
			m.picking = false
			return m, nil
		}
		var selected string
		var cmd tea.Cmd
		m.picker, selected, cmd = m.picker.Update(msg)
		if selected != "" {
			m.controls.SetValue(pathID, selected)
			m.event = "Directory selected"
			m.picking = false
		}
		return m, cmd
	}
	if m.confirming {
		var decision confirm.Decision
		m.confirmation, decision = m.confirmation.Update(key)
		switch decision {
		case confirm.Confirmed:
			m.event = "Confirmation accepted — no side effects"
			m.confirming = false
		case confirm.Cancelled:
			m.event = "Confirmation cancelled"
			m.confirming = false
		}
		return m, nil
	}
	if m.fields.Move(key) {
		m.syncControlFocus()
		return m, nil
	}
	if m.controls.HandleInteraction(key) {
		m.event = "Control value changed"
		return m, nil
	}
	if m.controls.UpdateNavigationWithin(m.activeControlIDs(), key) {
		return m, nil
	}
	if key == "enter" {
		switch m.controls.FocusedID() {
		case pathID:
			return m.openPicker()
		case buttonID:
			m.openConfirmation()
		}
	}
	if key == form.PrimaryActionKey {
		m.openConfirmation()
	}
	return m, nil
}

func (m *model) openConfirmation() {
	m.confirmation = confirm.New(confirm.Config{
		Title:        "RUN DEMO ACTION?",
		Message:      "This demonstrates the reusable confirmation dialog.",
		Detail:       "The demo never changes files.",
		ConfirmLabel: "Run",
		CancelLabel:  "No",
	})
	m.confirming = true
}

func (m model) activeControlIDs() []string {
	switch m.fields.Current() {
	case "selection":
		return []string{optionID, checkboxID, radioID}
	case "text":
		return []string{textID}
	case "action":
		return []string{buttonID}
	default:
		return []string{pathID}
	}
}

func (m model) openPicker() (tea.Model, tea.Cmd) {
	w, h := m.modalSize()
	m.picker = fileexplorer.New(m.controls.Value(pathID), w-2, h-6, fileexplorer.WithFilter(fileexplorer.Directories()))
	m.picking = true
	return m, m.picker.Init()
}

func (m *model) sizePicker() {
	w, h := m.modalSize()
	m.picker.SetSize(w-2, h-6)
}

func (m model) modalSize() (int, int) {
	return max(20, min(96, m.width-4)), max(8, min(30, m.height-2))
}

func (m model) View() string {
	width := min(tui.DefaultContentWidth, max(24, m.width-4))
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		titleStyle.Render("COMPONENT DEMO"),
		mutedStyle.Render("Interactive reference for shared dgs TUI components."),
		"",
		m.pathFieldView(width),
		"",
		m.selectionFieldView(width),
		"",
		m.textFieldView(width),
		"",
		m.actionFieldView(width),
	)
	workspace := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, content)
	if m.picking {
		return overlay.Place(workspace, m.pickerView(), m.width, m.height)
	}
	if m.confirming {
		return overlay.Place(workspace, m.confirmation.View(m.width), m.width, m.height)
	}
	return workspace
}

func (m model) pathFieldView(width int) string {
	return fieldset.ViewFocused("Path · opens File Explorer", m.controls.ViewFocusedWidth([]string{pathID}, m.fields.Current() == "path", width-4), width, m.fields.Current() == "path")
}

func (m model) selectionFieldView(width int) string {
	return fieldset.ViewFocused("Selection controls", m.controls.ViewFocusedWidth([]string{optionID, checkboxID, radioID}, m.fields.Current() == "selection", width-4), width, m.fields.Current() == "selection")
}

func (m model) textFieldView(width int) string {
	return fieldset.ViewFocused("Text input", m.controls.ViewFocusedWidth([]string{textID}, m.fields.Current() == "text", width-4), width, m.fields.Current() == "text")
}

func (m model) actionFieldView(width int) string {
	return fieldset.ViewFocused(
		"Actions · Local and Page Navigation",
		m.controls.ViewFocusedWidth([]string{buttonID}, m.fields.Current() == "action", width-4)+"\n\n"+
			divider.Anchored(width-4)+"\n"+mutedStyle.Render(m.event)+"\n\n"+
			pageactions.View(pageactions.Config{
				Prev: &pageactions.Action{Destination: "Parameters"},
				Next: &pageactions.Action{Destination: "Processing"},
			}, width-4),
		width,
		m.fields.Current() == "action",
	)
}

func (m model) pickerView() string {
	w, h := m.modalSize()
	label := titleStyle.Render("FILE EXPLORER")
	filter := titleStyle.Render(m.picker.FilterLabel())
	headerWidth := max(1, w-4)
	header := label + strings.Repeat(" ", max(1, headerWidth-lipgloss.Width(label)-lipgloss.Width(filter))) + filter
	selected := ansi.Truncate(m.picker.SelectedPath(), headerWidth, "…")
	body := lipgloss.JoinVertical(lipgloss.Left, header, mutedStyle.Render(selected), "", m.picker.View(), mutedStyle.Render(m.picker.Hint()))
	return modalStyle.Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(body)
}

func (m model) Status() tui.Status {
	if m.picking {
		if m.picker.HasDialog() {
			return tui.Status{Left: "BROWSE", Center: "File Explorer component"}
		}
		return tui.Status{Left: "BROWSE", Center: "File Explorer component", Right: m.picker.Hint()}
	}
	if m.confirming {
		return tui.Status{Left: "CONFIRM", Center: "Confirmation dialog component", Right: "tab Switch  ↵ Select  esc Continue"}
	}
	if m.controls.IsActive() {
		if m.controls.CapturesText() {
			return tui.Status{Left: "EDIT", Center: "Text input component", Right: "↵ Apply  esc Cancel"}
		}
		return tui.Status{Left: "SELECT", Center: "Option component", Right: "↑↓ Choose  ↵ Apply  esc Cancel"}
	}
	return tui.Status{Left: "DEMO", Center: m.event, Right: "n Next  hjkl Within  alt+hjkl Focus"}
}

func (m model) CapturesShellKey(key string) bool {
	if m.confirming {
		return key == "esc" || key == "q" || key == "ctrl+c"
	}
	if m.picking {
		return key == "esc" || (key == "q" && m.picker.CapturesText())
	}
	return m.controls.IsActive() && (key == "esc" || (key == "q" && m.controls.CapturesText()))
}
