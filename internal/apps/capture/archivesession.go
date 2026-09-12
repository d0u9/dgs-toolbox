package capture

import (
	"fmt"
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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	archiveCapturesField = "archive-captures"
	archiveRootField     = "archive-root"
	archiveDetailField   = "archive-detail"
	archiveFolderField   = "archive-folder"
	rejectFolderField    = "reject-folder"
	// archiveActionsHeight is what the page actions take from the column they
	// sit under: a blank row, the two-line buttons, and a row below them, so the
	// controls are not pressed into the corner of the screen.
	archiveActionsHeight = 4
	archiveButtonWidth   = 24
)

// archiveDestination is one of the three places a Capture can be sitting: the
// Capture root it arrived in, the archive it is kept in, or the folder rejected
// ones are set aside in.
type archiveDestination int

const (
	destinationCaptures archiveDestination = iota
	destinationArchive
	destinationReject
)

// archiveFolderLoadedMsg is one of the two destination folders read back. It is
// its own message rather than capturesLoadedMsg, which the Capture session
// broadcasts to every tab as the shared Capture root: these two folders belong
// to Archive alone.
type archiveFolderLoadedMsg struct {
	destination archiveDestination
	root        string
	captures    []captureEntry
	err         error
}

// archiveModel is the third Capture session: it takes Captures out of the
// Capture root and puts them where they belong. Filing has two answers, so the
// session has two folders: a Capture that has been organized is **archived**,
// and one that is not worth keeping is **rejected**. Neither is a deletion — a
// rejected Capture is moved out of the way, not destroyed — and neither changes
// what is inside the Capture: the directory is relocated whole.
//
// A move happens as soon as it is asked for, with no dialog in front of it,
// because every move can be taken back: `R` in either destination returns the
// Capture to the Capture root. A confirmation buys nothing that an undo does
// not already give, and it costs a keystroke on every Capture in a session
// whose whole job is going through a list of them.
//
// Archiving is not a Recipe Action. An Action writes a Capture somewhere; this
// moves the Capture itself, and one that has been moved is no longer in the
// root the other sessions read. Keeping it out of the Actions means no Recipe
// can quietly retire a Capture as a side effect of organizing it.
type archiveModel struct {
	width     int
	height    int
	root      string
	indexFile string
	// archiveRoot is where organized Captures are kept and rejectRoot where the
	// rest are set aside. Both come from the configuration and are not chosen in
	// the session: where an installation files things is decided once, and a
	// picker for it would be a control that is used on the first run and never
	// again.
	archiveRoot string
	rejectRoot  string
	rootControl rootControl
	entries     []captureEntry
	// archived and rejected are what the two folders hold, read the same way
	// the Capture root is read: a Capture is a Capture wherever it sits.
	archived  []captureEntry
	rejected  []captureEntry
	captures  scrolllist.Model
	archive   scrolllist.Model
	reject    scrolllist.Model
	loadError string
	// folderError is why a destination could not be read, keyed by destination,
	// and notice is what became of the last move: said where it was asked for
	// rather than in a dialog nobody opened.
	folderError map[archiveDestination]string
	notice      moveNotice
	// view is how the three lists name a Capture. It belongs to the session
	// rather than to one column: the same Capture is named the same way
	// wherever it is listed, or moving one between columns would look like it
	// changed.
	view      captureView
	fields    datafield.Navigator
	pendingGG bool
	lastClick routeClick
}

func newArchiveModel(root, indexFile, archiveRoot, rejectRoot string) archiveModel {
	if archiveRoot != "" {
		archiveRoot = expandHome(archiveRoot)
	}
	if rejectRoot != "" {
		rejectRoot = expandHome(rejectRoot)
	}
	m := archiveModel{
		width:       80,
		height:      22,
		root:        root,
		indexFile:   indexFile,
		archiveRoot: archiveRoot,
		rejectRoot:  rejectRoot,
		rootControl: newRootControl(root),
		captures:    scrolllist.New(),
		archive:     scrolllist.New(),
		reject:      scrolllist.New(),
		folderError: make(map[archiveDestination]string),
		fields: datafield.New(
			datafield.Field{ID: archiveCapturesField, Row: 0, Col: 0},
			datafield.Field{ID: archiveRootField, Row: 1, Col: 0},
			datafield.Field{ID: archiveDetailField, Row: 0, Col: 1},
			datafield.Field{ID: archiveFolderField, Row: 0, Col: 2},
			datafield.Field{ID: rejectFolderField, Row: 1, Col: 2},
		),
	}
	m.fields.Set(archiveCapturesField)
	m.rebuildItems()
	return m
}

