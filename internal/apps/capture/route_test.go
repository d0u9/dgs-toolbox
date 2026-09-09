package capture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/apps/capture/indexschema"
	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func routeTestRoot(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		index := []byte(`{"schema":"v1","source":{"app":"Shortcut","workflow":"been_here","device":{"os":"iOS","systemVersion":"26.4.2","name":"Phone"}},"coordinates":{"altitude":0,"longitude":0,"latitude":0},"id":"` + name + `","payload":{"note":""},"createdAt":"2026-09-09T16:34:35.556+10:00"}`)
		if err := os.WriteFile(filepath.Join(path, "index.json"), index, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func loadedRoute(t *testing.T, root string) routeModel {
	t.Helper()
	m := newRouteModel(root, "index.json")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m = updated.(routeModel)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	return updated.(routeModel)
}

func TestRouteWorkspaceShowsTheFourOrganizerColumns(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	m := loadedRoute(t, root)

	view := ansi.Strip(m.View())
	for _, want := range []string{"CAPTURES", "RECIPES", "ACTIONS", "FIELDS", "2026-09-09 16:34:35 +10", "Shortcut · been_here", "Location + Daily"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Route workspace is missing %q:\n%s", want, view)
		}
	}
	if got := strings.Count(view, "\n") + 1; got != m.height {
		t.Fatalf("Route workspace height = %d, want %d", got, m.height)
	}
}

// Recipes are candidates only: even with exactly one match, nothing is chosen
// until the user presses Enter.
func TestRouteNarrowsRecipesButNeverChooses(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := loadedRoute(t, root)

	if len(m.candidates()) == 0 {
		t.Fatal("been_here capture should offer candidates")
	}
	if _, _, ok := m.currentSelection(); ok {
		t.Fatal("a recipe was chosen without the user choosing it")
	}

	updated, _ := m.updateCaptures("enter")
	m = updated.(routeModel)
	if got := m.fields.Current(); got != routeRecipesField {
		t.Fatalf("focus after Enter = %q, want %q", got, routeRecipesField)
	}
	updated, _ = m.updateRecipes("enter")
	m = updated.(routeModel)
	if got := m.fields.Current(); got != routeActionsField {
		t.Fatalf("focus after choosing = %q, want %q", got, routeActionsField)
	}
	if _, _, ok := m.currentSelection(); !ok {
		t.Fatal("Enter did not record the recipe")
	}
}

func chooseRecipe(t *testing.T, m routeModel, name string) routeModel {
	t.Helper()
	updated, _ := m.updateCaptures("enter")
	m = updated.(routeModel)
	m.recipes.First()
	for range m.candidates() {
		if recipe, ok := m.focusedCandidate(); ok && recipe.Name == name {
			updated, _ = m.updateRecipes("enter")
			return updated.(routeModel)
		}
		updated, _ = m.updateRecipes("down")
		m = updated.(routeModel)
	}
	t.Fatalf("recipe %q is not a candidate", name)
	return m
}

func TestRouteMarksBlockedActionsAndClearsThemOnceFilled(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "[x] ○ Location note") {
		t.Fatalf("location action should be enabled and blocked:\n%s", view)
	}
	if !strings.Contains(view, "target unresolved") {
		t.Fatalf("a blocked action should show no invented target:\n%s", view)
	}

	// FIELDS lists the union across enabled actions, split by state: what
	// already has a value, then what is still missing.
	resolved, missing := splitFieldRows(m)
	if !equalFieldIDs(resolved, []organizer.FieldID{organizer.FieldLatitude, organizer.FieldLongitude, organizer.FieldCreatedAt}) {
		t.Fatalf("resolved = %v", fieldRowIDs(resolved))
	}
	if !equalFieldIDs(missing, []organizer.FieldID{organizer.FieldPlaceName, organizer.FieldContent, organizer.FieldTags}) {
		t.Fatalf("missing = %v", fieldRowIDs(missing))
	}
	if resolved[0].editable {
		t.Fatal("latitude comes from the capture and must not be editable")
	}

	selection, recipe, ok := m.currentSelection()
	if !ok {
		t.Fatal("no selection")
	}
	selection.Set(organizer.FieldPlaceName, "Epping Station")
	selection.Set(organizer.FieldContent, "晚上再来看看")
	m.refresh()

	if !organizer.Ready(mustContext(t, m), recipe, selection.EnabledActions(recipe)) {
		t.Fatal("capture should be ready once both fields are supplied")
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "[x] ● Location note") {
		t.Fatalf("location action should be ready:\n%s", view)
	}
	if !strings.Contains(view, "Locations/Epping Station.md") {
		t.Fatalf("plan should show the resolved target:\n%s", view)
	}
}

