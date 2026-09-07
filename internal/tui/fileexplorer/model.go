package fileexplorer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var explorerID atomic.Int64

var (
	textColor       = lipgloss.AdaptiveColor{Light: "#334155", Dark: "#E2E8F0"}
	mutedColor      = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	disclosureColor = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	selectedText    = lipgloss.AdaptiveColor{Light: "#134E4A", Dark: "#ECFEFF"}
	selectedBG      = lipgloss.AdaptiveColor{Light: "#CCFBF1", Dark: "#334155"}
	surfaceColor    = lipgloss.AdaptiveColor{Light: "#EAF4F4", Dark: "#243447"}
	errorColor      = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#FCA5A5"}

	rowStyle        = lipgloss.NewStyle().Foreground(textColor).Background(surfaceColor)
	disclosureStyle = lipgloss.NewStyle().Foreground(disclosureColor).Background(surfaceColor)
	selectedStyle   = lipgloss.NewStyle().Bold(true).Foreground(selectedText).Background(selectedBG)
	errorStyle      = lipgloss.NewStyle().Foreground(errorColor).Background(surfaceColor)
	mutedStyle      = lipgloss.NewStyle().Foreground(mutedColor).Background(surfaceColor)
)

type directoryNode struct {
	path     string
	name     string
	depth    int
	parent   *directoryNode
	children []*directoryNode
	expanded bool
	loaded   bool
	loading  bool
	err      error
	isDir    bool
}

type directoryEntry struct {
	name  string
	isDir bool
}

type directoryReadMsg struct {
	id      int64
	path    string
	entries []directoryEntry
	err     error
}

// Model is a lazy directory-only tree backed by Bubbles viewport.
// Nodes are read only when expanded and retained until the popup is closed.
type Model struct {
	id              int64
	root            *directoryNode
	visible         []*directoryNode
	selected        int
	viewport        viewport.Model
	filter          Filter
	action          actionMode
	target          *directoryNode
	editor          textinput.Model
	notice          string
	completions     []string
	completionIndex int
	focusAfterLoad  string
}

func New(root string, width, height int, options ...Option) Model {
	configuration := config{filter: Directories()}
	for _, option := range options {
		option(&configuration)
	}
	node := &directoryNode{
		path:     root,
		name:     displayPath(root),
		expanded: true,
		loading:  true,
		isDir:    true,
	}
	tree := Model{
		id:       explorerID.Add(1),
		root:     node,
		viewport: viewport.New(max(1, width), max(1, height)),
		filter:   configuration.filter,
	}
	tree.viewport.MouseWheelEnabled = false
	tree.editor = textinput.New()
	tree.editor.Prompt = ""
	tree.editor.CharLimit = 255
	tree.editor.Width = max(12, width-12)
	tree.editor.TextStyle = rowStyle
	tree.editor.PlaceholderStyle = mutedStyle
	tree.editor.Cursor.Style = disclosureStyle
	tree.refresh()
	return tree
}

func (t Model) Init() tea.Cmd {
	return readDirectory(t.id, t.root.path, t.filter)
}

func (t Model) Update(msg tea.Msg) (Model, string, tea.Cmd) {
	switch msg := msg.(type) {
	case directoryReadMsg:
		if msg.id != t.id {
			return t, "", nil
		}
		node := t.find(msg.path)
		if node == nil {
			return t, "", nil
		}
		node.loading = false
		node.loaded = true
		node.err = msg.err
		node.children = make([]*directoryNode, 0, len(msg.entries))
		for _, entry := range msg.entries {
			node.children = append(node.children, &directoryNode{
				path:   filepath.Join(node.path, entry.name),
				name:   entry.name,
				depth:  node.depth + 1,
				parent: node,
				isDir:  entry.isDir,
			})
		}
		t.refresh()
		if t.focusAfterLoad != "" {
			t.focusPath(t.focusAfterLoad)
			t.focusAfterLoad = ""
			t.refresh()
		}
		return t, "", nil
	case operationMsg:
		if msg.id != t.id {
			return t, "", nil
		}
		if msg.err != nil {
			t.notice = msg.err.Error()
			return t, "", nil
		}
		t.notice = ""
		t.applyOperation(msg)
		t.refresh()
		return t, "", nil
	case tea.KeyMsg:
		return t.updateKey(msg)
	default:
		return t, "", nil
	}
}

