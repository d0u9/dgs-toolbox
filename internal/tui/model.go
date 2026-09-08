package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type choice struct {
	appIndex     int
	commandIndex int
}

// Model is the single shell model. It owns the picker and exactly one active
// leaf command; replacing or leaving a command discards its model.
type Model struct {
	apps          []App
	pickerApp     int
	selected      int
	pickerHelp    bool
	active        CommandModel
	activeApp     int
	activeCommand int
	confirmQuit   bool
	width         int
	height        int
	now           time.Time
	launchErr     string
}

type tickMsg time.Time

// NewModel creates either a scoped picker or a directly active leaf command.
func NewModel(apps []App, launch Launch) Model {
	m := Model{
		apps:          apps,
		pickerApp:     -1,
		activeApp:     -1,
		activeCommand: -1,
		width:         80,
		height:        24,
		now:           time.Now(),
	}

	if launch.App == "" {
		return m
	}

	appIndex := findApp(apps, launch.App)
	if appIndex < 0 {
		m.launchErr = fmt.Sprintf("unknown app %q", launch.App)
		return m
	}
	m.pickerApp = appIndex

	if launch.Command == "" {
		return m
	}

	commandIndex := findCommand(apps[appIndex], launch.Command)
	if commandIndex < 0 {
		m.launchErr = fmt.Sprintf("unknown command %q for app %q", launch.Command, launch.App)
		return m
	}
	m.selected = commandIndex
	return m.activate(choice{appIndex: appIndex, commandIndex: commandIndex})
}

func (m Model) Init() tea.Cmd {
	if m.active == nil {
		return tick()
	}
	return tea.Batch(tick(), m.active.Init())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.active != nil {
			return m.forwardToActive(m.workspaceSizeMsg())
		}
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.MouseMsg:
		if m.active != nil && msg.Y > 0 && msg.Y < m.height-1 {
			msg.Y--
			return m.forwardToActive(msg)
		}
		return m, nil
	default:
		if m.active != nil {
			return m.forwardToActive(msg)
		}
		return m, nil
	}
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		if m.active != nil {
			if capturer, ok := m.active.(ShellKeyCapturer); ok && capturer.CapturesShellKey(key) {
				return m.forwardToActive(msg)
			}
		}
		return m, tea.Quit
	}
	if m.confirmQuit {
		switch key {
		case "y", "Y":
			return m, tea.Quit
		case "n", "N", "esc":
			m.confirmQuit = false
		}
		return m, nil
	}
	if m.active != nil {
		if capturer, ok := m.active.(ShellKeyCapturer); ok && capturer.CapturesShellKey(key) {
			return m.forwardToActive(msg)
		}
	}
	if key == "q" {
		m.confirmQuit = true
		return m, nil
	}
	if key == "esc" {
		if m.active != nil {
			m.active = nil
			m.activeApp = -1
			m.activeCommand = -1
			return m, nil
		}
		m.confirmQuit = true
		return m, nil
	}

	if m.active != nil {
		return m.forwardToActive(msg)
	}
	if m.launchErr != "" {
		return m, nil
	}

	switch key {
	case "?":
		m.pickerHelp = !m.pickerHelp
	case "up", "k":
		choices := m.choices()
		if len(choices) > 0 {
			m.selected = (m.selected - 1 + len(choices)) % len(choices)
		}
	case "down", "j":
		choices := m.choices()
		if len(choices) > 0 {
			m.selected = (m.selected + 1) % len(choices)
		}
	case "enter":
		choices := m.choices()
		if len(choices) > 0 {
			m = m.activate(choices[m.selected])
			if m.active != nil {
				return m, m.active.Init()
			}
		}
	}
	return m, nil
}

func (m Model) activate(selected choice) Model {
	command := m.apps[selected.appIndex].Commands[selected.commandIndex]
	if command.New == nil {
		m.launchErr = fmt.Sprintf("command %q has no model factory", command.ID)
		return m
	}
	m.active = command.New()
	m.activeApp = selected.appIndex
	m.activeCommand = selected.commandIndex
	m.pickerHelp = false
	updated, _ := m.active.Update(m.workspaceSizeMsg())
	if active, ok := updated.(CommandModel); ok {
		m.active = active
	}
	return m
}

func (m Model) forwardToActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.active.Update(msg)
	active, ok := updated.(CommandModel)
	if !ok {
		m.launchErr = fmt.Sprintf("command returned incompatible model %T", updated)
		m.active = nil
		return m, cmd
	}
	m.active = active
	return m, cmd
}

func (m Model) workspaceSizeMsg() tea.WindowSizeMsg {
	height := m.height - 2
	if height < 0 {
		height = 0
	}
	return tea.WindowSizeMsg{Width: m.width, Height: height}
}

func (m Model) choices() []choice {
	if m.pickerApp >= 0 {
		commands := m.apps[m.pickerApp].Commands
		choices := make([]choice, len(commands))
		for i := range commands {
			choices[i] = choice{appIndex: m.pickerApp, commandIndex: i}
		}
		return choices
	}

	var choices []choice
	for appIndex, app := range m.apps {
		for commandIndex := range app.Commands {
			choices = append(choices, choice{appIndex: appIndex, commandIndex: commandIndex})
		}
	}
	return choices
}

func (m Model) choiceLabel(selected choice) string {
	app := m.apps[selected.appIndex]
	command := app.Commands[selected.commandIndex]
	if app.Direct {
		return app.Name
	}
	if m.pickerApp >= 0 {
		return command.Name
	}
	return app.Name + " / " + command.Name
}

func findApp(apps []App, id string) int {
	for i := range apps {
		if apps[i].ID == id {
			return i
		}
	}
	return -1
}

func findCommand(app App, id string) int {
	for i := range app.Commands {
		if app.Commands[i].ID == id {
			return i
		}
	}
	return -1
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg {
		return tickMsg(now)
	})
}