// Init reads the two destination folders. The Capture root arrives from the
// shared load the Capture session runs for every tab.
func (m archiveModel) Init() tea.Cmd { return m.loadFolders() }

// loadFolders reads both destinations. A folder that does not exist yet is not
// an error: it is where the first move will create one.
func (m archiveModel) loadFolders() tea.Cmd {
	return tea.Batch(
		loadArchiveFolder(destinationArchive, m.archiveRoot, m.indexFile),
		loadArchiveFolder(destinationReject, m.rejectRoot, m.indexFile),
	)
}

func loadArchiveFolder(destination archiveDestination, root, indexFile string) tea.Cmd {
	if root == "" {
		return nil
	}
	return func() tea.Msg {
		captures, err := scanCaptures(root, indexFile)
		return archiveFolderLoadedMsg{destination: destination, root: root, captures: captures, err: err}
	}
}

func (m archiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeComponents()
		m.rootControl.Resize(m.width, m.height)
		return m, nil
	case rootChangedMsg:
		m.root = msg.root
		m.rootControl.SetRoot(msg.root)
		m.entries = nil
		m.loadError, m.notice = "", moveNotice{}
		m.rebuildItems()
		return m, nil
	case capturesLoadedMsg:
		m.root = msg.root
		m.loadError = ""
		if msg.err != nil {
			m.loadError = msg.err.Error()
			m.entries = nil
		} else {
			m.entries = msg.captures
		}
		m.rebuildItems()
		return m, nil
	case archiveFolderLoadedMsg:
		return m.applyFolder(msg), nil
	}

	if m.rootControl.Picking() {
		selected, cmd := m.rootControl.Update(msg)
		if selected == "" {
			return m, cmd
		}
		return m, tea.Batch(cmd, rootChanged(selected))
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	}
	return m, nil
}

func (m archiveModel) applyFolder(msg archiveFolderLoadedMsg) archiveModel {
	if msg.root != m.folderFor(msg.destination) {
		return m
	}
	delete(m.folderError, msg.destination)
	if msg.err != nil {
		// A folder that is not there yet is where the first move will create
		// one, so it reads as empty rather than as a failure.
		m.setFolder(msg.destination, nil)
		m.rebuildItems()
		return m
	}
	m.setFolder(msg.destination, msg.captures)
	m.rebuildItems()
	return m
}

func (m archiveModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "v" {
		// The view belongs to the session rather than to one column: the same
		// Capture is named the same way wherever it is listed, or moving one
		// between columns would look like it changed.
		m.view.Toggle()
		m.pendingGG = false
		m.rebuildItems()
		return m, nil
	}
	if m.fields.Move(key) || key == "tab" || key == "shift+tab" {
		if key == "tab" || key == "shift+tab" {
			m.cycleFields(key == "tab")
		}
		m.pendingGG = false
		return m, nil
	}
	switch m.fields.Current() {
	case archiveRootField:
		m.pendingGG = false
		switch key {
		case "enter":
			return m, m.rootControl.Open(m.width, m.height)
		case "R":
			return m, m.refresh()
		}
		return m, nil
	case archiveFolderField:
		return m.updateFolderList(destinationArchive, key)
	case rejectFolderField:
		return m.updateFolderList(destinationReject, key)
	case archiveCapturesField:
		return m.updateCaptures(key)
	}
	m.pendingGG = false
	return m, nil
}

func (m archiveModel) updateCaptures(key string) (tea.Model, tea.Cmd) {
	if moved, cmd, handled := m.moveKey(destinationCaptures, key); handled {
		return moved, cmd
	}
	listMoveKey(&m.captures, key, &m.pendingGG, true)
	if key == "R" {
		return m, m.refresh()
	}
	return m, nil
}

