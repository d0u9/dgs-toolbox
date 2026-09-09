package capture

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const treeSample = `{
  "string": "Hello World",
  "number": 123,
  "boolean": true,
  "null": null,
  "array": [1, 2, 3],
  "object": {"a": "b"},
  "empty": []
}`

func newTestTree(t *testing.T, body string) jsonTree {
	t.Helper()
	tree, err := newJSONTree([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func treeView(tree *jsonTree) string {
	return ansi.Strip(tree.View(80, 40, true))
}

func TestJSONTreeShowsStructureTypesAndCounts(t *testing.T) {
	tree := newTestTree(t, treeSample)
	got := treeView(&tree)

	for _, want := range []string{
		"▾ {  7 items",
		"}",
		"    ]",
		"    ▾ array : [  3 items",
		"        0 : 1",
		"    ▾ object : {  1 item",
		"        a : b",
		"    boolean : true",
		"    null : null",
		"    string : Hello World",
		"    empty : []",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("tree is missing %q:\n%s", want, got)
		}
	}
	// An empty container already says it is empty; a count beside it says it
	// twice.
	if strings.Contains(got, "0 items") {
		t.Errorf("an empty container should carry no count:\n%s", got)
	}
	// Keys are sorted, so two producers' files read the same way.
	if strings.Index(got, "array") > strings.Index(got, "boolean") {
		t.Errorf("keys are not sorted:\n%s", got)
	}
}

// The tree navigates like the File Explorer, because a tree the user already
// knows how to walk should not be walked differently here.
func TestJSONTreeNavigatesLikeTheFileExplorer(t *testing.T) {
	tree := newTestTree(t, treeSample)

	// l collapses nothing at a leaf but expands a container; h collapses it
	// again, and h on a collapsed node steps out to its parent.
	focusNode(t, &tree, "array")
	if !tree.Update("left") {
		t.Fatal("left was not handled")
	}
	if strings.Contains(treeView(&tree), "0 : 1") {
		t.Fatalf("left did not collapse the array:\n%s", treeView(&tree))
	}
	if !strings.Contains(treeView(&tree), "▸ array : [ … ]  3 items") {
		t.Fatalf("a collapsed container should say how much it hides:\n%s", treeView(&tree))
	}
	tree.Update("right")
	if !strings.Contains(treeView(&tree), "0 : 1") {
		t.Fatalf("right did not expand the array:\n%s", treeView(&tree))
	}

	// o toggles, w collapses everything below the root.
	focusNode(t, &tree, "object")
	tree.Update("o")
	if strings.Contains(treeView(&tree), "a : b") {
		t.Fatal("o did not collapse the object")
	}
	tree.Update("o")
	if !strings.Contains(treeView(&tree), "a : b") {
		t.Fatal("o did not reopen the object")
	}
	tree.Update("w")
	view := treeView(&tree)
	if strings.Contains(view, "a : b") || strings.Contains(view, "0 : 1") {
		t.Fatalf("w did not collapse the tree:\n%s", view)
	}
	if !strings.Contains(view, "▾ {  7 items") {
		t.Fatalf("w should leave the root open:\n%s", view)
	}

	// h from a collapsed child steps out to the parent.
	tree.Update("right")
	focusNode(t, &tree, "array")
	tree.Update("left")
	tree.Update("left")
	if node := tree.visible[tree.cursor]; node.parent != nil {
		t.Fatalf("left did not step out to the root, cursor on %q", node.label)
	}
}

// The cursor moves with j/k and g/G, and the tree scrolls itself to keep it on
// screen.
func TestJSONTreeCursorMovesAndDragsTheWindow(t *testing.T) {
	tree := newTestTree(t, treeSample)
	tree.Update("end")
	if tree.cursor != len(tree.visible)-1 {
		t.Fatalf("cursor = %d, want the last row", tree.cursor)
	}
	view := ansi.Strip(tree.View(80, 4, true))
	if strings.Contains(view, "▾ {  7 items") {
		t.Fatalf("the window did not follow the cursor:\n%s", view)
	}
	tree.Update("home")
	if tree.cursor != 0 {
		t.Fatalf("cursor = %d, want the first row", tree.cursor)
	}
	if !tree.Update("up") || !tree.Update("down") {
		t.Fatal("the tree should own its movement keys")
	}
	if tree.Update("q") {
		t.Fatal("the tree claimed a key it does not use")
	}
}

func TestJSONTreeRejectsInvalidJSON(t *testing.T) {
	if _, err := newJSONTree([]byte("{ truncated")); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
}

// A number keeps the text it was written with rather than being round-tripped
// through a float, so a long id does not come back in scientific notation.
func TestJSONTreeKeepsNumbersAsWritten(t *testing.T) {
	tree := newTestTree(t, `{"id": 20260909213122900}`)
	if got := treeView(&tree); !strings.Contains(got, "20260909213122900") {
		t.Fatalf("number was reformatted:\n%s", got)
	}
}

func focusNode(t *testing.T, tree *jsonTree, label string) {
	t.Helper()
	for index, node := range tree.visible {
		if node.label == label {
			tree.cursor = index
			return
		}
	}
	t.Fatalf("node %q is not visible:\n%s", label, treeView(tree))
}

// A selected row is clipped by terminal cells, not by counting runes: the row
// carries the escape sequences that colour it, and they occupy no cells.
func TestJSONTreeSelectedRowIsNotClippedByItsOwnColours(t *testing.T) {
	tree := newTestTree(t, `{"createdAt": "2026-09-01T09:12:04.118+09:00"}`)
	focusNode(t, &tree, "createdAt")

	const width = 60
	selected := ansi.Strip(tree.View(width, 10, true))
	unselected := ansi.Strip(tree.View(width, 10, false))
	if !strings.Contains(selected, "2026-09-01T09:12:04.118+09:00") {
		t.Fatalf("the selected row was clipped by its own colours:\n%s", selected)
	}
	// The selected row is padded to the column, so the comparison is per line.
	for index, line := range strings.Split(selected, "\n") {
		want := strings.Split(unselected, "\n")[index]
		if strings.TrimRight(line, " ") != strings.TrimRight(want, " ") {
			t.Fatalf("selecting a row changed line %d:\n%q\n%q", index, line, want)
		}
	}
}

// A wide rune occupies two cells, so clipping by rune count would leave a row
// wider than the column.
func TestJSONTreeClipsWideRunesByWidth(t *testing.T) {
	tree := newTestTree(t, `{"note": "晚上这里的人流和光线可以再看看一次好吗"}`)
	focusNode(t, &tree, "note")

	const width = 30
	for _, focused := range []bool{true, false} {
		if got := lipgloss.Width(strings.Split(tree.View(width, 10, focused), "\n")[0]); got > width {
			t.Fatalf("focused=%v row is %d cells wide, want at most %d", focused, got, width)
		}
	}
}

// A container's closing bracket is a line but not a node: the cursor steps over
// it, and clicking it selects the container it closes.
func TestJSONTreeClosingBracketsAreLinesNotNodes(t *testing.T) {
	tree := newTestTree(t, `{"array":[1],"tail":2}`)

	view := treeView(&tree)
	if !strings.Contains(view, "    ]") || !strings.HasSuffix(strings.TrimRight(view, "\n"), "}") {
		t.Fatalf("brackets are missing:\n%s", view)
	}

	// Walking down passes array, its item, then tail — never a bracket.
	var labels []string
	for range tree.visible {
		labels = append(labels, tree.visible[tree.cursor].label)
		tree.Update("down")
	}
	for _, label := range labels {
		if label == "]" || label == "}" {
			t.Fatalf("the cursor landed on a bracket: %v", labels)
		}
	}

	// The closing bracket of the array is the row after its single item.
	rows := tree.rows()
	closing := -1
	for index, row := range rows {
		if row.closing && row.node.label == "array" {
			closing = index
		}
	}
	if closing < 0 {
		t.Fatal("the array has no closing row")
	}
	if !tree.SelectRow(closing) {
		t.Fatal("clicking a closing bracket selected nothing")
	}
	if got := tree.visible[tree.cursor].label; got != "array" {
		t.Fatalf("clicking the closing bracket selected %q, want the container it closes", got)
	}
}
