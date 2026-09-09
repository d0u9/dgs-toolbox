package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/divider"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/overlay"
	"dgs-toolbox/internal/tui/scrolllist"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	routeCapturesField    = "route-captures"
	routeRootField        = "route-root"
	routeRecipesField     = "route-recipes"
	routeActionsField     = "route-actions"
	routeFieldsField      = "route-fields"
	routePropertyKeyWidth = 12
)

// routeModel is the Route session: it organizes each scanned Capture by
// choosing a Recipe, enabling the Actions to run, and supplying whatever those
// Actions still need. Every domain judgement belongs to the organizer package;
// this model calls FindRecipes, MissingFields, and Build, and draws the result.
// The session records the plan only: nothing is moved and no Action is run.
//
// The four columns are a progressive selection, each the result of the one to
// its left: Captures, Recipes, Actions, Fields.
type routeModel struct {
	width       int
	height      int
	root        string
	indexFile   string
	rootControl rootControl
	entries     []captureEntry
	captures    scrolllist.Model
	recipes     scrolllist.Model
	actions     scrolllist.Model
	// selections is the per-Capture decision, keyed by Capture path.
	selections map[string]organizer.Selection
	fieldRows  []routeFieldRow
	fieldIndex int
	editor     textinput.Model
	editing    bool
	// note is the multiline editor. A quarter-width column cannot hold a
	// paragraph, so a multiline field is edited in an overlay over the
	// workspace instead of in place.
	note        textarea.Model
	editingNote bool
	// runPlans is what the last Run showed. Actions are not implemented yet,
	// so running a Capture reports the plan it would execute instead of
	// touching anything.
	runPlans   []organizer.ActionPlan
	runCapture string
	runError   string
	running    bool
	// runContext is the Context the plan was built from, kept because running
	// moves the cursor to the next Capture: the dialog must keep reporting the
	// Capture it ran, not the one now selected behind it.
	runContext organizer.Context
	// recipeSet is the Set this session offers: the built-ins with whatever the
	// configured Recipe directory layered over them.
	recipeSet organizer.Set
	fields    datafield.Navigator
	loadError string
	pendingGG bool
	lastClick routeClick
}

// routeClick remembers the previous primary click so the next one can be
// recognised as a double click on the same row.
type routeClick struct {
	field string
	row   int
	at    time.Time
}

// doubleClickWindow is how long a second press on the same row still counts as
// a double click.
const doubleClickWindow = 500 * time.Millisecond

// routeFieldRow is one row of the FIELDS column. missing marks the rows below
// the rule: the fields the enabled Actions ask for and the Context cannot yet
// answer. A value the Capture itself carries is read-only; anything else,
// composed or supplied, is the user's to change.
type routeFieldRow struct {
	requirement organizer.FieldRequirement
	value       string
	editable    bool
	missing     bool
}

func newRouteModel(root, indexFile string) routeModel {
	return newRouteModelWithRecipes(root, indexFile, organizer.Builtin())
}

func newRouteModelWithRecipes(root, indexFile string, set organizer.Set) routeModel {
	editor := textinput.New()
	editor.Prompt = ""
	note := textarea.New()
	note.ShowLineNumbers = false
	note.Prompt = ""
	m := routeModel{
		width:       80,
		height:      22,
		root:        root,
		indexFile:   indexFile,
		rootControl: newRootControl(root),
		captures:    scrolllist.New(),
		recipes:     scrolllist.New(),
		actions:     scrolllist.New(),
		selections:  make(map[string]organizer.Selection),
		recipeSet:   set,
		editor:      editor,
		note:        note,
		fields: datafield.New(
			datafield.Field{ID: routeCapturesField, Row: 0, Col: 0},
			datafield.Field{ID: routeRootField, Row: 1, Col: 0},
			datafield.Field{ID: routeRecipesField, Row: 0, Col: 1},
			datafield.Field{ID: routeActionsField, Row: 0, Col: 2},
			datafield.Field{ID: routeFieldsField, Row: 0, Col: 3},
		),
	}
	m.fields.Set(routeCapturesField)
	m.refresh()
	return m
}

func (m routeModel) Init() tea.Cmd { return nil }

func (m routeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeComponents()
		m.rootControl.Resize(m.width, m.height)
		return m, nil
	case rootChangedMsg:
		m.applyRoot(msg.root)
		return m, nil
	case capturesLoadedMsg:
		m.root = msg.root
		m.loadError = ""
		if msg.err != nil {
			m.loadError = msg.err.Error()
			m.entries = nil
		} else {
			m.entries = msg.captures
			m.pruneSelections()
		}
		m.rebuildCaptureItems()
		m.refresh()
		return m, nil
	}

	if m.rootControl.Picking() {
		return m.updatePicker(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	}
	return m, nil
}

func (m routeModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	selected, cmd := m.rootControl.Update(msg)
	if selected == "" {
		return m, cmd
	}
	return m, tea.Batch(cmd, rootChanged(selected))
}

// updateMouse gives every DataField the same reach as the keyboard. A single
// click selects a row and focuses its field, as the shared list convention
// says, and a double click on that row does what Enter does there, so the whole
// session can be driven with the pointer alone. The one direct target is an
// Action's checkbox: a press on it toggles that Action and never pairs into a
// double click, because toggling twice would mean nothing.
func (m routeModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	hit := m.fields.HitAt(msg.X, msg.Y)
	if hit == "" {
		return m, nil
	}
	if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		return m.scrollField(hit, msg.Button == tea.MouseButtonWheelDown)
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if m.editing || m.editingNote || m.running {
		return m, nil
	}
	m.fields.FocusAt(msg.X, msg.Y)
	m.pendingGG = false
	double := m.isDoubleClick(hit, msg.Y)
	m.lastClick = routeClick{field: hit, row: msg.Y, at: time.Now()}

	switch hit {
	case routeRootField:
		if m.rootControl.Clicked(msg.X-2, msg.Y-m.captureListHeight()-1) {
			return m.openPicker()
		}
	case routeCapturesField:
		if !m.captures.SelectRow(msg.Y - 1) {
			return m, nil
		}
		m.refresh()
		if double {
			return m.updateCaptures("enter")
		}
	case routeRecipesField:
		m.recipes.SelectRow(msg.Y - 1)
		m.refresh()
		if double {
			return m.updateRecipes("enter")
		}
	case routeActionsField:
		if !m.actions.SelectRow(msg.Y - 1) {
			return m, nil
		}
		m.refresh()
		if m.clickedCheckbox(msg.X) {
			// A toggle is its own action, so it does not pair into a double
			// click: two presses on a checkbox mean off then on again.
			m.lastClick = routeClick{}
			m.toggleAction()
			m.refresh()
			return m, nil
		}
		if double {
			return m.updateActions("enter")
		}
	case routeFieldsField:
		if !m.selectFieldRow(msg.Y) {
			return m, nil
		}
		if double {
			return m.beginEdit()
		}
	}
	return m, nil
}

