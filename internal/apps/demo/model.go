package demo

import (
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/overlay"

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
	ids        = []string{pathID, optionID, checkboxID, radioID, textID, buttonID}
	accent     = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	muted      = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	panel      = lipgloss.AdaptiveColor{Light: "#EAF4F4", Dark: "#243447"}
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedStyle = lipgloss.NewStyle().Foreground(muted)
	modalStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Background(panel)
)

type model struct {
	controls form.Model
	picker   fileexplorer.Model
	picking  bool
	width    int
	height   int
	event    string
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
	if m.controls.HandleInteraction(key) {
		m.event = "Control value changed"
		return m, nil
	}
	if m.controls.UpdateNavigationWithin(ids, key) {
		return m, nil
	}
	if key == "enter" {
		switch m.controls.FocusedID() {
		case pathID:
			return m.openPicker()
		case buttonID:
			m.event = "Demo action triggered — no side effects"
		}
	}
	return m, nil
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
	return max(20, min(76, m.width-4)), max(8, min(22, m.height-4))
}

func (m model) View() string {
	width := min(tui.DefaultContentWidth, max(24, m.width-4))
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		titleStyle.Render("COMPONENT DEMO"),
		mutedStyle.Render("Interactive reference for shared dgs TUI components."),
		"",
		fieldset.View("Path · opens File Explorer", m.controls.View([]string{pathID}), width),
		"",
		fieldset.View("Selection controls", m.controls.View([]string{optionID, checkboxID, radioID}), width),
		"",
		fieldset.View("Text input", m.controls.View([]string{textID}), width),
		"",
		fieldset.View("Action", m.controls.View([]string{buttonID})+"\n"+mutedStyle.Render(m.event), width),
	)
	workspace := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, content)
	if m.picking {
		return overlay.Place(workspace, m.pickerView(), m.width, m.height)
	}
	return workspace
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
	if m.controls.IsActive() {
		if m.controls.CapturesText() {
			return tui.Status{Left: "EDIT", Center: "Text input component", Right: "↵ Apply  esc Cancel"}
		}
		return tui.Status{Left: "SELECT", Center: "Option component", Right: "↑/k ↓/j Choose  ↵ Apply  esc Cancel"}
	}
	return tui.Status{Left: "DEMO", Center: m.event, Right: "arrows/hjkl Move  ↵ Use  space Toggle"}
}

func (m model) CapturesShellKey(key string) bool {
	if m.picking {
		return key == "esc" || (key == "q" && m.picker.CapturesText())
	}
	return m.controls.IsActive() && (key == "esc" || (key == "q" && m.controls.CapturesText()))
}