func (t Model) updateKey(msg tea.KeyMsg) (Model, string, tea.Cmd) {
	key := msg.String()
	if t.action != actionNone {
		return t.updateAction(msg)
	}
	if len(t.visible) == 0 {
		return t, "", nil
	}
	switch key {
	case "up", "k":
		t.selected = max(0, t.selected-1)
	case "down", "j":
		t.selected = min(len(t.visible)-1, t.selected+1)
	case "home", "g":
		t.selected = 0
	case "end", "G":
		t.selected = len(t.visible) - 1
	case "enter":
		node := t.visible[t.selected]
		if t.selectable(node) {
			return t, node.path, nil
		}
		if node.isDir {
			return t.expandSelected(false)
		}
	case "right", "l":
		return t.expandSelected(true)
	case "o":
		node := t.visible[t.selected]
		if !node.isDir {
			return t, "", nil
		}
		if node.expanded {
			node.expanded = false
			t.refresh()
			return t, "", nil
		}
		return t.expandSelected(false)
	case "left", "h":
		node := t.visible[t.selected]
		if node.expanded {
			node.expanded = false
		} else if node.parent != nil {
			for i, visible := range t.visible {
				if visible == node.parent {
					t.selected = i
					break
				}
			}
		}
	case "O":
		node := t.visible[t.selected]
		if node.parent != nil {
			node.parent.expanded = false
			t.focusNode(node.parent)
		}
	case "w":
		collapseRecursively(t.root)
		t.selected = 0
	case "a":
		target := t.visible[t.selected]
		if !target.isDir {
			target = target.parent
		}
		if target != nil {
			t.beginEdit(actionCreate, target, "")
			return t, "", textinput.Blink
		}
	case "r":
		target := t.visible[t.selected]
		if target.parent == nil {
			t.notice = "the explorer root cannot be renamed"
			break
		}
		t.beginEdit(actionRename, target, target.name)
		return t, "", textinput.Blink
	case "d":
		target := t.visible[t.selected]
		if target.parent == nil {
			t.notice = "the explorer root cannot be deleted"
			break
		}
		t.action = actionDelete
		t.target = target
		t.notice = ""
	case "R":
		return t.reload()
	case "-":
		return t.moveRootUp()
	case "=":
		return t.makeSelectedRoot()
	case "/":
		t.action = actionPath
		t.notice = ""
		t.completions = nil
		t.completionIndex = 0
		t.editor.SetValue(displayPath(t.SelectedPath()))
		t.editor.Focus()
		return t, "", textinput.Blink
	}
	t.refresh()
	return t, "", nil
}

func (t Model) makeSelectedRoot() (Model, string, tea.Cmd) {
	target := t.visible[t.selected]
	if !target.isDir {
		target = target.parent
	}
	if target == nil || target == t.root {
		return t, "", nil
	}
	target.parent = nil
	target.name = displayPath(target.path)
	target.expanded = true
	setDepth(target, 0)
	t.root = target
	t.selected = 0
	t.notice = ""
	t.refresh()
	if !target.loaded && !target.loading {
		target.loading = true
		t.refresh()
		return t, "", readDirectory(t.id, target.path, t.filter)
	}
	return t, "", nil
}

func setDepth(node *directoryNode, depth int) {
	node.depth = depth
	for _, child := range node.children {
		setDepth(child, depth+1)
	}
}

func (t Model) moveRootUp() (Model, string, tea.Cmd) {
	oldRoot := t.root.path
	parent := filepath.Dir(oldRoot)
	if parent == oldRoot {
		return t, "", nil
	}
	t.id = explorerID.Add(1)
	t.root = &directoryNode{
		path: parent, name: displayPath(parent), expanded: true, loading: true, isDir: true,
	}
	t.selected = 0
	t.focusAfterLoad = oldRoot
	t.notice = ""
	t.refresh()
	return t, "", readDirectory(t.id, parent, t.filter)
}

func (t *Model) beginEdit(action actionMode, target *directoryNode, value string) {
	t.action = action
	t.target = target
	t.notice = ""
	t.editor.SetValue(value)
	t.editor.CursorEnd()
	t.editor.Focus()
}