// isDoubleClick reports whether this press pairs with the previous one: the
// same row of the same field, soon enough after it.
func (m routeModel) isDoubleClick(field string, row int) bool {
	if m.lastClick.field != field || m.lastClick.row != row {
		return false
	}
	return !m.lastClick.at.IsZero() && time.Since(m.lastClick.at) <= doubleClickWindow
}

// scrollField moves the list under the pointer without changing focus, matching
// how the wheel behaves in Scan.
func (m routeModel) scrollField(hit string, down bool) (tea.Model, tea.Cmd) {
	delta := -3
	if down {
		delta = 3
	}
	switch hit {
	case routeCapturesField:
		m.captures.Scroll(delta)
	case routeRecipesField:
		m.recipes.Scroll(delta)
	case routeActionsField:
		m.actions.Scroll(delta)
	default:
		return m, nil
	}
	m.refresh()
	return m, nil
}

// clickedCheckbox reports whether the pointer landed on an Action's [x], which
// is the three cells at the start of its label.
func (m routeModel) clickedCheckbox(x int) bool {
	widths := m.columnWidths()
	column := widths[0] + columnGutter + widths[1] + columnGutter
	start := column + 2 + m.actions.LabelOffset()
	return x >= start && x < start+3
}

// selectFieldRow moves the FIELDS cursor to the clicked row. The MISSING rule
// takes a row of its own without being one, so a click below it maps one row
// higher, and a click on the rule selects nothing.
func (m *routeModel) selectFieldRow(y int) bool {
	index := y - 1
	if missingAt := m.missingStart(); missingAt >= 0 && index >= missingAt {
		if index == missingAt {
			return false
		}
		index--
	}
	if index < 0 || index >= len(m.fieldRows) {
		return false
	}
	m.fieldIndex = index
	return true
}

func (m routeModel) openPicker() (tea.Model, tea.Cmd) {
	return m, m.rootControl.Open(m.width, m.height)
}

// applyRoot points Route at a new root and drops the previous scan result.
// Selections are keyed by Capture path, so those under the old root go with it.
func (m *routeModel) applyRoot(root string) {
	m.root = root
	m.rootControl.SetRoot(root)
	m.entries = nil
	m.captures.SetItems(nil)
	m.selections = make(map[string]organizer.Selection)
	m.loadError = ""
	m.cancelEdit()
	m.refresh()
}

func (m routeModel) View() string {
	if m.width < 72 || m.height < 8 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, scanMutedStyle.Render("Resize terminal for Capture Route"))
	}
	widths := m.columnWidths()
	workspace := lipgloss.JoinHorizontal(lipgloss.Top,
		m.leftColumn(widths[0]), " ",
		m.recipesColumn(widths[1]), " ",
		m.actionsColumn(widths[2]), " ",
		m.fieldsColumn(widths[3]))
	if m.rootControl.Picking() {
		return overlay.Place(workspace, m.rootControl.OverlayView(m.width, m.height), m.width, m.height)
	}
	if m.editingNote {
		return overlay.Place(workspace, m.noteOverlay(), m.width, m.height)
	}
	if m.running {
		return overlay.Place(workspace, m.runOverlay(), m.width, m.height)
	}
	return workspace
}

// runOverlay reports what the plan would do. Every Action is a stub: the dialog
// names each one, the target it resolved, and the values it would carry, so the
// pipeline can be walked end to end before any Action writes anything.
func (m routeModel) runOverlay() string {
	width := max(30, min(76, m.width-8))
	inner := max(10, width-4)
	ctx := m.runContext
	lines := []string{scanMutedStyle.Render(m.runCapture), ""}
	if m.runError != "" {
		lines = append(lines, scanMutedStyle.Render("! "+m.runError), "")
	}
	for index, plan := range m.runPlans {
		marker := "●"
		if !plan.Ready() {
			marker = "○"
		}
		lines = append(lines, fmt.Sprintf("%s %d %s", marker, index+1, plan.Action))
		lines = append(lines, m.runDetails(ctx, plan)...)
	}
	if len(m.runPlans) == 0 {
		lines = append(lines, scanMutedStyle.Render("· No action is enabled, so there is nothing to run"))
	}
	note := "Actions are not implemented yet; only " + organizer.RecordFilename + " was appended to."
	if !m.runReady() {
		note = "Blocked, so nothing was recorded; fill the missing fields first."
	}
	lines = append(lines,
		"",
		scanMutedStyle.Render(note),
		scanMutedStyle.Render("esc Close"),
	)
	for i, line := range lines {
		lines[i] = truncateStyled(line, inner)
	}
	return fieldset.View("RUN", strings.Join(lines, "\n"), width)
}

// runReady reports whether the plan just shown could actually run.
func (m routeModel) runReady() bool {
	if len(m.runPlans) == 0 {
		return false
	}
	for _, plan := range m.runPlans {
		if !plan.Ready() {
			return false
		}
	}
	return true
}

// runDetails lists what one Action would carry: its target, then the value of
// every requirement it declares, or the fields still blocking it.
func (m routeModel) runDetails(ctx organizer.Context, plan organizer.ActionPlan) []string {
	var lines []string
	target := plan.Target
	if target == "" {
		target = "· unresolved"
	}
	lines = append(lines, scanMutedStyle.Render(fmt.Sprintf("      %-*s%s", routePropertyKeyWidth, "target", target)))
	def, ok := organizer.LookupAction(plan.Action)
	if !ok {
		return lines
	}
	for _, req := range def.Required {
		value := ctx.String(req.Field)
		if value == "" {
			value = "· missing"
		}
		label := truncate(req.Label, routePropertyKeyWidth-1)
		lines = append(lines, scanMutedStyle.Render(fmt.Sprintf("      %-*s%s", routePropertyKeyWidth, label, singleLine(value))))
	}
	return lines
}