func fieldRowIDs(rows []routeFieldRow) []organizer.FieldID {
	ids := make([]organizer.FieldID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.requirement.Field)
	}
	return ids
}

func equalFieldIDs(rows []routeFieldRow, want []organizer.FieldID) bool {
	got := fieldRowIDs(rows)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func splitFieldRows(m routeModel) (resolved, missing []routeFieldRow) {
	for _, row := range m.fieldRows {
		if row.missing {
			missing = append(missing, row)
			continue
		}
		resolved = append(resolved, row)
	}
	return resolved, missing
}

// runAndConfirm presses x and confirms the dialog, which is what carrying out a
// plan now takes.
func runAndConfirm(t *testing.T, m routeModel) routeModel {
	t.Helper()
	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(routeModel)
	if !m.running {
		t.Fatal("x did not open the run dialog")
	}
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(routeModel)
}

func mustContext(t *testing.T, m routeModel) organizer.Context {
	t.Helper()
	ctx, ok := m.context()
	if !ok {
		t.Fatal("no context for the selected capture")
	}
	return ctx
}

// A disabled Action leaves the plan and takes its requirements with it.
func TestRouteToggleDropsAnActionAndItsRequirements(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")

	selection, recipe, _ := m.currentSelection()
	before := organizer.MissingFields(mustContext(t, m), recipe, selection.EnabledActions(recipe))
	if len(before) != 2 {
		t.Fatalf("missing before = %d, want place name and content", len(before))
	}

	m.actions.SelectID("action:" + string(organizer.ActionDailyAppend))
	updated, _ := m.updateActions(" ")
	m = updated.(routeModel)

	selection, recipe, _ = m.currentSelection()
	after := organizer.MissingFields(mustContext(t, m), recipe, selection.EnabledActions(recipe))
	if len(after) != 1 || after[0].Field != organizer.FieldPlaceName {
		t.Fatalf("missing after disabling the daily note = %v", after)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "[ ]   Daily note") {
		t.Fatalf("disabled action should render unchecked and without markers:\n%s", view)
	}
}

func TestRouteEditingAFieldRecordsEnrichment(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location")

	updated, _ := m.updateActions("enter")
	m = updated.(routeModel)
	if got := m.fields.Current(); got != routeFieldsField {
		t.Fatalf("focus = %q, want %q", got, routeFieldsField)
	}
	for m.fieldRows[m.fieldIndex].requirement.Field != organizer.FieldPlaceName {
		updated, _ = m.updateFields("down")
		m = updated.(routeModel)
	}
	updated, _ = m.updateFields("enter")
	m = updated.(routeModel)
	if !m.editing {
		t.Fatal("Enter on an editable row did not start editing")
	}
	m.editor.SetValue("Epping Station")
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(routeModel)

	if m.editing {
		t.Fatal("Enter did not commit the edit")
	}
	selection, _, ok := m.currentSelection()
	if !ok || selection.Enrichment[organizer.FieldPlaceName] != "Epping Station" {
		t.Fatalf("enrichment = %v", selection.Enrichment)
	}
	// Focus stays on the row so the value can be revised immediately.
	if got := m.fields.Current(); got != routeFieldsField {
		t.Fatalf("focus after committing = %q, want %q", got, routeFieldsField)
	}
}

// Backspace and Delete walk back a column like Esc, but never leave the
// command: a key held down should not fall out of the session.
func TestRouteBackspaceWalksBackWithoutLeavingTheCommand(t *testing.T) {
	for _, key := range []string{"backspace", "delete"} {
		t.Run(key, func(t *testing.T) {
			root := routeTestRoot(t, "alpha")
			m := chooseRecipe(t, loadedRoute(t, root), "Location")
			updated, _ := m.updateActions("enter")
			m = updated.(routeModel)

			for _, want := range []string{routeActionsField, routeRecipesField, routeCapturesField} {
				if !m.CapturesShellKey(key) {
					t.Fatalf("%s from %q was handed to the shell", key, m.fields.Current())
				}
				switch m.fields.Current() {
				case routeFieldsField:
					updated, _ = m.updateFields(key)
				case routeActionsField:
					updated, _ = m.updateActions(key)
				case routeRecipesField:
					updated, _ = m.updateRecipes(key)
				}
				m = updated.(routeModel)
				if got := m.fields.Current(); got != want {
					t.Fatalf("%s moved to %q, want %q", key, got, want)
				}
			}

			// From CAPTURES there is nowhere to go back to, and unlike Esc it
			// must not leave the command or clear anything.
			if m.CapturesShellKey(key) {
				t.Fatalf("%s from CAPTURES should be inert, not handled", key)
			}
			updated, _ = m.updateCaptures(key)
			m = updated.(routeModel)
			if _, _, ok := m.currentSelection(); !ok {
				t.Fatalf("%s cleared the selection from CAPTURES", key)
			}
		})
	}
}

func TestRouteClearingACaptureDropsTheWholeSelection(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location")
	m.fields.Set(routeCapturesField)

	updated, _ := m.updateCaptures("u")
	m = updated.(routeModel)
	if len(m.selections) != 0 {
		t.Fatalf("u did not clear the selection: %v", m.selections)
	}
	if item, _ := m.captures.Selected(); !strings.HasPrefix(item.Label, "○ ") {
		t.Fatalf("cleared capture row = %q, want the unchosen marker", item.Label)
	}
}

// Esc walks back one column and only leaves the command from CAPTURES, so it
// never drops the operator out to the command picker mid-selection.
func TestRouteEscapeWalksBackBeforeLeavingTheCommand(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location")

	updated, _ := m.updateActions("enter")
	m = updated.(routeModel)
	for _, want := range []string{routeActionsField, routeRecipesField, routeCapturesField} {
		if !m.CapturesShellKey("esc") {
			t.Fatalf("Esc from %q was handed to the shell", m.fields.Current())
		}
		switch m.fields.Current() {
		case routeFieldsField:
			updated, _ = m.updateFields("esc")
		case routeActionsField:
			updated, _ = m.updateActions("esc")
		case routeRecipesField:
			updated, _ = m.updateRecipes("esc")
		}
		m = updated.(routeModel)
		if got := m.fields.Current(); got != want {
			t.Fatalf("Esc moved to %q, want %q", got, want)
		}
	}
	if m.CapturesShellKey("esc") {
		t.Fatal("Esc from CAPTURES should reach the shell and leave the command")
	}
}

// A quarter-width column cannot hold a paragraph, so a multiline field is
// edited in an overlay; Enter belongs to the text, and ctrl+s commits.
func TestRouteEditsAMultilineFieldInAnOverlay(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")

	m.actions.SelectID("action:" + string(organizer.ActionDailyAppend))
	m.refresh()
	updated, _ := m.updateActions("enter")
	m = updated.(routeModel)
	for m.fieldRows[m.fieldIndex].requirement.Field != organizer.FieldContent {
		updated, _ = m.updateFields("down")
		m = updated.(routeModel)
	}

	updated, _ = m.updateFields("enter")
	m = updated.(routeModel)
	if !m.editingNote {
		t.Fatal("a multiline field did not open the overlay")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "ctrl+s Commit") {
		t.Fatalf("overlay does not state how to commit:\n%s", view)
	}

	m.note.SetValue("first line\nsecond line")
	updated, _ = m.updateNote(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(routeModel)
	if !m.editingNote {
		t.Fatal("Enter must insert a newline rather than commit")
	}
	updated, _ = m.updateNote(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(routeModel)
	if m.editingNote {
		t.Fatal("ctrl+s did not commit")
	}
	selection, _, ok := m.currentSelection()
	if !ok || !strings.Contains(fmt.Sprint(selection.Enrichment[organizer.FieldContent]), "second line") {
		t.Fatalf("multiline enrichment = %v", selection.Enrichment)
	}
}

func TestCaptureSessionExposesScanAndRouteTabs(t *testing.T) {
	s := newSession()
	if want := []tui.Tab{{Label: "Scan", Active: true}, {Label: "Route"}}; s.Tabs()[0] != want[0] || s.Tabs()[1] != want[1] {
		t.Fatalf("tabs = %#v", s.Tabs())
	}
	updated, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	s = updated.(session)
	if s.active != 1 || !s.Tabs()[1].Active {
		t.Fatalf("] did not activate Route: %#v", s.Tabs())
	}
	if got := s.Status().Left; got != "ROUTE" {
		t.Fatalf("Route status left = %q", got)
	}
	updated, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	s = updated.(session)
	if s.active != 0 || !s.Tabs()[0].Active {
		t.Fatalf("[ did not return to Scan: %#v", s.Tabs())
	}
}

func TestCaptureSessionActivatesClickedTab(t *testing.T) {
	s := newSession()
	updated, _ := s.Update(tui.TabSelectedMsg{Index: 1})
	s = updated.(session)
	if s.active != 1 || !s.Tabs()[1].Active {
		t.Fatalf("clicking Route did not activate it: %#v", s.Tabs())
	}
	updated, _ = s.Update(tui.TabSelectedMsg{Index: 0})
	s = updated.(session)
	if s.active != 0 || !s.Tabs()[0].Active {
		t.Fatalf("clicking Scan did not activate it: %#v", s.Tabs())
	}
}

func TestCaptureSessionSharesOneCaptureLoad(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	s := newSessionWithSettings(root, "index.json", organizer.Builtin())
	updated, _ := s.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	s = updated.(session)
	updated, _ = s.Update(loadCaptures(root, "index.json")())
	s = updated.(session)

	if len(s.scan.entries) != 1 || len(s.route.entries) != 1 {
		t.Fatalf("scan entries = %d, route entries = %d, want 1 each", len(s.scan.entries), len(s.route.entries))
	}
}

func TestRouteRootControlOpensSharedFileExplorer(t *testing.T) {
	m := newRouteModel(t.TempDir(), "index.json")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m = updated.(routeModel)
	if view := m.View(); !strings.Contains(view, "CAPTURE ROOT") {
		t.Fatal("Route left column does not pin the Capture Root control")
	}
	m.fields.Set(routeRootField)
	opened, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = opened.(routeModel)
	if !m.rootControl.Picking() || cmd == nil {
		t.Fatal("Enter on the Root control did not open the File Explorer")
	}
	if !strings.Contains(m.View(), "FILE EXPLORER") {
		t.Fatal("File Explorer overlay is not rendered over the Route workspace")
	}
	if status := m.Status(); status.Left != "BROWSE" {
		t.Fatalf("browsing status = %q, want BROWSE", status.Left)
	}
}

func TestRouteRootChangeResetsPlan(t *testing.T) {
	m := newRouteModel(t.TempDir(), "index.json")
	m.selections["/old/capture"] = organizer.Selection{Recipe: "obsidian_location"}
	updated, _ := m.Update(rootChangedMsg{root: "/new/root"})
	m = updated.(routeModel)
	if m.root != "/new/root" {
		t.Fatalf("root = %q", m.root)
	}
	if len(m.selections) != 0 {
		t.Fatalf("selections survived a root change: %v", m.selections)
	}
	if m.rootControl.Display() != "/new/root" {
		t.Fatalf("Root control = %q", m.rootControl.Display())
	}
}

func TestRouteCaptureRowsUseIndexIdentityNotFolderName(t *testing.T) {
	entry := captureEntry{
		name: "aaaa-xxxxx",
		index: indexschema.Index{
			CreatedAt: "2026-10-10T12:23:24+11:00",
			Source:    indexschema.Source{App: "app1", Workflow: "wf"},
		},
	}
	if got, want := routeCaptureLabel(entry), "2026-10-10 12:23:24 +11"; got != want {
		t.Fatalf("label = %q, want %q", got, want)
	}
	if got, want := routeCaptureDetail(entry), "app1 · wf"; got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}

	entry.index.CreatedAt = "2026-10-10T12:23:24+05:30"
	if got, want := routeCaptureLabel(entry), "2026-10-10 12:23:24 +5:30"; got != want {
		t.Fatalf("half-hour offset label = %q, want %q", got, want)
	}

	entry.index.CreatedAt = "2026-10-10T12:23:24Z"
	if got, want := routeCaptureLabel(entry), "2026-10-10 12:23:24 Z"; got != want {
		t.Fatalf("UTC label = %q, want %q", got, want)
	}

	entry.index.CreatedAt = "not a timestamp"
	if got, want := routeCaptureLabel(entry), "not a timestamp"; got != want {
		t.Fatalf("unparseable createdAt = %q, want %q", got, want)
	}

	bare := captureEntry{name: "aaaa-xxxxx"}
	if got, want := routeCaptureLabel(bare), "aaaa-xxxxx"+string(os.PathSeparator); got != want {
		t.Fatalf("identity-less label = %q, want %q", got, want)
	}
	if got := routeCaptureDetail(bare); got != "" {
		t.Fatalf("identity-less detail = %q, want empty", got)
	}
}

// Actions are stubs: running a Capture reports the plan it would execute and
// writes nothing but the organizer's own record.
func TestRouteRunShowsThePlanWithoutExecutingIt(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldPlaceName, "Epping Station")
	selection.Set(organizer.FieldContent, "晚上再来看看")
	m.refresh()

	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(routeModel)
	if !m.running {
		t.Fatal("x did not open the run dialog")
	}
	if len(m.runPlans) != 2 {
		t.Fatalf("run plans = %d, want one per enabled action", len(m.runPlans))
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"RUN", "obsidian.location.upsert", "Locations/Epping Station.md", "Nothing has happened yet", "↵ Run"} {
		if !strings.Contains(view, want) {
			t.Fatalf("run dialog is missing %q:\n%s", want, view)
		}
	}
	// The dialog states what each Action would do, because it is now the point
	// where that is decided.
	if !strings.Contains(view, "Creates Locations/") {
		t.Fatalf("the confirmation does not say what an action does:\n%s", view)
	}
	if !m.CapturesShellKey("esc") {
		t.Fatal("Esc must close the dialog rather than leave the command")
	}

	// Esc cancels, and nothing has happened.
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(routeModel)
	if m.running {
		t.Fatal("Esc did not close the run dialog")
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", organizer.RecordFilename)); !os.IsNotExist(err) {
		t.Fatal("cancelling still recorded the capture")
	}

	// Confirming carries it out and returns focus to CAPTURES.
	m = runAndConfirm(t, m)
	if m.running {
		t.Fatal("confirming did not close the dialog")
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", organizer.RecordFilename)); err != nil {
		t.Fatalf("confirming did not record the capture: %v", err)
	}
	if got := m.fields.Current(); got != routeCapturesField {
		t.Fatalf("focus after running = %q, want %q", got, routeCapturesField)
	}
}

// A blocked plan is shown but not recorded: a Capture is handled only once its
// plan could actually run.
func TestRouteRunRecordsNothingWhenBlocked(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")

	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(routeModel)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Blocked") {
		t.Fatalf("blocked run should say so:\n%s", view)
	}
	if !strings.Contains(view, "· missing") {
		t.Fatalf("blocked run should name the missing values:\n%s", view)
	}
	if strings.Contains(view, "↵ Run") {
		t.Fatalf("a blocked plan should not offer to run:\n%s", view)
	}

	// Confirming a blocked plan does nothing at all.
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(routeModel)
	if _, err := os.Stat(filepath.Join(root, "alpha", organizer.RecordFilename)); !os.IsNotExist(err) {
		t.Fatal("a blocked run must not record the capture as organized")
	}
}

