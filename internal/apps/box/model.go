// Package box is the dgs box command: a Box of scanned paper. The terminal
// picks the folders, starts the local pages and reports; the pages own
// everything that involves looking at a scan, because a 200 dpi receipt has to
// be legible enough to read a date and a total off it.
//
// The design is docs/apps/box/.
package box

import (
	"fmt"
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

// Page is which of the two pages a command opens. They share one server and
// one Box; they are separate commands because their risk differs — intake
// copies, verifies and publishes bytes, and browse never writes a scan at all.
type Page string

const (
	// Intake is dgs box import: describing new scans from the inbox.
	Intake Page = "intake"
	// Browse is dgs box view: looking through a Box and correcting it.
	Browse Page = "view"
)

// Model is one box command: it starts the pages, says where they are, and
// shows what this Box is.
type Model struct {
	settings boxweb.Settings
	page     Page
	url      string
	err      error
	width    int
	height   int
}

// NewModel builds the command model for one page.
func NewModel(settings boxweb.Settings, page Page) Model {
	return Model{settings: settings, page: page, width: 80, height: 22}
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

// pageURL is the address of this command's page.
func (m Model) pageURL() string {
	if m.url == "" {
		return ""
	}
	switch m.page {
	case Intake:
		return m.url + "intake/"
	default:
		return m.url + "browse/"
	}
}

func (m Model) View() string {
	heading := "BOX · INTAKE"
	if m.page == Browse {
		heading = "BOX · BROWSE"
	}
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
	if m.page == Intake {
		lines = append(lines, hintStyle.Render("Inbox   ")+orAsk(m.settings.Inbox, "not set — box.inbox"))
	}
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
	} else if m.page == Intake {
		lines = append(lines, "", hintStyle.Render(
			"Filing copies a scan into the Box and publishes it only after reading it back.",
		))
	} else {
		lines = append(lines, "", hintStyle.Render(
			"Browsing writes sidecars and never a scan. Nothing is ever deleted.",
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
	center := "Intake"
	if m.page == Browse {
		center = "Browse"
	}
	state := "STARTING"
	if m.url != "" {
		state = "SERVING"
	}
	if m.err != nil {
		state = "FAILED"
	}
	return tui.Status{
		Left:   state,
		Center: fmt.Sprintf("Box · %s", center),
		Right:  "o Open page  esc Back  q Quit",
	}
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		_ = desktop.Open(url)
		return nil
	}
}
