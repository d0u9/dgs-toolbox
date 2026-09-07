package placeholder

import (
	"strings"

	"dgs-toolbox/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(
		lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"},
	)
	hintStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"},
	)
)

// Model is a disposable leaf command used to demonstrate the shell boundary.
type Model struct {
	label  string
	width  int
	height int
	help   bool
}

func New(label string) Model {
	return Model{label: strings.ToUpper(label), width: 80, height: 22}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "?" {
			m.help = !m.help
		}
	}
	return m, nil
}

func (m Model) View() string {
	content := titleStyle.Render(m.label)
	if m.help {
		content = titleStyle.Render("HELP") + "\n\n" + hintStyle.Render("esc back  •  q quit")
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) Status() tui.Status {
	if m.help {
		return tui.Status{Left: "ACTIVE", Center: "COMMAND HELP", Right: "? Close  esc Back  q Quit"}
	}
	return tui.Status{Left: "ACTIVE", Center: m.label, Right: "? Help  esc Back  q Quit"}
}
