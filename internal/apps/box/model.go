// Package box is the dgs box command: a Box of scanned paper. The terminal
// picks the folders, starts the local pages and reports; the pages own
// everything that involves looking at a scan, because a 200 dpi receipt has to
// be legible enough to read a date and a total off it.
//
// The design is docs/apps/box/.
package box

import (
	"strings"

	boxweb "dgs-toolbox/internal/apps/box/web"
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

// Model is dgs box: it starts the pages, says where they are, and shows what
// this Box is.
type Model struct {
	settings boxweb.Settings
	url      string
	err      error
	width    int
	height   int
}

// NewModel builds the command model.
func NewModel(settings boxweb.Settings) Model {
	return Model{settings: settings, width: 80, height: 22}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		url, err := boxweb.Serve(m.settings)
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
			return m, openBrowser(m.pageURL())
		}
	}
	return m, nil
}

// pageURL is the address to open. The server's root lands on browse, which
// links to intake and, when there are any, the exceptions.
func (m Model) pageURL() string { return m.url }

func (m Model) View() string {
	heading := "BOX"
	lines := []string{}
	switch {
	case m.err != nil:
		lines = append(lines, warnStyle.Render("Web server failed: "+m.err.Error()))
	case m.url == "":
		lines = append(lines, hintStyle.Render("Starting the local pages…"))
	default:
		lines = append(lines, hintStyle.Render("Page    ")+m.pageURL())
	}
	lines = append(lines, hintStyle.Render("Box     ")+orAsk(m.settings.Root, "not set — box.root"))
	lines = append(lines, hintStyle.Render("Inbox   ")+orAsk(m.settings.Inbox, "not set — box.inbox"))
	if m.settings.Currency != "" {
		lines = append(lines, hintStyle.Render("Amounts ")+m.settings.Currency+hintStyle.Render(" unless the amount names another"))
	} else {
		lines = append(lines, hintStyle.Render("Amounts ")+hintStyle.Render("every amount names its currency"))
	}
	lines = append(lines, hintStyle.Render("Zone    ")+orAsk(m.settings.Zone, "this machine's"))
	if m.settings.Root == "" {
		lines = append(lines, "", hintStyle.Render(
			"No Box: the page opens on made-up scans and says so. Nothing is written to disk.",
		))
	} else {
		lines = append(lines, "", hintStyle.Render(
			"Filing publishes a scan only after reading it back. Nothing is ever deleted.",
		))
	}
	content := titleStyle.Render(heading) + "\n\n" + strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func orAsk(value, fallback string) string {
	if value == "" {
		return hintStyle.Render(fallback)
	}
	return value
}

func (m Model) Status() tui.Status {
	state := "STARTING"
	if m.url != "" {
		state = "SERVING"
	}
	if m.err != nil {
		state = "FAILED"
	}
	return tui.Status{
		Left:   state,
		Center: "Box",
		Right:  "o Open page  esc Back  q Quit",
	}
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		_ = desktop.Open(url)
		return nil
	}
}
