package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"dgs-toolbox/internal/tui/overlay"

	"github.com/charmbracelet/lipgloss"
)

var (
	accentColor = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"}
	barColor    = lipgloss.AdaptiveColor{Light: "#E8E8F2", Dark: "#242433"}
	barText     = lipgloss.AdaptiveColor{Light: "#20202A", Dark: "#F2F2F2"}

	topBarStyle      = lipgloss.NewStyle().Foreground(barText).Background(barColor)
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
	left := " " + m.breadcrumb()
	right := m.now.Format("15:04:05") + " "
	return topBarStyle.Render(joinLeftRight(left, right, width))
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

func (m Model) breadcrumb() string {
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
		return strings.Join(parts, " › ")
	}
	if m.pickerApp >= 0 {
		parts = append(parts, m.apps[m.pickerApp].ID)
	}
	return strings.Join(append(parts, "commands"), " › ")
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
