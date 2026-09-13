package gpx

import (
	"os/exec"
	"runtime"

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

type servedMsg struct {
	url string
	err error
}

// Model is the empty GPX command: it starts the local web page and shows
// where to find it.
type Model struct {
	addr   string
	url    string
	err    error
	width  int
	height int
}

func New(addr string) Model {
	return Model{addr: addr, width: 80, height: 22}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		url, err := Serve(m.addr)
		return servedMsg{url: url, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case servedMsg:
		m.url, m.err = msg.url, msg.err
	case tea.KeyMsg:
		if msg.String() == "o" && m.url != "" {
			return m, openBrowser(m.url)
		}
	}
	return m, nil
}

func (m Model) View() string {
	line := hintStyle.Render("Starting web server…")
	switch {
	case m.err != nil:
		line = hintStyle.Render("Web server failed: " + m.err.Error())
	case m.url != "":
		line = hintStyle.Render("Web  " + m.url)
	}
	content := titleStyle.Render("GPX") + "\n\n" + line
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) Status() tui.Status {
	return tui.Status{Left: "ACTIVE", Center: "GPX", Right: "o Open web  esc Back  q Quit"}
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		command := exec.Command("xdg-open", url)
		switch runtime.GOOS {
		case "darwin":
			command = exec.Command("open", url)
		case "windows":
			command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		}
		_ = command.Start()
		return nil
	}
}