// updateFolderList drives one of the two destinations. `R` there returns the
// Capture to the Capture root: a move is taken back where it landed, which is
// the row the reader is looking at when they decide it was wrong. It is the
// same letter as the refresh key it replaces in this column, because a list of
// filed Captures is not a scan and has nothing to re-read that the move itself
// did not already update.
func (m archiveModel) updateFolderList(destination archiveDestination, key string) (tea.Model, tea.Cmd) {
	list := m.listFor(destination)
	if moved, cmd, handled := m.moveKey(destination, key); handled {
		return moved, cmd
	}
	listMoveKey(list, key, &m.pendingGG, true)
	if key == "R" {
		return m.move(destination, destinationCaptures)
	}
	return m, nil
}

// moveKey is the pair of keys every column shares: the two that file a Capture
// away. They mean the same thing wherever the cursor is — `a` archives what is
// under it and Backspace rejects it — so a Capture in the wrong folder is moved
// to the right one without going back through the Capture root.
//
// Either case of the letter archives. A reader working through a list holds no
// modifier for the other keys in this session, and a Shift that is on by
// accident should not turn the one key that files a Capture into nothing at
// all.
func (m archiveModel) moveKey(from archiveDestination, key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "a", "A":
		if from == destinationArchive {
			return m, nil, false
		}
		updated, cmd := m.move(from, destinationArchive)
		return updated, cmd, true
	case "backspace", "delete":
		if from == destinationReject {
			return m, nil, false
		}
		updated, cmd := m.move(from, destinationReject)
		return updated, cmd, true
	}
	return m, nil, false
}

// move carries the selected Capture from one place to another, now, on disk.
// Nothing is asked first: every move can be taken back with `R`, and a session
// whose job is going through a list should not charge a confirmation per row.
// A Capture that has not been organized is refused the archive — the archive is
// where handled Captures are kept, and one that was never organized would
// arrive there having had nothing done to it — while any Capture can be
// rejected, because deciding a Capture is not worth organizing is exactly the
// case rejection exists for.
func (m archiveModel) move(from, to archiveDestination) (tea.Model, tea.Cmd) {
	entry, ok := m.selectedIn(from)
	if !ok {
		return m, nil
	}
	if to == destinationArchive && !entry.organized {
		m.notice = failed("Not organized — it can be rejected, not archived")
		return m, nil
	}
	target := m.folderFor(to)
	if target == "" {
		m.notice = unsetFolderNotice(to)
		return m, nil
	}
	relocated, notice, ok := moveCaptureTo(entry, target, folderName(to))
	m.notice = notice
	if !ok {
		return m, nil
	}
	m.remove(from, entry.path)
	m.add(to, relocated)
	m.rebuildItems()
	m.listFor(to).SelectID(archiveItemID(to, relocated.path))
	return m, nil
}

func (m archiveModel) refresh() tea.Cmd {
	return tea.Batch(loadCaptures(m.root, m.indexFile), m.loadFolders())
}

func (m archiveModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		if action, ok := m.buttonHit(msg.X, msg.Y); ok {
			return m.pressButton(action)
		}
	}
	hit := m.fields.HitAt(msg.X, msg.Y)
	if hit == "" {
		return m, nil
	}
	if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		delta := -3
		if msg.Button == tea.MouseButtonWheelDown {
			delta = 3
		}
		if list, ok := m.listForField(hit); ok {
			list.Scroll(delta)
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	m.fields.FocusAt(msg.X, msg.Y)
	m.pendingGG = false
	double := pairsWithLastClick(m.lastClick, hit, msg.Y)
	m.lastClick = routeClick{field: hit, row: msg.Y, at: time.Now()}

	if hit == archiveRootField {
		if m.rootControl.Clicked(msg.X-2, msg.Y-m.captureListHeight()-1) {
			return m, m.rootControl.Open(m.width, m.height)
		}
		return m, nil
	}
	list, ok := m.listForField(hit)
	if !ok {
		return m, nil
	}
	row := msg.Y - 1
	if hit == rejectFolderField {
		row = msg.Y - m.destinationHeight() - 1
	}
	if hit == archiveFolderField {
		row -= lipgloss.Height(m.destinationHeader(destinationArchive, max(1, m.columnWidths()[2]-4)))
	}
	if hit == rejectFolderField {
		row -= lipgloss.Height(m.destinationHeader(destinationReject, max(1, m.columnWidths()[2]-4)))
	}
	if !list.SelectRow(row) {
		return m, nil
	}
	if double {
		// A double click does what the column's own key does: file it away from
		// CAPTURES, and return it from a destination. A move is a change on
		// disk, so a stray single click never makes one.
		m.lastClick = routeClick{}
		switch hit {
		case archiveCapturesField:
			return m.move(destinationCaptures, destinationArchive)
		case archiveFolderField:
			return m.move(destinationArchive, destinationCaptures)
		case rejectFolderField:
			return m.move(destinationReject, destinationCaptures)
		}
	}
	return m, nil
}

