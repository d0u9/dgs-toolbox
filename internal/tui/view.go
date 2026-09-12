package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"dgs-toolbox/internal/tui/overlay"

	"github.com/charmbracelet/lipgloss"
)

// minTabWidth is the smallest rendered width of a top-bar tab, and tabGap is
// the number of top-bar cells between two tabs, both in cells.
const (
	minTabWidth = 9
	tabGap      = 1
)

var (
	accentColor = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"}
	barColor    = lipgloss.AdaptiveColor{Light: "#E8E8F2", Dark: "#242433"}
	barText     = lipgloss.AdaptiveColor{Light: "#20202A", Dark: "#F2F2F2"}

	topBarStyle    = lipgloss.NewStyle().Foreground(barText).Background(barColor)
	topBarTabStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#101010"}).
			Background(accentColor).
			Padding(0, 1)
	inactiveTabColor       = lipgloss.AdaptiveColor{Light: "#CFCFE4", Dark: "#3A3A52"}
	topBarInactiveTabStyle = lipgloss.NewStyle().
				Foreground(barText).
				Background(inactiveTabColor).
				Padding(0, 1)
	topBarMetaStyle  = lipgloss.NewStyle().Foreground(barText).Background(barColor)
	pickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(accentColor)
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)
	mutedStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"},
	)
	statusLeftStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#101010"}).
			Background(accentColor)
	statusCenterStyle = lipgloss.NewStyle().Foreground(barText).Background(barColor)
	statusRightStyle  = lipgloss.NewStyle().
				Foreground(barText).
				Background(barColor)
)

// View renders a one-row top bar, a flexible workspace, and a one-row status
// bar. The workspace is delegated to the active command when one exists.
func (m Model) View() string {
	width, height := m.viewportSize()
	top := m.topBar(width)
	if height == 1 {
		return top
	}
	status := m.statusBar(width)
	if height == 2 {
		return top + "\n" + status
	}

	workspace := m.pickerView(width, height-2)
	if m.active != nil {
		workspace = m.active.View()
	}
	if m.confirmQuit {
		workspace = overlay.Place(workspace, m.quitDialog.View(width), width, height-2)
	}
	return top + "\n" + workspace + "\n" + status
}

func (m Model) viewportSize() (int, int) {
	width, height := m.width, m.height
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return width, height
}

func (m Model) topBar(width int) string {
	left := m.topBarTabs()
	// The metadata is fitted to what is left after the tabs, the cell of gap
	// between them, and the one it ends with: sized to the gap alone, it wins
	// that last cell back from the tabs and clips the one the reader is on.
	available := max(0, width-lipgloss.Width(left)-2)
	right := topBarMetaStyle.Render(m.topBarMetadata(available) + " ")
	return renderStatusContent(topBarStyle, joinLeftRight(left, right, width))
}

func (m Model) topBarTabs() string {
	tabs := m.commandTabs()
	if len(tabs) == 0 {
		return topBarTabStyle.Render(m.tabLabel())
	}
	var rendered strings.Builder
	for index, tab := range tabs {
		if index > 0 {
			rendered.WriteString(topBarStyle.Render(strings.Repeat(" ", tabGap)))
		}
		style := topBarInactiveTabStyle
		if tab.Active {
			style = topBarTabStyle
		}
		rendered.WriteString(renderTab(style, tab.Label))
	}
	return rendered.String()
}

// renderTab draws one tab at no less than minTabWidth cells so short labels
// keep a stable, clickable target; longer labels expand instead of truncating.
func renderTab(style lipgloss.Style, label string) string {
	text := strings.ToUpper(label)
	width := max(minTabWidth, lipgloss.Width(text)+2)
	return style.Width(width).Align(lipgloss.Center).Render(text)
}

func (m Model) commandTabs() []Tab {
	if m.active == nil {
		return nil
	}
	contributor, ok := m.active.(TabContributor)
	if !ok {
		return nil
	}
	return contributor.Tabs()
}

