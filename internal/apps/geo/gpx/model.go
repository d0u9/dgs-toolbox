package gpx

import (
	"fmt"
	"strings"
	"time"

	"dgs-toolbox/internal/desktop"
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

// focusMsg carries the track focused on the page, nil when none, and the
// channel that closes when it next changes.
type focusMsg struct {
	summary *Summary
	next    <-chan struct{}
}

// Model is the GPX command: it starts the local web page, shows where to find
// it, and summarises the track focused there.
type Model struct {
	settings Settings
	url      string
	err      error
	focus    *Summary
	width    int
	height   int
}

func New(settings Settings) Model {
	return Model{settings: settings, width: 80, height: 22}
}

func (m Model) Init() tea.Cmd {
	serve := func() tea.Msg {
		url, err := Serve(m.settings)
		return servedMsg{url: url, err: err}
	}
	return tea.Batch(serve, watchFocus(nil))
}

// watchFocus reports the focused track once after changed closes; nil reports
// it at once.
func watchFocus(changed <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if changed != nil {
			<-changed
		}
		summary, next := focused.current()
		return focusMsg{summary: summary, next: next}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case servedMsg:
		m.url, m.err = msg.url, msg.err
	case focusMsg:
		m.focus = msg.summary
		return m, watchFocus(msg.next)
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
		line = hintStyle.Render("Web     " + m.url + "\nFolder  " + m.settings.Root)
	}
	content := titleStyle.Render("GPX") + "\n\n" + line
	if m.url != "" {
		content += "\n\n" + summaryView(m.focus)
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) Status() tui.Status {
	return tui.Status{Left: "ACTIVE", Center: "GPX", Right: "o Open web  esc Back  q Quit"}
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		_ = desktop.Open(url)
		return nil
	}
}

// summaryView is the focused track's duration, distance and stop count.
func summaryView(summary *Summary) string {
	if summary == nil {
		return hintStyle.Render("Focus a track on the page to see its summary here.")
	}
	stops := fmt.Sprintf("%d stops", summary.Stops)
	if summary.Stops == 1 {
		stops = "1 stop"
	}
	return hintStyle.Render("Track   ") + summary.Name + "\n" + hintStyle.Render(
		"        "+strings.Join([]string{formatDuration(summary.Duration), formatDistance(summary.Distance), stops}, " · "),
	)
}

func formatDistance(metres float64) string {
	if metres < 1000 {
		return fmt.Sprintf("%.0f m", metres)
	}
	return fmt.Sprintf("%.1f km", metres/1000)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "no time"
	}
	d = d.Round(time.Minute)
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}