// archiveAction is what one of the two buttons under the right column does.
type archiveAction int

const (
	actionArchive archiveAction = iota
	actionReject
	actionRestore
)

func (m archiveModel) pressButton(action archiveAction) (tea.Model, tea.Cmd) {
	from := m.focusedDestination()
	switch action {
	case actionArchive:
		return m.move(from, destinationArchive)
	case actionReject:
		return m.move(from, destinationReject)
	default:
		return m.move(from, destinationCaptures)
	}
}

// focusedDestination is which of the three lists the cursor is in. The Capture
// Root control belongs to the Captures column, so it answers for that one.
func (m archiveModel) focusedDestination() archiveDestination {
	switch m.fields.Current() {
	case archiveFolderField:
		return destinationArchive
	case rejectFolderField:
		return destinationReject
	}
	return destinationCaptures
}

func (m archiveModel) folderFor(destination archiveDestination) string {
	switch destination {
	case destinationArchive:
		return m.archiveRoot
	case destinationReject:
		return m.rejectRoot
	}
	return m.root
}

func folderName(destination archiveDestination) string {
	switch destination {
	case destinationArchive:
		return "archive"
	case destinationReject:
		return "reject"
	}
	return "captures"
}

// folderKey is the configuration key a missing folder is set in, said in full
// so the reader is told what to write rather than that something is missing.
func folderKey(destination archiveDestination) string {
	if destination == destinationReject {
		return "capture.archive.reject"
	}
	return "capture.archive.root"
}

func (m archiveModel) entriesIn(destination archiveDestination) []captureEntry {
	switch destination {
	case destinationArchive:
		return m.archived
	case destinationReject:
		return m.rejected
	}
	return m.entries
}

func (m *archiveModel) setFolder(destination archiveDestination, captures []captureEntry) {
	switch destination {
	case destinationArchive:
		m.archived = captures
	case destinationReject:
		m.rejected = captures
	default:
		m.entries = captures
	}
}

func (m *archiveModel) remove(destination archiveDestination, path string) {
	entries := m.entriesIn(destination)
	kept := make([]captureEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.path != path {
			kept = append(kept, entry)
		}
	}
	m.setFolder(destination, kept)
}

// add puts a Capture into a list in the order that list is read in, so a
// Capture that has just moved is where a reader would look for it rather than
// at the end.
func (m *archiveModel) add(destination archiveDestination, entry captureEntry) {
	entries := append([]captureEntry{}, m.entriesIn(destination)...)
	index := len(entries)
	for position, existing := range entries {
		if strings.ToLower(entry.name) < strings.ToLower(existing.name) {
			index = position
			break
		}
	}
	entries = append(entries, captureEntry{})
	copy(entries[index+1:], entries[index:])
	entries[index] = entry
	m.setFolder(destination, entries)
}

func (m *archiveModel) listFor(destination archiveDestination) *scrolllist.Model {
	switch destination {
	case destinationArchive:
		return &m.archive
	case destinationReject:
		return &m.reject
	}
	return &m.captures
}

func (m *archiveModel) listForField(field string) (*scrolllist.Model, bool) {
	switch field {
	case archiveCapturesField:
		return &m.captures, true
	case archiveFolderField:
		return &m.archive, true
	case rejectFolderField:
		return &m.reject, true
	}
	return nil, false
}