// tabAtX maps a top-bar column to a command tab. It mirrors the widths and
// gaps used when the tabs are rendered so hit testing cannot drift from the
// view; a click in the gap between two tabs selects neither.
func (m Model) tabAtX(x int) (int, bool) {
	if x < 0 {
		return 0, false
	}
	start := 0
	for index, tab := range m.commandTabs() {
		width := lipgloss.Width(renderTab(topBarTabStyle, tab.Label))
		if x < start+width {
			return index, true
		}
		start += width + tabGap
	}
	return 0, false
}

func (m Model) topBarMetadata(width int) string {
	metricCells := make([]string, 0, 3)
	if m.topBarVisibility.Disk {
		metricCells = append(metricCells, fmt.Sprintf("DISK R%8s W%8s", fixedMetricValue(formatRate(m.metrics.diskRead), m.metrics.ready), fixedMetricValue(formatRate(m.metrics.diskWrite), m.metrics.ready)))
	}
	if m.topBarVisibility.Network {
		metricCells = append(metricCells, fmt.Sprintf("NET ↑%8s ↓%8s", fixedMetricValue(formatRate(m.metrics.networkUp), m.metrics.ready), fixedMetricValue(formatRate(m.metrics.networkDown), m.metrics.ready)))
	}
	if m.topBarVisibility.CPU {
		metricCells = append(metricCells, fmt.Sprintf("CPU %4s", fixedMetricValue(formatCPU(m.metrics), m.metrics.ready)))
	}
	const separator = " │ "
	parts := make([]string, 0, len(metricCells)+2)
	for count := len(metricCells); count >= 0; count-- {
		parts = append(parts[:0], m.breadcrumb())
		parts = append(parts, metricCells[:count]...)
		if m.topBarVisibility.Time {
			parts = append(parts, m.now.Format("15:04:05"))
		}
		if lipgloss.Width(strings.Join(parts, separator)) <= width || count == 0 {
			break
		}
	}
	return truncate(strings.Join(parts, separator), width)
}

func fixedMetricValue(value string, ready bool) string {
	if !ready {
		return "--"
	}
	return truncate(value, 8)
}

func formatCPU(metrics shellMetrics) string {
	if !metrics.ready {
		return "--"
	}
	return fmt.Sprintf("%.0f%%", metrics.cpuPercent)
}

func formatRate(bytesPerSecond float64) string {
	switch {
	case bytesPerSecond >= 1024*1024*1024:
		return fmt.Sprintf("%.1fG/s", bytesPerSecond/(1024*1024*1024))
	case bytesPerSecond >= 1024*1024:
		return fmt.Sprintf("%.1fM/s", bytesPerSecond/(1024*1024))
	case bytesPerSecond >= 1024:
		return fmt.Sprintf("%.1fK/s", bytesPerSecond/1024)
	default:
		return fmt.Sprintf("%.0fB/s", bytesPerSecond)
	}
}

func (m Model) tabLabel() string {
	if m.active != nil {
		app := m.apps[m.activeApp]
		if app.Direct {
			return strings.ToUpper(app.Name)
		}
		return strings.ToUpper(app.Name + " " + app.Commands[m.activeCommand].Name)
	}
	if m.pickerApp >= 0 {
		return strings.ToUpper(m.apps[m.pickerApp].Name + " COMMANDS")
	}
	return "COMMANDS"
}

