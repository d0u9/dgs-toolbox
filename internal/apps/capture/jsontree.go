package capture

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"dgs-toolbox/internal/tui/scrolllist"

	"github.com/charmbracelet/lipgloss"
)

// A JSON preview is offered in two shapes because the two answer different
// questions. The source shows the file as written — key order, formatting,
// exactly what a producer wrote. The tree shows the structure: what is nested
// in what, how many items a container holds, and what type each value is.
// Neither substitutes for the other, so Scan carries both and toggles between
// them.
var (
	jsonKeyStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#E2E8F0"})
	jsonIndexStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#94A3B8", Dark: "#64748B"})
	jsonStringStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"})
	jsonNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"})
	jsonBoolStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"})
	jsonNullStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#60A5FA"})
	jsonPunctStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#94A3B8", Dark: "#64748B"})
)

// jsonIndent is the tree's indentation per level.
const jsonIndent = "    "

// jsonNode is one entry of the tree: a leaf, or a container that can be
// expanded. Containers keep their children whether or not they are expanded, so
// collapsing and reopening one costs nothing and forgets nothing.
type jsonNode struct {
	label    string
	isIndex  bool
	value    any
	open     byte // '{' or '[' for a container, 0 for a leaf
	close    byte
	children []*jsonNode
	parent   *jsonNode
	expanded bool
	depth    int
}

func (n *jsonNode) isContainer() bool { return n.open != 0 }

// jsonTree is the interactive tree: which nodes are visible, and which one the
// cursor is on. It navigates like the File Explorer, because a tree the user
// already knows how to walk should not be walked differently here.
type jsonTree struct {
	root    *jsonNode
	visible []*jsonNode
	cursor  int
	top     int
}

// newJSONTree decodes JSON into a tree with its top level expanded. Deeper
// containers start collapsed, so a file opens as an outline rather than as
// everything at once.
func newJSONTree(data []byte) (jsonTree, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return jsonTree{}, fmt.Errorf("invalid JSON: %w", err)
	}
	root := buildJSONNode("", false, value, nil, 0)
	root.expanded = true
	for _, child := range root.children {
		child.expanded = true
	}
	tree := jsonTree{root: root}
	tree.refresh()
	return tree, nil
}

// buildJSONNode converts one decoded value, sorting object keys so two files
// written by different producers read the same way. The source view is where
// the original order is preserved.
func buildJSONNode(label string, isIndex bool, value any, parent *jsonNode, depth int) *jsonNode {
	node := &jsonNode{label: label, isIndex: isIndex, value: value, parent: parent, depth: depth}
	switch typed := value.(type) {
	case map[string]any:
		node.open, node.close = '{', '}'
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			node.children = append(node.children, buildJSONNode(key, false, typed[key], node, depth+1))
		}
	case []any:
		node.open, node.close = '[', ']'
		for index, item := range typed {
			node.children = append(node.children, buildJSONNode(strconv.Itoa(index), true, item, node, depth+1))
		}
	}
	return node
}

func (t *jsonTree) refresh() {
	t.visible = appendVisible(t.visible[:0], t.root)
	t.cursor = min(max(0, len(t.visible)-1), max(0, t.cursor))
}

func appendVisible(visible []*jsonNode, node *jsonNode) []*jsonNode {
	visible = append(visible, node)
	if !node.isContainer() || !node.expanded {
		return visible
	}
	for _, child := range node.children {
		visible = appendVisible(visible, child)
	}
	return visible
}

// Update handles a key, reporting whether the tree used it so the caller can
// fall back to its own bindings. The movement keys are the File Explorer's.
func (t *jsonTree) Update(key string) bool {
	if len(t.visible) == 0 {
		return false
	}
	node := t.visible[t.cursor]
	switch key {
	case "up", "k":
		t.cursor = max(0, t.cursor-1)
	case "down", "j":
		t.cursor = min(len(t.visible)-1, t.cursor+1)
	case "home", "g":
		t.cursor = 0
	case "end", "G":
		t.cursor = len(t.visible) - 1
	case "right", "l":
		// Expand, or step into what is already expanded.
		if node.isContainer() && !node.expanded && len(node.children) > 0 {
			node.expanded = true
			t.refresh()
			return true
		}
		if node.expanded && len(node.children) > 0 {
			t.cursor = min(len(t.visible)-1, t.cursor+1)
		}
	case "left", "h":
		// Collapse, or step out to the parent.
		if node.isContainer() && node.expanded {
			node.expanded = false
			t.refresh()
			return true
		}
		t.focus(node.parent)
	case "o":
		if node.isContainer() && len(node.children) > 0 {
			node.expanded = !node.expanded
			t.refresh()
		}
	case "O":
		if node.parent != nil {
			node.parent.expanded = false
			t.focus(node.parent)
			t.refresh()
		}
	case "w":
		collapseJSON(t.root)
		t.root.expanded = true
		t.cursor = 0
		t.refresh()
	default:
		return false
	}
	return true
}

func (t *jsonTree) focus(node *jsonNode) {
	if node == nil {
		return
	}
	for index, visible := range t.visible {
		if visible == node {
			t.cursor = index
			return
		}
	}
}

