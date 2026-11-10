package capture

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func routeKey(t *testing.T, m routeModel, key string) routeModel {
	t.Helper()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "enter" {
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	}
	updated, _ := m.updateKey(msg)
	return updated.(routeModel)
}

func recordOpens(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	previous := openURL
	openURL = func(url string) error { opened = append(opened, url); return nil }
	t.Cleanup(func() { openURL = previous })
	return &opened
}

func mapRow(m routeModel) (int, bool) {
	for index, row := range m.fieldRows {
		if row.mapLinks {
			return index, true
		}
	}
	return 0, false
}

// A Recipe writing the position offers it on a map, in FIELDS and on `o`.
func TestRouteOffersTheMapWhenThePositionIsWritten(t *testing.T) {
	opened := recordOpens(t)
	m := chooseRecipe(t, loadedRoute(t, routeTestRoot(t, "alpha")), "Location")

	index, ok := mapRow(m)
	if !ok {
		t.Fatal("no map row for a recipe writing the position")
	}
	if links := m.fieldRows[index].value; !strings.Contains(links, "maps.apple.com") || !strings.Contains(ansi.Strip(links), "Apple · Google · Amap") {
		t.Errorf("map row = %q", links)
	}
	if !strings.Contains(m.Status().Right, "o Map") {
		t.Errorf("status hints = %q, want o Map", m.Status().Right)
	}

	m = routeKey(t, m, "o")
	if len(*opened) != 1 || !strings.Contains((*opened)[0], "maps.apple.com/place?coordinate=0.000000,0.000000") {
		t.Fatalf("opened = %v", *opened)
	}
	if !strings.Contains(m.Status().Center, "Apple Maps") {
		t.Errorf("status = %q", m.Status().Center)
	}

	// Enter on the row opens it too, rather than an editor.
	m.fields.Set(routeFieldsField)
	m.fieldIndex = index
	m = routeKey(t, m, "enter")
	if len(*opened) != 2 || m.editing {
		t.Errorf("enter on the map row: opened %d, editing %v", len(*opened), m.editing)
	}
}

// FIELDS keeps the map row for a position that will be written, but `o` opens
// the Capture's position whatever Recipe is chosen, and before one is.
func TestRouteOpensTheMapBeforeARecipeWritesThePosition(t *testing.T) {
	opened := recordOpens(t)
	m := loadedRoute(t, routeTestRoot(t, "alpha"))
	if !strings.Contains(m.Status().Right, "o Map") {
		t.Errorf("status hints = %q, want o Map", m.Status().Right)
	}
	m = routeKey(t, m, "o")
	if len(*opened) != 1 || m.notice.bad {
		t.Fatalf("no recipe: opened %v, notice %+v", *opened, m.notice)
	}

	m = chooseRecipe(t, m, "Daily")
	if _, ok := mapRow(m); ok {
		t.Error("a map row for a recipe that writes no position")
	}
	m = routeKey(t, m, "o")
	if len(*opened) != 2 || m.notice.bad {
		t.Errorf("daily: opened %v, notice %+v", *opened, m.notice)
	}
}

func TestRouteMapFailureIsReported(t *testing.T) {
	previous := openURL
	openURL = func(string) error { return errors.New("no browser") }
	t.Cleanup(func() { openURL = previous })
	m := chooseRecipe(t, loadedRoute(t, routeTestRoot(t, "alpha")), "Location")
	m = routeKey(t, m, "o")
	if !m.notice.bad || !strings.Contains(m.notice.text, "no browser") {
		t.Errorf("notice = %+v", m.notice)
	}
}