func (m Model) pickerView(width, height int) string {
	if m.launchErr != "" {
		content := pickerTitleStyle.Render("LAUNCH ERROR") + "\n\n" + mutedStyle.Render(m.launchErr)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
	}
	if m.pickerHelp {
		content := pickerTitleStyle.Render("PICKER HELP") + "\n\n" +
			mutedStyle.Render("↑/k ↓/j select  •  enter open  •  esc exit  •  q quit")
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
	}

	choices := m.choices()
	var b strings.Builder
	b.WriteString(pickerTitleStyle.Render("CHOOSE A COMMAND"))
	b.WriteString("\n\n")
	if len(choices) == 0 {
		b.WriteString(mutedStyle.Render("No commands available."))
	}
	for i, choice := range choices {
		label := truncate(m.choiceLabel(choice), max(1, width-6))
		if i == m.selected {
			fmt.Fprintf(&b, "› %s", selectedStyle.Render(label))
		} else {
			fmt.Fprintf(&b, "  %s", label)
		}
		if i < len(choices)-1 {
			b.WriteByte('\n')
		}
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, b.String())
}

func (m Model) statusBar(width int) string {
	status := m.pickerStatus()
	if m.active != nil {
		status = m.active.Status()
	}
	if m.confirmQuit {
		status = Status{Left: "CONFIRM · QUIT", Center: "QUIT DGS?"}
	}
	return renderStatusBar(status, width)
}

func renderStatusBar(status Status, width int) string {
	// The state is a compact chip, not a percentage-based panel: on a wide
	// terminal it should remain proportional to its label while the workflow
	// track owns the flexible middle region.
	leftWidth := min(lipgloss.Width(status.Left)+2, max(1, width/4))
	remaining := max(0, width-leftWidth)
	rightWidth := min(lipgloss.Width(status.Right)+2, remaining/2)
	centerWidth := remaining - rightWidth
	return statusCell(statusLeftStyle, status.Left, leftWidth, lipgloss.Left) +
		statusCenterCell(statusCenterStyle, status.Center, centerWidth, leftWidth, width) +
		statusCell(statusRightStyle, status.Right, rightWidth, lipgloss.Right)
}

// statusCell gives every status region a small horizontal inset. At extreme
// narrow widths it relinquishes the inset rather than allowing the one-row
// shell chrome to overflow.
func statusCell(style lipgloss.Style, text string, width int, position lipgloss.Position) string {
	if width <= 0 {
		return ""
	}
	if width < 3 {
		return renderStatusContent(style, placeText(text, width, position))
	}
	return renderStatusContent(style.Padding(0, 1), placeText(text, width-2, position))
}

// statusCenterCell positions its text against the center of the complete bar,
// not the center of the leftover region between asymmetrical side cells.
func statusCenterCell(style lipgloss.Style, text string, width, leftWidth, totalWidth int) string {
	if width <= 0 {
		return ""
	}
	if width < 3 {
		return renderStatusContent(style, placeText(text, width, lipgloss.Center))
	}
	innerWidth := width - 2
	text = truncate(text, innerWidth)
	textWidth := lipgloss.Width(text)
	desiredX := (totalWidth - textWidth) / 2
	localX := desiredX - leftWidth - 1
	localX = max(0, min(localX, innerWidth-textWidth))
	content := strings.Repeat(" ", localX) + text + strings.Repeat(" ", innerWidth-localX-textWidth)
	return renderStatusContent(style.Padding(0, 1), content)
}

// renderStatusContent restores the cell style after nested ANSI spans reset
// themselves. Without this, a styled stepper can expose the terminal's own
// background in rectangular patches inside the status bar.
func renderStatusContent(style lipgloss.Style, content string) string {
	probe := style.Render("x")
	marker := strings.Index(probe, "x")
	if marker > 0 {
		prefix := strings.TrimRight(probe[:marker], " ")
		content = strings.ReplaceAll(content, "\x1b[0m", "\x1b[0m"+prefix)
	}
	return style.Render(content)
}

func (m Model) pickerStatus() Status {
	context := "ALL COMMANDS"
	if m.pickerApp >= 0 {
		context = strings.ToUpper(m.apps[m.pickerApp].Name) + " COMMANDS"
	}
	if m.pickerHelp {
		return Status{Left: "PICKER", Center: context, Right: "? Close  esc Exit  q Quit"}
	}
	return Status{Left: "PICKER", Center: context, Right: "↑/k ↓/j Move  ↵ Open  q Quit"}
}

