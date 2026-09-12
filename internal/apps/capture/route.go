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
	"dgs-toolbox/internal/tui/pageactions"
	"dgs-toolbox/internal/tui/scrolllist"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The run dialog and the Run button share the page-action language: a solid
// fill marks something that acts, as against the bordered fields that hold
// data. Colours are the shell's, so the dialog does not become its own theme.
var (
	runAccent    = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	runPanel     = lipgloss.AdaptiveColor{Light: "#DDF3F0", Dark: "#173F3B"}
	runQuietFill = lipgloss.AdaptiveColor{Light: "#EEF2F4", Dark: "#303747"}

	runHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(runAccent).Background(runQuietFill)
	runActionStyle  = lipgloss.NewStyle().Bold(true).Foreground(runAccent)
	runGroupStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"})
	runFailedStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"})
	runPrimaryStyle = lipgloss.NewStyle().Bold(true).Foreground(runPanel).Background(runAccent).Padding(0, 1)
	runQuietStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}).
			Background(runQuietFill).Padding(0, 1)
)

const (
	routeCapturesField    = "route-captures"
	routeRootField        = "route-root"
	routeRecipesField     = "route-recipes"
	routeActionsField     = "route-actions"
	routeFieldsField      = "route-fields"
	routeAttachmentsField = "route-attachments"
	routePropertyKeyWidth = 12
)

// routeModel is the Route session: it organizes each scanned Capture by
// choosing a Recipe, enabling the Actions to run, and supplying whatever those
// Actions still need. Every domain judgement belongs to the organizer package;
// this model calls Set.Find, MissingFields, and Build, and draws the result.
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
	// runResults is what became of the plan once it was confirmed, kept so the
	// dialog can report a failure instead of closing over it.
	runResults []organizer.Result
	// runAdvance is the Capture a held-open dialog will move on from once it is
	// closed. A run that wrote everything it planned closes on its own; one
	// that skipped an Action stays open to say so, and the move to the next
	// Capture waits for the reader to have read it.
	runAdvance *captureEntry
	// settings are where the Actions write, from the configuration.
	settings organizer.Settings
	// recipeSet is the Set this session offers: the built-ins with whatever the
	// configured Recipe directory layered over them.
	recipeSet organizer.Set
	// attachments is the Capture's own files, listed below RECIPES and ACTIONS
	// with the details of the one under the cursor beside them. Organizing a
	// Capture is deciding what to do with what it holds, and until now that
	// could only be read one tab away in Scan: the columns above say what will
	// be written and this says what is being written about.
	attachments      scrolllist.Model
	attachmentProps  []property
	attachmentPath   string
	attachmentError  string
	attachmentLoaded uint64
	fields           datafield.Navigator
	loadError        string
	pendingGG        bool
	lastClick        routeClick
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
	// parameter marks a row that says how an Action behaves rather than what it
	// needs: a default answers for almost every Capture, and this is where the
	// occasional one is given something else. Overridden marks a value given
	// for this Capture rather than inherited.
	parameter  bool
	action     organizer.ActionID
	name       string
	overridden bool
}

func newRouteModel(root, indexFile string) routeModel {
	return newRouteModelWithRecipes(root, indexFile, organizer.Set{})
}

func newRouteModelWithRecipes(root, indexFile string, set organizer.Set) routeModel {
	return newRouteModelWithSettings(root, indexFile, set, organizer.DefaultSettings())
}

func newRouteModelWithSettings(root, indexFile string, set organizer.Set, settings organizer.Settings) routeModel {
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
		attachments: scrolllist.New(),
		selections:  make(map[string]organizer.Selection),
		recipeSet:   set,
		settings:    settings,
		editor:      editor,
		note:        note,
		fields: datafield.New(
			datafield.Field{ID: routeCapturesField, Row: 0, Col: 0},
			datafield.Field{ID: routeRootField, Row: 1, Col: 0},
			datafield.Field{ID: routeRecipesField, Row: 0, Col: 1},
			datafield.Field{ID: routeActionsField, Row: 0, Col: 2},
			datafield.Field{ID: routeFieldsField, Row: 0, Col: 3},
			datafield.Field{ID: routeAttachmentsField, Row: 1, Col: 1},
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
	case filePropertiesLoadedMsg:
		// A late answer for a file the cursor has already left is dropped:
		// the pane belongs to the row that is selected now.
		if msg.path != m.attachmentPath || msg.requestID != m.attachmentLoaded {
			return m, nil
		}
		m.attachmentProps = msg.properties
		m.attachmentError = ""
		if msg.err != nil {
			m.attachmentError = msg.err.Error()
		}
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
		return m, m.loadAttachment()
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
	// The page action is below the workspace and belongs to no field, so it is
	// tested before the fields are. A button is pressed rather than selected:
	// one click acts.
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.runButtonHit(msg.X, msg.Y) {
		if m.editing || m.editingNote || m.running {
			return m, nil
		}
		return m.run()
	}
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
	case routeAttachmentsField:
		if !m.attachments.SelectRow(msg.Y - m.middleHeight() - 1) {
			return m, nil
		}
		return m, m.loadAttachment()
	}
	return m, nil
}

// isDoubleClick reports whether this press pairs with the previous one: the
// same row of the same field, soon enough after it.
func (m routeModel) isDoubleClick(field string, row int) bool {
	return pairsWithLastClick(m.lastClick, field, row)
}