// The record lives in the Capture directory, so a Capture organized in an
// earlier session is still recognised after a reload.
func TestRouteRecordsOrganizedCapturesAndSortsThemBelowTheDivider(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldPlaceName, "Epping Station")
	selection.Set(organizer.FieldContent, "note")
	m.refresh()

	m = runAndConfirm(t, m)

	data, err := os.ReadFile(filepath.Join(root, "alpha", organizer.RecordFilename))
	if err != nil {
		t.Fatalf("no record written: %v", err)
	}
	var record organizer.Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if len(record.Runs) != 1 {
		t.Fatalf("record holds %d runs, want 1", len(record.Runs))
	}
	run := record.Runs[0]
	if run.Recipe != "obsidian_location_daily" || len(run.Actions) != 2 {
		t.Fatalf("run = %+v", run)
	}
	if run.Fields[organizer.FieldPlaceName] != "Epping Station" {
		t.Fatalf("run fields = %v", run.Fields)
	}
	if run.Actions[0].Executed {
		t.Fatal("no action is implemented, so none may be recorded as executed")
	}

	// The cursor lands on the next Capture still to handle.
	if item, _ := m.captures.Selected(); item.ID != "capture:"+filepath.Join(root, "beta") {
		t.Fatalf("cursor after running = %q, want beta", item.ID)
	}

	reloaded, _ := m.Update(loadCaptures(root, "index.json")())
	m = reloaded.(routeModel)

	ordered, boundary := m.orderedEntries()
	if boundary != 1 || ordered[0].name != "beta" || ordered[1].name != "alpha" {
		t.Fatalf("ordering = %v, boundary = %d, want the organized capture last", ordered, boundary)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "ORGANIZED") {
		t.Fatalf("the divider is not drawn:\n%s", view)
	}
}