func (t Model) updateAction(msg tea.KeyMsg) (Model, string, tea.Cmd) {
	key := msg.String()
	if key == "esc" {
		t.action = actionNone
		t.target = nil
		t.editor.Blur()
		return t, "", nil
	}
	if t.action == actionDelete {
		switch key {
		case "y", "Y":
			path := t.target.path
			t.action = actionNone
			return t, "", runOperation(t.id, deleteEntry, path, "")
		case "n", "N":
			t.action = actionNone
			t.target = nil
		}
		return t, "", nil
	}
	if t.action == actionPath {
		if msg.Paste {
			t.editor.SetValue(trimPathInput(string(msg.Runes)))
			t.editor.CursorEnd()
			t.completions = nil
			t.completionIndex = 0
			t.notice = ""
			return t, "", nil
		}
		switch key {
		case "tab":
			t.completePath(false)
			return t, "", nil
		case "shift+tab":
			t.completePath(true)
			return t, "", nil
		case "enter":
			updated, selected, err := t.acceptPath()
			if err != nil {
				t.notice = err.Error()
				return t, "", nil
			}
			if selected == "" && updated.action == actionNone {
				return updated, "", readDirectory(updated.id, updated.root.path, updated.filter)
			}
			return updated, selected, nil
		}
		var cmd tea.Cmd
		t.editor, cmd = t.editor.Update(msg)
		t.completions = nil
		t.completionIndex = 0
		t.notice = ""
		return t, "", cmd
	}
	if key == "enter" {
		parent := t.target
		kind := createDirectory
		source := ""
		if t.action == actionRename {
			parent = t.target.parent
			kind = renameEntry
			source = t.target.path
		}
		destination, err := childPath(parent.path, t.editor.Value())
		if err != nil {
			t.notice = err.Error()
			return t, "", nil
		}
		t.action = actionNone
		t.editor.Blur()
		return t, "", runOperation(t.id, kind, source, destination)
	}
	var cmd tea.Cmd
	t.editor, cmd = t.editor.Update(msg)
	return t, "", cmd
}

func (t Model) reload() (Model, string, tea.Cmd) {
	t.id = explorerID.Add(1)
	t.root = &directoryNode{
		path: t.root.path, name: displayPath(t.root.path), expanded: true, loading: true, isDir: true,
	}
	t.selected = 0
	t.action = actionNone
	t.target = nil
	t.refresh()
	return t, "", readDirectory(t.id, t.root.path, t.filter)
}

func (t *Model) applyOperation(msg operationMsg) {
	switch msg.kind {
	case createDirectory:
		parent := t.find(filepath.Dir(msg.destination))
		if parent == nil {
			return
		}
		node := &directoryNode{
			path: msg.destination, name: filepath.Base(msg.destination), depth: parent.depth + 1,
			parent: parent, isDir: true,
		}
		parent.children = append(parent.children, node)
		sortChildren(parent.children)
		t.focusPath(msg.destination)
	case renameEntry:
		node := t.find(msg.source)
		if node == nil {
			return
		}
		updateNodePath(node, msg.source, msg.destination)
		node.name = filepath.Base(msg.destination)
		sortChildren(node.parent.children)
		t.focusPath(msg.destination)
	case deleteEntry:
		node := t.find(msg.source)
		if node == nil || node.parent == nil {
			return
		}
		parent := node.parent
		for i, child := range parent.children {
			if child == node {
				parent.children = append(parent.children[:i], parent.children[i+1:]...)
				break
			}
		}
		t.focusNode(parent)
	}
}

func updateNodePath(node *directoryNode, oldPrefix, newPrefix string) {
	node.path = newPrefix + strings.TrimPrefix(node.path, oldPrefix)
	for _, child := range node.children {
		updateNodePath(child, oldPrefix, newPrefix)
	}
}

func sortChildren(children []*directoryNode) {
	sort.Slice(children, func(i, j int) bool {
		if children[i].isDir != children[j].isDir {
			return children[i].isDir
		}
		return strings.ToLower(children[i].name) < strings.ToLower(children[j].name)
	})
}

func (t *Model) focusPath(path string) {
	t.refresh()
	if node := t.find(path); node != nil {
		t.focusNode(node)
	}
}