// pairsWithLastClick is the rule itself, shared by the two workspaces: a double
// click means the same thing in Scan as it does in Route, and two copies of the
// rule would eventually stop meaning the same thing.
func pairsWithLastClick(last routeClick, field string, row int) bool {
	if last.field != field || last.row != row {
		return false
	}
	return !last.at.IsZero() && time.Since(last.at) <= doubleClickWindow
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
	case routeAttachmentsField:
		m.attachments.Scroll(delta)
		m.refresh()
		return m, m.loadAttachment()
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
	middle := lipgloss.JoinHorizontal(lipgloss.Top, m.recipesColumn(widths[1]), " ", m.actionsColumn(widths[2]))
	middle += "\n" + m.attachmentsPane(widths[1]+columnGutter+widths[2])
	workspace := lipgloss.JoinHorizontal(lipgloss.Top,
		m.leftColumn(widths[0]), " ",
		middle, " ",
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

// runOverlay is where a reader agrees to what will happen, so it is written to
// be read rather than to be complete: the Capture and Recipe as a heading, one
// block per Action with its name in the accent, and its details as two aligned
// columns — what it writes, how it is set, what it will do, and the values it
// carries. Groups rather than a flat list, because the four answer different
// questions and a reader is usually checking one of them.
func (m routeModel) runOverlay() string {
	width := max(40, min(104, m.width-8))
	inner := max(10, width-4)
	ctx := m.runContext

	lines := []string{runHeaderStyle.Width(inner).Render(" " + m.runCapture)}
	if m.runError != "" {
		lines = append(lines, "", runFailedStyle.Render("! "+m.runError))
	}
	for index, plan := range m.runPlans {
		lines = append(lines, "")
		lines = append(lines, m.runActionBlock(ctx, index, plan, inner)...)
	}
	if len(m.runPlans) == 0 {
		lines = append(lines, "", scanMutedStyle.Render("· No action is enabled, so there is nothing to run"))
	}

	lines = append(lines, "")
	lines = append(lines, detailParagraph(m.runNote(), inner)...)
	lines = append(lines, "", m.runButtons())
	return fieldset.View("RUN", strings.Join(lines, "\n"), width)
}

// runActionBlock is one Action of the plan: its outcome, its name, and its
// details grouped by the question they answer.
func (m routeModel) runActionBlock(ctx organizer.Context, index int, plan organizer.ActionPlan, width int) []string {
	marker, style := "●", runActionStyle
	if !plan.Ready() {
		marker = "○"
	}
	if outcome, ok := m.resultMarker(plan.Action); ok {
		marker = outcome
		if outcome == "✗" {
			style = runFailedStyle
		}
	}
	lines := []string{style.Render(fmt.Sprintf("%s %d %s", marker, index+1, plan.Action))}

	target := plan.Target
	if target == "" {
		target = "· unresolved"
	}
	lines = append(lines, dialogGroup("Writes", [][2]string{{"", target}}, width)...)
	def, ok := organizer.LookupAction(plan.Action)
	if !ok {
		return lines
	}

	settings := make([][2]string, 0, len(def.Parameters))
	for _, parameter := range def.Parameters {
		settings = append(settings, [2]string{parameter.Label, ctx.Parameter(plan.Action, parameter.Name)})
	}
	lines = append(lines, dialogGroup("Set", settings, width)...)

	values := make([][2]string, 0, len(def.Required))
	for _, req := range def.Required {
		value := ctx.String(req.Field)
		if value == "" {
			value = "· missing"
		}
		values = append(values, [2]string{req.Label, singleLine(value)})
	}
	lines = append(lines, dialogGroup("With", values, width)...)

	effects := make([][2]string, 0, len(def.Effects))
	for _, effect := range def.Effects {
		effects = append(effects, [2]string{"", "· " + effect})
	}
	return append(lines, dialogGroup("Will", effects, width)...)
}

// runNote is the one sentence that says where the reader stands.
func (m routeModel) runNote() string {
	switch {
	case !m.runReady():
		return "Blocked: fill the missing values before running this."
	case len(organizer.Unimplemented(m.runPlans)) > 0:
		return "Not implemented yet: " + actionList(organizer.Unimplemented(m.runPlans)) +
			". Disable it, or choose a recipe without it."
	case m.runError != "":
		return "Nothing was recorded."
	case len(m.runResults) > 0:
		return m.resultSummary()
	}
	return "Nothing has happened yet."
}

// runButtons are filled rather than written as keys, because they are what the
// dialog is for. The affirmative one is quiet until it can be taken.
func (m routeModel) runButtons() string {
	run, cancel := runQuietStyle.Render("↵ Run"), runQuietStyle.Render("esc Close")
	if m.runRunnable() && len(m.runResults) == 0 {
		run = runPrimaryStyle.Render("↵ Run")
		cancel = runQuietStyle.Render("esc Cancel")
		return run + "  " + cancel
	}
	return cancel
}

// resultSummary says what became of the plan, naming the Action that stopped it
// rather than reporting a count: which one failed is the only useful part.
func (m routeModel) resultSummary() string {
	for _, result := range m.runResults {
		if result.Err != nil {
			return "! " + string(result.Action) + ": " + result.Err.Error()
		}
	}
	written, skipped := 0, 0
	for _, result := range m.runResults {
		if result.Skipped {
			skipped++
			continue
		}
		written++
	}
	if skipped > 0 && written == 0 {
		// The reason is the Action's own words, so the dialog and the record
		// say the same thing rather than two summaries of it.
		return "Nothing was written: " + skipReason(m.runResults) + ". Delete that entry to have it written again."
	}
	if skipped > 0 {
		return fmt.Sprintf("Done: %d written, %d already there.", written, skipped)
	}
	return "Done."
}

// skipReason is why the run wrote nothing, taken from the first Action that
// had nothing to do. The first rather than all of them: a Recipe's Actions skip
// for the same reason — the Capture is already filed — and repeating it once
// per Action says nothing more.
func skipReason(results []organizer.Result) string {
	for _, result := range results {
		if result.Skipped && result.Reason != "" {
			return result.Reason
		}
	}
	return "this Capture is already in the note"
}

// resultMarker is how one Action's outcome is shown beside it in the dialog.
func (m routeModel) resultMarker(action organizer.ActionID) (string, bool) {
	for _, result := range m.runResults {
		if result.Action != action {
			continue
		}
		switch {
		case result.Err != nil:
			return "✗", true
		case result.Skipped:
			return "·", true
		default:
			return "✓", true
		}
	}
	return "", false
}

// runReady reports whether the plan just shown could actually run: every value
// it needs is present, and every Action it names is implemented.
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

// runRunnable reports whether confirming would do anything: a plan naming an
// Action that has only been declared is refused before it is offered, not after
// it has failed.
func (m routeModel) runRunnable() bool {
	return m.runReady() && len(organizer.Unimplemented(m.runPlans)) == 0
}

func actionList(actions []organizer.ActionID) string {
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		names = append(names, string(action))
	}
	return strings.Join(names, ", ")
}

// Column widths inside the run dialog: the group answers which question, the
// key names the item within it.
const (
	dialogIndent    = 4
	dialogGroupWide = 8
	dialogKeyWide   = 12
)

// dialogGroup renders one group of an Action's block: a label naming what the
// group answers, then its items as a second column. Grouping rather than a flat
// list of labels, because a reader is usually checking one question — where it
// writes, what it is set to, what it carries, what it will do — and a flat list
// makes them find it among the others.
func dialogGroup(group string, items [][2]string, width int) []string {
	// A group whose items have no names of their own does not reserve the
	// column for them: an empty column is a gap the eye has to cross.
	keyed := false
	for _, item := range items {
		if item[0] != "" {
			keyed = true
		}
	}
	var lines []string
	for index, item := range items {
		label := ""
		if index == 0 {
			label = group
		}
		lines = append(lines, dialogRow(label, item[0], item[1], keyed, width)...)
	}
	return lines
}

// dialogRow is one line of a group, wrapped under a hanging indent rather than
// clipped: the dialog is where a reader agrees to what will happen, and half a
// sentence is not something anyone can agree to.
func dialogRow(group, key, value string, keyed bool, width int) []string {
	prefix := strings.Repeat(" ", dialogIndent) +
		fmt.Sprintf("%-*s", dialogGroupWide, truncate(group, dialogGroupWide-1))
	if keyed {
		prefix += fmt.Sprintf("%-*s", dialogKeyWide, truncate(key, dialogKeyWide-1))
	}
	pad := strings.Repeat(" ", lipgloss.Width(prefix))
	// A bulleted line indents its continuation under the text, not under the
	// bullet, so a wrapped sentence is not read as the next one.
	hanging := ""
	if strings.HasPrefix(value, "· ") {
		hanging = "  "
	}
	wrapped := strings.Split(lipgloss.NewStyle().Width(max(8, width-lipgloss.Width(prefix)-lipgloss.Width(hanging))).Render(value), "\n")
	lines := make([]string, 0, len(wrapped))
	for index, part := range wrapped {
		start := prefix
		if index > 0 {
			start = pad + hanging
		}
		lines = append(lines, runGroupStyle.Render(start)+strings.TrimRight(part, " "))
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
		case routeRecipesField, routeActionsField, routeFieldsField, routeAttachmentsField:
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
		if m.runRunnable() && len(m.runResults) == 0 {
			return tui.Status{Left: "RUN", Center: center, Right: "↵ Run  esc Cancel"}
		}
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
	case routeAttachmentsField:
		return tui.Status{Left: "ATTACHMENTS", Center: m.attachmentStatus(), Right: "↑/k ↓/j Move  esc/⌫ Back  alt+hjkl Focus"}
	default:
		return tui.Status{Left: "ROUTE", Center: center, Right: "tab Next  alt+hjkl Focus"}
	}
}

// attachmentStatus names the file under the cursor, since the pane shows its
// name without the directory it is in.
func (m routeModel) attachmentStatus() string {
	if attachment, ok := m.focusedAttachment(); ok {
		return attachment.path
	}
	return m.statusValue()
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
		switch key {
		case "enter", "y":
			return m.commitRun()
		case "esc", "q":
			m.running = false
			// A dialog held open to report a skip still owes the reader the
			// move it deferred, so closing it does what a clean run did
			// straight away.
			if organized := m.runAdvance; organized != nil {
				m.runAdvance = nil
				m.advanceToNextPending(*organized)
				m.fields.Set(routeCapturesField)
				m.refresh()
			}
		}
		return m, nil
	}
	// An open editor owns every key it is given: x is a letter in a note, not
	// the key that opens the run dialog, and the same goes for the refresh.
	if m.editingNote {
		return m.updateNote(msg)
	}
	if m.editing {
		return m.updateEditor(msg)
	}
	if key == "x" {
		return m.run()
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
	case routeAttachmentsField:
		return m.updateAttachments(key)
	}
	m.pendingGG = false
	return m, nil
}