func archiveItemID(destination archiveDestination, path string) string {
	return fmt.Sprintf("%d:%s", destination, path)
}

func (m archiveModel) selectedIn(destination archiveDestination) (captureEntry, bool) {
	list := m.listFor(destination)
	item, ok := list.Selected()
	if !ok {
		return captureEntry{}, false
	}
	path := strings.TrimPrefix(item.ID, fmt.Sprintf("%d:", destination))
	for _, entry := range m.entriesIn(destination) {
		if entry.path == path {
			return entry, true
		}
	}
	return captureEntry{}, false
}

// selected is the Capture the centre column describes: the one under the cursor
// of whichever list has focus, so the pane is always about the row being acted
// on.
func (m archiveModel) selected() (captureEntry, bool) {
	return m.selectedIn(m.focusedDestination())
}

func (m *archiveModel) rebuildItems() {
	for _, destination := range []archiveDestination{destinationCaptures, destinationArchive, destinationReject} {
		list := m.listFor(destination)
		selected := ""
		if item, ok := list.Selected(); ok {
			selected = item.ID
		}
		entries := m.entriesIn(destination)
		items := make([]scrolllist.Item, 0, len(entries))
		boundary := 0
		if destination == destinationCaptures {
			entries, boundary = orderByOrganized(entries)
		}
		for _, entry := range entries {
			// Only the Capture root carries the Recipe that handled a Capture:
			// in the two destinations every row below the rule would carry one,
			// which says nothing about which of them is which.
			detail := m.view.Detail(entry)
			if latest, ok := entry.record.Latest(); ok && entry.organized &&
				destination == destinationCaptures && !m.view.byFolder {
				detail += "  → " + latest.RecipeName
			}
			items = append(items, scrolllist.Item{
				ID:     archiveItemID(destination, entry.path),
				Label:  m.view.Label(entry),
				Detail: indentDetail(detail),
			})
		}
		list.SetItems(items)
		if destination == destinationCaptures {
			list.SetDivider(boundary, "ORGANIZED")
		}
		if selected != "" {
			list.SelectID(selected)
		}
	}
}

// tabOrder is what Tab walks: the three lists. The Capture Root is not in the
// ring — it is a setting rather than a step — and Alt and the arrows still
// reach it below CAPTURES, as they do in Route.
func (m archiveModel) tabOrder() []string {
	return []string{archiveCapturesField, archiveFolderField, rejectFolderField}
}

func (m *archiveModel) cycleFields(forward bool) {
	cycleFieldOrder(&m.fields, m.tabOrder(), forward)
}

func (m archiveModel) View() string {
	if m.width < 72 || m.height < 10 || !m.pathsFit() {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, scanMutedStyle.Render("Resize terminal for Capture Archive"))
	}
	widths := m.columnWidths()
	workspace := lipgloss.JoinHorizontal(lipgloss.Top,
		m.leftColumn(widths[0]), " ",
		m.centerColumn(widths[1]), " ",
		m.rightColumn(widths[2]))
	if m.rootControl.Picking() {
		return overlay.Place(workspace, m.rootControl.OverlayView(m.width, m.height), m.width, m.height)
	}
	return workspace
}

// columnWidths is the shared centre-wide three-column variant Scan uses: a
// narrow list, a wide pane, and a narrow pair of destinations. The middle
// column is what is being read — what this Capture is and what was done with
// it — while the columns either side hold a row each.
func (m archiveModel) columnWidths() [3]int {
	available := max(6, m.width-2*columnGutter)
	quarter := available / 4
	return [3]int{quarter, available - 2*quarter, quarter}
}

func (m archiveModel) captureListHeight() int { return m.height - rootHeight }

// destinationHeight is what each of the two folder panes gets. The buttons
// below them take their room from this column alone, the way the Capture Root
// control takes its room from the column above it.
func (m archiveModel) destinationHeight() int {
	return max(4, (m.height-archiveActionsHeight)/2)
}

