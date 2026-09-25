// Package doc is the dgs doc command: a tree of important documents. The
// terminal starts the local page and says where it is; the page does the rest.
//
// The design is docs/apps/doc/.
package doc

import (
	"strings"

	docweb "dgs-toolbox/internal/apps/doc/web"
	"dgs-toolbox/internal/desktop"
	"dgs-toolbox/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(
		lipgloss.AdaptiveColor{Light: "#024ad8", Dark: "#296ef9"},
	)
	hintStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#636363", Dark: "#999999"},
	)
	warnStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#b3262b", Dark: "#ff8a8a"},
	)
)

type servedMsg struct {
	url string
	err error
}

// Model is dgs doc: it starts the page and says where it is.
type Model struct {
	settings docweb.Settings
	url      string
	err      error
	width    int
	height   int
}

// NewModel builds the command model.
func NewModel(settings docweb.Settings) Model {
	return Model{settings: settings, width: 80, height: 22}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		url, err := docweb.Serve(m.settings)
		return servedMsg{url: url, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case servedMsg:
		m.url, m.err = msg.url, msg.err
	case tea.KeyMsg:
		if msg.String() == "o" && m.url != "" {
			url := m.url
			return m, func() tea.Msg {
				_ = desktop.Open(url)
				return nil
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	lines := []string{}
	switch {
	case m.err != nil:
		lines = append(lines, warnStyle.Render("Web server failed: "+m.err.Error()))
	case m.url == "":
		lines = append(lines, hintStyle.Render("Starting the local page…"))
	default:
		lines = append(lines, hintStyle.Render("Page  ")+m.url)
	}
	lines = append(lines, hintStyle.Render("Tree  ")+m.settings.ResolvedRoot())
	lines = append(lines, "", hintStyle.Render("The page lists and shows the PDFs in the tree. Nothing is written."))
	content := titleStyle.Render("DOC") + "\n\n" + strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) Status() tui.Status {
	state := "STARTING"
	if m.url != "" {
		state = "SERVING"
	}
	if m.err != nil {
		state = "FAILED"
	}
	return tui.Status{Left: state, Center: "Doc", Right: "o Open page  esc Back  q Quit"}
}