// noteOverlay is the multiline editor floated over the workspace, titled by the
// field being edited so it is clear which Action is waiting on it.
func (m routeModel) noteOverlay() string {
	legend := "NOTE"
	if row, ok := m.focusedFieldRow(); ok {
		legend = strings.ToUpper(row.requirement.Label)
	}
	body := m.note.View() + "\n" + scanMutedStyle.Render("ctrl+s Commit  esc Cancel")
	return fieldset.View(legend, body, m.note.Width()+4)
}

// CapturesShellKey keeps Esc inside Route wherever it still has work to do:
// while a field is being edited, and in every column but the leftmost, where it
// walks back one column. Only from CAPTURES does Esc reach the shell and leave
// the command. Backspace and Delete walk back the same way, but never leave the
// command, so a key held down cannot fall out of the session.
func (m routeModel) CapturesShellKey(key string) bool {
	if m.editing || m.editingNote || m.running {
		return true
	}
	switch key {
	case "esc", "backspace", "delete":
		switch m.fields.Current() {
		case routeRecipesField, routeActionsField, routeFieldsField:
			return true
		}
	}
	return m.rootControl.CapturesShellKey(key)
}

func (m routeModel) Status() tui.Status {
	if m.rootControl.Picking() {
		if m.rootControl.HasDialog() {
			return tui.Status{Left: "BROWSE", Center: "CAPTURE ROOT"}
		}
		return tui.Status{Left: "BROWSE", Center: "CAPTURE ROOT", Right: m.rootControl.Hint()}
	}
	center := m.statusValue()
	if m.running {
		return tui.Status{Left: "RUN", Center: center, Right: "esc Close"}
	}
	if m.editingNote {
		return tui.Status{Left: "EDIT", Center: center, Right: "ctrl+s Commit  esc Cancel"}
	}
	if m.editing {
		return tui.Status{Left: "EDIT", Center: center, Right: "↵ Commit  esc Cancel"}
	}
	switch m.fields.Current() {
	case routeCapturesField:
		return tui.Status{Left: "ROUTE", Center: center, Right: "↑/k ↓/j Move  ↵ Recipe  x Run  u Clear  R Refresh"}
	case routeRootField:
		return tui.Status{Left: "CAPTURE ROOT", Center: center, Right: "↵ Browse  R Refresh  tab Next  alt+hjkl Focus"}
	case routeRecipesField:
		return tui.Status{Left: "RECIPES", Center: center, Right: "↑/k ↓/j Move  ↵ Choose  esc/⌫ Back"}
	case routeActionsField:
		return tui.Status{Left: "ACTIONS", Center: center, Right: "↑/k ↓/j Move  space Toggle  ↵ Fields  x Run  esc/⌫ Back"}
	case routeFieldsField:
		return tui.Status{Left: "FIELDS", Center: center, Right: "↑/k ↓/j Move  ↵ Edit  x Run  esc/⌫ Back"}
	default:
		return tui.Status{Left: "ROUTE", Center: center, Right: "tab Next  alt+hjkl Focus"}
	}
}

func (m routeModel) statusValue() string {
	if m.fields.Current() == routeRootField {
		return m.root
	}
	if entry, ok := m.selectedCapture(); ok {
		if recipe, ok := m.selectedRecipe(); ok {
			return entry.path + " → " + recipe.Name
		}
		return entry.path
	}
	ready, blocked := m.readyCount()
	return fmt.Sprintf("%d/%d READY · %d BLOCKED", ready, len(m.entries), blocked)
}

func (m routeModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.running {
		if key == "esc" || key == "enter" || key == "q" {
			m.running = false
		}
		return m, nil
	}
	if key == "x" {
		return m.run()
	}
	if m.editingNote {
		return m.updateNote(msg)
	}
	if m.editing {
		return m.updateEditor(msg)
	}
	if key == "R" {
		m.pendingGG = false
		return m, loadCaptures(m.root, m.indexFile)
	}
	if m.fields.Move(key) || key == "tab" || key == "shift+tab" {
		if key == "tab" || key == "shift+tab" {
			m.cycleFields(key == "tab")
		}
		m.pendingGG = false
		m.refresh()
		return m, nil
	}
	switch m.fields.Current() {
	case routeRootField:
		m.pendingGG = false
		if key == "enter" {
			return m.openPicker()
		}
		return m, nil
	case routeCapturesField:
		return m.updateCaptures(key)
	case routeRecipesField:
		return m.updateRecipes(key)
	case routeActionsField:
		return m.updateActions(key)
	case routeFieldsField:
		return m.updateFields(key)
	}
	m.pendingGG = false
	return m, nil
}

func (m routeModel) updateCaptures(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.captures.Move(-1)
	case "down", "j":
		m.captures.Move(1)
	case "left", "h":
		m.captures.Pan(-4)
	case "right", "l":
		m.captures.Pan(4)
	case "G", "end":
		m.captures.Last()
	case "g":
		if m.pendingGG {
			m.captures.First()
			m.pendingGG = false
			m.refresh()
			return m, nil
		}
		m.pendingGG = true
		return m, nil
	case "enter":
		m.fields.Set(routeRecipesField)
	case "u":
		// Clearing drops the whole Selection: recipe, enabled set, and the
		// values supplied under it. Backspace does not do this: it walks back a
		// column everywhere else, and one key with two meanings on one screen
		// is how a Selection gets cleared by accident.
		if entry, ok := m.selectedCapture(); ok {
			delete(m.selections, entry.path)
		}
	}
	m.pendingGG = false
	m.refresh()
	return m, nil
}

func (m routeModel) updateRecipes(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.recipes.Move(-1)
	case "down", "j":
		m.recipes.Move(1)
	case "G", "end":
		m.recipes.Last()
	case "g":
		if m.pendingGG {
			m.recipes.First()
			m.pendingGG = false
			m.refresh()
			return m, nil
		}
		m.pendingGG = true
		return m, nil
	case "enter":
		m.chooseRecipe()
	case "esc", "backspace", "delete":
		m.fields.Set(routeCapturesField)
	}
	m.pendingGG = false
	m.refresh()
	return m, nil
}

// chooseRecipe records the focused Recipe with its default Action set. Values
// already supplied are kept: enrichment is keyed by field, not by Recipe.
func (m *routeModel) chooseRecipe() {
	entry, ok := m.selectedCapture()
	if !ok {
		return
	}
	recipe, ok := m.focusedCandidate()
	if !ok {
		return
	}
	selection := organizer.NewSelection(recipe)
	if previous, ok := m.selections[entry.path]; ok {
		selection.Enrichment = previous.Enrichment
	}
	m.selections[entry.path] = selection
	m.fields.Set(routeActionsField)
}

