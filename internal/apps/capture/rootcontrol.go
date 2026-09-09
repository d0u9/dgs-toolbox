package capture

import (
	"strings"

	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// rootControl is the compact CAPTURE ROOT path control together with the
// directory-only File Explorer overlay it opens. Scan and Route share one
// implementation so both sessions present the same control and the same
// browsing behavior.
type rootControl struct {
	root     string
	controls form.Model
	picking  bool
	picker   fileexplorer.Model
}

func newRootControl(root string) rootControl {
	return rootControl{
		root: root,
		controls: form.New(form.Field{
			ID: rootPathID, Kind: form.Path, Label: "Root", Value: displayPath(root),
		}),
	}
}

// Root reports the directory the control currently points at.
func (c rootControl) Root() string { return c.root }

// Display is the path text shown in the control.
func (c rootControl) Display() string { return c.controls.Value(rootPathID) }

// Picking reports whether the File Explorer overlay is open.
func (c rootControl) Picking() bool { return c.picking }

// SetRoot points the control at a directory without opening the explorer.
func (c *rootControl) SetRoot(root string) {
	c.root = root
	c.controls.SetValue(rootPathID, displayPath(root))
}

// Open shows the File Explorer overlay inside a workspace of the given size.
func (c *rootControl) Open(width, height int) tea.Cmd {
	modalWidth, modalHeight := rootModalSize(width, height)
	c.picker = fileexplorer.New(c.root, modalWidth-2, modalHeight-6, fileexplorer.WithFilter(fileexplorer.Directories()))
	c.picking = true
	return c.picker.Init()
}

// Resize keeps an open overlay proportional to the workspace.
func (c *rootControl) Resize(width, height int) {
	if !c.picking {
		return
	}
	modalWidth, modalHeight := rootModalSize(width, height)
	c.picker.SetSize(modalWidth-2, modalHeight-6)
}

// Update forwards a message to the open overlay. It returns the newly chosen
// root, or an empty string when nothing was selected. Esc closes the overlay
// without changing the root, unless the explorer owns a dialog.
func (c *rootControl) Update(msg tea.Msg) (string, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" && !c.picker.HasDialog() {
		c.picking = false
		return "", nil
	}
	picker, selected, cmd := c.picker.Update(msg)
	c.picker = picker
	if selected == "" {
		return "", cmd
	}
	c.SetRoot(selected)
	c.picking = false
	return selected, cmd
}

// CapturesShellKey reports whether the overlay owns a global shell key.
func (c rootControl) CapturesShellKey(key string) bool {
	return c.picking && (key == "esc" || (key == "q" && c.picker.CapturesText()))
}

// Clicked reports whether a primary click at the given control-relative
// position hit the path row.
func (c *rootControl) Clicked(x, y int) bool {
	id, used := c.controls.Click([]string{rootPathID}, x, y)
	return used && id == rootPathID
}

// Fieldset renders the whole CAPTURE ROOT Fieldset at the given width.
func (c rootControl) Fieldset(width int, focused bool) string {
	return fieldset.ViewFocused("CAPTURE ROOT", c.pathView(width-4, focused), width, focused)
}

func (c rootControl) pathView(width int, focused bool) string {
	marker := "  "
	style := lipgloss.NewStyle()
	if focused {
		marker = "› "
		style = scanFocusedPathStyle
	}
	path := ansi.Truncate(c.controls.Value(rootPathID), max(1, width-2), "…")
	return style.Width(max(1, width)).MaxWidth(max(1, width)).Render(marker + path)
}

// OverlayView renders the File Explorer modal for the given workspace size.
func (c rootControl) OverlayView(width, height int) string {
	modalWidth, modalHeight := rootModalSize(width, height)
	label := "FILE EXPLORER · CAPTURE ROOT"
	filter := scanFilterStyle.Render(c.picker.FilterLabel())
	headerWidth := max(1, modalWidth-4)
	label = ansi.Truncate(label, max(1, headerWidth-lipgloss.Width(filter)-1), "…")
	header := scanTitleStyle.Render(label) + strings.Repeat(" ", max(1, headerWidth-lipgloss.Width(label)-lipgloss.Width(filter))) + filter
	selected := ansi.Truncate(displayPath(c.picker.SelectedPath()), headerWidth, "…")
	if notice := c.picker.Notice(); notice != "" {
		selected = ansi.Truncate(notice, headerWidth, "…")
	}
	body := lipgloss.JoinVertical(lipgloss.Left, header, scanMutedStyle.Render(selected), "", c.picker.View(), scanMutedStyle.Render(c.picker.Hint()))
	return scanModalStyle.Width(modalWidth - 2).Height(modalHeight - 2).MaxWidth(modalWidth).MaxHeight(modalHeight).Render(body)
}

// HasDialog reports whether the explorer currently owns a dialog.
func (c rootControl) HasDialog() bool { return c.picker.HasDialog() }

// Hint is the explorer's status-bar hint.
func (c rootControl) Hint() string { return c.picker.Hint() }

func rootModalSize(width, height int) (int, int) {
	return max(20, min(96, width-4)), max(8, min(30, height-2))
}