func (m archiveModel) leftColumn(width int) string {
	listHeight := m.captureListHeight()
	focused := m.fields.Current() == archiveCapturesField
	captures := m.captures
	captures.SetSize(max(1, width-4), max(1, listHeight-2))
	content := fitHeight(captures.View(focused, scanTitleStyle, scanMutedStyle), listHeight-2, width-4)
	if m.loadError != "" {
		content = fitHeight(scanMutedStyle.Render("! "+m.loadError), listHeight-2, width-4)
	} else if len(m.entries) == 0 {
		content = fitHeight(scanMutedStyle.Render("· No captures found"), listHeight-2, width-4)
	}
	list := fieldset.ViewFocused("CAPTURES", content, width, focused)
	return list + "\n" + m.rootControl.Fieldset(width, m.fields.Current() == archiveRootField)
}

func (m archiveModel) centerColumn(width int) string {
	focused := m.fields.Current() == archiveDetailField
	inner := max(1, width-4)
	return fieldset.ViewFocused("CAPTURE", fitHeight(m.detail(inner), m.height-2, inner), width, focused)
}

// rightColumn is the two destinations, one above the other, with the controls
// that move a Capture between them below. Each pane is a list of what that
// folder actually holds, read with the same rule the Capture root is read by,
// so a move is seen landing rather than reported.
func (m archiveModel) rightColumn(width int) string {
	top := m.destinationHeight()
	bottom := max(4, m.height-archiveActionsHeight-top)
	return m.destinationPane(destinationArchive, width, top) + "\n" +
		m.destinationPane(destinationReject, width, bottom) + "\n" +
		m.actions(width)
}

func (m archiveModel) destinationPane(destination archiveDestination, width, height int) string {
	field := archiveFolderField
	legend := "ARCHIVE"
	if destination == destinationReject {
		field, legend = rejectFolderField, "REJECT"
	}
	focused := m.fields.Current() == field
	inner := max(1, width-4)
	folder := m.folderFor(destination)

	var content string
	list := *m.listFor(destination)
	header := m.destinationHeader(destination, inner)
	headerHeight := lipgloss.Height(header)
	list.SetSize(inner, max(1, height-2-headerHeight))
	switch {
	case folder == "":
		content = scanMutedStyle.Render("· Set " + folderKey(destination))
	case m.folderError[destination] != "":
		content = scanMutedStyle.Render("! " + m.folderError[destination])
	case len(m.entriesIn(destination)) == 0:
		content = scanMutedStyle.Render("· Empty")
	default:
		content = list.View(focused, scanTitleStyle, scanMutedStyle)
	}
	// The folder is named above its contents rather than in a control: it is
	// configuration, so it is something to read and not something to change
	// here.
	body := header + "\n" + fitHeight(content, max(1, height-2-headerHeight), inner)
	return fieldset.ViewFocused(legend, body, width, focused)
}

// detail says what the Capture under the cursor is, where it is sitting, and
// what was done with it. The pane follows the focused list, so it is about the
// row that the next keystroke would move.
func (m archiveModel) detail(width int) string {
	entry, ok := m.selected()
	if !ok {
		return scanMutedStyle.Render("· Select a capture")
	}
	lines := []string{
		detailInline("Capture", routeCaptureLabel(entry), width),
		detailInline("Name", entry.name+string(filepath.Separator), width),
		detailInline("Source", routeCaptureDetail(entry), width),
		detailInline("Sitting", m.sittingIn(), width),
		scanMutedStyle.Render("Folder") + "\n" + ansi.Hardwrap(displayPath(entry.path), max(1, width), true),
	}
	lines = append(lines, "", divider.Labelled("ORGANIZED", width))
	if !entry.organized {
		// A Capture nobody has organized can still be rejected, so the pane
		// says what state it is in rather than leaving the group empty.
		lines = append(lines, scanMutedStyle.Render("  · Not organized — it can be rejected, not archived"))
		return strings.Join(lines, "\n")
	}
	for index := len(entry.record.Runs) - 1; index >= 0; index-- {
		run := entry.record.Runs[index]
		// The run is headed by when it happened and what it ran. Both are
		// longer than a key column, so they head the block rather than sitting
		// either side of one.
		lines = append(lines, scanMutedStyle.Render(truncate(organizer.FormatTimestamp(run.OrganizedAt)+"  ·  "+run.RecipeName, width)))
		for _, action := range run.Actions {
			marker := "✓"
			if !action.Executed {
				marker = "✗"
			}
			if action.Skipped {
				marker = "·"
			}
			target := action.Target
			if target == "" {
				target = "· no target"
			}
			lines = append(lines, detailItem(marker+" "+shortAction(action.Action)+"  "+target, width))
		}
	}
	return strings.Join(lines, "\n")
}