func (m routeModel) updateActions(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.actions.Move(-1)
	case "down", "j":
		m.actions.Move(1)
	case "G", "end":
		m.actions.Last()
	case " ", "space":
		m.toggleAction()
	case "enter":
		m.fields.Set(routeFieldsField)
	case "esc", "backspace", "delete":
		m.fields.Set(routeRecipesField)
	}
	m.pendingGG = false
	m.refresh()
	return m, nil
}

// toggleAction enables or disables the focused Action for this Capture only.
// A disabled Action leaves the plan and takes its requirements with it.
func (m *routeModel) toggleAction() {
	selection, _, ok := m.currentSelection()
	if !ok {
		return
	}
	if id, ok := m.focusedAction(); ok {
		selection.Toggle(id)
	}
}

func (m routeModel) updateFields(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.moveFieldCursor(-1)
	case "down", "j":
		m.moveFieldCursor(1)
	case "enter":
		return m.beginEdit()
	case "esc", "backspace", "delete":
		m.fields.Set(routeActionsField)
	}
	m.pendingGG = false
	m.refresh()
	return m, nil
}

func (m *routeModel) moveFieldCursor(delta int) {
	if len(m.fieldRows) == 0 {
		return
	}
	m.fieldIndex = min(len(m.fieldRows)-1, max(0, m.fieldIndex+delta))
}

func (m routeModel) beginEdit() (tea.Model, tea.Cmd) {
	row, ok := m.focusedFieldRow()
	if !ok || !row.editable {
		return m, nil
	}
	if row.requirement.Input == organizer.InputMultiline {
		m.editingNote = true
		m.note.SetValue(row.value)
		m.note.SetWidth(max(20, min(72, m.width-8)))
		m.note.SetHeight(max(3, min(10, m.height-8)))
		m.note.CursorEnd()
		m.note.Focus()
		return m, textarea.Blink
	}
	m.editing = true
	m.editor.SetValue(row.value)
	m.editor.CursorEnd()
	m.editor.Focus()
	return m, textinput.Blink
}

func (m routeModel) updateEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancelEdit()
		m.refresh()
		return m, nil
	case "enter":
		// Committing keeps focus on the row, so a value can be revised without
		// walking back to it.
		if selection, _, ok := m.currentSelection(); ok {
			if row, ok := m.focusedFieldRow(); ok {
				selection.Set(row.requirement.Field, strings.TrimSpace(m.editor.Value()))
			}
		}
		m.cancelEdit()
		m.refresh()
		return m, nil
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

// updateNote drives the multiline overlay. Enter inserts a newline there, so
// the edit is committed with ctrl+s and abandoned with Esc.
func (m routeModel) updateNote(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancelEdit()
		m.refresh()
		return m, nil
	case "ctrl+s":
		if selection, _, ok := m.currentSelection(); ok {
			if row, ok := m.focusedFieldRow(); ok {
				selection.Set(row.requirement.Field, strings.TrimSpace(m.note.Value()))
			}
		}
		m.cancelEdit()
		m.refresh()
		return m, nil
	}
	var cmd tea.Cmd
	m.note, cmd = m.note.Update(msg)
	return m, cmd
}

func (m *routeModel) cancelEdit() {
	m.editing = false
	m.editor.Blur()
	m.editor.SetValue("")
	m.editingNote = false
	m.note.Blur()
	m.note.SetValue("")
}

// run builds the plan for the selected Capture and shows it. No Action is
// implemented, so nothing is written outside the Capture directory; what is
// written is the organizer's own record, marking the Capture handled and moving
// it below the divider. A blocked plan is shown but not recorded: a Capture is
// handled only once its plan could actually run.
func (m routeModel) run() (tea.Model, tea.Cmd) {
	entry, ok := m.selectedCapture()
	if !ok {
		return m, nil
	}
	selection, recipe, ok := m.currentSelection()
	if !ok {
		return m, nil
	}
	ctx, _ := m.context()
	enabled := selection.EnabledActions(recipe)
	m.runPlans = organizer.Build(ctx, recipe, enabled)
	m.runContext = ctx
	m.runCapture = entry.name + "  ·  " + recipe.Name
	m.runError = ""
	m.running = true
	if organizer.Ready(ctx, recipe, enabled) {
		run := organizer.NewRun(recipe, selection, m.runPlans, time.Now())
		record, err := organizer.AppendRun(entry.path, run)
		if err != nil {
			m.runError = err.Error()
		} else {
			m.markOrganized(entry.path, record)
			m.advanceToNextPending(entry)
		}
	}
	// Focus returns to CAPTURES so the next Capture can be handled without
	// walking back through the columns.
	m.fields.Set(routeCapturesField)
	m.refresh()
	return m, nil
}

// markOrganized records the new state on the Capture itself, so Scan and Route
// read the same answer without either re-reading the directory.
func (m *routeModel) markOrganized(path string, record organizer.Record) {
	for index := range m.entries {
		if m.entries[index].path == path {
			m.entries[index].record = record
			m.entries[index].organized = true
			return
		}
	}
}

// advanceToNextPending moves the cursor to the first Capture still to handle,
// preferring the one after the Capture just organized so a run of Captures is
// worked through in order.
func (m *routeModel) advanceToNextPending(organized captureEntry) {
	after := false
	var first string
	for _, entry := range m.entries {
		if entry.path == organized.path {
			after = true
			continue
		}
		if m.organized(entry) {
			continue
		}
		if after {
			m.captures.SetItems(nil)
			m.rebuildCaptureItems()
			m.captures.SelectID("capture:" + entry.path)
			return
		}
		if first == "" {
			first = entry.path
		}
	}
	m.rebuildCaptureItems()
	if first != "" {
		m.captures.SelectID("capture:" + first)
	}
}

func (m *routeModel) cycleFields(forward bool) {
	order := []string{routeCapturesField, routeRootField, routeRecipesField, routeActionsField, routeFieldsField}
	current := 0
	for index, id := range order {
		if id == m.fields.Current() {
			current = index
		}
	}
	step := 1
	if !forward {
		step = -1
	}
	m.fields.Set(order[(current+step+len(order))%len(order)])
}