// updateAttachments walks the Capture's own files. Enter and the back keys are
// not the progressive selection here: the pane is below that walk rather than
// in it, so Esc leaves it the way it came, back to the column above.
func (m routeModel) updateAttachments(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.attachments.Move(-1)
	case "down", "j":
		m.attachments.Move(1)
	case "G", "end":
		m.attachments.Last()
	case "g":
		if m.pendingGG {
			m.attachments.First()
			m.pendingGG = false
			return m, m.loadAttachment()
		}
		m.pendingGG = true
		return m, nil
	case "esc", "backspace", "delete":
		m.fields.Set(routeActionsField)
	}
	m.pendingGG = false
	cmd := m.loadAttachment()
	m.refresh()
	return m, cmd
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
	// The attachments below belong to the Capture under the cursor, so moving
	// that cursor re-aims them.
	return m, m.loadAttachment()
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
		m.commitEdit(strings.TrimSpace(m.editor.Value()))
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
		m.commitEdit(strings.TrimSpace(m.note.Value()))
		m.cancelEdit()
		m.refresh()
		return m, nil
	}
	var cmd tea.Cmd
	m.note, cmd = m.note.Update(msg)
	return m, cmd
}

// commitEdit writes an edited row back where it came from: a field is
// enrichment, a parameter is an override of how one Action behaves.
func (m *routeModel) commitEdit(value string) {
	selection, _, ok := m.currentSelection()
	if !ok {
		return
	}
	row, ok := m.focusedFieldRow()
	if !ok {
		return
	}
	if row.parameter {
		selection.SetParameter(row.action, row.name, value)
		return
	}
	selection.Set(row.requirement.Field, value)
}