func (m archiveModel) sittingIn() string {
	switch m.focusedDestination() {
	case destinationArchive:
		return "Archive"
	case destinationReject:
		return "Reject"
	}
	return "Capture root"
}

// actions are the two controls under the right column: what can be done to the
// row under the cursor, as the shared two-line filled buttons every other
// command puts a page action in. Two rather than one, because filing has two
// answers and a single button would have to be aimed before it was pressed.
// From a destination the pair becomes the one thing that is left to do there —
// putting the Capture back.
func (m archiveModel) actions(width int) string {
	rendered := make([][]string, 0, 2)
	for _, spec := range m.buttons() {
		rendered = append(rendered, pageactions.Button(spec.title, spec.subtitle, spec.width, spec.primary))
	}
	return pageActionRow(width, rendered)
}

// archiveButton is one of those controls: what it says, whether it is lit, and
// how wide it is drawn.
type archiveButton struct {
	action   archiveAction
	title    string
	subtitle string
	primary  bool
	width    int
}

// buttons is the pair the current column offers. A button is lit only when
// pressing it would move something, and says why not when it would not: the
// control answers "can I do this to this row?" without it being tried.
func (m archiveModel) buttons() []archiveButton {
	widths := m.columnWidths()
	from := m.focusedDestination()
	entry, has := m.selectedIn(from)

	if from != destinationCaptures {
		width := min(archiveButtonWidth, max(12, widths[2]-2))
		subtitle := "To capture root"
		if !has {
			subtitle = "Nothing selected"
		}
		return []archiveButton{{action: actionRestore, title: "Restore  R", subtitle: subtitle, primary: has, width: width}}
	}

	// The pair shares the column with a gutter between them, so each takes half
	// of it. They shrink rather than overflow on a narrow terminal: a control
	// that says "Archive" without its key is still a control, while one drawn
	// past the edge of its column is not.
	width := max(9, min(archiveButtonWidth, (widths[2]-1)/2))
	archiveSubtitle, archiveReady := "Keep it", has && entry.organized && m.archiveRoot != ""
	switch {
	case !has:
		archiveSubtitle = "· none"
	case !entry.organized:
		archiveSubtitle = "Not organized"
	case m.archiveRoot == "":
		archiveSubtitle = "No folder"
	}
	rejectSubtitle, rejectReady := "Set aside", has && m.rejectRoot != ""
	switch {
	case !has:
		rejectSubtitle = "· none"
	case m.rejectRoot == "":
		rejectSubtitle = "No folder"
	}
	return []archiveButton{
		{action: actionArchive, title: "Archive  a", subtitle: archiveSubtitle, primary: archiveReady, width: width},
		{action: actionReject, title: "Reject  ⌫", subtitle: rejectSubtitle, primary: rejectReady, width: width},
	}
}

// buttonHit reports which control a click landed on. The controls are laid out
// right to left from the edge of the column, the way they are drawn.
func (m archiveModel) buttonHit(x, y int) (archiveAction, bool) {
	top := m.height - archiveActionsHeight + 1
	if y < top || y > top+1 {
		return 0, false
	}
	widths := m.columnWidths()
	end := widths[0] + widths[1] + 2*columnGutter + widths[2] - 1
	buttons := m.buttons()
	for index := len(buttons) - 1; index >= 0; index-- {
		start := end - buttons[index].width
		if x >= start && x < end {
			return buttons[index].action, true
		}
		end = start - 1
	}
	return 0, false
}

