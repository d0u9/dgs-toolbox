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

// routeVault is a vault with a daily note template in it, configured the way a
// reader configures one.
// starterSet is the Recipes this version ships, as a session has them: written
// into a directory and loaded back. Nothing is compiled in, so a test that
// wants a Recipe asks for the ones a reader will actually have.
func starterSet(t *testing.T) organizer.Set {
	t.Helper()
	dir := t.TempDir()
	for _, starter := range organizer.StartersOf(organizer.StarterRecipes) {
		if err := os.WriteFile(filepath.Join(dir, starter.Name), starter.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loaded := organizer.Load(dir)
	if len(loaded.Failures) > 0 {
		t.Fatalf("the shipped recipes do not load: %v", loaded.Failures)
	}
	return loaded.Set
}

// starterSources is where the workflows this version ships keep their fields,
// loaded the way a run loads them: the compiled-in reading is a last resort
// only, so a session in a test reads the shipped files as one on disk does.
func starterSources(t *testing.T) map[string]map[organizer.FieldID][]string {
	t.Helper()
	dir := t.TempDir()
	for _, starter := range organizer.StartersOf(organizer.StarterWorkflows) {
		if err := os.WriteFile(filepath.Join(dir, starter.Name), starter.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loaded := organizer.LoadWorkflows(dir)
	if len(loaded.Failures) > 0 {
		t.Fatalf("the shipped workflows do not load: %v", loaded.Failures)
	}
	return loaded.Sources
}

func routeVault(t *testing.T) organizer.Settings {
	t.Helper()
	templates := t.TempDir()
	if err := os.WriteFile(filepath.Join(templates, organizer.DailyNoteTemplate), []byte("---\nDate: {{.Date}}\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := organizer.DefaultSettings()
	settings.Sources = starterSources(t)
	settings.ObsidianVault = t.TempDir()
	settings.DailyNote = "Daily/{{.Date}}.md"
	settings.LocationNote = "88 Inbox/06 Locations.md"
	settings.LocationArchive = "88 Inbox/06 Locations"
	settings.TemplateDir = templates
	return settings
}

func loadedRoute(t *testing.T, root string) routeModel {
	t.Helper()
	settings := routeVault(t)
	m := newRouteModelWithSettings(root, "index.json", starterSet(t), settings)
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
	// The location note's path does not depend on anything the Capture is
	// missing, so it is shown even while the Action is blocked.
	if !strings.Contains(view, "88 Inbox/06 Locations.md") {
		t.Fatalf("the target the action would write to is not shown:\n%s", view)
	}

	// FIELDS lists the union across enabled actions, split by state: what
	// already has a value, then what is still missing.
	resolved, missing := splitFieldRows(m)
	if !equalFieldIDs(resolved, []organizer.FieldID{organizer.FieldCreatedAt}) {
		t.Fatalf("resolved = %v", fieldRowIDs(resolved))
	}
	// What is still to supply comes first.
	if !m.fieldRows[0].missing {
		t.Fatalf("the first row is %q, want a missing field", m.fieldRows[0].requirement.Field)
	}
	if !equalFieldIDs(missing, []organizer.FieldID{organizer.FieldContent, organizer.FieldTags}) {
		t.Fatalf("missing = %v", fieldRowIDs(missing))
	}
	if resolved[0].editable {
		t.Fatal("createdAt comes from the capture and must not be editable")
	}

	selection, recipe, ok := m.currentSelection()
	if !ok {
		t.Fatal("no selection")
	}
	selection.Set(organizer.FieldContent, "晚上再来看看")
	m.refresh()

	if !organizer.Ready(mustContext(t, m), recipe, selection.EnabledActions(recipe)) {
		t.Fatal("capture should be ready once the note is supplied")
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "[x] ● Location note") {
		t.Fatalf("location action should be ready:\n%s", view)
	}
	if !strings.Contains(view, "88 Inbox/06 Locations.md") {
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

// focusField moves the FIELDS cursor onto a field, failing rather than walking
// for ever when the column does not hold it.
func fieldIDsOf(requirements []organizer.FieldRequirement) []organizer.FieldID {
	ids := make([]organizer.FieldID, 0, len(requirements))
	for _, requirement := range requirements {
		ids = append(ids, requirement.Field)
	}
	return ids
}

func focusField(t *testing.T, m routeModel, field organizer.FieldID) routeModel {
	t.Helper()
	for index, row := range m.fieldRows {
		if row.requirement.Field == field {
			m.fieldIndex = index
			return m
		}
	}
	t.Fatalf("FIELDS holds no %q: %v", field, fieldRowIDs(m.fieldRows))
	return m
}

func splitFieldRows(m routeModel) (resolved, missing []routeFieldRow) {
	for _, row := range m.fieldRows {
		switch {
		case row.parameter:
		case row.missing:
			missing = append(missing, row)
		default:
			resolved = append(resolved, row)
		}
	}
	return resolved, missing
}

func parameterRows(m routeModel) []routeFieldRow {
	var rows []routeFieldRow
	for _, row := range m.fieldRows {
		if row.parameter {
			rows = append(rows, row)
		}
	}
	return rows
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
	// Both Actions want the note, so it is reported once for each of them.
	if len(before) != 2 {
		t.Fatalf("missing before = %v, want the note for each action", fieldIDsOf(before))
	}

	m.actions.SelectID("action:" + string(organizer.ActionDailyAppend))
	updated, _ := m.updateActions(" ")
	m = updated.(routeModel)

	selection, recipe, _ = m.currentSelection()
	after := organizer.MissingFields(mustContext(t, m), recipe, selection.EnabledActions(recipe))
	if len(after) != 1 || after[0].Action != organizer.ActionLocationAppend {
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
	// The note is a multiline field, so it is edited in the overlay.
	m = focusField(t, m, organizer.FieldContent)
	updated, _ = m.updateFields("enter")
	m = updated.(routeModel)
	if !m.editingNote {
		t.Fatal("Enter on an editable row did not start editing")
	}
	m.note.SetValue("Epping Station")
	updated, _ = m.updateNote(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(routeModel)

	if m.editingNote {
		t.Fatal("ctrl+s did not commit the edit")
	}
	selection, _, ok := m.currentSelection()
	if !ok || selection.Enrichment[organizer.FieldContent] != "Epping Station" {
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
	m = focusField(t, m, organizer.FieldContent)

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
	s := newSessionWithSettings(root, "index.json", starterSet(t), organizer.DefaultSettings())
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
	m := chooseRecipe(t, loadedRoute(t, root), "Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "晚上再来看看")
	m.refresh()

	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(routeModel)
	if !m.running {
		t.Fatal("x did not open the run dialog")
	}
	if len(m.runPlans) != 1 {
		t.Fatalf("run plans = %d, want one per enabled action", len(m.runPlans))
	}
	dialog := ansi.Strip(m.runOverlay())
	for _, want := range []string{"obsidian.daily.append", "Writes", "With", "Nothing has happened yet", "↵ Run"} {
		if !strings.Contains(dialog, want) {
			t.Fatalf("run dialog is missing %q:\n%s", want, dialog)
		}
	}
	// The dialog states what each Action would do, because it is now the point
	// where that is decided.
	if !strings.Contains(dialog, "Appends the Capture") {
		t.Fatalf("the confirmation does not say what an action does:\n%s", dialog)
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
	m := chooseRecipe(t, loadedRoute(t, root), "Daily")

	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(routeModel)
	dialog := ansi.Strip(m.runOverlay())
	if !strings.Contains(dialog, "Blocked") {
		t.Fatalf("blocked run should say so:\n%s", dialog)
	}
	if !strings.Contains(dialog, "· missing") {
		t.Fatalf("blocked run should name the missing values:\n%s", dialog)
	}
	if strings.Contains(dialog, "↵ Run") {
		t.Fatalf("a blocked plan should not offer to run:\n%s", dialog)
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
	m := chooseRecipe(t, loadedRoute(t, root), "Daily")
	selection, _, _ := m.currentSelection()
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
	if run.Recipe != "obsidian_daily" || len(run.Actions) != 1 {
		t.Fatalf("run = %+v", run)
	}
	if run.Fields[organizer.FieldContent] != "note" {
		t.Fatalf("run fields = %v", run.Fields)
	}
	if !run.Actions[0].Executed || !record.Organized() {
		t.Fatalf("the run did not record what happened: %+v", run.Actions[0])
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
	m := chooseRecipe(t, loadedRoute(t, root), "Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "first pass")
	m.refresh()
	m = runAndConfirm(t, m)

	// Organize the same Capture again.
	m = chooseRecipe(t, m, "Daily")
	m = runAndConfirm(t, m)

	record, ok := organizer.ReadRecord(filepath.Join(root, "alpha"))
	if !ok {
		t.Fatal("no record")
	}
	if len(record.Runs) != 2 {
		t.Fatalf("record holds %d runs, want the first pass kept beside the second", len(record.Runs))
	}
	latest, _ := record.Latest()
	if latest.Recipe != "obsidian_daily" || !latest.Actions[0].Skipped {
		t.Fatalf("the second pass should find its work already done: %+v", latest.Actions)
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

	// RECIPES: Enter chooses the recipe and moves to ACTIONS. Row 3 is
	// Location + Daily: candidates are listed in the order their files sort in.
	m = doubleClick(m, columnX(1), 3)
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

	// FIELDS: Enter edits the row. The rows still to supply come first, so the
	// first row below the border is editable — the note, which opens in the
	// overlay because it is multiline.
	m = doubleClick(m, columnX(3), 1)
	if !m.editing && !m.editingNote {
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

// FIELDS follows the enabled Actions: a field two of them want is asked for
// once and stays while either is enabled, and the Recipe's own field stays
// whatever is enabled.
func TestRouteFieldsFollowTheEnabledActions(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Location + Daily")

	_, missing := splitFieldRows(m)
	if !equalFieldIDs(missing, []organizer.FieldID{organizer.FieldContent, organizer.FieldTags}) {
		t.Fatalf("missing = %v", fieldRowIDs(missing))
	}

	m.actions.SelectID("action:" + string(organizer.ActionDailyAppend))
	updated, _ := m.updateActions(" ")
	m = updated.(routeModel)

	// The location note wants the same two, so disabling the daily note
	// changes nothing here — and the parameter that belonged to it is gone.
	_, missing = splitFieldRows(m)
	if !equalFieldIDs(missing, []organizer.FieldID{organizer.FieldContent, organizer.FieldTags}) {
		t.Fatalf("missing after disabling the daily note = %v", fieldRowIDs(missing))
	}
	if len(parameterRows(m)) != 0 {
		t.Fatalf("the disabled action's parameter is still listed: %+v", parameterRows(m))
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
	m.actions.SelectID("action:" + string(organizer.ActionLocationAppend))
	m.refresh()

	view := ansi.Strip(m.View())
	for _, want := range []string{"obsidian.location.append", "Note*", "Effects", "running list of places"} {
		if !strings.Contains(view, want) {
			t.Fatalf("action detail is missing %q:\n%s", want, view)
		}
	}

	m.actions.SelectID("action:" + string(organizer.ActionDailyAppend))
	m.refresh()
	view = ansi.Strip(m.View())
	for _, want := range []string{"obsidian.daily.append", "Note*     · missing", "Appends the Capture"} {
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
	for _, want := range []string{"obsidian_location_daily", "Runs", "Location note", "Daily note", "Source  obsidian_location_daily.yaml", "Matches", "- been_here"} {
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
	m = focusField(t, m, organizer.FieldContent)

	// "Note*" also appears in the ACTIONS detail pane, so the lines are read
	// from the FIELDS column alone.
	widths := m.columnWidths()
	start := widths[0] + widths[1] + widths[2] + 3*columnGutter
	marker := selectionSGR(t)
	var selected, unselected string
	for _, line := range strings.Split(m.View(), "\n") {
		column := ansi.Cut(line, start, start+widths[3])
		switch {
		case strings.Contains(ansi.Strip(column), "Note*"):
			selected = column
		case strings.Contains(ansi.Strip(column), "Created*"):
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
	m := chooseRecipe(t, loadedRoute(t, root), "Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "the note I typed")
	m.refresh()

	// Confirming moves the cursor to the next Capture, so the dialog is read
	// while it is still open, and again on the way out.
	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(routeModel)
	before := ansi.Strip(m.runOverlay())
	if !strings.Contains(before, "the note I typed") {
		t.Fatalf("the dialog does not report the capture it is about to run:\n%s", before)
	}
	m = runAndConfirm(t, m)

	if item, _ := m.captures.Selected(); item.ID != "capture:"+filepath.Join(root, "beta") {
		t.Fatalf("cursor = %q, want the next capture", item.ID)
	}
	view := ansi.Strip(m.runOverlay())
	if !strings.Contains(view, "the note I typed") {
		t.Fatalf("the dialog lost the value it ran with:\n%s", view)
	}
	if strings.Contains(view, "· missing") {
		t.Fatalf("the dialog is reporting the next capture's values:\n%s", view)
	}
}

// A plan naming an Action that has only been declared is refused before it is
// offered, not after it has failed.
func TestRouteRefusesAPlanWithAnUnimplementedAction(t *testing.T) {
	root := quickMarkRoot(t, "alpha")
	m := chooseRecipe(t, loadedRoute(t, root), "Apple Note")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldTitle, "a title")
	selection.Set(organizer.FieldContent, "a note")
	m.refresh()

	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(routeModel)
	dialog := ansi.Strip(m.runOverlay())
	if !strings.Contains(dialog, "Not implemented yet: apple.notes.create") {
		t.Fatalf("the dialog does not say what cannot run:\n%s", dialog)
	}
	if strings.Contains(dialog, "↵ Run") {
		t.Fatalf("a plan that cannot run should not offer to:\n%s", dialog)
	}

	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(routeModel)
	if _, err := os.Stat(filepath.Join(root, "alpha", organizer.RecordFilename)); !os.IsNotExist(err) {
		t.Fatal("confirming an unrunnable plan recorded something")
	}

	// Choosing a recipe whose Actions all exist makes it runnable again.
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(routeModel)
	m = chooseRecipe(t, m, "Daily")
	selection, _, _ = m.currentSelection()
	selection.Set(organizer.FieldContent, "a note")
	m.refresh()

	m = runAndConfirm(t, m)
	record, ok := organizer.ReadRecord(filepath.Join(root, "alpha"))
	if !ok || !record.Organized() {
		t.Fatalf("the remaining action did not run: %+v", record)
	}
}

// quickMarkRoot is a Capture root whose Captures match the Apple Recipes, which
// are the ones still waiting on their APIs.
func quickMarkRoot(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		index := []byte(`{"schema":"v1","source":{"app":"Shortcut","workflow":"quick_mark","device":{"os":"iOS","systemVersion":"26.4.2","name":"Phone"}},"id":"` + name + `","payload":{"mark":"a note"},"createdAt":"2026-09-09T16:34:35.556+10:00"}`)
		if err := os.WriteFile(filepath.Join(path, "index.json"), index, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The entry lands in the vault, under the configured section, once.
func TestRouteWritesTheEntryIntoTheVault(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	settings := routeVault(t)
	vault := settings.ObsidianVault
	m := newRouteModelWithSettings(root, "index.json", starterSet(t), settings)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m = updated.(routeModel)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(routeModel)

	m = chooseRecipe(t, m, "Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "the note I typed")
	m.refresh()
	m = runAndConfirm(t, m)

	note, err := os.ReadFile(filepath.Join(vault, "Daily", "2026-09-09.md"))
	if err != nil {
		t.Fatalf("nothing was written to the vault: %v", err)
	}
	for _, want := range []string{"# Captured - Phone", "the note I typed", "^dgs-alpha"} {
		if !strings.Contains(string(note), want) {
			t.Fatalf("note is missing %q:\n%s", want, note)
		}
	}
}

// Running lands on the top of what is left, not on whatever followed the
// Capture just organized: a Capture skipped earlier would otherwise be stranded
// above the cursor for the rest of the session.
func TestRouteAdvancesToTheTopOfWhatIsLeft(t *testing.T) {
	root := routeTestRoot(t, "a", "b", "c")
	m := loadedRoute(t, root)

	// Work on the second Capture, leaving the first for later.
	m.captures.SelectID("capture:" + filepath.Join(root, "b"))
	m.refresh()
	m = chooseRecipe(t, m, "Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "note")
	m.refresh()
	m = runAndConfirm(t, m)

	if item, _ := m.captures.Selected(); item.ID != "capture:"+filepath.Join(root, "a") {
		t.Fatalf("cursor = %q, want the first capture still to handle", item.ID)
	}
	if got := m.captures.Cursor(); got != 0 {
		t.Fatalf("cursor row = %d, want the top of the list", got)
	}
}

// A Capture no Recipe matches cannot be worked on, so the cursor passes over it
// rather than landing there after every run.
func TestRouteAdvancesPastCapturesNoRecipeMatches(t *testing.T) {
	root := routeTestRoot(t, "a", "b")
	// A workflow the built-in Recipes do not match.
	index := []byte(`{"schema":"v1","source":{"app":"Shortcut","workflow":"voice_memo","device":{"os":"iOS","systemVersion":"26.4.2","name":"Phone"}},"id":"unmatched","payload":{},"createdAt":"2026-09-09T16:34:35.556+10:00"}`)
	if err := os.Mkdir(filepath.Join(root, "0-unmatched"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "0-unmatched", "index.json"), index, 0o600); err != nil {
		t.Fatal(err)
	}
	m := loadedRoute(t, root)

	m.captures.SelectID("capture:" + filepath.Join(root, "a"))
	m.refresh()
	m = chooseRecipe(t, m, "Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "note")
	m.refresh()
	m = runAndConfirm(t, m)

	if item, _ := m.captures.Selected(); item.ID != "capture:"+filepath.Join(root, "b") {
		t.Fatalf("cursor = %q, want the first capture that can be organized", item.ID)
	}
}

// A parameter says how an Action behaves rather than what it needs. It is
// listed whether or not it has been changed — one nobody can see is one nobody
// knows to change — and editing it overrides that Action for this Capture only.
func TestRouteParametersAreListedAndOverridable(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	m := chooseRecipe(t, loadedRoute(t, root), "Daily")

	rows := parameterRows(m)
	if len(rows) != 1 || rows[0].action != organizer.ActionDailyAppend || rows[0].name != organizer.ParameterSection {
		t.Fatalf("parameters = %+v", rows)
	}
	if rows[0].value != organizer.DefaultDailySection || rows[0].overridden {
		t.Fatalf("parameter = %+v, want the default, not an override", rows[0])
	}
	// They come last, below the work and the reference.
	if !m.fieldRows[len(m.fieldRows)-1].parameter {
		t.Fatal("parameters are not pinned to the bottom")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "PARAMETERS") || !strings.Contains(view, "daily.append · Section") {
		t.Fatalf("the column does not name the action a parameter belongs to:\n%s", view)
	}

	// Editing one overrides it for this Capture.
	m.fields.Set(routeFieldsField)
	for !m.fieldRows[m.fieldIndex].parameter {
		updated, _ := m.updateFields("down")
		m = updated.(routeModel)
	}
	updated, _ := m.updateFields("enter")
	m = updated.(routeModel)
	m.editor.SetValue("今日活动")
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(routeModel)

	rows = parameterRows(m)
	if rows[0].value != "今日活动" || !rows[0].overridden {
		t.Fatalf("parameter = %+v, want the override", rows[0])
	}
	selection, _, _ := m.currentSelection()
	if selection.Parameters[organizer.ActionDailyAppend][organizer.ParameterSection] != "今日活动" {
		t.Fatalf("the override was not recorded on the selection: %v", selection.Parameters)
	}

	// It belongs to this Capture: the next one starts from the default again.
	m.fields.Set(routeCapturesField)
	m.captures.SelectID("capture:" + filepath.Join(root, "beta"))
	m.refresh()
	m = chooseRecipe(t, m, "Daily")
	if rows = parameterRows(m); rows[0].value != organizer.DefaultDailySection || rows[0].overridden {
		t.Fatalf("the next capture inherited an override: %+v", rows[0])
	}
}

// The override decides where the entry is written, and the record says so: a
// reader coming back later would otherwise assume the default.
func TestRouteRunHonoursAndRecordsAParameter(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	settings := routeVault(t)
	vault := settings.ObsidianVault
	m := newRouteModelWithSettings(root, "index.json", starterSet(t), settings)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m = updated.(routeModel)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(routeModel)

	m = chooseRecipe(t, m, "Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "the note")
	selection.SetParameter(organizer.ActionDailyAppend, organizer.ParameterSection, "今日活动")
	m.refresh()
	m = runAndConfirm(t, m)

	note, err := os.ReadFile(filepath.Join(vault, "Daily", "2026-09-09.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(note), "\n# 今日活动\n") {
		t.Fatalf("the entry ignored the override:\n%s", note)
	}
	record, _ := organizer.ReadRecord(filepath.Join(root, "alpha"))
	latest, _ := record.Latest()
	if got := latest.Actions[0].Parameters[organizer.ParameterSection]; got != "今日活动" {
		t.Fatalf("the record does not say where it wrote: %+v", latest.Actions[0])
	}
}

// The control that runs the plan sits under the column it acts on, taking its
// room from that column alone rather than shortening all four, and not pressed
// into the corner: a row of space above it, a row below, and clear of the
// column's right edge.
func TestRouteRunButtonSitsUnderTheFieldsColumn(t *testing.T) {
	m := loadedRoute(t, routeTestRoot(t, "alpha"))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m = updated.(routeModel)

	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) != 24 {
		t.Fatalf("the view is %d rows, want the session height", len(lines))
	}
	widths := m.columnWidths()
	column := func(line string) string {
		start := widths[0] + widths[1] + widths[2] + 3*columnGutter
		return ansi.Cut(line, start, start+widths[3])
	}

	// The other columns still run to the last row; only the fourth ends early.
	if !strings.Contains(lines[len(lines)-1], "╰") {
		t.Fatalf("the other columns were shortened:\n%s", lines[len(lines)-1])
	}
	if got := column(lines[len(lines)-1]); strings.TrimSpace(got) != "" {
		t.Fatalf("the fourth column runs to the last row: %q", got)
	}

	// A row of space above the button and a row below it.
	button := len(lines) - 3
	if got := strings.TrimSpace(column(lines[button])); got != "Run  x" {
		t.Fatalf("the button is not two rows above the bottom: %q", got)
	}
	for _, blank := range []int{button - 1, button + 2} {
		if got := strings.TrimSpace(column(lines[blank])); got != "" {
			t.Fatalf("row %d beside the button is not empty: %q", blank, got)
		}
	}
	if strings.Contains(column(lines[button]), "│") {
		t.Fatalf("the button is inside the fieldset: %q", column(lines[button]))
	}

	// It is clear of the column's right edge rather than flush against it.
	if !strings.HasSuffix(column(lines[button]), " ") {
		t.Fatalf("the button touches the column edge: %q", column(lines[button]))
	}
}

// A button is pressed, not selected: one click acts.
func TestRouteRunButtonActsOnASingleClick(t *testing.T) {
	m := chooseRecipe(t, loadedRoute(t, routeTestRoot(t, "alpha")), "Daily")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m = updated.(routeModel)

	updated, _ = m.updateMouse(tea.MouseMsg{
		X: m.width - 10, Y: m.height - 3,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	m = updated.(routeModel)
	if !m.running {
		t.Fatal("clicking the button did not open the run dialog")
	}
}

// The button answers "can I run this yet?" without the dialog being opened: it
// is filled only when pressing it would carry the plan out, and says what is in
// the way otherwise.
func TestRouteRunButtonIsQuietUntilThePlanCanRun(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := loadedRoute(t, root)

	if subtitle, ready := m.runReadiness(); ready || subtitle != "Choose a recipe" {
		t.Fatalf("readiness = %q %v before a recipe is chosen", subtitle, ready)
	}

	m = chooseRecipe(t, m, "Daily")
	subtitle, ready := m.runReadiness()
	if ready || !strings.Contains(subtitle, "Missing Note") {
		t.Fatalf("readiness = %q %v with a field still missing", subtitle, ready)
	}

	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "note")
	m.refresh()
	if subtitle, ready = m.runReadiness(); !ready || subtitle != "Daily" {
		t.Fatalf("readiness = %q %v once nothing is missing", subtitle, ready)
	}

	// An Action that cannot run keeps it quiet too.
	quick := chooseRecipe(t, loadedRoute(t, quickMarkRoot(t, "alpha")), "Apple Note")
	selection, _, _ = quick.currentSelection()
	selection.Set(organizer.FieldTitle, "a title")
	quick.refresh()
	if subtitle, ready = quick.runReadiness(); ready || subtitle != "Not implemented" {
		t.Fatalf("readiness = %q %v with an unimplemented action", subtitle, ready)
	}
}

// Tab walks the fields that have something to select. A column waiting on a
// decision nobody has made holds nothing, and stopping there teaches the reader
// only that they are somewhere useless.
func TestRouteTabFollowsWhatIsSelectable(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := loadedRoute(t, root)

	// With no Recipe chosen, Tab walks the Captures, the Root, and the
	// candidates — not the Actions or Fields of a Recipe nobody has picked.
	var walked []string
	for range 4 {
		m.cycleFields(true)
		walked = append(walked, m.fields.Current())
	}
	want := []string{routeRecipesField, routeCapturesField, routeRecipesField, routeCapturesField}
	for index, field := range want {
		if walked[index] != field {
			t.Fatalf("tab walked %v, want %v", walked, want)
		}
	}

	// Choosing one brings its columns into the walk.
	m = chooseRecipe(t, m, "Daily")
	m.fields.Set(routeRecipesField)
	m.cycleFields(true)
	if got := m.fields.Current(); got != routeActionsField {
		t.Fatalf("tab from RECIPES = %q, want %q once a recipe is chosen", got, routeActionsField)
	}
	m.cycleFields(true)
	if got := m.fields.Current(); got != routeFieldsField {
		t.Fatalf("tab from ACTIONS = %q, want %q", got, routeFieldsField)
	}

	// Clearing it takes them out again, from wherever the focus happens to be.
	updated, _ := m.updateCaptures("u")
	m = updated.(routeModel)
	m.fields.Set(routeFieldsField)
	m.cycleFields(true)
	if got := m.fields.Current(); got != routeCapturesField {
		t.Fatalf("tab from a column that is no longer selectable = %q", got)
	}

	// The Capture Root is a setting rather than a step, so it is not in the
	// ring, but it is still directly below CAPTURES.
	m.fields.Set(routeCapturesField)
	if !m.fields.Move("alt+down") || m.fields.Current() != routeRootField {
		t.Fatalf("alt+down = %q, want the Capture Root", m.fields.Current())
	}

	// The arrows are spatial and still reach every column: a field is where it
	// is whether or not it is ready.
	m.fields.Set(routeCapturesField)
	if !m.fields.Move("alt+right") || m.fields.Current() != routeRecipesField {
		t.Fatalf("alt+right = %q, want %q", m.fields.Current(), routeRecipesField)
	}
	if !m.fields.Move("alt+right") || m.fields.Current() != routeActionsField {
		t.Fatalf("alt+right = %q, want %q", m.fields.Current(), routeActionsField)
	}
}

// A Capture already in the note is written once, and the second run says so
// rather than closing over it: the reader typed a note and nothing was added,
// which they have to be told before the dialog goes away.
func TestRouteHoldsTheDialogOpenWhenAnEntryWasAlreadyWritten(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	settings := routeVault(t)
	m := newRouteModelWithSettings(root, "index.json", starterSet(t), settings)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m = updated.(routeModel)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(routeModel)

	organize := func(m routeModel, note string) routeModel {
		m.captures.SelectID("capture:" + filepath.Join(root, "alpha"))
		m.refresh()
		m = chooseRecipe(t, m, "Daily")
		selection, _, _ := m.currentSelection()
		selection.Set(organizer.FieldContent, note)
		m.refresh()
		return runAndConfirm(t, m)
	}

	m = organize(m, "the first note")
	if m.running {
		t.Fatal("a run that wrote everything it planned should close on its own")
	}

	m = organize(m, "a second note the note will not take")
	if !m.running {
		t.Fatal("a skipped action should hold the dialog open")
	}
	if got := m.runNote(); !strings.Contains(got, "already in Daily/2026-09-09.md") {
		t.Fatalf("run note = %q, want it to name the note the entry is already in", got)
	}

	// Closing it does the move a clean run did straight away.
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(routeModel)
	if m.running {
		t.Fatal("esc should close the held dialog")
	}
	if item, _ := m.captures.Selected(); item.ID != "capture:"+filepath.Join(root, "beta") {
		t.Fatalf("cursor = %q, want the next capture to handle", item.ID)
	}
}

// A value wider than the column folds under its label rather than being clipped
// at the edge: a section written as a template is unreadable cut in half.
func TestRouteFoldsALongFieldValueUnderItsLabel(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := loadedRoute(t, root)
	m = chooseRecipe(t, m, "Daily")
	selection, _, _ := m.currentSelection()
	selection.Set(organizer.FieldContent, "a note long enough that it cannot sit on one line of this column")
	m.refresh()

	var rows int
	for _, line := range m.fieldLines(40) {
		if line.row >= 0 && strings.Contains(ansi.Strip(line.text), "cannot sit") {
			rows++
		}
	}
	if rows == 0 {
		t.Fatal("the value was clipped away instead of folded")
	}
	for _, line := range m.fieldLines(40) {
		if width := lipgloss.Width(line.text); width > 40 {
			t.Fatalf("line is %d cells wide, want it to fit the column:\n%s", width, ansi.Strip(line.text))
		}
	}
}

// A run that wrote nothing says why in the record, not only in the dialog it
// showed at the time: the reader comes back to the Capture months later and
// asks what happened to it.
func TestRouteRecordsWhyAnActionWroteNothing(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	settings := routeVault(t)
	m := newRouteModelWithSettings(root, "index.json", starterSet(t), settings)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m = updated.(routeModel)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(routeModel)

	organize := func(m routeModel, note string) routeModel {
		m.captures.SelectID("capture:" + filepath.Join(root, "alpha"))
		m.refresh()
		m = chooseRecipe(t, m, "Daily")
		selection, _, _ := m.currentSelection()
		selection.Set(organizer.FieldContent, note)
		m.refresh()
		return runAndConfirm(t, m)
	}
	m = organize(m, "the first note")
	m = organize(m, "a second note")

	record, ok := organizer.ReadRecord(filepath.Join(root, "alpha"))
	if !ok {
		t.Fatal("no record was written")
	}
	latest, _ := record.Latest()
	action := latest.Actions[0]
	if !action.Skipped {
		t.Fatalf("action = %+v, want it skipped", action)
	}
	if !strings.Contains(action.Reason, "already in Daily/2026-09-09.md") ||
		!strings.Contains(action.Reason, "^dgs-alpha") {
		t.Fatalf("reason = %q, want it to name the note and the mark it found", action.Reason)
	}
}