func (m *routeModel) cancelEdit() {
	m.editing = false
	m.editor.Blur()
	m.editor.SetValue("")
	m.editingNote = false
	m.note.Blur()
	m.note.SetValue("")
}

// run builds the plan for the selected Capture and asks about it. Nothing
// happens yet: the dialog states what would be done, and only confirming it
// carries the plan out. The asymmetry decides this — confirming costs one
// keystroke each time, while a mistaken x would write into a vault and move
// Captures out of reach.
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
	m.runResults = nil
	m.runAdvance = nil
	m.running = true
	m.refresh()
	return m, nil
}

// commitRun carries out the plan the dialog is showing. No Action is
// implemented, so what is written is the organizer's own record, marking the
// Capture handled and moving it below the divider. A blocked plan cannot be
// confirmed: a Capture is handled only once its plan could actually run.
func (m routeModel) commitRun() (tea.Model, tea.Cmd) {
	if !m.runRunnable() {
		return m, nil
	}
	entry, ok := m.selectedCapture()
	if !ok {
		return m, nil
	}
	selection, recipe, ok := m.currentSelection()
	if !ok {
		return m, nil
	}
	// The Actions run first and the record says what happened, so a failure is
	// written down rather than reported only to whoever was watching.
	m.runResults = organizer.Execute(m.runContext, m.runPlans)
	run := organizer.NewRun(recipe, selection, m.runResults, time.Now())
	record, err := organizer.AppendRun(entry.path, run)
	if err != nil {
		m.runError = err.Error()
		m.refresh()
		return m, nil
	}
	m.markOrganized(entry.path, record)
	if !record.Organized() {
		// A pass that failed part way through leaves the Capture where it is,
		// with the dialog open on what went wrong.
		m.refresh()
		return m, nil
	}
	// An Action that found its entry already in the note wrote nothing, and a
	// dialog that closes on its own takes that news with it. The dialog is held
	// open instead, with the outcome beside each Action, so "already written"
	// is something the reader is told rather than something they discover in
	// the note later.
	if organizer.Skipped(m.runResults) {
		m.runAdvance = &entry
		m.refresh()
		return m, nil
	}
	m.advanceToNextPending(entry)
	m.running = false
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

// advanceToNextPending moves the cursor to the top of what is left to do. The
// Captures still to handle are the run above the rule, so the first of them is
// where the work continues — not the one that followed the Capture just
// organized, which leaves the cursor stranded below anything skipped earlier.
//
// A Capture no Recipe matches cannot be worked on, so it is passed over when
// choosing where to land: it would otherwise take the cursor after every run
// and have to be stepped past every time.
func (m *routeModel) advanceToNextPending(organized captureEntry) {
	m.rebuildCaptureItems()
	ordered, boundary := m.orderedEntries()
	pending := ordered[:boundary]

	for _, entry := range pending {
		if entry.path == organized.path {
			continue
		}
		if len(m.recipeSet.Find(captureFor(entry), m.settings)) == 0 {
			continue
		}
		m.captures.SelectID("capture:" + entry.path)
		return
	}
	// Everything left is unorganizable, so land on it anyway rather than
	// leaving the cursor on a Capture that has just moved below the rule.
	for _, entry := range pending {
		if entry.path != organized.path {
			m.captures.SelectID("capture:" + entry.path)
			return
		}
	}
}

// tabOrder is the fields Tab walks: the ones that have something to select
// right now. A column waiting on a decision that has not been made yet — the
// Recipes of no Capture, the Actions of no Recipe — holds nothing to select,
// and stopping there teaches the reader only that they are somewhere useless.
//
// Alt and the arrow keys still reach every column, because those are spatial:
// they mean "the field to the right", and that field is where it is whether or
// not it is ready.
func (m routeModel) tabOrder() []string {
	// The Capture Root is not in the ring. It is a setting rather than a step,
	// and a ring is walked to get through the work: putting a setting in it
	// costs a keystroke every time round for something touched once a session.
	// Alt and the arrows still reach it, since it is directly below CAPTURES.
	order := []string{routeCapturesField}
	if _, ok := m.selectedCapture(); ok && len(m.candidates()) > 0 {
		order = append(order, routeRecipesField)
	}
	if _, _, ok := m.currentSelection(); ok {
		order = append(order, routeActionsField, routeFieldsField)
	}
	// The attachments are in the ring as soon as the Capture has any, whether
	// or not a Recipe has been chosen: what a Capture holds is read before
	// deciding what to do with it, not after.
	if len(m.captureAttachments()) > 0 {
		order = append(order, routeAttachmentsField)
	}
	return order
}

func (m *routeModel) cycleFields(forward bool) {
	order := m.tabOrder()
	// Focus can sit in a column that has since been left behind — clearing a
	// Selection empties ACTIONS under the cursor — so a field that is not in
	// the order walks from the start rather than nowhere.
	current, found := 0, false
	for index, id := range order {
		if id == m.fields.Current() {
			current, found = index, true
		}
	}
	if !found {
		m.fields.Set(order[0])
		return
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
	return m.recipeSet.Find(captureFor(entry), m.settings)
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
	var parameters map[organizer.ActionID]map[string]string
	if selection, ok := m.selections[entry.path]; ok {
		enrichment, parameters = selection.Enrichment, selection.Parameters
	}
	return organizer.NewContext(captureFor(entry), enrichment).
		WithSettings(m.settings).
		WithParameters(parameters), true
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
		ctx := organizer.NewContext(captureFor(entry), selection.Enrichment).WithSettings(m.settings)
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
	if created := organizer.FormatTimestamp(entry.index.CreatedAt); created != "" {
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
	ctx := organizer.NewContext(captureFor(entry), selection.Enrichment).WithSettings(m.settings)
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
// list changes as the Capture cursor moves, because Set.Find narrows by
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
	// What is still to supply comes first: it is the work, and the cursor
	// lands on it without being moved there. What is already answered follows
	// as reference.
	for _, state := range missing {
		m.fieldRows = append(m.fieldRows, routeFieldRow{
			requirement: state.Requirement,
			editable:    true,
			missing:     true,
		})
	}
	for _, state := range resolved {
		m.fieldRows = append(m.fieldRows, routeFieldRow{
			requirement: state.Requirement,
			value:       state.Value,
			editable:    !state.FromCapture,
		})
	}
	// The knobs come last: they are pinned below the work and the reference
	// because almost every Capture leaves them alone.
	for _, state := range organizer.Parameters(ctx, recipe, selection.EnabledActions(recipe)) {
		m.fieldRows = append(m.fieldRows, routeFieldRow{
			requirement: organizer.FieldRequirement{Label: state.Definition.Label, Input: state.Definition.Input},
			value:       state.Value,
			editable:    true,
			parameter:   true,
			action:      state.Action,
			name:        state.Definition.Name,
			overridden:  state.Overridden,
		})
	}
	m.fieldIndex = min(max(0, len(m.fieldRows)-1), max(0, m.fieldIndex))
}

// routeFieldLine is one rendered line of the FIELDS column. row is the field it
// shows, or -1 for a rule and for the line that stands in for one: those take a
// line without being selectable, and mapping a click has to know the difference.
type routeFieldLine struct {
	text string
	row  int
}

// fieldLines lays the column out once, so what is drawn and what a click means
// cannot disagree.
func (m routeModel) fieldLines(width int) []routeFieldLine {
	var lines []routeFieldLine
	rule := func(label string) {
		lines = append(lines, routeFieldLine{text: divider.Labelled(label, width), row: -1})
	}
	if len(m.fieldRows) == 0 {
		lines = append(lines, routeFieldLine{text: scanMutedStyle.Render("· Choose a recipe"), row: -1})
	}
	section := ""
	for index, row := range m.fieldRows {
		switch {
		case row.parameter && section != "parameters":
			section = "parameters"
			rule("PARAMETERS")
		case !row.missing && !row.parameter && section != "resolved":
			section = "resolved"
			if index == 0 {
				lines = append(lines, routeFieldLine{text: scanMutedStyle.Render("· Nothing missing"), row: -1})
			}
			rule("RESOLVED")
		}
		for _, text := range m.fieldRowText(index, row, width) {
			lines = append(lines, routeFieldLine{text: text, row: index})
		}
	}
	return lines
}

func (m routeModel) fieldsContent(width int) string {
	texts := make([]string, 0, len(m.fieldRows)+3)
	for _, line := range m.fieldLines(width) {
		texts = append(texts, line.text)
	}
	return strings.Join(texts, "\n")
}

// runActionsHeight is what the page action takes from the column it sits under:
// a row of breathing space, the two-line button, and a row below it, so the
// control is not pressed into the corner of the screen.
const runActionsHeight = 4

// runButtonWidth is fixed rather than sized to its label, so the control does
// not move as the Recipe under it changes.
const runButtonWidth = 24

// fieldsHeight is what the FIELDS fieldset gets. The action takes its room from
// that column alone — the way the Capture Root control takes its room from the
// column above it — rather than shortening all four, which would leave three
// columns ending early for the sake of one control.
func (m routeModel) fieldsHeight() int { return max(4, m.height-runActionsHeight) }

// runActions renders the control that runs the plan, at the bottom right: the
// shared two-line filled button, so it reads as the same kind of thing as the
// page actions of other commands. It stays quiet until a Recipe has been
// chosen, because until then there is no plan to run.
func (m routeModel) runActions(width int) string {
	subtitle, ready := m.runReadiness()
	button := min(runButtonWidth, max(12, width-2))
	indent := strings.Repeat(" ", max(0, width-button-1))
	lines := []string{strings.Repeat(" ", max(0, width))}
	for _, line := range pageactions.Button("Run  x", subtitle, button, ready) {
		row := indent + line
		lines = append(lines, row+strings.Repeat(" ", max(0, width-lipgloss.Width(row))))
	}
	return strings.Join(lines, "\n") + "\n" + strings.Repeat(" ", max(0, width))
}

// runReadiness is what the button says and whether it is lit. It is filled only
// when pressing it would carry the plan out; anything still in the way leaves it
// quiet and says what that is, so the button answers "can I run this yet?"
// without the reader opening the dialog to find out.
func (m routeModel) runReadiness() (subtitle string, ready bool) {
	selection, recipe, chosen := m.currentSelection()
	if !chosen {
		return "Choose a recipe", false
	}
	ctx, ok := m.context()
	if !ok {
		return recipe.Name, false
	}
	enabled := selection.EnabledActions(recipe)
	if len(enabled) == 0 {
		return "No action enabled", false
	}
	if missing := organizer.MissingFields(ctx, recipe, enabled); len(missing) > 0 {
		if len(missing) == 1 {
			return "Missing " + missing[0].Label, false
		}
		return fmt.Sprintf("%d fields missing", len(missing)), false
	}
	if pending := organizer.Unimplemented(organizer.Build(ctx, recipe, enabled)); len(pending) > 0 {
		return "Not implemented", false
	}
	return recipe.Name, true
}

// runButtonHit reports whether a click landed on the button, which sits under
// the fourth column rather than across the screen.
func (m routeModel) runButtonHit(x, y int) bool {
	top := m.fieldsHeight() + 1
	if y < top || y > top+1 {
		return false
	}
	widths := m.columnWidths()
	column := widths[0] + widths[1] + widths[2] + 3*columnGutter
	button := min(runButtonWidth, max(12, widths[3]-2))
	start := column + max(0, widths[3]-button-1)
	return x >= start && x < start+button
}

// routeFieldValueLines is how far a value is allowed to fold. A value is worth
// reading in full, and a column of them is worth scanning: a note of twenty
// lines would push every other field off the screen, so a long one folds to
// here and says it was cut.
const routeFieldValueLines = 3

// fieldRowText draws one field. A row under the cursor is marked the way every
// other list on the screen marks one, rather than by a character the muted
// style then paints over.
//
// It returns the lines of one row rather than a line: a value wider than the
// column — a section written as a template, a note of more than a few words —
// folds instead of being clipped, because a value the reader cannot see is one
// they cannot check. A parameter puts its value on a line of its own: its name
// carries the Action it belongs to as well, and the two together leave nothing
// of the column for the value to sit in.
func (m routeModel) fieldRowText(index int, row routeFieldRow, width int) []string {
	focused := m.fields.Current() == routeFieldsField
	selected := focused && index == m.fieldIndex
	editing := m.editing && index == m.fieldIndex

	keyWidth := routePropertyKeyWidth
	label := requirementLabel(row.requirement)
	if row.parameter {
		// A knob is named by the Action it belongs to: FIELDS lists every
		// enabled Action's, and two of them may well share a name.
		label = shortAction(row.action) + " · " + row.requirement.Label
		keyWidth = 0
	} else {
		label = fmt.Sprintf("%-*s", keyWidth, truncate(label, keyWidth-1))
	}

	value := row.value
	if editing {
		// The editor draws its own line and carries a cursor, so it is left
		// alone: folding it would put the cursor somewhere it cannot be.
		value = m.editor.View()
	} else if value == "" {
		value = "___"
	}

	cursor := "  "
	if selected {
		cursor = "› "
	}
	paint := func(line string) string {
		if selected {
			return scrolllist.SelectedRowStyle().Width(width).Render(truncate(line, width))
		}
		return truncateStyled(line, width)
	}
	// The label is scaffolding and the value is the content, so only the label
	// is muted by default. A parameter left at its default is muted whole: it
	// is not something the reader has to do anything about.
	render := func(text string) string {
		if row.editable && (!row.parameter || row.overridden) {
			return text
		}
		return scanMutedStyle.Render(text)
	}

	if row.parameter {
		lines := []string{paint(cursor + scanMutedStyle.Render(truncate(label, width-2)))}
		if editing {
			return append(lines, paint("    "+value))
		}
		for _, part := range foldValue(value, max(8, width-4), routeFieldValueLines) {
			lines = append(lines, paint("    "+render(part)))
		}
		return lines
	}
	if editing {
		return []string{paint(cursor + scanMutedStyle.Render(label) + value)}
	}

	parts := foldValue(value, max(8, width-keyWidth-2), routeFieldValueLines)
	// A folded line continues under the value rather than under the label, so
	// the second line of one field is not read as another field's.
	indent := strings.Repeat(" ", keyWidth)
	lines := make([]string, 0, len(parts))
	for part := range parts {
		key, mark := indent, "  "
		if part == 0 {
			key, mark = label, cursor
		}
		lines = append(lines, paint(mark+scanMutedStyle.Render(key)+render(parts[part])))
	}
	return lines
}

// foldValue breaks a value into at most limit lines of width cells, marking the
// last one when there was more. Its own newlines break lines too: a note is
// written in lines, and running them together would misreport what it says.
func foldValue(value string, width, limit int) []string {
	var folded []string
	for _, paragraph := range strings.Split(value, "\n") {
		wrapped := lipgloss.NewStyle().Width(width).Render(paragraph)
		for _, line := range strings.Split(wrapped, "\n") {
			folded = append(folded, strings.TrimRight(line, " "))
		}
	}
	if len(folded) > limit {
		folded = folded[:limit]
		folded[limit-1] = truncate(folded[limit-1], max(1, width-1)) + "…"
	}
	return folded
}

// shortAction drops the namespace an Action shares with its siblings, which is
// the part that says nothing when they are listed together.
func shortAction(action organizer.ActionID) string {
	name := string(action)
	if index := strings.Index(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

// selectFieldRow moves the FIELDS cursor to the clicked line, which is not
// necessarily a field: the rules take lines of their own.
func (m *routeModel) selectFieldRow(y int) bool {
	lines := m.fieldLines(max(1, m.columnWidths()[3]-4))
	index := y - 1
	if index < 0 || index >= len(lines) || lines[index].row < 0 {
		return false
	}
	m.fieldIndex = lines[index].row
	return true
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
	m.attachments.SetSize(max(1, (widths[1]+columnGutter+widths[2]-4)/3), max(1, routeAttachmentsHeight-2))
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
	m.fields.SetBounds(routeAttachmentsField, datafield.Bounds{
		X: x, Y: m.middleHeight(), Width: widths[1] + columnGutter + widths[2], Height: routeAttachmentsHeight,
	})
	for i, id := range [3]string{routeRecipesField, routeActionsField, routeFieldsField} {
		width, height := widths[i+1], m.middleHeight()
		if id == routeFieldsField {
			// The action under this column is not part of the field, so a
			// click on it is not a click into the list.
			height = m.fieldsHeight()
		}
		m.fields.SetBounds(id, datafield.Bounds{X: x, Y: 0, Width: width, Height: height})
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
	m.rebuildAttachmentItems()
}

// routeAttachment is one file of the Capture as the pane lists it: the name it
// is stored under, and what the index says it is.
type routeAttachment struct {
	name string
	kind string
	path string
}

// captureAttachments are the Capture's own files, in manifest order. The index
// describes them and the scan has already dropped the ones that are not there
// or are not safe to name, so this lists what both agree on. The index file and
// the organizer's record are left out: they are what the other columns are
// about, not what the Capture holds.
func (m routeModel) captureAttachments() []routeAttachment {
	entry, ok := m.selectedCapture()
	if !ok {
		return nil
	}
	present := make(map[string]bool, len(entry.files))
	for _, file := range entry.files {
		present[file] = true
	}
	attachments := make([]routeAttachment, 0, len(entry.index.Attachments))
	for _, attachment := range entry.index.Attachments {
		if attachment.Name == "" || !present[attachment.Name] {
			continue
		}
		attachments = append(attachments, routeAttachment{
			name: attachment.Name,
			kind: attachment.Kind,
			path: filepath.Join(entry.path, attachment.Name),
		})
	}
	return attachments
}

func (m *routeModel) rebuildAttachmentItems() {
	selected := ""
	if item, ok := m.attachments.Selected(); ok {
		selected = item.ID
	}
	attachments := m.captureAttachments()
	items := make([]scrolllist.Item, 0, len(attachments))
	for _, attachment := range attachments {
		items = append(items, scrolllist.Item{ID: "attachment:" + attachment.path, Label: attachment.name})
	}
	m.attachments.SetItems(items)
	if selected == "" || !m.attachments.SelectID(selected) {
		m.attachments.First()
	}
}

func (m routeModel) focusedAttachment() (routeAttachment, bool) {
	item, ok := m.attachments.Selected()
	if !ok {
		return routeAttachment{}, false
	}
	path := strings.TrimPrefix(item.ID, "attachment:")
	for _, attachment := range m.captureAttachments() {
		if attachment.path == path {
			return attachment, true
		}
	}
	return routeAttachment{}, false
}

// loadAttachment reads the details of the file under the cursor, if they are
// not the ones already shown. Every request carries a generation, so a slow
// read cannot overwrite what a later selection put on screen.
func (m *routeModel) loadAttachment() tea.Cmd {
	attachment, ok := m.focusedAttachment()
	if !ok {
		m.attachmentPath, m.attachmentProps, m.attachmentError = "", nil, ""
		return nil
	}
	if attachment.path == m.attachmentPath {
		return nil
	}
	m.attachmentPath, m.attachmentProps, m.attachmentError = attachment.path, nil, ""
	m.attachmentLoaded++
	return loadFileProperties(attachment.path, m.attachmentLoaded)
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
	return fieldset.ViewFocused("RECIPES", fitHeight(body, m.middleHeight()-2, inner), width, focused)
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
	return fieldset.ViewFocused("ACTIONS", fitHeight(body, m.middleHeight()-2, inner), width, focused)
}

// routeDetailHeight is fixed so a list does not resize as the cursor moves
// between entries that describe themselves at different lengths. RECIPES and
// ACTIONS share it, so their rules line up across the two columns.
const routeDetailHeight = 10

// routeDetailKeyWidth is the label column of a detail pane's single-value rows.
// Labels are quiet; values carry the normal foreground, so the eye lands on the
// content rather than on the scaffolding.
const routeDetailKeyWidth = 8

func (m routeModel) actionListHeight() int { return max(1, m.middleHeight()-2-routeDetailHeight) }
func (m routeModel) recipeListHeight() int { return max(1, m.middleHeight()-2-routeDetailHeight) }

// routeAttachmentsHeight is what the Capture's own files take from the bottom
// of the two middle columns: a legend, its border, and rows enough to read a
// file's details beside its name.
const routeAttachmentsHeight = 10

// middleHeight is what RECIPES and ACTIONS are left with above the attachments.
// The pane takes its room from those two columns alone, the way the Capture
// Root takes its room from the column above it: the outer two columns still
// run the height of the workspace.
func (m routeModel) middleHeight() int { return max(6, m.height-routeAttachmentsHeight) }

// attachmentsPane is what the Capture holds: its files on the left and the
// details of the one under the cursor on the right. It spans both middle
// columns because neither half is worth a column of its own — a list of two
// filenames, and a dozen short rows — and because they are one question asked
// in two parts. The details are the ones Scan shows for a file, read by the
// same inspection, so a reader moving between the tabs is told the same things
// about the same file.
func (m routeModel) attachmentsPane(width int) string {
	focused := m.fields.Current() == routeAttachmentsField
	inner := max(1, width-4)
	listWidth := max(12, inner/3)
	detailWidth := max(8, inner-listWidth-columnGutter)
	height := max(1, routeAttachmentsHeight-2)

	attachments := m.attachments
	attachments.SetSize(listWidth, height)
	list := attachments.View(focused, scanTitleStyle, scanMutedStyle)
	switch {
	case !m.hasSelectedCapture():
		list = scanMutedStyle.Render("· Select a capture")
	case len(m.captureAttachments()) == 0:
		list = scanMutedStyle.Render("· No attachments")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		fitHeight(list, height, listWidth), " ",
		fitHeight(m.attachmentDetail(detailWidth), height, detailWidth))
	return fieldset.ViewFocused("ATTACHMENTS", body, width, focused)
}

// attachmentDetail is the file beside its name: what the index calls it, and
// what the filesystem and the file itself say about it.
func (m routeModel) attachmentDetail(width int) string {
	attachment, ok := m.focusedAttachment()
	if !ok {
		return scanMutedStyle.Render("· Nothing to show")
	}
	// What the index calls the file heads the same key column its filesystem
	// details are in, so the two read as one list rather than as a caption over
	// a table. The details themselves are Scan's, read by the same inspection,
	// so one file does not read as two different files in two tabs.
	kind := attachment.kind
	if kind == "" {
		kind = "· unstated"
	}
	properties := append([]property{{name: "Kind", value: kind}}, m.attachmentProps...)
	if m.attachmentError != "" {
		properties = append(properties, property{name: "", value: "! " + m.attachmentError})
	} else if len(m.attachmentProps) == 0 {
		properties = append(properties, property{name: "", value: "· Reading…"})
	}
	return renderProperties(properties, width, -1, false, routePropertyKeyWidth)
}

// hasSelectedCapture reports whether a Capture is under the cursor at all.
func (m routeModel) hasSelectedCapture() bool {
	_, ok := m.selectedCapture()
	return ok
}

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
	lines = append(lines, detailInline("Source", recipeSource(recipe), width))
	// Matches comes last and takes a line per workflow: it is the longest of
	// the groups and the one a reader checks least often, and a run of names on
	// one line is read as a sentence rather than as a list.
	lines = append(lines, detailHeader("Matches"))
	for _, workflow := range matchedWorkflows(recipe) {
		lines = append(lines, detailItem("- "+workflow, width))
	}
	return strings.Join(lines, "\n")
}

// matchedWorkflows are the workflows a Recipe is offered for, one per line. A
// Recipe naming none is offered for all of them, which is a fact about it
// rather than an empty list.
func matchedWorkflows(recipe organizer.Recipe) []string {
	if len(recipe.Match.Workflows) == 0 {
		return []string{"any workflow"}
	}
	return recipe.Match.Workflows
}

// recipeSource names the file a Recipe came from by its filename: the directory
// is the same for every one of them and would crowd out the name.
func recipeSource(recipe organizer.Recipe) string {
	if recipe.Source == "" {
		return "· unknown"
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
	column := fieldset.ViewFocused("FIELDS", fitHeight(content, m.fieldsHeight()-2, inner), width, focused)
	return column + "\n" + m.runActions(width)
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