// `d` marks the Capture to discard in its record, at once, and takes the mark back.
func TestRouteMarksACaptureToDiscard(t *testing.T) {
	previous := discardNow
	discardNow = func() time.Time { return time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { discardNow = previous })
	root := routeTestRoot(t, "alpha")
	m := loadedRoute(t, root)

	m = routeKey(t, m, "d")
	record, _, err := organizer.ReadRecord(filepath.Join(root, "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if !record.Discarded() || record.Discard.DiscardedAt != "2026-09-14T21:00:00Z" {
		t.Fatalf("record = %+v", record)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "✕ ") || !strings.Contains(view, "· DISCARD") {
		t.Errorf("the discarded row is not marked:\n%s", view)
	}
	// The list counts it, once, as discarded rather than processed.
	if view := ansi.Strip(m.View()); !strings.Contains(view, "CAPTURES 1/1 · 0 PROCESSED · 1 DISCARD") {
		t.Errorf("the legend does not count the mark:\n%s", view)
	}
	if !strings.Contains(m.Status().Center, "marked to discard") {
		t.Errorf("status = %q", m.Status().Center)
	}
	// The notice answers one keystroke.
	m = routeKey(t, m, "j")
	if !m.notice.empty() {
		t.Errorf("notice outlived its keystroke: %q", m.notice.text)
	}

	m = routeKey(t, m, "d")
	if _, err := os.Stat(filepath.Join(root, "alpha", organizer.RecordFilename)); !os.IsNotExist(err) {
		t.Errorf("taking the mark back from a capture nobody organized left a record: %v", err)
	}
	if view := ansi.Strip(m.View()); strings.Contains(view, "✕ ") || strings.Contains(view, "· DISCARD") || !strings.Contains(view, "0 DISCARD") {
		t.Errorf("the mark stayed after taking it back:\n%s", view)
	}
}

// Archive reads the mark: the row leads its group, says why, and is rejected
// rather than kept.
func TestArchiveRejectsWhatRouteMarkedToDiscard(t *testing.T) {
	root := organizedRoot(t, []string{"good", "wrong"}, nil)
	if _, err := organizer.SetDiscard(filepath.Join(root, "wrong"), true, time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	archiveRoot, rejectRoot := filepath.Join(t.TempDir(), "Archive"), filepath.Join(t.TempDir(), "Reject")
	m := loadedArchive(t, root, archiveRoot, rejectRoot)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "CAPTURES 2/2 ✓1 ✕1") {
		t.Errorf("the legend does not count what was done:\n%s", view)
	}

	m.captures.First()
	if first, ok := m.captures.Selected(); !ok || !strings.HasPrefix(ansi.Strip(first.Label), "✕ ") || !strings.HasSuffix(first.Detail, "DISCARD") || !strings.HasSuffix(first.ID, "wrong") {
		t.Fatalf("first = %+v, want the discarded capture first and marked", first)
	}

	m = selectCapture(t, m, destinationCaptures, "wrong")
	if detail := ansi.Strip(m.detail(60)); !strings.Contains(detail, "✕ Marked to discard 2026-09-14 Mon 21:00:00 Z") {
		t.Errorf("detail does not say it was discarded:\n%s", detail)
	}
	if buttons := m.buttons(); buttons[0].primary || buttons[0].subtitle != "Discard" {
		t.Errorf("archive button = %+v", buttons[0])
	}
	m = press(t, m, "a")
	if _, err := os.Stat(filepath.Join(archiveRoot, "wrong")); !os.IsNotExist(err) {
		t.Fatalf("a discarded capture was archived: %v", err)
	}
	if !m.notice.bad || !strings.Contains(m.notice.text, "Marked to discard") {
		t.Errorf("notice = %+v", m.notice)
	}
	m = press(t, m, "backspace")
	if _, err := os.Stat(filepath.Join(rejectRoot, "wrong")); err != nil {
		t.Fatalf("the discarded capture was not rejected: %v", err)
	}

	// The one not marked is kept as before.
	m = selectCapture(t, m, destinationCaptures, "good")
	m = press(t, m, "a")
	if _, err := os.Stat(filepath.Join(archiveRoot, "good")); err != nil {
		t.Errorf("an organized capture was not archived: %v", err)
	}
}

// Route and Archive list the Capture root in one order — the work, then the
// ones marked to discard, then the organized ones — so a Capture sits in the
// same place in both.
func TestRouteAndArchiveListTheRootInOneOrder(t *testing.T) {
	root := organizedRoot(t, []string{"c"}, []string{"a", "b"})
	if _, err := organizer.SetDiscard(filepath.Join(root, "b"), true, time.Now()); err != nil {
		t.Fatal(err)
	}
	names := func(list interface {
		ItemAt(int) (scrolllist.Item, bool)
	}) string {
		var out []string
		for index := 0; ; index++ {
			item, ok := list.ItemAt(index)
			if !ok {
				return strings.Join(out, ",")
			}
			out = append(out, filepath.Base(item.ID))
		}
	}
	route := loadedRoute(t, root)
	archive := loadedArchive(t, root, filepath.Join(t.TempDir(), "Archive"), filepath.Join(t.TempDir(), "Reject"))
	if got := names(route.captures); got != "a,b,c" {
		t.Fatalf("route order = %s", got)
	}
	if got := names(archive.captures); got != "a,b,c" {
		t.Fatalf("archive order = %s", got)
	}
	for _, view := range []string{ansi.Strip(route.View()), ansi.Strip(archive.View())} {
		if !strings.Contains(view, "◆ DISCARD") || !strings.Contains(view, "◆ ORGANIZED") {
			t.Fatalf("the groups are not ruled:\n%s", view)
		}
	}
}

// The Captures marked to discard are gathered, so rejecting one leaves the
// cursor on the next.
func TestArchiveRejectsTheDiscardedOneAfterAnother(t *testing.T) {
	root := organizedRoot(t, nil, []string{"a", "b", "c"})
	for _, name := range []string{"a", "b"} {
		if _, err := organizer.SetDiscard(filepath.Join(root, name), true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	m := loadedArchive(t, root, filepath.Join(t.TempDir(), "Archive"), filepath.Join(t.TempDir(), "Reject"))
	m = selectCapture(t, m, destinationCaptures, "a")
	m = press(t, m, "backspace")
	if item, _ := m.captures.Selected(); filepath.Base(item.ID) != "b" {
		t.Fatalf("cursor = %q, want the next one marked to discard", item.ID)
	}
}

// `D` rejects every Capture marked to discard at once, after one question; any
// other key than Enter leaves them all where they are.
func TestArchiveRejectsEveryDiscardedOneAtOnce(t *testing.T) {
	root := organizedRoot(t, []string{"c"}, []string{"a", "b"})
	for _, name := range []string{"a", "b"} {
		if _, err := organizer.SetDiscard(filepath.Join(root, name), true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	rejectRoot := filepath.Join(t.TempDir(), "Reject")
	m := loadedArchive(t, root, filepath.Join(t.TempDir(), "Archive"), rejectRoot)
	if !strings.Contains(m.Status().Right, "D Reject discarded") {
		t.Errorf("hints = %q", m.Status().Right)
	}

	m = press(t, m, "D")
	if m.Status().Left != "REJECT ALL" || !strings.Contains(m.Status().Center, "2") {
		t.Fatalf("status = %+v, want the question", m.Status())
	}
	m = press(t, m, "j")
	if _, err := os.Stat(filepath.Join(root, "a")); err != nil || m.confirmingRejectAll {
		t.Fatalf("another key moved something or left the question open: %v", err)
	}

	m = press(t, m, "D", "enter")
	for _, name := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(rejectRoot, name)); err != nil {
			t.Fatalf("%s was not rejected: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "c")); err != nil {
		t.Fatalf("a capture not marked was moved: %v", err)
	}
	if m.notice.bad || !strings.Contains(m.notice.text, "Rejected 2") {
		t.Errorf("notice = %+v", m.notice)
	}
	if strings.Contains(m.Status().Right, "D Reject discarded") {
		t.Errorf("D still offered with nothing marked: %q", m.Status().Right)
	}
	m = press(t, m, "D")
	if m.confirmingRejectAll || !m.notice.bad {
		t.Errorf("D with nothing marked: confirming %v, notice %+v", m.confirmingRejectAll, m.notice)
	}
}

// `u` takes the last step back: one move, or every Capture `D` rejected at
// once, and again further back.
func TestArchiveUndoesTheLastStep(t *testing.T) {
	root := organizedRoot(t, []string{"c"}, []string{"a", "b"})
	for _, name := range []string{"a", "b"} {
		if _, err := organizer.SetDiscard(filepath.Join(root, name), true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	archiveRoot, rejectRoot := filepath.Join(t.TempDir(), "Archive"), filepath.Join(t.TempDir(), "Reject")
	m := loadedArchive(t, root, archiveRoot, rejectRoot)
	exists := func(path string) bool { _, err := os.Stat(path); return err == nil }

	m = press(t, m, "u")
	if !m.notice.bad {
		t.Fatalf("undo with nothing done: %+v", m.notice)
	}
	m = selectCapture(t, m, destinationCaptures, "c")
	m = press(t, m, "a")
	m = press(t, m, "D", "enter")
	if !exists(filepath.Join(archiveRoot, "c")) || !exists(filepath.Join(rejectRoot, "a")) || !exists(filepath.Join(rejectRoot, "b")) {
		t.Fatal("the moves did not happen")
	}
	if !strings.Contains(m.Status().Right, "u Undo") {
		t.Errorf("hints = %q", m.Status().Right)
	}

	m = press(t, m, "u")
	if !exists(filepath.Join(root, "a")) || !exists(filepath.Join(root, "b")) || exists(filepath.Join(rejectRoot, "a")) {
		t.Fatal("undo did not bring back both Captures D rejected")
	}
	if !exists(filepath.Join(archiveRoot, "c")) {
		t.Fatal("undo went back further than one step")
	}
	m = press(t, m, "u")
	if !exists(filepath.Join(root, "c")) || exists(filepath.Join(archiveRoot, "c")) {
		t.Fatal("a second undo did not take the archive back")
	}
	if len(m.entries) != 3 || strings.Contains(m.Status().Right, "u Undo") {
		t.Fatalf("entries %d, hints %q", len(m.entries), m.Status().Right)
	}
}

// A move read back the root, so Scan and Route stop offering what has gone.
func TestArchiveMoveReloadsTheSharedRoot(t *testing.T) {
	root := organizedRoot(t, []string{"c"}, nil)
	m := loadedArchive(t, root, filepath.Join(t.TempDir(), "Archive"), filepath.Join(t.TempDir(), "Reject"))
	m = selectCapture(t, m, destinationCaptures, "c")
	_, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if cmd == nil {
		t.Fatal("a move read nothing back")
	}
	found := false
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, part := range batch {
			if loaded, ok := part().(capturesLoadedMsg); ok && len(loaded.captures) == 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("the move did not read the root back without the Capture it moved")
	}
}

// `u` in Route takes back a mark or a clear, and puts the cursor on that
// Capture wherever the step had sent it; again goes further back.
func TestRouteUndoesAMarkAndAClear(t *testing.T) {
	root := routeTestRoot(t, "a", "b", "c")
	m := loadedRoute(t, root)
	selected := func() string {
		item, _ := m.captures.Selected()
		return filepath.Base(strings.TrimPrefix(item.ID, "capture:"))
	}
	m.captures.SelectID("capture:" + filepath.Join(root, "b"))
	m.refresh()

	m = chooseRecipe(t, m, "Daily")
	m.fields.Set(routeCapturesField)
	m = routeKey(t, m, "c")
	if len(m.selections) != 0 {
		t.Fatal("c did not clear")
	}
	m = routeKey(t, m, "d")
	if selected() == "b" {
		t.Fatal("the cursor stayed on the marked capture")
	}
	if !strings.Contains(m.Status().Right, "u Undo") {
		t.Errorf("hints = %q", m.Status().Right)
	}

	m = routeKey(t, m, "u")
	if selected() != "b" || m.entries[1].record.Discarded() {
		t.Fatalf("after undoing the mark: cursor %s, discarded %v", selected(), m.entries[1].record.Discarded())
	}
	if _, err := os.Stat(filepath.Join(root, "b", organizer.RecordFilename)); !os.IsNotExist(err) {
		t.Errorf("undoing a first mark left a record: %v", err)
	}
	m = routeKey(t, m, "u")
	if _, _, ok := m.currentSelection(); !ok || selected() != "b" {
		t.Fatal("undoing the clear did not give the Selection back")
	}
	m = routeKey(t, m, "u")
	if !m.notice.bad || strings.Contains(m.Status().Right, "u Undo") {
		t.Fatalf("undo with nothing left: %+v", m.notice)
	}
}

// Undoing taking a mark back marks it again, at the time it was first made.
func TestRouteUndoKeepsTheMarksTime(t *testing.T) {
	root := routeTestRoot(t, "a")
	at := time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC)
	if _, err := organizer.SetDiscard(filepath.Join(root, "a"), true, at); err != nil {
		t.Fatal(err)
	}
	m := loadedRoute(t, root)
	m = routeKey(t, m, "d")
	m = routeKey(t, m, "u")
	record, _, err := organizer.ReadRecord(filepath.Join(root, "a"))
	if err != nil || !record.Discarded() || record.Discard.DiscardedAt != "2026-09-14T21:00:00Z" {
		t.Fatalf("record = %+v, err %v", record, err)
	}
}
