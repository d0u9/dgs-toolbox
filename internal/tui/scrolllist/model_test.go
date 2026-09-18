package scrolllist

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestNumberedViewportNavigationAndMouseSelection(t *testing.T) {
	m := New()
	m.SetSize(30, 3)
	m.SetItems([]Item{{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}})
	m.Scroll(2)
	if m.Top() != 1 || !m.SelectRow(2) || m.Cursor() != 3 {
		t.Fatalf("top=%d cursor=%d", m.Top(), m.Cursor())
	}
}

func TestTwoLineItemsRenderDetailAndHalveTheViewport(t *testing.T) {
	m := New()
	m.SetSize(24, 4)
	m.SetItems([]Item{
		{ID: "a", Label: "first", Detail: "detail a"},
		{ID: "b", Label: "second", Detail: "detail b"},
		{ID: "c", Label: "third", Detail: "detail c"},
	})

	lines := strings.Split(ansi.Strip(m.View(true, lipgloss.NewStyle(), lipgloss.NewStyle())), "\n")
	if len(lines) != 4 {
		t.Fatalf("rendered %d rows, want 4", len(lines))
	}
	if !strings.Contains(lines[0], "first") || !strings.Contains(lines[1], "detail a") {
		t.Fatalf("first item did not render two rows: %q, %q", lines[0], lines[1])
	}
	if strings.Contains(lines[3], "third") {
		t.Fatalf("a third item fitted a four-row viewport: %q", lines[3])
	}
	if !strings.Contains(lines[3], "↓ more") {
		t.Fatalf("clipped items are not marked: %q", lines[3])
	}

	// Either row of a two-line item selects that item.
	if !m.SelectRow(3) || m.Cursor() != 1 {
		t.Fatalf("clicking a detail row selected %d, want 1", m.Cursor())
	}

	// Moving past the viewport scrolls by items, not by rows.
	m.Move(1)
	if m.Top() != 1 {
		t.Fatalf("top after moving to the third item = %d, want 1", m.Top())
	}
}

func TestDividerSplitsTheListWithoutBecomingSelectable(t *testing.T) {
	m := New()
	m.SetSize(24, 6)
	m.SetItems([]Item{{ID: "a", Label: "alpha"}, {ID: "b", Label: "beta"}, {ID: "c", Label: "gamma"}})
	m.SetDivider(1, "DONE")

	view := m.View(true, lipgloss.NewStyle(), lipgloss.NewStyle())
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], "alpha") {
		t.Fatalf("first row = %q, want the item above the rule", lines[0])
	}
	if rule := ansi.Strip(lines[1]); !strings.Contains(rule, "── ◆ DONE ─") {
		t.Fatalf("second row = %q, want the shared anchored rule with its label", rule)
	}
	if !strings.Contains(lines[2], "beta") {
		t.Fatalf("third row = %q, want the first item below the rule", lines[2])
	}

	// Clicking the rule selects nothing; the rows around it still select their
	// own items.
	if m.SelectRow(1) {
		t.Fatal("the rule must not be selectable")
	}
	if !m.SelectRow(2) || m.Cursor() != 1 {
		t.Fatalf("cursor after clicking below the rule = %d, want 1", m.Cursor())
	}
	if !m.SelectRow(0) || m.Cursor() != 0 {
		t.Fatalf("cursor after clicking above the rule = %d, want 0", m.Cursor())
	}

	// A rule with nothing on one side separates nothing.
	m.SetDivider(0, "DONE")
	if strings.Contains(ansi.Strip(m.View(true, lipgloss.NewStyle(), lipgloss.NewStyle())), "DONE") {
		t.Fatal("a rule at the start of the list should be dropped")
	}
}

// TestHideNumbers covers a list that draws its own structure. The number
// column has to go entirely — not be blanked — so the label starts right
// after the cursor marker and a detail line lines up under it.
func TestHideNumbers(t *testing.T) {
	m := New()
	m.SetSize(30, 6)
	m.SetItems([]Item{
		{ID: "a", Label: "▾ node", Detail: "  2 instances"},
		{ID: "b", Label: "  └─ inst", Detail: "        service / role"},
	})

	withNumbers := m.View(true, lipgloss.NewStyle(), lipgloss.NewStyle())
	if !strings.Contains(withNumbers, "1 ▾ node") {
		t.Fatalf("View() = %q, want numbered rows by default", withNumbers)
	}
	if m.LabelOffset() != 4 {
		t.Fatalf("LabelOffset() = %d, want marker plus a one-digit number and its gap", m.LabelOffset())
	}

	m.HideNumbers(true)
	got := m.View(true, lipgloss.NewStyle(), lipgloss.NewStyle())
	if strings.Contains(got, "1 ▾") || strings.Contains(got, "2   └─") {
		t.Fatalf("View() = %q, want no line numbers", got)
	}
	if !strings.Contains(got, "› ▾ node") {
		t.Fatalf("View() = %q, want the label right after the cursor marker", got)
	}
	if !strings.Contains(got, "\n    └─ inst") {
		t.Fatalf("View() = %q, want an unselected row indented by the marker alone", got)
	}
	if m.LabelOffset() != 2 {
		t.Fatalf("LabelOffset() = %d, want the cursor marker alone", m.LabelOffset())
	}
}