// A Capture may be organized again — a new Action appears, or the first pass
// was wrong — and the record grows rather than being replaced.
func TestRouteOrganizingAgainAppendsARun(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldPlaceName, "Epping Station")
	selection.Set(organizer.FieldContent, "first pass")
	m.refresh()
	m = runAndConfirm(t, m)

	// Organize the same Capture again, under a different Recipe.
	m = chooseRecipe(t, m, "Location")
	m = runAndConfirm(t, m)

	record, ok := organizer.ReadRecord(filepath.Join(root, "alpha"))
	if !ok {
		t.Fatal("no record")
	}
	if len(record.Runs) != 2 {
		t.Fatalf("record holds %d runs, want the first pass kept beside the second", len(record.Runs))
	}
	if record.Runs[0].Recipe != "obsidian_location_daily" || record.Runs[1].Recipe != "obsidian_location" {
		t.Fatalf("runs = %q, %q", record.Runs[0].Recipe, record.Runs[1].Recipe)
	}
	latest, _ := record.Latest()
	if latest.Recipe != "obsidian_location" {
		t.Fatalf("latest = %q, want the most recent pass", latest.Recipe)
	}
	if item, _ := m.captures.Selected(); !strings.Contains(item.Detail, "×2") {
		t.Fatalf("row = %q, want the pass count", item.Detail)
	}
}