func (m routeModel) selectedCapture() (captureEntry, bool) {
	item, ok := m.captures.Selected()
	if !ok {
		return captureEntry{}, false
	}
	for _, entry := range m.entries {
		if entry.path == strings.TrimPrefix(item.ID, "capture:") {
			return entry, true
		}
	}
	return captureEntry{}, false
}

// currentSelection returns the Selection and Recipe of the selected Capture.
// A Selection carries its state in maps, so the returned value shares the
// enabled set and the enrichment with the stored one and Toggle and Set on it
// are visible to the model.
func (m routeModel) currentSelection() (organizer.Selection, organizer.Recipe, bool) {
	entry, ok := m.selectedCapture()
	if !ok {
		return organizer.Selection{}, organizer.Recipe{}, false
	}
	selection, ok := m.selections[entry.path]
	if !ok {
		return organizer.Selection{}, organizer.Recipe{}, false
	}
	recipe, ok := m.recipeSet.Lookup(selection.Recipe)
	if !ok {
		return organizer.Selection{}, organizer.Recipe{}, false
	}
	return selection, recipe, true
}

// selectedRecipe is the Recipe chosen for the selected Capture, if any.
func (m routeModel) selectedRecipe() (organizer.Recipe, bool) {
	_, recipe, ok := m.currentSelection()
	return recipe, ok
}

// candidates are the Recipes offered for the selected Capture. Nothing is
// preselected: choosing among them is always the user's decision.
func (m routeModel) candidates() []organizer.Recipe {
	entry, ok := m.selectedCapture()
	if !ok {
		return nil
	}
	return m.recipeSet.Find(captureFor(entry))
}

func (m routeModel) focusedCandidate() (organizer.Recipe, bool) {
	item, ok := m.recipes.Selected()
	if !ok {
		return organizer.Recipe{}, false
	}
	id := organizer.RecipeID(strings.TrimPrefix(item.ID, "recipe:"))
	for _, recipe := range m.candidates() {
		if recipe.ID == id {
			return recipe, true
		}
	}
	return organizer.Recipe{}, false
}

func (m routeModel) focusedAction() (organizer.ActionID, bool) {
	item, ok := m.actions.Selected()
	if !ok {
		return "", false
	}
	return organizer.ActionID(strings.TrimPrefix(item.ID, "action:")), true
}

func (m routeModel) focusedFieldRow() (routeFieldRow, bool) {
	if m.fieldIndex < 0 || m.fieldIndex >= len(m.fieldRows) {
		return routeFieldRow{}, false
	}
	return m.fieldRows[m.fieldIndex], true
}

// context resolves the selected Capture together with what the user supplied.
func (m routeModel) context() (organizer.Context, bool) {
	entry, ok := m.selectedCapture()
	if !ok {
		return organizer.Context{}, false
	}
	var enrichment map[organizer.FieldID]any
	if selection, ok := m.selections[entry.path]; ok {
		enrichment = selection.Enrichment
	}
	return organizer.NewContext(captureFor(entry), enrichment), true
}

func captureFor(entry captureEntry) organizer.Capture {
	return organizer.Capture{Path: entry.path, Name: entry.name, Index: entry.index}
}

func (m *routeModel) pruneSelections() {
	known := make(map[string]bool, len(m.entries))
	for _, entry := range m.entries {
		known[entry.path] = true
	}
	for path := range m.selections {
		if !known[path] {
			delete(m.selections, path)
		}
	}
}

// readyCount summarizes the session: a Capture is ready when it has a Recipe,
// at least one enabled Action, and nothing missing. Everything else is blocked,
// including a Capture with no Recipe yet.
func (m routeModel) readyCount() (ready int, blocked int) {
	for _, entry := range m.entries {
		if entry.organized {
			ready++
			continue
		}
		selection, ok := m.selections[entry.path]
		if !ok {
			blocked++
			continue
		}
		recipe, ok := m.recipeSet.Lookup(selection.Recipe)
		if !ok {
			blocked++
			continue
		}
		ctx := organizer.NewContext(captureFor(entry), selection.Enrichment)
		if organizer.Ready(ctx, recipe, selection.EnabledActions(recipe)) {
			ready++
		} else {
			blocked++
		}
	}
	return ready, blocked
}

// routeCaptureLabel identifies a Capture by what organizing decisions are made
// on—when it was taken—rather than by its directory name, which carries no
// meaning for the operator. The folder path remains available in the status bar.
func routeCaptureLabel(entry captureEntry) string {
	if created := formatCaptureTime(entry.index.CreatedAt); created != "" {
		return created
	}
	return entry.name + string(os.PathSeparator)
}

// routeCaptureDetail is the second row of a Route Capture: what produced the
// Capture. Together the two rows carry more identity than one row of a quarter
// column can hold without truncating the timestamp.
func routeCaptureDetail(entry captureEntry) string {
	parts := make([]string, 0, 2)
	if app := entry.index.Source.App; app != "" {
		parts = append(parts, app)
	}
	if workflow := entry.index.Source.Workflow; workflow != "" {
		parts = append(parts, workflow)
	}
	return strings.Join(parts, " · ")
}

// formatCaptureTime renders an RFC 3339 createdAt as a compact local-to-the-
// Capture wall clock keeping its original offset: 2026-10-10 12:23:24 +11.
// Whole-hour offsets drop their minutes; UTC reads as Z. An unparseable value
// is shown verbatim so a malformed index is visible rather than hidden.
func formatCaptureTime(value string) string {
	if value == "" {
		return ""
	}
	created, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return created.Format("2006-01-02 15:04:05") + " " + captureOffset(created)
}

func captureOffset(created time.Time) string {
	_, seconds := created.Zone()
	if seconds == 0 {
		return "Z"
	}
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hours, minutes := seconds/3600, (seconds%3600)/60
	if minutes == 0 {
		return fmt.Sprintf("%s%d", sign, hours)
	}
	return fmt.Sprintf("%s%d:%02d", sign, hours, minutes)
}

// captureMarker distinguishes the three states a Capture can be in: no Recipe
// chosen, a Recipe whose plan is still blocked, and a plan ready to run.
func (m routeModel) captureMarker(entry captureEntry) string {
	if entry.organized {
		return "● "
	}
	selection, ok := m.selections[entry.path]
	if !ok {
		return "○ "
	}
	recipe, ok := m.recipeSet.Lookup(selection.Recipe)
	if !ok {
		return "○ "
	}
	ctx := organizer.NewContext(captureFor(entry), selection.Enrichment)
	if organizer.Ready(ctx, recipe, selection.EnabledActions(recipe)) {
		return "● "
	}
	return "◐ "
}