func collapseRecursively(node *directoryNode) {
	if node.isDir {
		node.expanded = false
	}
	for _, child := range node.children {
		collapseRecursively(child)
	}
}

func (t *Model) focusNode(target *directoryNode) {
	for i, node := range t.visible {
		if node == target {
			t.selected = i
			return
		}
	}
}

func (t Model) expandSelected(moveToChild bool) (Model, string, tea.Cmd) {
	node := t.visible[t.selected]
	if !node.isDir {
		return t, "", nil
	}
	if !node.expanded {
		node.expanded = true
		if !node.loaded && !node.loading {
			node.loading = true
			t.refresh()
			return t, "", readDirectory(t.id, node.path, t.filter)
		}
	} else if moveToChild && len(node.children) > 0 {
		t.selected++
	}
	t.refresh()
	return t, "", nil
}

func (t *Model) SetSize(width, height int) {
	t.viewport.Width = max(1, width)
	t.viewport.Height = max(1, height)
	t.editor.Width = max(12, width-12)
	t.refresh()
}

func (t Model) View() string {
	if t.action == actionDelete {
		prompt := errorStyle.Bold(true).Render("DELETE ") + rowStyle.Render(t.target.name) +
			mutedStyle.Render("  y confirm  •  n/esc cancel")
		return t.viewWithBottomPrompt(prompt)
	}
	if t.action == actionCreate || t.action == actionRename {
		title := "NEW FOLDER"
		if t.action == actionRename {
			title = "RENAME"
		}
		content := disclosureStyle.Bold(true).Render(title+"  ") + t.editor.View()
		if t.notice != "" {
			content = errorStyle.Render(t.notice+"  ") + t.editor.View()
		}
		return t.viewWithBottomPrompt(content)
	}
	if t.action == actionPath {
		return t.pathInputView()
	}
	return t.viewport.View()
}

func (t Model) pathInputView() string {
	const maxVisibleCompletions = 5
	start := 0
	if t.completionIndex >= maxVisibleCompletions {
		start = t.completionIndex - maxVisibleCompletions + 1
	}
	end := min(len(t.completions), start+maxVisibleCompletions)
	completionLines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		marker := "  "
		style := rowStyle
		if index == t.completionIndex {
			marker = "› "
			style = selectedStyle
		}
		name := filepath.Base(strings.TrimSuffix(t.completions[index], string(filepath.Separator)))
		completionLines = append(completionLines, style.Render(marker+name))
	}
	prompt := disclosureStyle.Bold(true).Render("PATH  ") + t.editor.View()
	if t.notice != "" {
		prompt = errorStyle.Render(t.notice+"  ") + t.editor.View()
	}
	panelHeight := len(completionLines) + 2
	view := t.viewport
	view.Height = max(1, t.viewport.Height-panelHeight)
	if t.selected >= view.YOffset+view.Height {
		view.SetYOffset(t.selected - view.Height + 1)
	}
	parts := []string{view.View(), mutedStyle.Render(strings.Repeat("─", max(1, t.viewport.Width)))}
	parts = append(parts, completionLines...)
	parts = append(parts, rowStyle.Width(t.viewport.Width).MaxWidth(t.viewport.Width).Render(prompt))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Model) viewWithBottomPrompt(prompt string) string {
	view := t.viewport
	view.Height = max(1, t.viewport.Height-2)
	if t.selected >= view.YOffset+view.Height {
		view.SetYOffset(t.selected - view.Height + 1)
	}
	separator := mutedStyle.Render(strings.Repeat("─", max(1, t.viewport.Width)))
	prompt = rowStyle.Width(t.viewport.Width).MaxWidth(t.viewport.Width).Render(prompt)
	return lipgloss.JoinVertical(lipgloss.Left, view.View(), separator, prompt)
}

// CapturesText reports whether printable keys belong to an active prompt.
func (t Model) CapturesText() bool {
	return t.action == actionCreate || t.action == actionRename || t.action == actionPath
}

// HasDialog reports whether Esc should close an explorer operation instead of the explorer.
func (t Model) HasDialog() bool {
	return t.action != actionNone
}

