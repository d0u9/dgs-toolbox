package photo

import (
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/fileexplorer"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	sourceField = iota
	destinationField
	pathFieldCount
)

var (
	importAccent = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	importMuted  = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"}
	importPanel  = lipgloss.AdaptiveColor{Light: "#EAF4F4", Dark: "#243447"}

	importTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(importAccent)
	importLabelStyle  = lipgloss.NewStyle().Foreground(importMuted).Width(13)
	importFocusStyle  = lipgloss.NewStyle().Bold(true).Foreground(importAccent)
	importNoteStyle   = lipgloss.NewStyle().Foreground(importMuted)
	importActionStyle = lipgloss.NewStyle().Foreground(importMuted)
	importFilterStyle = lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#ECFEFF", Dark: "#134E4A"}).
				Background(importAccent).
				Padding(0, 1)
	importModalStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(importAccent).
				Background(importPanel)
)

// importModel owns the Photo Import setup workspace. Its path fields open a
// read-only directory picker; review and scan behavior remain disabled.
type importModel struct {
	paths     [pathFieldCount]string
	focus     int
	picking   bool
	pickerFor int
	picker    fileexplorer.Model
	width     int
	height    int
	help      bool
}

func newImportModel() tui.CommandModel {
	return importModel{
		paths:  [pathFieldCount]string{"/Volumes/LEICA", "~/Pictures/Photos"},
		width:  80,
		height: 22,
	}
}

func (m importModel) Init() tea.Cmd { return nil }

func (m importModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
		return m, nil
	}
}

func (m importModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			m.paths[m.pickerFor] = displayPath(selected)
			m.picking = false
		}
		return m, cmd
	}

	switch key {
	case "up", "k", "shift+tab":
		m.help = false
		m.focus = (m.focus - 1 + pathFieldCount) % pathFieldCount
	case "down", "j", "tab":
		m.help = false
		m.focus = (m.focus + 1) % pathFieldCount
	case "enter":
		m.help = false
		return m.openPicker(m.focus)
	case "?":
		m.help = !m.help
	}
	return m, nil
}

func (m importModel) openPicker(field int) (tea.Model, tea.Cmd) {
	width, height := m.modalSize()
	m.picker = fileexplorer.New(
		pickerStart(m.paths[field]),
		width-2,
		height-6,
		fileexplorer.WithFilter(fileexplorer.Directories()),
	)
	m.pickerFor = field
	m.picking = true
	return m, m.picker.Init()
}

func (m *importModel) sizePicker() {
	width, height := m.modalSize()
	m.picker.SetSize(width-2, height-6)
}

func (m importModel) modalSize() (int, int) {
	width := min(72, m.width-4)
	height := min(22, m.height-4)
	return max(20, width), max(8, height)
}

func pickerStart(path string) string {
	path = expandHome(path)
	for path != "" {
		info, err := os.Stat(path)
		if err == nil {
			if info.IsDir() {
				return path
			}
			return filepath.Dir(path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return "."
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

func displayPath(path string) string {
	cleaned := filepath.Clean(path)
	home, err := os.UserHomeDir()
	if err == nil && (cleaned == home || strings.HasPrefix(cleaned, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(cleaned, home)
	}
	return cleaned
}

func (m importModel) View() string {
	content := m.formView()
	if m.help && !m.picking {
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			importTitleStyle.Render("PHOTO IMPORT HELP"),
			"",
			"↑/k ↓/j or Tab  Move between paths",
			"Enter       Browse for a directory",
			"Esc         Return to commands",
			"q           Quit dgs",
		)
	}
	workspace := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	if m.picking {
		return overlay(workspace, m.pickerView(), m.width, m.height)
	}
	return workspace
}

func (m importModel) pickerView() string {
	width, height := m.modalSize()
	label := "FILE EXPLORER · SOURCE"
	if m.pickerFor == destinationField {
		label = "FILE EXPLORER · DESTINATION"
	}
	filter := importFilterStyle.Render(m.picker.FilterLabel())
	headerWidth := max(1, width-4)
	label = ansi.Truncate(label, max(1, headerWidth-lipgloss.Width(filter)-1), "…")
	header := importTitleStyle.Render(label) +
		strings.Repeat(" ", max(1, headerWidth-lipgloss.Width(label)-lipgloss.Width(filter))) +
		filter
	selected := ansi.Truncate(displayPath(m.picker.SelectedPath()), max(1, width-4), "…")
	if notice := m.picker.Notice(); notice != "" {
		selected = ansi.Truncate(notice, max(1, width-4), "…")
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		importNoteStyle.Render(selected),
		"",
		m.picker.View(),
		importNoteStyle.Render(m.picker.Hint()),
	)
	return importModalStyle.Width(width - 2).Height(height - 2).MaxWidth(width).MaxHeight(height).Render(body)
}

func (m importModel) formView() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		importTitleStyle.Render("PHOTO IMPORT"),
		"",
		m.pathRow(sourceField, "Source"),
		m.pathRow(destinationField, "Destination"),
		"",
		importActionStyle.Render("· Review import"),
		"",
		importNoteStyle.Render("Review the plan before any files are copied."),
	)
}

func (m importModel) pathRow(index int, label string) string {
	marker := "  "
	valueWidth := min(48, max(8, m.width-20))
	value := ansi.Truncate(m.paths[index], valueWidth, "…")
	if index == m.focus {
		marker = importFocusStyle.Render("› ")
		value = importFocusStyle.Render(value)
	}
	return marker + importLabelStyle.Render(label) + value
}

func (m importModel) Status() tui.Status {
	if m.picking {
		label := "Source"
		if m.pickerFor == destinationField {
			label = "Destination"
		}
		if m.picker.HasDialog() {
			return tui.Status{Left: "BROWSE", Center: "Select " + label}
		}
		return tui.Status{
			Left:   "BROWSE",
			Center: "Select " + label,
			Right:  m.picker.Hint(),
		}
	}
	if m.help {
		return tui.Status{Left: "HELP", Center: "Photo Import paths", Right: "? Close  esc Commands  q Quit"}
	}
	return tui.Status{
		Left:   "READY",
		Center: "No source scanned",
		Right:  "↑/k ↓/j  ↵ Browse  ? Help  esc Back  q Quit",
	}
}

// CapturesShellKey keeps Esc inside the temporary picker so it cancels the
// picker instead of leaving Photo Import.
func (m importModel) CapturesShellKey(key string) bool {
	return m.picking && (key == "esc" || (key == "q" && m.picker.CapturesText()))
}

func overlay(background, foreground string, width, height int) string {
	frontWidth := lipgloss.Width(foreground)
	frontHeight := lipgloss.Height(foreground)
	x := max(0, (width-frontWidth)/2)
	y := max(0, (height-frontHeight)/2)

	backLines := strings.Split(background, "\n")
	frontLines := strings.Split(foreground, "\n")
	for i := range backLines {
		backLines[i] = padWidth(backLines[i], width)
	}
	for len(backLines) < height {
		backLines = append(backLines, strings.Repeat(" ", width))
	}
	for i, frontLine := range frontLines {
		row := y + i
		if row >= len(backLines) || row >= height {
			break
		}
		backLine := backLines[row]
		left := ansi.Cut(backLine, 0, x)
		right := ansi.Cut(backLine, x+frontWidth, width)
		backLines[row] = padWidth(left, x) + padWidth(frontLine, frontWidth) + padWidth(right, width-x-frontWidth)
	}
	return strings.Join(backLines[:height], "\n")
}

func padWidth(value string, width int) string {
	value = ansi.Truncate(value, max(0, width), "")
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}