// organized reports whether the Capture carries an organizing record. The
// record is read with the Capture rather than by this session, so a Capture
// organized in an earlier session is recognised on load.
func (m routeModel) organized(entry captureEntry) bool { return entry.organized }

// orderedEntries puts the Captures still to handle first and the organized ones
// after them, each run keeping the load order. The list is one field with a
// rule through it rather than two fields, so the cursor walks from the last
// unhandled Capture into the handled ones without leaving the column.
func (m routeModel) orderedEntries() ([]captureEntry, int) {
	pending := make([]captureEntry, 0, len(m.entries))
	done := make([]captureEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		if m.organized(entry) {
			done = append(done, entry)
			continue
		}
		pending = append(pending, entry)
	}
	return append(pending, done...), len(pending)
}

func (m *routeModel) rebuildCaptureItems() {
	selected := ""
	if item, ok := m.captures.Selected(); ok {
		selected = item.ID
	}
	ordered, boundary := m.orderedEntries()
	items := make([]scrolllist.Item, 0, len(ordered))
	for _, entry := range ordered {
		detail := routeCaptureDetail(entry)
		if latest, ok := entry.record.Latest(); entry.organized && ok {
			// An organized Capture is labelled by its most recent pass, with a
			// count when there has been more than one.
			detail += "  → " + latest.RecipeName
			if len(entry.record.Runs) > 1 {
				detail += fmt.Sprintf(" ×%d", len(entry.record.Runs))
			}
		} else if selection, ok := m.selections[entry.path]; ok {
			if recipe, ok := m.recipeSet.Lookup(selection.Recipe); ok {
				detail += "  → " + recipe.Name
			}
		}
		if detail != "" {
			detail = "  " + detail
		}
		items = append(items, scrolllist.Item{
			ID:     "capture:" + entry.path,
			Label:  m.captureMarker(entry) + routeCaptureLabel(entry),
			Detail: detail,
		})
	}
	m.captures.SetItems(items)
	m.captures.SetDivider(boundary, "ORGANIZED")
	if selected != "" {
		m.captures.SelectID(selected)
	}
}

// rebuildRecipeItems refills the candidate list for the selected Capture. The
// list changes as the Capture cursor moves, because FindRecipes narrows by
// workflow and by what the Capture already carries.
func (m *routeModel) rebuildRecipeItems() {
	selected := ""
	if item, ok := m.recipes.Selected(); ok {
		selected = item.ID
	}
	candidates := m.candidates()
	items := make([]scrolllist.Item, 0, len(candidates))
	for _, recipe := range candidates {
		items = append(items, scrolllist.Item{ID: "recipe:" + string(recipe.ID), Label: recipe.Name})
	}
	m.recipes.SetItems(items)
	if selected == "" || !m.recipes.SelectID(selected) {
		if selection, _, ok := m.currentSelection(); ok {
			m.recipes.SelectID("recipe:" + string(selection.Recipe))
		}
	}
}

// rebuildActionItems shows the Action Plan of the chosen Recipe. Each row
// carries two independent markers: enabled, and whether that Action's
// requirements are met. A disabled Action shows neither readiness nor target,
// because it does not enter the plan at all.
func (m *routeModel) rebuildActionItems() {
	selected := ""
	if item, ok := m.actions.Selected(); ok {
		selected = item.ID
	}
	selection, recipe, ok := m.currentSelection()
	if !ok {
		m.actions.SetItems(nil)
		return
	}
	ctx, _ := m.context()
	plans := organizer.Build(ctx, recipe, selection.EnabledActions(recipe))
	byAction := make(map[organizer.ActionID]organizer.ActionPlan, len(plans))
	for _, plan := range plans {
		byAction[plan.Action] = plan
	}

	items := make([]scrolllist.Item, 0, len(recipe.Actions))
	for _, id := range recipe.Actions {
		def, ok := organizer.LookupAction(id)
		if !ok {
			continue
		}
		label, detail := "[ ]   "+def.Label, ""
		if plan, enabled := byAction[id]; enabled {
			marker := "○ "
			if plan.Ready() {
				marker = "● "
			}
			label = "[x] " + marker + def.Label
			detail = "      " + plan.Target
			if plan.Target == "" {
				detail = "      · target unresolved"
			}
		}
		items = append(items, scrolllist.Item{ID: "action:" + string(id), Label: label, Detail: detail})
	}
	m.actions.SetItems(items)
	if selected != "" {
		m.actions.SelectID(selected)
	}
}

// rebuildFieldRows lists what the enabled Actions ask for, deduplicated and
// split by state: the fields that already have a value, then the ones still to
// supply. A field two Actions both require appears once, so it is filled once
// and satisfies both; which Action needed it is answered by the ACTIONS column
// rather than by repeating the field here. Toggling an Action off takes the
// fields only it required out of both halves.
func (m *routeModel) rebuildFieldRows() {
	m.fieldRows = nil
	selection, recipe, ok := m.currentSelection()
	if !ok {
		m.fieldIndex = 0
		return
	}
	ctx, _ := m.context()
	resolved, missing := organizer.Fields(ctx, recipe, selection.EnabledActions(recipe))
	for _, state := range resolved {
		m.fieldRows = append(m.fieldRows, routeFieldRow{
			requirement: state.Requirement,
			value:       state.Value,
			editable:    !state.FromCapture,
		})
	}
	for _, state := range missing {
		m.fieldRows = append(m.fieldRows, routeFieldRow{
			requirement: state.Requirement,
			editable:    true,
			missing:     true,
		})
	}
	m.fieldIndex = min(max(0, len(m.fieldRows)-1), max(0, m.fieldIndex))
}

// missingStart is the row the MISSING rule is drawn above, or -1 when nothing
// is missing.
func (m routeModel) missingStart() int {
	for index, row := range m.fieldRows {
		if row.missing {
			return index
		}
	}
	return -1
}

// columnWidths splits the workspace into four equal columns. Remainder cells
// are handed to the leftmost columns so the four widths never differ by more
// than one cell and always sum to the available width.
func (m routeModel) columnWidths() [4]int {
	available := max(4, m.width-3*columnGutter)
	base := available / 4
	extra := available % 4
	var widths [4]int
	for i := range widths {
		widths[i] = base
		if i < extra {
			widths[i]++
		}
	}
	return widths
}

