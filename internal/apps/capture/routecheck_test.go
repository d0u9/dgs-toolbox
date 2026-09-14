package capture

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/apps/capture/organizer"

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
	if links := m.fieldRows[index].value; !strings.Contains(links, "maps.apple.com") || !strings.Contains(ansi.Strip(links), "Apple · Google · 高德") {
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

func TestRouteOffersNoMapForAPositionNothingWrites(t *testing.T) {
	opened := recordOpens(t)
	m := chooseRecipe(t, loadedRoute(t, routeTestRoot(t, "alpha")), "Daily")
	if _, ok := mapRow(m); ok {
		t.Error("a map row for a recipe that writes no position")
	}
	m = routeKey(t, m, "o")
	if len(*opened) != 0 || !m.notice.bad {
		t.Errorf("opened %v, notice %+v", *opened, m.notice)
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

// `f` marks the Capture wrong in its record, at once, and takes the mark back.
func TestRouteFlagsACapture(t *testing.T) {
	previous := flagNow
	flagNow = func() time.Time { return time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { flagNow = previous })
	root := routeTestRoot(t, "alpha")
	m := loadedRoute(t, root)

	m = routeKey(t, m, "f")
	record, _ := organizer.ReadRecord(filepath.Join(root, "alpha"))
	if !record.Flagged() || record.Flag.FlaggedAt != "2026-09-14T21:00:00Z" {
		t.Fatalf("record = %+v", record)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "⚑ ") || !strings.Contains(view, "FLAGGED") {
		t.Errorf("the flagged row is not marked:\n%s", view)
	}
	if !strings.Contains(m.Status().Center, "flagged") {
		t.Errorf("status = %q", m.Status().Center)
	}
	// The notice answers one keystroke.
	m = routeKey(t, m, "j")
	if !m.notice.empty() {
		t.Errorf("notice outlived its keystroke: %q", m.notice.text)
	}

	m = routeKey(t, m, "f")
	if _, err := os.Stat(filepath.Join(root, "alpha", organizer.RecordFilename)); !os.IsNotExist(err) {
		t.Errorf("unflagging a capture nobody organized left a record: %v", err)
	}
	if view := ansi.Strip(m.View()); strings.Contains(view, "⚑ ") || strings.Contains(view, "FLAGGED") {
		t.Errorf("the mark stayed after unflagging:\n%s", view)
	}
}

// Archive reads the mark: the row leads its group, says why, and is rejected
// rather than kept.
func TestArchiveRejectsWhatRouteFlagged(t *testing.T) {
	root := organizedRoot(t, []string{"good", "wrong"}, nil)
	if _, err := organizer.SetFlag(filepath.Join(root, "wrong"), true, time.Now()); err != nil {
		t.Fatal(err)
	}
	archiveRoot, rejectRoot := filepath.Join(t.TempDir(), "Archive"), filepath.Join(t.TempDir(), "Reject")
	m := loadedArchive(t, root, archiveRoot, rejectRoot)

	m.captures.First()
	if first, ok := m.captures.Selected(); !ok || !strings.HasPrefix(ansi.Strip(first.Label), "⚑ ") || !strings.HasSuffix(first.Detail, "FLAGGED") || !strings.HasSuffix(first.ID, "wrong") {
		t.Fatalf("first = %+v, want the flagged capture first and marked", first)
	}

	m = selectCapture(t, m, destinationCaptures, "wrong")
	if detail := ansi.Strip(m.detail(60)); !strings.Contains(detail, "⚑ Flagged") {
		t.Errorf("detail does not say it was flagged:\n%s", detail)
	}
	if buttons := m.buttons(); buttons[0].primary || buttons[0].subtitle != "Flagged" {
		t.Errorf("archive button = %+v", buttons[0])
	}
	m = press(t, m, "a")
	if _, err := os.Stat(filepath.Join(archiveRoot, "wrong")); !os.IsNotExist(err) {
		t.Fatalf("a flagged capture was archived: %v", err)
	}
	if !m.notice.bad || !strings.Contains(m.notice.text, "Flagged") {
		t.Errorf("notice = %+v", m.notice)
	}
	m = press(t, m, "backspace")
	if _, err := os.Stat(filepath.Join(rejectRoot, "wrong")); err != nil {
		t.Fatalf("the flagged capture was not rejected: %v", err)
	}

	// The unflagged one is kept as before.
	m = selectCapture(t, m, destinationCaptures, "good")
	m = press(t, m, "a")
	if _, err := os.Stat(filepath.Join(archiveRoot, "good")); err != nil {
		t.Errorf("an organized capture was not archived: %v", err)
	}
}