// breadcrumbSeparator is the filled chevron between two crumbs. It is drawn in
// the colour of the crumb to its right on the background of the one to its
// left, so the trail reads as solid arrows pointing back the way it came
// rather than as a punctuation mark between words.
const breadcrumbSeparator = "\ue0b2"

// breadcrumb is the trail to what is on screen, deepest first. It sits at the
// right-hand end of the top bar, where a trail reading left to right would put
// the part the reader cares about — where they are — furthest from the edge
// they are looking at, so it runs the other way: "scan ◀ capture ◀ dgs".
func (m Model) breadcrumb() string {
	trail := m.breadcrumbTrail()
	var rendered strings.Builder
	for index, crumb := range trail {
		if index > 0 {
			// The chevron belongs to the boundary rather than to either crumb:
			// its point is the colour of what follows it, so the segments
			// interlock instead of sitting in a row of separate blocks.
			rendered.WriteString(lipgloss.NewStyle().
				Foreground(breadcrumbBackground(index, len(trail))).
				Background(breadcrumbBackground(index-1, len(trail))).
				Render(breadcrumbSeparator))
		}
		rendered.WriteString(breadcrumbStyle(index, len(trail)).Render(crumb))
	}
	return rendered.String()
}

// breadcrumbTrail is the path itself, deepest first: what is on screen, then
// what it sits inside, ending at the shell.
func (m Model) breadcrumbTrail() []string {
	parts := []string{"dgs"}
	if m.active != nil {
		app := m.apps[m.activeApp]
		command := app.Commands[m.activeCommand]
		if app.Direct {
			parts = append(parts, app.ID)
		} else {
			parts = append(parts, app.ID, command.ID)
		}
		if contributor, ok := m.active.(CommandPathContributor); ok {
			parts = append(parts, contributor.CommandPath()...)
		}
	} else {
		if m.pickerApp >= 0 {
			parts = append(parts, m.apps[m.pickerApp].ID)
		}
		parts = append(parts, "commands")
	}
	slices.Reverse(parts)
	return parts
}

// breadcrumbBackground is the ground one crumb sits on: the accent for where
// the reader is, a quieter shade for what contains it, and the bar itself for
// the shell, so the trail fades into the bar as it goes back.
func breadcrumbBackground(index, count int) lipgloss.TerminalColor {
	switch {
	case index == 0:
		return accentColor
	case index == count-1:
		return barColor
	default:
		return inactiveTabColor
	}
}

func breadcrumbStyle(index, count int) lipgloss.Style {
	style := lipgloss.NewStyle().Padding(0, 1).Background(breadcrumbBackground(index, count))
	if index == 0 {
		return style.Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#101010"})
	}
	return style.Foreground(barText)
}

func joinLeftRight(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	right = truncate(right, width)
	rightWidth := lipgloss.Width(right)
	if rightWidth >= width {
		return placeText(right, width, lipgloss.Right)
	}
	left = truncate(left, width-rightWidth-1)
	gap := width - lipgloss.Width(left) - rightWidth
	return left + strings.Repeat(" ", gap) + right
}

func placeText(text string, width int, position lipgloss.Position) string {
	if width <= 0 {
		return ""
	}
	text = truncate(text, width)
	padding := width - lipgloss.Width(text)
	switch position {
	case lipgloss.Right:
		return strings.Repeat(" ", padding) + text
	case lipgloss.Center:
		left := padding / 2
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", padding-left)
	default:
		return text + strings.Repeat(" ", padding)
	}
}

func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}

	var b strings.Builder
	used := 0
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		runeWidth := lipgloss.Width(string(r))
		if used+runeWidth > width-1 {
			break
		}
		b.WriteRune(r)
		used += runeWidth
		text = text[size:]
	}
	return b.String() + "…"
}