func (m *routeModel) resizeComponents() {
	if m.width < 72 || m.height < 8 {
		return
	}
	widths := m.columnWidths()
	m.captures.SetSize(max(1, widths[0]-4), max(1, m.captureListHeight()-2))
	m.recipes.SetSize(max(1, widths[1]-4), max(1, m.recipeListHeight()))
	m.actions.SetSize(max(1, widths[2]-4), max(1, m.actionListHeight()))
	m.editor.Width = max(1, widths[3]-6)
	m.setFieldBounds()
	m.refresh()
}

func (m *routeModel) setFieldBounds() {
	widths := m.columnWidths()
	listHeight := m.captureListHeight()
	m.fields.SetBounds(routeCapturesField, datafield.Bounds{X: 0, Y: 0, Width: widths[0], Height: listHeight})
	m.fields.SetBounds(routeRootField, datafield.Bounds{X: 0, Y: listHeight, Width: widths[0], Height: rootHeight})
	x := widths[0] + columnGutter
	for i, id := range [3]string{routeRecipesField, routeActionsField, routeFieldsField} {
		width := widths[i+1]
		m.fields.SetBounds(id, datafield.Bounds{X: x, Y: 0, Width: width, Height: m.height})
		x += width + columnGutter
	}
}

// refresh rebuilds each column from the one to its left, so a move in CAPTURES
// re-narrows the candidates and a toggle in ACTIONS re-computes the plan.
func (m *routeModel) refresh() {
	m.rebuildCaptureItems()
	m.rebuildRecipeItems()
	m.rebuildActionItems()
	m.rebuildFieldRows()
}

func (m routeModel) leftColumn(width int) string {
	listHeight := m.captureListHeight()
	captures := m.captures
	captures.SetSize(max(1, width-4), max(1, listHeight-2))
	content := fitHeight(captures.View(m.fields.Current() == routeCapturesField, scanTitleStyle, scanMutedStyle), listHeight-2, width-4)
	if m.loadError != "" {
		content = fitHeight(scanMutedStyle.Render("! "+m.loadError), listHeight-2, width-4)
	} else if len(m.entries) == 0 {
		content = fitHeight(scanMutedStyle.Render("· No captures found"), listHeight-2, width-4)
	}
	list := fieldset.ViewFocused("CAPTURES", content, width, m.fields.Current() == routeCapturesField)
	return list + "\n" + m.rootControl.Fieldset(width, m.fields.Current() == routeRootField)
}

// captureListHeight reserves the compact CAPTURE ROOT control at the bottom of
// the left column, exactly as Scan does.
func (m routeModel) captureListHeight() int { return m.height - rootHeight }

func (m routeModel) recipesColumn(width int) string {
	focused := m.fields.Current() == routeRecipesField
	inner := max(1, width-4)
	recipes := m.recipes
	recipes.SetSize(inner, max(1, m.recipeListHeight()))
	content := recipes.View(focused, scanTitleStyle, scanMutedStyle)
	if _, ok := m.selectedCapture(); !ok {
		content = scanMutedStyle.Render("· Select a capture")
	} else if len(m.candidates()) == 0 {
		content = scanMutedStyle.Render("· No recipe matches this capture")
	}
	body := fitHeight(content, m.recipeListHeight(), inner) + "\n" + m.recipeDetail(inner)
	return fieldset.ViewFocused("RECIPES", fitHeight(body, m.height-2, inner), width, focused)
}

func (m routeModel) actionsColumn(width int) string {
	focused := m.fields.Current() == routeActionsField
	inner := max(1, width-4)
	actions := m.actions
	actions.SetSize(inner, max(1, m.actionListHeight()))
	content := actions.View(focused, scanTitleStyle, scanMutedStyle)
	if _, _, ok := m.currentSelection(); !ok {
		content = scanMutedStyle.Render("· Choose a recipe")
	}
	body := fitHeight(content, m.actionListHeight(), inner) + "\n" + m.actionDetail(inner)
	return fieldset.ViewFocused("ACTIONS", fitHeight(body, m.height-2, inner), width, focused)
}

// routeDetailHeight is fixed so a list does not resize as the cursor moves
// between entries that describe themselves at different lengths. RECIPES and
// ACTIONS share it, so their rules line up across the two columns.
const routeDetailHeight = 10

// routeDetailKeyWidth is the label column of a detail pane's single-value rows.
// Labels are quiet; values carry the normal foreground, so the eye lands on the
// content rather than on the scaffolding.
const routeDetailKeyWidth = 8

func (m routeModel) actionListHeight() int { return max(1, m.height-2-routeDetailHeight) }
func (m routeModel) recipeListHeight() int { return max(1, m.height-2-routeDetailHeight) }

// actionDetail describes the Action under the cursor: what it needs of this
// Capture, and what it will do outside it. The needs are here rather than in
// FIELDS because FIELDS lists the union across enabled Actions; this is where
// a field is attributed to the Action that asked for it. Fields are named by
// the same label FIELDS uses, so one screen does not call one thing two names.
func (m routeModel) actionDetail(width int) string {
	id, ok := m.focusedAction()
	if !ok {
		return ""
	}
	def, hasDef := organizer.LookupAction(id)
	if !hasDef {
		return ""
	}
	ctx, _ := m.context()
	lines := []string{divider.Labelled(string(id), width)}

	needs := organizer.Needs(ctx, id)
	if len(needs) == 0 {
		lines = append(lines, detailInline("Needs", scanMutedStyle.Render("nothing"), width))
	} else {
		lines = append(lines, detailHeader("Needs"))
		lines = append(lines, detailFields(needs, width)...)
	}
	if len(def.Effects) > 0 {
		lines = append(lines, detailHeader("Effects"))
		for _, effect := range def.Effects {
			lines = append(lines, detailParagraph(effect, width)...)
		}
	}
	return strings.Join(lines, "\n")
}