func (m *archiveModel) resizeComponents() {
	if m.width < 72 || m.height < 10 || !m.pathsFit() {
		return
	}
	widths := m.columnWidths()
	listHeight := m.captureListHeight()
	m.captures.SetSize(max(1, widths[0]-4), max(1, listHeight-2))
	m.fields.SetBounds(archiveCapturesField, datafield.Bounds{X: 0, Y: 0, Width: widths[0], Height: listHeight})
	m.fields.SetBounds(archiveRootField, datafield.Bounds{X: 0, Y: listHeight, Width: widths[0], Height: rootHeight})
	center := widths[0] + columnGutter
	m.fields.SetBounds(archiveDetailField, datafield.Bounds{X: center, Y: 0, Width: widths[1], Height: m.height})
	right := center + widths[1] + columnGutter
	top := m.destinationHeight()
	bottom := max(4, m.height-archiveActionsHeight-top)
	m.archive.SetSize(max(1, widths[2]-4), max(1, top-2-lipgloss.Height(m.destinationHeader(destinationArchive, max(1, widths[2]-4)))))
	m.reject.SetSize(max(1, widths[2]-4), max(1, bottom-2-lipgloss.Height(m.destinationHeader(destinationReject, max(1, widths[2]-4)))))
	m.fields.SetBounds(archiveFolderField, datafield.Bounds{X: right, Y: 0, Width: widths[2], Height: top})
	m.fields.SetBounds(rejectFolderField, datafield.Bounds{X: right, Y: top, Width: widths[2], Height: bottom})
}

// CapturesShellKey keeps Esc inside Archive while the explorer is open, and
// keeps Backspace and Delete in the session everywhere: they file a Capture
// away here, and a key held down while working through a list must not fall out
// of the command.
func (m archiveModel) CapturesShellKey(key string) bool {
	if key == "backspace" || key == "delete" {
		return true
	}
	return m.rootControl.CapturesShellKey(key)
}

func (m archiveModel) Status() tui.Status {
	if m.rootControl.Picking() {
		if m.rootControl.HasDialog() {
			return tui.Status{Left: "BROWSE", Center: "CAPTURE ROOT"}
		}
		return tui.Status{Left: "BROWSE", Center: "CAPTURE ROOT", Right: m.rootControl.Hint()}
	}
	center := m.statusValue()
	switch m.fields.Current() {
	case archiveRootField:
		return tui.Status{Left: "CAPTURE ROOT", Center: m.root, Right: "↵ Browse  R Refresh  tab Next"}
	case archiveFolderField:
		return tui.Status{Left: "ARCHIVE", Center: center, Right: "↑/k ↓/j Move  R Restore  ⌫ Reject  v View  tab Next"}
	case rejectFolderField:
		return tui.Status{Left: "REJECT", Center: center, Right: "↑/k ↓/j Move  R Restore  a Archive  v View  tab Next"}
	default:
		return tui.Status{Left: "ARCHIVE", Center: center, Right: "↑/k ↓/j Move  a Archive  ⌫ Reject  v View  R Refresh"}
	}
}

// statusValue is the last thing that happened when there is one, and otherwise
// where the selected Capture sits. A move is reported here rather than in a
// dialog: it has already happened, and `R` is how it is taken back.
func (m archiveModel) statusValue() string {
	if !m.notice.empty() {
		return m.notice.status()
	}
	if entry, ok := m.selected(); ok {
		return entry.path
	}
	return fmt.Sprintf("%d CAPTURES · %d ARCHIVED · %d REJECTED",
		len(m.entries), len(m.archived), len(m.rejected))
}

// Paths get the full column width and wrap without dropping the final component.
func (m archiveModel) destinationHeader(destination archiveDestination, width int) string {
	folder := m.folderFor(destination)
	if folder == "" {
		return scanMutedStyle.Render("· No folder")
	}
	return scanMutedStyle.Render(ansi.Hardwrap(displayPath(folder), max(1, width), true))
}

// Keep complete paths and at least one two-line Capture row inside each pane.
func (m archiveModel) pathsFit() bool {
	width := max(1, m.columnWidths()[2]-4)
	top := m.destinationHeight()
	bottom := max(4, m.height-archiveActionsHeight-top)
	return lipgloss.Height(m.destinationHeader(destinationArchive, width))+4 <= top &&
		lipgloss.Height(m.destinationHeader(destinationReject, width))+4 <= bottom
}