// Hint returns controls for the explorer's current interaction state.
func (t Model) Hint() string {
	switch t.action {
	case actionDelete:
		// Delete controls are rendered beside the confirmation prompt.
		return ""
	case actionCreate, actionRename:
		return "enter apply  •  esc cancel"
	case actionPath:
		return "tab complete  •  shift+tab previous  •  enter select  •  esc cancel"
	default:
		return "- up  •  = root  •  / path  •  a new  •  d del  •  r ren  •  R reload"
	}
}

// Notice returns the latest non-fatal operation error.
func (t Model) Notice() string {
	return t.notice
}

func (t Model) SelectedPath() string {
	if len(t.visible) == 0 {
		return ""
	}
	return t.visible[t.selected].path
}

// FilterLabel returns a compact description such as DIR, JPG, PNG, or ALL FILES.
func (t Model) FilterLabel() string {
	return t.filter.Label()
}

func (t Model) selectable(node *directoryNode) bool {
	if t.filter.kind == directoryFilter {
		return node.isDir
	}
	return !node.isDir
}

func (t *Model) refresh() {
	t.visible = t.visible[:0]
	appendVisible(&t.visible, t.root)
	if len(t.visible) == 0 {
		t.selected = 0
		return
	}
	t.selected = min(max(0, t.selected), len(t.visible)-1)

	lines := make([]string, len(t.visible))
	for i, node := range t.visible {
		lines[i] = renderDirectoryNode(node, i == t.selected)
	}
	t.viewport.SetContent(strings.Join(lines, "\n"))
	if t.selected < t.viewport.YOffset {
		t.viewport.SetYOffset(t.selected)
	}
	if t.selected >= t.viewport.YOffset+t.viewport.Height {
		t.viewport.SetYOffset(t.selected - t.viewport.Height + 1)
	}
}

func appendVisible(visible *[]*directoryNode, node *directoryNode) {
	*visible = append(*visible, node)
	if !node.expanded {
		return
	}
	for _, child := range node.children {
		appendVisible(visible, child)
	}
}

func renderDirectoryNode(node *directoryNode, selected bool) string {
	if !node.isDir {
		marker := "  "
		style := rowStyle
		if selected {
			marker = "› "
			style = selectedStyle
		}
		return style.Render(marker + strings.Repeat("  ", node.depth) + "  " + node.name)
	}
	icon := "▸"
	switch {
	case node.loading:
		icon = "…"
	case node.err != nil:
		icon = "!"
	case node.expanded:
		icon = "▾"
	case node.loaded && len(node.children) == 0:
		icon = "·"
	}
	marker := "  "
	style := rowStyle
	if selected {
		marker = "› "
		style = selectedStyle
	}
	iconText := disclosureStyle.Render(icon)
	if node.loading {
		iconText = mutedStyle.Render(icon)
	}
	line := marker + strings.Repeat("  ", node.depth) + iconText + " " + node.name
	if node.err != nil {
		line += "  " + errorStyle.Render(fmt.Sprintf("(%v)", node.err))
	}
	return style.Render(line)
}

func (t Model) find(path string) *directoryNode {
	var visit func(*directoryNode) *directoryNode
	visit = func(node *directoryNode) *directoryNode {
		if node.path == path {
			return node
		}
		for _, child := range node.children {
			if found := visit(child); found != nil {
				return found
			}
		}
		return nil
	}
	return visit(t.root)
}

func readDirectory(id int64, path string, filter Filter) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(path)
		if err != nil {
			return directoryReadMsg{id: id, path: path, err: err}
		}
		visible := make([]directoryEntry, 0, len(entries))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			directory := isDirectory(path, entry)
			if !directory && !filter.includesFile(entry.Name()) {
				continue
			}
			visible = append(visible, directoryEntry{name: entry.Name(), isDir: directory})
		}
		sort.Slice(visible, func(i, j int) bool {
			if visible[i].isDir != visible[j].isDir {
				return visible[i].isDir
			}
			return strings.ToLower(visible[i].name) < strings.ToLower(visible[j].name)
		})
		return directoryReadMsg{id: id, path: path, entries: visible}
	}
}

func isDirectory(parent string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(parent, entry.Name()))
	return err == nil && info.IsDir()
}

func displayPath(path string) string {
	cleaned := filepath.Clean(path)
	home, err := os.UserHomeDir()
	if err == nil && (cleaned == home || strings.HasPrefix(cleaned, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(cleaned, home)
	}
	return cleaned
}