// recipeDetail describes the Recipe under the cursor before it is chosen,
// because choosing one resets the enabled Action set and should not be the way
// to find out what it does.
func (m routeModel) recipeDetail(width int) string {
	recipe, ok := m.focusedCandidate()
	if !ok {
		return ""
	}
	lines := []string{divider.Labelled(string(recipe.ID), width)}
	labels := make([]string, 0, len(recipe.Actions))
	for _, id := range recipe.Actions {
		if def, ok := organizer.LookupAction(id); ok {
			labels = append(labels, def.Label)
			continue
		}
		labels = append(labels, string(id)+" (unknown)")
	}
	lines = append(lines, detailHeader("Runs"))
	for _, label := range labels {
		lines = append(lines, detailItem(label, width))
	}
	if len(recipe.Fields) > 0 {
		lines = append(lines, detailHeader("Asks"))
		for _, field := range recipe.Fields {
			lines = append(lines, detailItem(requirementLabel(field), width))
		}
	}
	lines = append(lines, detailInline("Matches", workflowSummary(recipe), width))
	lines = append(lines, detailInline("Source", recipeSource(recipe), width))
	return strings.Join(lines, "\n")
}

func workflowSummary(recipe organizer.Recipe) string {
	if len(recipe.Match.Workflows) == 0 {
		return "any workflow"
	}
	return strings.Join(recipe.Match.Workflows, ", ")
}

// recipeSource names the file a Recipe came from by its filename: the directory
// is the same for every one of them and would crowd out the name.
func recipeSource(recipe organizer.Recipe) string {
	if recipe.Source == "" || recipe.Source == organizer.BuiltinSource {
		return organizer.BuiltinSource
	}
	return filepath.Base(recipe.Source)
}

func requirementLabel(requirement organizer.FieldRequirement) string {
	if requirement.Required {
		return requirement.Label + "*"
	}
	return requirement.Label
}

// A detail pane is written as headed sections rather than as a hanging key
// column: the key column costs width the content needs, and at a quarter of the
// screen that showed up as truncated field names. A header on its own line
// gives every item the full column.

// detailHeader names a group of items.
func detailHeader(text string) string { return scanMutedStyle.Render(text) }

// detailItem is one line under a header.
func detailItem(text string, width int) string {
	return truncateStyled("  "+truncate(text, max(1, width-2)), width)
}

// detailFields lists requirements as two aligned columns under a header. The
// name column is sized to the longest name present rather than fixed, so a
// short list is not padded out to fit one it does not contain. A value is
// clipped rather than wrapped: it is a datum, not a sentence, and FIELDS
// carries it in full a column away.
func detailFields(states []organizer.FieldState, width int) []string {
	nameWidth := 0
	for _, state := range states {
		nameWidth = max(nameWidth, lipgloss.Width(requirementLabel(state.Requirement)))
	}
	nameWidth = min(nameWidth, max(4, width/2))
	lines := make([]string, 0, len(states))
	for _, state := range states {
		name := fmt.Sprintf("%-*s", nameWidth, truncate(requirementLabel(state.Requirement), nameWidth))
		value := singleLine(state.Value)
		if value == "" {
			value = scanMutedStyle.Render("· missing")
		} else {
			value = truncate(value, max(1, width-nameWidth-4))
		}
		lines = append(lines, truncateStyled("  "+scanMutedStyle.Render(name)+"  "+value, width))
	}
	return lines
}

// detailParagraph is one sentence under a header, wrapped with its continuation
// indented further than the first line, so a wrapped effect cannot be mistaken
// for the next one.
func detailParagraph(text string, width int) []string {
	wrapped := strings.Split(lipgloss.NewStyle().Width(max(1, width-4)).Render(text), "\n")
	lines := make([]string, 0, len(wrapped))
	for index, part := range wrapped {
		indent := "  "
		if index > 0 {
			indent = "    "
		}
		lines = append(lines, truncateStyled(indent+strings.TrimRight(part, " "), width))
	}
	return lines
}

// detailInline is a single value small enough to sit beside its key, where a
// header of its own would waste a row.
func detailInline(key, value string, width int) string {
	row := scanMutedStyle.Render(fmt.Sprintf("%-*s", routeDetailKeyWidth, key)) + value
	return truncateStyled(row, width)
}

func (m routeModel) fieldsColumn(width int) string {
	focused := m.fields.Current() == routeFieldsField
	inner := max(1, width-4)
	content := m.fieldsContent(inner)
	if _, _, ok := m.currentSelection(); !ok {
		content = scanMutedStyle.Render("· Choose a recipe")
	} else if len(m.fieldRows) == 0 {
		content = scanMutedStyle.Render("· This action needs nothing")
	}
	return fieldset.ViewFocused("FIELDS", fitHeight(content, m.height-2, inner), width, focused)
}

// fieldsContent renders one requirement per row: its label, then the resolved
// value muted when it comes from the Capture, the supplied value when the user
// gave one, and an empty slot when it is still missing. A required field is
// marked so the blockage is visible without a separate missing-field count.
func (m routeModel) fieldsContent(width int) string {
	focused := m.fields.Current() == routeFieldsField
	missingAt := m.missingStart()
	lines := make([]string, 0, len(m.fieldRows)+1)
	for index, row := range m.fieldRows {
		if index == missingAt {
			lines = append(lines, divider.Labelled("MISSING", width))
		}
		selected := focused && index == m.fieldIndex
		editing := m.editing && index == m.fieldIndex
		label := fmt.Sprintf("%-*s", routePropertyKeyWidth, truncate(requirementLabel(row.requirement), routePropertyKeyWidth-1))
		value := row.value
		if editing {
			value = m.editor.View()
		} else if value == "" {
			value = "___"
		}

		if selected && !editing {
			// The row under the cursor is marked the way every other list on
			// the screen marks one, rather than by a character the muted style
			// then paints over.
			row := "› " + label + value
			lines = append(lines, scrolllist.SelectedRowStyle().Width(width).Render(truncate(row, width)))
			continue
		}
		cursor := "  "
		if selected {
			cursor = "› "
		}
		// The label is scaffolding and the value is the content, so only the
		// label is muted by default; a value the Capture owns is muted too,
		// which is what marks it read-only.
		line := cursor + scanMutedStyle.Render(label)
		if row.editable {
			line += value
		} else {
			line += scanMutedStyle.Render(value)
		}
		lines = append(lines, truncateStyled(line, width))
	}
	if missingAt < 0 && len(m.fieldRows) > 0 {
		lines = append(lines, "", scanMutedStyle.Render("· Nothing missing"))
	}
	return strings.Join(lines, "\n")
}

// truncate clips to width terminal cells, not to a count of runes: a styled
// string carries escape sequences that occupy no cells, and a wide rune
// occupies two, so counting runes clips a coloured or CJK line far too early.
func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

// truncateStyled clips a rendered line to width. A styled line carries escape
// sequences, so it is measured with lipgloss rather than by rune count.
func truncateStyled(line string, width int) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}