func collapseJSON(node *jsonNode) {
	node.expanded = false
	for _, child := range node.children {
		collapseJSON(child)
	}
}

// jsonRow is one rendered line. A container's closing bracket is a line of its
// own but not a node: it cannot be selected, and the cursor never lands on it.
// Clicking one selects the container it closes, which is the only thing that
// click could reasonably mean.
type jsonRow struct {
	node    *jsonNode
	closing bool
}

// rows lays the visible nodes out as lines, adding a closing bracket after the
// children of every expanded container. The brackets are drawn because a reader
// asked to read JSON should see JSON: an opening brace with no closing one
// reads as truncated, however clear the indentation is.
func (t jsonTree) rows() []jsonRow {
	var rows []jsonRow
	var walk func(node *jsonNode)
	walk = func(node *jsonNode) {
		rows = append(rows, jsonRow{node: node})
		if !node.isContainer() || !node.expanded || len(node.children) == 0 {
			return
		}
		for _, child := range node.children {
			walk(child)
		}
		rows = append(rows, jsonRow{node: node, closing: true})
	}
	walk(t.root)
	return rows
}

// cursorRow is where the cursor sits in rendered lines rather than in nodes.
func (t jsonTree) cursorRow(rows []jsonRow) int {
	if t.cursor < 0 || t.cursor >= len(t.visible) {
		return 0
	}
	target := t.visible[t.cursor]
	for index, row := range rows {
		if row.node == target && !row.closing {
			return index
		}
	}
	return 0
}

// SelectRow moves the cursor to a rendered row, for a click.
func (t *jsonTree) SelectRow(row int) bool {
	rows := t.rows()
	index := t.top + row
	if index < 0 || index >= len(rows) {
		return false
	}
	for cursor, node := range t.visible {
		if node == rows[index].node {
			t.cursor = cursor
			return true
		}
	}
	return false
}

// View renders the visible rows for a viewport of the given height, scrolled to
// keep the cursor on screen. The tree owns its own scrolling because a cursor
// that leaves the window is worse than one that drags it.
func (t *jsonTree) View(width, height int, focused bool) string {
	height = max(1, height)
	rows := t.rows()
	cursor := t.cursorRow(rows)
	if cursor < t.top {
		t.top = cursor
	}
	if cursor >= t.top+height {
		t.top = cursor - height + 1
	}
	t.top = min(max(0, len(rows)-height), max(0, t.top))

	lines := make([]string, 0, height)
	for index := t.top; index < min(len(rows), t.top+height); index++ {
		row := rows[index]
		text := strings.Repeat(jsonIndent, row.node.depth)
		if row.closing {
			text += "  " + jsonPunctStyle.Render(string(row.node.close))
		} else {
			text += jsonNodeLine(row.node)
		}
		if focused && index == cursor {
			lines = append(lines, scrolllist.SelectedRowStyle().Width(width).Render(truncate(text, width)))
			continue
		}
		lines = append(lines, truncateStyled(text, width))
	}
	return strings.Join(lines, "\n")
}

// jsonNodeLine renders one row: its marker, its key, and either its value or a
// summary of what it contains.
func jsonNodeLine(node *jsonNode) string {
	marker := "  "
	if node.isContainer() && len(node.children) > 0 {
		marker = "▸ "
		if node.expanded {
			marker = "▾ "
		}
	}
	label := ""
	if node.label != "" {
		style := jsonKeyStyle
		if node.isIndex {
			style = jsonIndexStyle
		}
		label = style.Render(node.label) + jsonPunctStyle.Render(" : ")
	}
	return marker + label + jsonNodeValue(node)
}

func jsonNodeValue(node *jsonNode) string {
	if !node.isContainer() {
		return jsonScalar(node.value)
	}
	open, closing := string(node.open), string(node.close)
	if len(node.children) == 0 {
		return jsonPunctStyle.Render(open + closing)
	}
	if !node.expanded {
		// A collapsed container says how much it is hiding, so a reader can
		// tell an empty one from one worth opening.
		return jsonPunctStyle.Render(open+" … "+closing) + jsonIndexStyle.Render(itemCount(len(node.children)))
	}
	return jsonPunctStyle.Render(open) + jsonIndexStyle.Render(itemCount(len(node.children)))
}

// itemCount is written only for a container that holds something: "{}" already
// says a container is empty, and "0 items" beside it would say it twice.
func itemCount(count int) string {
	switch count {
	case 0:
		return ""
	case 1:
		return "  1 item"
	default:
		return fmt.Sprintf("  %d items", count)
	}
}

// jsonScalar styles a leaf by its type, so a number that was written as a
// string does not read as a number.
func jsonScalar(value any) string {
	switch leaf := value.(type) {
	case nil:
		return jsonNullStyle.Render("null")
	case bool:
		return jsonBoolStyle.Render(strconv.FormatBool(leaf))
	case json.Number:
		return jsonNumberStyle.Render(leaf.String())
	case string:
		return jsonStringStyle.Render(singleLine(leaf))
	default:
		return fmt.Sprint(leaf)
	}
}