// Every column is reachable with the pointer, not only with the keyboard.
func TestRouteMouseDrivesEveryColumn(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	m := loadedRoute(t, root)
	widths := m.columnWidths()
	columnX := func(index int) int {
		x := 0
		for i := 0; i < index; i++ {
			x += widths[i] + columnGutter
		}
		return x + 4
	}
	click := func(m routeModel, x, y int) routeModel {
		updated, _ := m.updateMouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return updated.(routeModel)
	}

	// CAPTURES: a click selects the row and focuses the column.
	m = click(m, columnX(0), 3)
	if got := m.fields.Current(); got != routeCapturesField {
		t.Fatalf("focus = %q, want %q", got, routeCapturesField)
	}
	if got := m.captures.Cursor(); got != 1 {
		t.Fatalf("cursor = %d, want the clicked row", got)
	}

	// RECIPES: a click selects, and choosing stays on Enter.
	m = click(m, columnX(1), 2)
	if got := m.fields.Current(); got != routeRecipesField {
		t.Fatalf("focus = %q, want %q", got, routeRecipesField)
	}
	if m.recipes.Cursor() != 1 {
		t.Fatalf("recipe cursor = %d, want the clicked row", m.recipes.Cursor())
	}
	if _, _, ok := m.currentSelection(); ok {
		t.Fatal("clicking a recipe must not choose it")
	}

	updated, _ := m.updateRecipes("enter")
	m = updated.(routeModel)

	// ACTIONS: a click on the checkbox toggles, a click on the label selects.
	selection, recipe, _ := m.currentSelection()
	before := len(selection.EnabledActions(recipe))
	checkbox := columnX(2) - 2 + m.actions.LabelOffset()
	m = click(m, checkbox, 1)
	selection, recipe, _ = m.currentSelection()
	if got := len(selection.EnabledActions(recipe)); got != before-1 {
		t.Fatalf("enabled actions = %d, want %d after clicking the checkbox", got, before-1)
	}
	m = click(m, checkbox+8, 1)
	selection, recipe, _ = m.currentSelection()
	if got := len(selection.EnabledActions(recipe)); got != before-1 {
		t.Fatal("clicking the label must not toggle the action")
	}

	// FIELDS: a single click selects the row without opening its editor.
	// Re-enable the action first: a disabled one contributes no editable field.
	m = click(m, checkbox, 1)
	m = click(m, columnX(3), 1)
	if got := m.fields.Current(); got != routeFieldsField {
		t.Fatalf("focus = %q, want %q", got, routeFieldsField)
	}
	if m.editing {
		t.Fatal("a single click should select the row, not start editing")
	}

	// The wheel scrolls the list under the pointer without changing focus.
	before = m.captures.Top()
	updated, _ = m.updateMouse(tea.MouseMsg{X: columnX(0), Y: 3, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = updated.(routeModel)
	if got := m.fields.Current(); got != routeFieldsField {
		t.Fatalf("the wheel moved focus to %q", got)
	}
	if m.captures.Top() < before {
		t.Fatal("the wheel did not scroll the captures list")
	}
}

// A double click does what Enter does in that column, so the pointer alone can
// drive the whole session.
func TestRouteDoubleClickActsLikeEnter(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	m := loadedRoute(t, root)
	widths := m.columnWidths()
	columnX := func(index int) int {
		x := 0
		for i := 0; i < index; i++ {
			x += widths[i] + columnGutter
		}
		return x + 4
	}
	click := func(m routeModel, x, y int) routeModel {
		updated, _ := m.updateMouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return updated.(routeModel)
	}
	doubleClick := func(m routeModel, x, y int) routeModel {
		return click(click(m, x, y), x, y)
	}

	// CAPTURES: Enter moves to RECIPES.
	m = doubleClick(m, columnX(0), 1)
	if got := m.fields.Current(); got != routeRecipesField {
		t.Fatalf("focus after double-clicking a capture = %q, want %q", got, routeRecipesField)
	}

	// RECIPES: Enter chooses the recipe and moves to ACTIONS.
	m = doubleClick(m, columnX(1), 2)
	if got := m.fields.Current(); got != routeActionsField {
		t.Fatalf("focus after double-clicking a recipe = %q, want %q", got, routeActionsField)
	}
	selection, _, ok := m.currentSelection()
	if !ok || selection.Recipe != "obsidian_location_daily" {
		t.Fatalf("double-clicking a recipe did not choose it: %+v", selection)
	}

	// ACTIONS: Enter moves to FIELDS.
	m = doubleClick(m, columnX(2), 1)
	if got := m.fields.Current(); got != routeFieldsField {
		t.Fatalf("focus after double-clicking an action = %q, want %q", got, routeFieldsField)
	}

	// FIELDS: Enter edits the row. The rows above the rule come from the
	// capture, so the editable one is below it.
	row := m.missingStart() + 2 // one for the border, one for the rule
	m = doubleClick(m, columnX(3), row)
	if !m.editing {
		t.Fatalf("double-clicking a missing field row did not open its editor:\n%s", ansi.Strip(m.View()))
	}
}

// Two presses on a checkbox are two toggles, not a double click.
func TestRouteCheckboxClicksDoNotPairIntoADoubleClick(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")
	widths := m.columnWidths()
	checkbox := widths[0] + columnGutter + widths[1] + columnGutter + 2 + m.actions.LabelOffset()
	click := func(m routeModel, y int) routeModel {
		updated, _ := m.updateMouse(tea.MouseMsg{X: checkbox, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return updated.(routeModel)
	}

	selection, recipe, _ := m.currentSelection()
	before := len(selection.EnabledActions(recipe))
	m = click(click(m, 1), 1)
	selection, recipe, _ = m.currentSelection()
	if got := len(selection.EnabledActions(recipe)); got != before {
		t.Fatalf("enabled actions = %d, want %d after toggling off and on again", got, before)
	}
	if got := m.fields.Current(); got != routeActionsField {
		t.Fatalf("focus = %q, want the checkbox not to have advanced a column", got)
	}
}

// A second press long after the first is a fresh click, not a double one.
func TestRouteDoubleClickWindowExpires(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	m := loadedRoute(t, root)
	press := func(m routeModel) routeModel {
		updated, _ := m.updateMouse(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return updated.(routeModel)
	}

	m = press(m)
	m.lastClick.at = time.Now().Add(-2 * doubleClickWindow)
	m = press(m)
	if got := m.fields.Current(); got != routeCapturesField {
		t.Fatalf("focus = %q, want the slow second press to be a fresh click", got)
	}
}

// Toggling an Action off takes the fields only it required out of both halves,
// and leaves a field another enabled Action still needs.
func TestRouteFieldsFollowTheEnabledActions(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")

	_, missing := splitFieldRows(m)
	if !equalFieldIDs(missing, []organizer.FieldID{organizer.FieldPlaceName, organizer.FieldContent, organizer.FieldTags}) {
		t.Fatalf("missing = %v", fieldRowIDs(missing))
	}

	m.actions.SelectID("action:" + string(organizer.ActionDailyAppend))
	updated, _ := m.updateActions(" ")
	m = updated.(routeModel)

	resolved, missing := splitFieldRows(m)
	if !equalFieldIDs(missing, []organizer.FieldID{organizer.FieldPlaceName, organizer.FieldTags}) {
		t.Fatalf("missing after disabling the daily note = %v", fieldRowIDs(missing))
	}
	for _, row := range resolved {
		if row.requirement.Field == organizer.FieldCreatedAt {
			t.Fatal("createdAt was required only by the disabled action and should be gone")
		}
	}
}

// A field two enabled Actions both require appears once, so it is filled once.
func TestRouteFieldsDeduplicateAcrossActions(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")

	count := 0
	for _, row := range m.fieldRows {
		if row.requirement.Field == organizer.FieldContent {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("content appears %d times, want once", count)
	}
}

// The ACTIONS detail says what the focused Action needs of this Capture and
// what it will do outside it, which is where a field is attributed.
func TestRouteActionDetailNamesNeedsAndEffects(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")
	m.actions.SelectID("action:" + string(organizer.ActionLocationUpsert))
	m.refresh()

	view := ansi.Strip(m.View())
	for _, want := range []string{"obsidian.location.upsert", "Place name*", "Effects", "Creates Locations/"} {
		if !strings.Contains(view, want) {
			t.Fatalf("action detail is missing %q:\n%s", want, view)
		}
	}

	m.actions.SelectID("action:" + string(organizer.ActionDailyAppend))
	m.refresh()
	view = ansi.Strip(m.View())
	for _, want := range []string{"obsidian.daily.append", "Note*     · missing", "Appends one entry"} {
		if !strings.Contains(view, want) {
			t.Fatalf("action detail is missing %q:\n%s", want, view)
		}
	}
}

// A Recipe explains itself before it is chosen, because choosing one resets the
// enabled Action set and should not be how its content is discovered.
func TestRouteRecipeDetailDescribesTheCandidateUnderTheCursor(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := loadedRoute(t, root)
	updated, _ := m.updateCaptures("enter")
	m = updated.(routeModel)
	m.recipes.SelectID("recipe:obsidian_location_daily")
	m.refresh()

	view := ansi.Strip(m.View())
	for _, want := range []string{"obsidian_location_daily", "Runs", "Location note", "Daily note", "Matches been_here", "Source  built-in"} {
		if !strings.Contains(view, want) {
			t.Fatalf("recipe detail is missing %q:\n%s", want, view)
		}
	}
	if _, _, ok := m.currentSelection(); ok {
		t.Fatal("describing a recipe must not choose it")
	}
}

// The row under the FIELDS cursor is marked the way every other list on the
// screen marks one: the same style, not a lone character the muted style then
// paints over.
func TestRouteFieldsHighlightMatchesTheOtherLists(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")
	updated, _ := m.updateActions("enter")
	m = updated.(routeModel)
	for m.fieldRows[m.fieldIndex].requirement.Field != organizer.FieldPlaceName {
		updated, _ = m.updateFields("down")
		m = updated.(routeModel)
	}

	// "Place name*" also appears in the ACTIONS detail pane, so the lines are
	// read from the FIELDS column alone.
	widths := m.columnWidths()
	start := widths[0] + widths[1] + widths[2] + 3*columnGutter
	marker := selectionSGR(t)
	var selected, unselected string
	for _, line := range strings.Split(m.View(), "\n") {
		column := ansi.Cut(line, start, start+widths[3])
		switch {
		case strings.Contains(ansi.Strip(column), "Place name*"):
			selected = column
		case strings.Contains(ansi.Strip(column), "Note*"):
			unselected = column
		}
	}
	if selected == "" || unselected == "" {
		t.Fatalf("field rows not found:\n%s", ansi.Strip(m.View()))
	}
	if !strings.Contains(selected, marker) {
		t.Errorf("the selected field row does not use the shared selection style:\n%q", selected)
	}
	if strings.Contains(unselected, marker) {
		t.Errorf("an unselected field row carries the selection style:\n%q", unselected)
	}
}

// selectionSGR is the escape sequence the shared selected-row style emits.
func selectionSGR(t *testing.T) string {
	t.Helper()
	rendered := scrolllist.SelectedRowStyle().Render("x")
	index := strings.Index(rendered, "x")
	if index <= 0 {
		t.Fatalf("selection style emits no sequence: %q", rendered)
	}
	return rendered[:index]
}

// Running moves the cursor to the next Capture, so the dialog must keep
// reporting the Capture it ran rather than re-resolving whatever is selected
// behind it.
func TestRouteRunDialogReportsTheCaptureItRan(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldPlaceName, "Epping Station")
	selection.Set(organizer.FieldContent, "the note I typed")
	m.refresh()

	// Confirming moves the cursor to the next Capture, so the dialog is read
	// while it is still open, and again on the way out.
	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(routeModel)
	before := ansi.Strip(m.View())
	if !strings.Contains(before, "the note I typed") {
		t.Fatalf("the dialog does not report the capture it is about to run:\n%s", before)
	}
	m = runAndConfirm(t, m)

	if item, _ := m.captures.Selected(); item.ID != "capture:"+filepath.Join(root, "beta") {
		t.Fatalf("cursor = %q, want the next capture", item.ID)
	}
	m.running = true
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "the note I typed") {
		t.Fatalf("the dialog lost the value it ran with:\n%s", view)
	}
	if strings.Contains(view, "· missing") {
		t.Fatalf("the dialog is reporting the next capture's values:\n%s", view)
	}
}
