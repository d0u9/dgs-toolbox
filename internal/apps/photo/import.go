package photo

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/contextmenu"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/overlay"
	"dgs-toolbox/internal/tui/pageactions"
	"dgs-toolbox/internal/tui/scrolllist"
	"dgs-toolbox/internal/tui/stepper"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	sourceField = iota
	destinationField
	pathFieldCount
)

const (
	sourceID      = "source"
	destinationID = "destination"
	operationID   = "operation"
	extensionsID  = "extensions"
	duplicatesID  = "duplicates"
	parallelID    = "parallelism"
	deleteStateID = "delete-state"
)

var (
	setupIDs     = []string{sourceID, destinationID}
	parameterIDs = []string{operationID, extensionsID, duplicatesID, parallelID}
)

type importStage int

const (
	setupStage importStage = iota
	scanStage
	parameterStage
	processingStage
	resultStage
)

type scannedFile struct {
	side string
	path string
	size int64
}

type scanSummary struct {
	source      []scannedFile
	destination []scannedFile
	errors      []string
}

type importPlan struct {
	eligible   int
	duplicates int
	skipped    int
	processed  int
	operation  string
}

type scanTickMsg time.Time
type scanDoneMsg struct {
	generation int
	summary    scanSummary
}

type transferEventMsg importer.Event
type transferDoneMsg importer.Result

type workerPhase int

const (
	workerIdle workerPhase = iota
	workerCopying
	workerVerifying
	workerPublishing
	workerDone
)

type processingWorker struct {
	number          int
	jobIndex        int
	file            scannedFile
	phase           workerPhase
	progress        int
	sourceHash      string
	destinationHash string
}

type processingState struct {
	files        []scannedFile
	verified     []scannedFile
	results      []importer.FileResult
	failed       int
	skipped      int
	finished     map[int]importer.Phase
	workers      []processingWorker
	paused       bool
	complete     bool
	workerOffset int
}

var (
	importAccent = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	importMuted  = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"}
	importPanel  = lipgloss.AdaptiveColor{Light: "#EAF4F4", Dark: "#243447"}

	importTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(importAccent)
	importNoteStyle    = lipgloss.NewStyle().Foreground(importMuted)
	importStepStyle    = lipgloss.NewStyle().Bold(true).Foreground(importMuted)
	importSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(importAccent)
	importHeadingStyle = lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#29243D", Dark: "#F4F1FF"})
	importFilterStyle = lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#ECFEFF", Dark: "#134E4A"}).
				Background(importAccent).
				Padding(0, 1)
	importModalStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(importAccent).
				Background(importPanel)
)

// importModel owns the Photo Import setup workspace. Its path fields open a
// read-only directory picker; review and scan behavior remain disabled.
type importModel struct {
	paths                [pathFieldCount]string
	stateFilename        string
	controls             form.Model
	picking              bool
	pickerFor            int
	picker               fileexplorer.Model
	width                int
	height               int
	help                 bool
	stage                importStage
	progress             float64
	scan                 scanSummary
	scanGeneration       int
	parameterFields      datafield.Navigator
	resultFields         datafield.Navigator
	sourceList           scrolllist.Model
	destinationList      scrolllist.Model
	resultList           scrolllist.Model
	menu                 contextmenu.Model
	menuTarget           string
	pendingListG         bool
	actionNotice         string
	extensionsConfigured bool
	processing           processingState
	leaveConfirm         bool
	leaveExits           bool
	leaveWasPaused       bool
	leaveDialog          confirm.Model
	transferCancel       context.CancelFunc
	transferController   *importer.Controller
	transferUpdates      <-chan tea.Msg
	pendingExit          bool
	pendingReturn        bool
}

func newImportModel() tui.CommandModel {
	return newImportModelWithStateFile(".dgs-state")
}

func newImportModelWithStateFile(stateFilename string) tui.CommandModel {
	return newImportModelWithSettings(stateFilename, "", "")
}

func newImportModelWithSettings(stateFilename, source, destination string) tui.CommandModel {
	paths := [pathFieldCount]string{
		demoPath("src"),
		demoPath("dst"),
	}
	if source != "" {
		paths[sourceField] = displayPath(expandHome(source))
	}
	if destination != "" {
		paths[destinationField] = displayPath(expandHome(destination))
	}
	return importModel{
		paths:         paths,
		stateFilename: stateFilename,
		controls: form.New(
			form.Field{ID: sourceID, Kind: form.Path, Label: "Source", Value: paths[sourceField]},
			form.Field{ID: destinationID, Kind: form.Path, Label: "Destination", Value: paths[destinationField]},
			form.Field{ID: operationID, Kind: form.Radio, Label: "Operation", Value: "Copy", Options: []string{"Copy", "Move"}},
			form.Field{ID: extensionsID, Kind: form.MultiCheckbox, Label: "Extensions", SelectAll: true},
			form.Field{ID: duplicatesID, Kind: form.Option, Label: "Duplicates", Value: "Skip", Options: []string{"Skip", "Replace", "Keep both"}},
			form.Field{ID: parallelID, Kind: form.Number, Label: "Parallelism", Value: "1", Min: 1, Max: 8, Step: 1},
			form.Field{ID: deleteStateID, Kind: form.Checkbox, Label: "Delete " + stateFilename, Checked: true},
		),
		parameterFields: datafield.New(
			datafield.Field{ID: "parameters", Row: 0, Col: 0},
			datafield.Field{ID: "summary", Row: 1, Col: 0},
			datafield.Field{ID: "source-results", Row: 0, Col: 1},
			datafield.Field{ID: "destination-results", Row: 1, Col: 1},
		),
		resultFields: datafield.New(
			datafield.Field{ID: "result-actions", Row: 0, Col: 0},
			datafield.Field{ID: "result-files", Row: 0, Col: 1},
		),
		sourceList:      scrolllist.New(),
		destinationList: scrolllist.New(),
		resultList:      scrolllist.New(),
		menu:            contextmenu.New(contextmenu.Item{ID: "open", Label: "Open with default app"}),
		width:           80, height: 22,
	}
}

func demoPath(name string) string {
	path, err := filepath.Abs(filepath.Join("testdata", "photo-import", name))
	if err != nil {
		return filepath.Join("testdata", "photo-import", name)
	}
	return displayPath(path)
}

func scanCmd(generation int, paths [pathFieldCount]string) tea.Cmd {
	return func() tea.Msg {
		return scanDoneMsg{generation: generation, summary: scanDirectories(paths)}
	}
}

func scanTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(now time.Time) tea.Msg { return scanTickMsg(now) })
}

func waitTransferUpdate(updates <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-updates
		if !ok {
			return nil
		}
		return message
	}
}

func scanDirectories(paths [pathFieldCount]string) scanSummary {
	var summary scanSummary
	scanRoot := func(side, root string, target *[]scannedFile) {
		root = expandHome(root)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				relative = path
			}
			*target = append(*target, scannedFile{side: side, path: filepath.ToSlash(relative), size: info.Size()})
			return nil
		})
		if err != nil {
			summary.errors = append(summary.errors, side+": "+err.Error())
		}
		sort.Slice(*target, func(i, j int) bool { return (*target)[i].path < (*target)[j].path })
	}
	scanRoot("SRC", paths[sourceField], &summary.source)
	scanRoot("DST", paths[destinationField], &summary.destination)
	return summary
}

func (m importModel) Init() tea.Cmd { return nil }

func (m importModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.picking {
			m.sizePicker()
		}
		return m, nil
	case scanTickMsg:
		if m.stage != scanStage {
			return m, nil
		}
		m.progress = min(.92, m.progress+.06)
		return m, scanTick()
	case scanDoneMsg:
		if msg.generation != m.scanGeneration || m.stage != scanStage {
			return m, nil
		}
		m.scan = msg.summary
		m.configureExtensions()
		m.syncResultLists()
		m.progress = 1
		m.stage = parameterStage
		m.parameterFields.Set("parameters")
		m.controls.SetFocusID(operationID)
		return m, nil
	case transferEventMsg:
		m.applyTransferEvent(importer.Event(msg))
		return m, waitTransferUpdate(m.transferUpdates)
	case transferDoneMsg:
		m.applyTransferResult(importer.Result(msg))
		if m.pendingExit {
			return m, tea.Quit
		}
		if m.pendingReturn {
			m.pendingReturn = false
			m.stage = parameterStage
		}
		return m, nil
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		if m.picking {
			var cmd tea.Cmd
			m.picker, _, cmd = m.picker.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

func (m importModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.picking {
		width, height := m.modalSize()
		msg.X -= (m.width-width)/2 + 1
		msg.Y -= (m.height-height)/2 + 4
		var cmd tea.Cmd
		var selected string
		m.picker, selected, cmd = m.picker.Update(msg)
		if selected != "" {
			m.paths[m.pickerFor] = displayPath(selected)
			m.controls.SetValue(setupIDs[m.pickerFor], m.paths[m.pickerFor])
			m.picking = false
		}
		return m, cmd
	}
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.stage == setupStage {
		return m.clickSetup(msg.X, msg.Y)
	}
	if m.stage == resultStage {
		return m.updateResultMouse(msg)
	}
	if m.stage == processingStage && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y >= m.height-4 {
		leftWidth, _ := landscapeColumnWidths(m.width)
		navigation := pageactions.Config{Prev: &pageactions.Action{Destination: "Parameters"}}
		if m.processingComplete() {
			navigation.Next = &pageactions.Action{Destination: "Result"}
		}
		switch pageactions.Hit(navigation, leftWidth, msg.X, msg.Y-(m.height-4)) {
		case pageactions.Prev:
			m.beginLeaveConfirmation(false)
		case pageactions.Next:
			m.openResult()
		}
		return m, nil
	}
	if m.stage != parameterStage || m.width < 63 {
		return m, nil
	}
	m.registerParameterBounds()
	if m.menu.IsOpen() {
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			if item, ok := m.menu.Click(msg.X, msg.Y); ok {
				return m.runMenuAction(item.ID)
			}
			m.menu.Close()
		}
		return m, nil
	}
	hit := m.parameterFields.HitAt(msg.X, msg.Y)
	if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		delta := -3
		if msg.Button == tea.MouseButtonWheelDown {
			delta = 3
		}
		m.scrollResultList(hit, delta)
		return m, nil
	}
	if msg.Button == tea.MouseButtonRight && msg.Action == tea.MouseActionPress {
		if m.selectHoveredResult(hit, msg.Y) {
			m.menuTarget = hit
			x, y := msg.X, msg.Y
			x = min(x, max(0, m.width-m.menu.Width()))
			y = min(y, max(0, m.height-m.menu.Height()))
			m.menu.Open(x, y)
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if msg.Y >= m.height-4 {
		return m.clickParameters(msg.X, msg.Y)
	}
	m.selectHoveredResult(hit, msg.Y)
	if m.parameterFields.FocusAt(msg.X, msg.Y) {
		m.pendingListG = false
		if m.parameterFields.Current() == "parameters" {
			return m.clickParameters(msg.X, msg.Y)
		}
	}
	return m, nil
}

func (m importModel) updateResultMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.width < 80 || m.height < 16 {
		return m, nil
	}
	lowerY := lipgloss.Height(fieldset.View("Import result", m.resultHeaderContent(), m.width)) + 1
	lowerHeight := max(8, m.height-lowerY)
	leftWidth, rightWidth := landscapeColumnWidths(m.width)
	cleanupHeight := 5
	integrityHeight := max(5, lowerHeight-cleanupHeight-5)
	cleanupY := lowerY + integrityHeight + 1
	m.resultFields.SetBounds("result-actions", datafield.Bounds{X: 0, Y: cleanupY, Width: leftWidth, Height: cleanupHeight})
	m.resultFields.SetBounds("result-files", datafield.Bounds{X: leftWidth + 1, Y: lowerY, Width: rightWidth, Height: lowerHeight})
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y >= m.height-4 {
		navigation := resultPageActions()
		switch pageactions.Hit(navigation, leftWidth, msg.X, msg.Y-(m.height-4)) {
		case pageactions.Prev:
			if !m.cleanupResultState() {
				return m, nil
			}
			m.restartImport()
		case pageactions.Next:
			m.beginLeaveConfirmation(true)
		}
		return m, nil
	}
	hit := m.resultFields.HitAt(msg.X, msg.Y)
	if hit == "result-files" && msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		delta := -3
		if msg.Button == tea.MouseButtonWheelDown {
			delta = 3
		}
		m.resultList.Scroll(delta)
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress || !m.resultFields.FocusAt(msg.X, msg.Y) {
		return m, nil
	}
	if hit == "result-files" {
		m.resultList.SelectRow(msg.Y - lowerY - 2)
		return m, nil
	}
	m.controls.SetFocusID(deleteStateID)
	m.controls.Click([]string{deleteStateID}, msg.X-2, msg.Y-cleanupY-1)
	return m, nil
}

func (m importModel) clickSetup(x, y int) (tea.Model, tea.Cmd) {
	contentWidth := min(tui.DefaultContentWidth, max(24, m.width-4))
	contentHeight := 11
	topPadding := max(0, (max(1, m.height-4)-contentHeight)/2)
	left := max(0, (m.width-contentWidth)/2)
	if id, ok := m.controls.Click([]string{sourceID, destinationID}, x-left-2, y-topPadding-7); ok {
		if id == sourceID {
			return m.openPicker(sourceField)
		}
		if id == destinationID {
			return m.openPicker(destinationField)
		}
	}
	if pageactions.Hit(pageactions.Config{Next: &pageactions.Action{Destination: "Parameters"}}, m.width, x, y-(m.height-4)) == pageactions.Next {
		return m.startScan()
	}
	return m, nil
}

func (m importModel) clickParameters(x, y int) (tea.Model, tea.Cmd) {
	layout := m.parameterLayout()
	localX := x - 2
	importIDs := []string{operationID, extensionsID, duplicatesID, parallelID}
	if _, ok := m.controls.Click(importIDs, localX, y-2); ok {
		m.syncResultLists()
		return m, nil
	}
	if y >= m.height-4 {
		navigation := pageactions.Config{
			Prev: &pageactions.Action{Destination: "Directories"},
			Next: &pageactions.Action{Destination: "Processing"},
		}
		switch pageactions.Hit(navigation, layout.leftWidth, x, y-(m.height-4)) {
		case pageactions.Prev:
			m.stage = setupStage
			m.controls.SetFocusID(destinationID)
			return m, nil
		case pageactions.Next:
			return m.startProcessing()
		}
	}
	return m, nil
}

func (m *importModel) registerParameterBounds() {
	layout := m.parameterLayout()
	m.parameterFields.SetBounds("parameters", datafield.Bounds{X: 0, Y: 0, Width: layout.leftWidth, Height: layout.parameterHeight})
	m.parameterFields.SetBounds("summary", datafield.Bounds{X: 0, Y: layout.parameterHeight + 1, Width: layout.leftWidth, Height: layout.summaryHeight})
	m.parameterFields.SetBounds("source-results", datafield.Bounds{X: layout.leftWidth + 1, Y: 0, Width: layout.rightWidth, Height: layout.sourceHeight})
	m.parameterFields.SetBounds("destination-results", datafield.Bounds{X: layout.leftWidth + 1, Y: layout.sourceHeight + 1, Width: layout.rightWidth, Height: layout.destinationHeight})
}

func (m *importModel) scrollResultList(id string, delta int) {
	layout := m.parameterLayout()
	switch id {
	case "source-results":
		m.sourceList.SetSize(max(1, layout.rightWidth-4), max(1, layout.sourceHeight-4))
		m.sourceList.Scroll(delta)
	case "destination-results":
		m.destinationList.SetSize(max(1, layout.rightWidth-4), max(1, layout.destinationHeight-4))
		m.destinationList.Scroll(delta)
	}
}

func (m importModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.leaveConfirm {
		var decision confirm.Decision
		m.leaveDialog, decision = m.leaveDialog.Update(key)
		switch decision {
		case confirm.Confirmed:
			m.leaveConfirm = false
			if m.stage == resultStage && !m.cleanupResultState() {
				return m, nil
			}
			if m.transferCancel != nil && !m.processingComplete() {
				m.pendingExit = m.leaveExits
				m.pendingReturn = !m.leaveExits
				m.transferCancel()
				return m, nil
			}
			if m.leaveExits {
				return m, tea.Quit
			}
			m.stage = parameterStage
		case confirm.Cancelled:
			m.leaveConfirm = false
			if !m.leaveWasPaused && !m.processingComplete() {
				m.processing.paused = false
				m.transferController.Resume()
			}
		}
		return m, nil
	}
	if m.menu.IsOpen() {
		switch key {
		case "esc":
			m.menu.Close()
		case "up", "k":
			m.menu.Move(-1)
		case "down", "j":
			m.menu.Move(1)
		case "enter":
			if item, ok := m.menu.Selected(); ok {
				m.menu.Close()
				return m.runMenuAction(item.ID)
			}
		}
		return m, nil
	}
	if m.stage == scanStage {
		if key == "esc" {
			m.scanGeneration++
			m.stage = setupStage
			m.controls.SetFocusID(destinationID)
		}
		return m, nil
	}
	if m.stage == processingStage {
		switch key {
		case "esc", "q", "ctrl+c":
			m.beginLeaveConfirmation(key != "esc")
		case "p":
			m.processing.paused = !m.processing.paused
			if m.processing.paused {
				m.transferController.Pause()
			} else {
				m.transferController.Resume()
			}
		case "up", "k":
			m.processing.workerOffset = max(0, m.processing.workerOffset-1)
		case "down", "j":
			m.processing.workerOffset = min(max(0, len(m.processing.workers)-1), m.processing.workerOffset+1)
		case "g", "home":
			m.processing.workerOffset = 0
		case "G", "end":
			m.processing.workerOffset = max(0, len(m.processing.workers)-1)
		case "enter", form.PrimaryActionKey:
			if m.processingComplete() {
				m.openResult()
			}
		}
		return m, nil
	}
	if m.stage == resultStage {
		if key == "esc" || key == "q" || key == "ctrl+c" {
			m.beginLeaveConfirmation(true)
			return m, nil
		}
		if key == "r" {
			if !m.cleanupResultState() {
				return m, nil
			}
			m.restartImport()
			return m, nil
		}
		if m.resultFields.Move(key) {
			if m.resultFields.Current() == "result-actions" {
				m.controls.SetFocusID(deleteStateID)
			}
			return m, nil
		}
		if m.resultFields.Current() == "result-files" {
			switch key {
			case "up", "k":
				m.resultList.Move(-1)
			case "down", "j":
				m.resultList.Move(1)
			case "g", "home":
				m.resultList.First()
			case "G", "end":
				m.resultList.Last()
			case "left", "h":
				m.resultList.Pan(-4)
			case "right", "l":
				m.resultList.Pan(4)
			}
			return m, nil
		}
		if m.controls.HandleInteraction(key) {
			return m, nil
		}
		return m, nil
	}
	if m.stage == parameterStage && key == "esc" {
		m.stage = setupStage
		m.controls.SetFocusID(destinationID)
		return m, nil
	}
	if m.picking {
		if key == "esc" && !m.picker.HasDialog() {
			m.picking = false
			return m, nil
		}
		var selected string
		var cmd tea.Cmd
		m.picker, selected, cmd = m.picker.Update(msg)
		if selected != "" {
			m.paths[m.pickerFor] = displayPath(selected)
			if m.pickerFor == sourceField {
				m.controls.SetValue(sourceID, m.paths[m.pickerFor])
			} else {
				m.controls.SetValue(destinationID, m.paths[m.pickerFor])
			}
			m.picking = false
		}
		return m, cmd
	}

	if m.stage == parameterStage && m.parameterFields.Move(key) {
		if m.parameterFields.Current() == "parameters" {
			m.controls.SetFocusID(operationID)
		}
		return m, nil
	}
	if m.stage == parameterStage && m.listFocused() {
		if key == " " {
			return m.openSelected(true)
		}
		if m.updateListNavigation(key) {
			return m, nil
		}
	}
	controlsActive := m.stage == setupStage || m.parameterFields.Current() == "parameters"
	if controlsActive && m.controls.HandleInteraction(key) {
		if m.stage == parameterStage {
			m.syncResultLists()
		}
		m.help = false
		return m, nil
	}
	visibleIDs := setupIDs
	if m.stage == parameterStage {
		visibleIDs = parameterIDs
	}
	if controlsActive && m.controls.UpdateNavigationWithin(visibleIDs, key) {
		m.help = false
		return m, nil
	}
	switch key {
	case "enter":
		m.help = false
		switch m.controls.FocusedID() {
		case sourceID:
			return m.openPicker(sourceField)
		case destinationID:
			return m.openPicker(destinationField)
		}
	case form.PrimaryActionKey:
		m.help = false
		if m.stage == setupStage {
			return m.startScan()
		}
		return m.startProcessing()
	case "?":
		m.help = !m.help
	}
	return m, nil
}

func (m importModel) listFocused() bool {
	current := m.parameterFields.Current()
	return current == "source-results" || current == "destination-results"
}

func (m *importModel) updateListNavigation(key string) bool {
	list := m.activeResultList()
	if list == nil {
		return false
	}
	m.sizeResultList(list, m.parameterFields.Current())
	if key == "g" {
		if m.pendingListG {
			list.First()
			m.pendingListG = false
		} else {
			m.pendingListG = true
		}
		return true
	}
	m.pendingListG = false
	switch key {
	case "down", "j":
		list.Move(1)
	case "up", "k":
		list.Move(-1)
	case "G":
		list.Last()
	case "left", "h":
		list.Pan(-2)
	case "right", "l":
		list.Pan(2)
	default:
		return false
	}
	return true
}

func (m importModel) resultListWidth() int {
	_, rightWidth := landscapeColumnWidths(m.width)
	return max(1, rightWidth-4)
}

func (m *importModel) activeResultList() *scrolllist.Model {
	if m.parameterFields.Current() == "source-results" {
		return &m.sourceList
	}
	if m.parameterFields.Current() == "destination-results" {
		return &m.destinationList
	}
	return nil
}

func (m *importModel) sizeResultList(list *scrolllist.Model, id string) {
	layout := m.parameterLayout()
	height := layout.sourceHeight
	if id == "destination-results" {
		height = layout.destinationHeight
	}
	list.SetSize(max(1, layout.rightWidth-4), max(1, height-4))
}

func (m *importModel) syncResultLists() {
	toItems := func(files []scannedFile) []scrolllist.Item {
		items := make([]scrolllist.Item, len(files))
		for index, file := range files {
			items[index] = scrolllist.Item{ID: file.path, Label: file.path}
		}
		return items
	}
	m.sourceList.SetItems(toItems(m.filteredSource()))
	m.destinationList.SetItems(toItems(m.scan.destination))
}

func (m *importModel) configureExtensions() {
	seen := make(map[string]struct{})
	for _, file := range m.scan.source {
		seen[fileExtension(file.path)] = struct{}{}
	}
	extensions := make([]string, 0, len(seen))
	for extension := range seen {
		extensions = append(extensions, extension)
	}
	sort.Strings(extensions)
	m.controls.SetOptions(extensionsID, extensions, true)
	m.extensionsConfigured = true
}

func (m importModel) filteredSource() []scannedFile {
	if !m.extensionsConfigured {
		return m.scan.source
	}
	selected := make(map[string]struct{})
	for _, extension := range m.controls.Values(extensionsID) {
		selected[extension] = struct{}{}
	}
	files := make([]scannedFile, 0, len(m.scan.source))
	for _, file := range m.scan.source {
		if _, ok := selected[fileExtension(file.path)]; ok {
			files = append(files, file)
		}
	}
	return files
}

func fileExtension(path string) string {
	extension := strings.TrimPrefix(strings.ToUpper(filepath.Ext(path)), ".")
	if extension == "" {
		return "NO EXTENSION"
	}
	return extension
}

func (m *importModel) selectHoveredResult(id string, mouseY int) bool {
	layout := m.parameterLayout()
	switch id {
	case "source-results":
		m.sizeResultList(&m.sourceList, id)
		return m.sourceList.SelectRow(mouseY - 3)
	case "destination-results":
		m.sizeResultList(&m.destinationList, id)
		return m.destinationList.SelectRow(mouseY - layout.sourceHeight - 4)
	default:
		return false
	}
}

func (m importModel) selectedPath(target string) (string, bool) {
	list, root := m.sourceList, m.paths[sourceField]
	if target == "destination-results" {
		list, root = m.destinationList, m.paths[destinationField]
	}
	item, ok := list.Selected()
	if !ok {
		return "", false
	}
	return filepath.Join(expandHome(root), filepath.FromSlash(item.ID)), true
}

func (m importModel) openSelected(quickLook bool) (tea.Model, tea.Cmd) {
	path, ok := m.selectedPath(m.parameterFields.Current())
	if !ok {
		return m, nil
	}
	m.actionNotice = "Opening " + filepath.Base(path)
	return m, openFileCmd(path, quickLook)
}

func (m importModel) runMenuAction(id string) (tea.Model, tea.Cmd) {
	if id != "open" {
		return m, nil
	}
	path, ok := m.selectedPath(m.menuTarget)
	if !ok {
		return m, nil
	}
	m.actionNotice = "Opening " + filepath.Base(path)
	return m, openFileCmd(path, false)
}

func openFileCmd(path string, quickLook bool) tea.Cmd {
	return func() tea.Msg {
		var command *exec.Cmd
		switch {
		case runtime.GOOS == "darwin" && quickLook:
			command = exec.Command("qlmanage", "-p", path)
		case runtime.GOOS == "darwin":
			command = exec.Command("open", path)
		default:
			command = exec.Command("xdg-open", path)
		}
		return command.Start()
	}
}

func (m importModel) startProcessing() (tea.Model, tea.Cmd) {
	if m.controls.Value(duplicatesID) == "Replace" {
		m.actionNotice = "Replace is unavailable until atomic backup and rollback are implemented"
		return m, nil
	}
	files := append([]scannedFile(nil), m.filteredSource()...)
	jobs := make([]importer.Job, 0, len(files))
	sourceRoot := expandHome(m.paths[sourceField])
	destinationRoot := expandHome(m.paths[destinationField])
	for _, file := range files {
		jobs = append(jobs, importer.Job{
			Source:      filepath.Join(sourceRoot, filepath.FromSlash(file.path)),
			Destination: filepath.Join(destinationRoot, filepath.Base(filepath.FromSlash(file.path))),
		})
	}
	operation := importer.Copy
	if m.controls.Value(operationID) == "Move" {
		operation = importer.Move
	}
	conflict := importer.Skip
	if m.controls.Value(duplicatesID) == "Keep both" {
		conflict = importer.KeepBoth
	}
	controller := &importer.Controller{}
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan importer.Event, max(16, len(jobs)))
	updates := make(chan tea.Msg, max(16, len(jobs)))
	plan := importer.Plan{
		Jobs: jobs, Operation: operation, Conflict: conflict,
		Workers:    max(1, m.controls.IntValue(parallelID)),
		StatePath:  filepath.Join(destinationRoot, m.stateFilename),
		Controller: controller,
	}
	m.stage = processingStage
	m.actionNotice = ""
	m.processing = processingState{files: files, finished: make(map[int]importer.Phase)}
	count := max(1, m.controls.IntValue(parallelID))
	m.processing.workers = make([]processingWorker, count)
	for index := range m.processing.workers {
		m.processing.workers[index] = processingWorker{number: index + 1, jobIndex: -1, phase: workerIdle}
	}
	m.transferCancel = cancel
	m.transferController = controller
	m.transferUpdates = updates
	go func() {
		resultChannel := make(chan importer.Result, 1)
		go func() {
			resultChannel <- importer.Run(ctx, plan, events)
			close(events)
		}()
		for event := range events {
			updates <- transferEventMsg(event)
		}
		updates <- transferDoneMsg(<-resultChannel)
		close(updates)
	}()
	return m, waitTransferUpdate(updates)
}

func (m *importModel) openResult() {
	items := make([]scrolllist.Item, 0, len(m.processing.results))
	for _, result := range m.processing.results {
		label := fmt.Sprintf("✓  %s  ·  %s  ·  SHA-256 MATCH", filepath.Base(result.Destination), formatBytes(result.Size))
		if result.Phase == importer.PhaseSkipped {
			label = fmt.Sprintf("–  %s  ·  skipped", filepath.Base(result.Destination))
		}
		if result.Phase == importer.PhaseFailed {
			label = fmt.Sprintf("!  %s  ·  %s", filepath.Base(result.Source), result.Error)
		}
		items = append(items, scrolllist.Item{
			ID: result.Source, Label: label,
		})
	}
	m.resultList.SetItems(items)
	m.resultFields.Set("result-files")
	m.stage = resultStage
}

func (m importModel) processingComplete() bool {
	return m.processing.complete
}

func (m *importModel) workerForJob(index int) *processingWorker {
	for workerIndex := range m.processing.workers {
		if m.processing.workers[workerIndex].jobIndex == index {
			return &m.processing.workers[workerIndex]
		}
	}
	for workerIndex := range m.processing.workers {
		if m.processing.workers[workerIndex].jobIndex < 0 {
			m.processing.workers[workerIndex].jobIndex = index
			if index >= 0 && index < len(m.processing.files) {
				m.processing.workers[workerIndex].file = m.processing.files[index]
			}
			return &m.processing.workers[workerIndex]
		}
	}
	return nil
}

func (m *importModel) applyTransferEvent(event importer.Event) {
	worker := m.workerForJob(event.Index)
	if worker == nil {
		return
	}
	switch event.Phase {
	case importer.PhaseCopying:
		worker.phase = workerCopying
	case importer.PhaseVerifying:
		worker.phase = workerVerifying
	case importer.PhasePublishing, importer.PhaseDeletingSource:
		worker.phase = workerPublishing
	case importer.PhaseComplete, importer.PhaseSkipped, importer.PhaseFailed:
		m.processing.finished[event.Index] = event.Phase
		if event.Phase == importer.PhaseComplete && event.Index >= 0 && event.Index < len(m.processing.files) {
			m.processing.verified = append(m.processing.verified, m.processing.files[event.Index])
		}
		if event.Phase == importer.PhaseSkipped {
			m.processing.skipped++
		}
		if event.Phase == importer.PhaseFailed {
			m.processing.failed++
		}
		worker.phase = workerDone
		worker.jobIndex = -1
	}
	if event.Total > 0 {
		worker.progress = min(100, int(event.Bytes*100/event.Total))
	}
	worker.sourceHash = event.SourceHash
	worker.destinationHash = event.DestinationHash
}

func (m *importModel) applyTransferResult(result importer.Result) {
	m.processing.results = result.Files
	m.processing.verified = nil
	m.processing.failed, m.processing.skipped = 0, 0
	for _, item := range result.Files {
		if item.Phase == importer.PhaseComplete && item.Index >= 0 && item.Index < len(m.processing.files) {
			m.processing.verified = append(m.processing.verified, m.processing.files[item.Index])
		}
		if item.Phase == importer.PhaseFailed {
			m.processing.failed++
		}
		if item.Phase == importer.PhaseSkipped {
			m.processing.skipped++
		}
	}
	for index := range m.processing.workers {
		m.processing.workers[index].phase = workerDone
		m.processing.workers[index].jobIndex = -1
	}
	m.processing.complete = true
	m.processing.paused = false
}

func (m importModel) startScan() (tea.Model, tea.Cmd) {
	m.stage = scanStage
	m.scanGeneration++
	m.progress = 0
	generation := m.scanGeneration
	paths := m.paths
	return m, tea.Batch(scanCmd(generation, paths), scanTick())
}

func (m importModel) openPicker(field int) (tea.Model, tea.Cmd) {
	width, height := m.modalSize()
	m.picker = fileexplorer.New(
		pickerStart(m.paths[field]),
		width-2,
		height-6,
		fileexplorer.WithFilter(fileexplorer.Directories()),
	)
	m.pickerFor = field
	m.picking = true
	return m, m.picker.Init()
}

func (m *importModel) sizePicker() {
	width, height := m.modalSize()
	m.picker.SetSize(width-2, height-6)
}

func (m importModel) modalSize() (int, int) {
	width := min(92, m.width-4)
	height := min(30, m.height-2)
	return max(20, width), max(8, height)
}

func pickerStart(path string) string {
	path = expandHome(path)
	for path != "" {
		info, err := os.Stat(path)
		if err == nil {
			if info.IsDir() {
				return path
			}
			return filepath.Dir(path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return "."
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

func displayPath(path string) string {
	cleaned := filepath.Clean(path)
	home, err := os.UserHomeDir()
	if err == nil && (cleaned == home || strings.HasPrefix(cleaned, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(cleaned, home)
	}
	return cleaned
}

func (m importModel) View() string {
	if m.stage == resultStage {
		return m.resultView()
	}
	if m.stage == processingStage {
		return m.processingView()
	}
	if m.stage == scanStage {
		return m.scanView()
	}
	content := m.setupView()
	if m.stage == parameterStage {
		content = m.parameterView()
	}
	if m.help && !m.picking {
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			importTitleStyle.Render("PHOTO IMPORT HELP"),
			"",
			"Arrows or h/j/k/l  Move between controls",
			"Tab / Shift+Tab    Move forward / backward",
			"Enter              Select the focused control",
			"Esc         Return to commands",
			"q           Quit dgs",
		)
	}
	workspace := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, content)
	if m.stage == setupStage {
		workspace = overlay.PlaceAt(workspace, pageactions.View(pageactions.Config{
			Next: &pageactions.Action{Destination: "Parameters"},
		}, m.width), 0, max(0, m.height-4), m.width, m.height)
	}
	if m.picking {
		return overlay.Place(workspace, m.pickerView(), m.width, m.height)
	}
	return workspace
}

func (m importModel) resultView() string {
	if m.width < 80 || m.height < 16 {
		content := lipgloss.JoinVertical(
			lipgloss.Left,
			importStepStyle.Render("RESULT  ·  PHOTO IMPORT"),
			importHeadingStyle.Render("More space required"),
			importNoteStyle.Render("Use at least 80 columns and 16 workspace rows for the landscape result view."),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	bytes := m.publishedBytes()
	headerContent := m.resultHeaderContent()
	header := fieldset.View("Import result", headerContent, m.width)

	lowerHeight := max(8, m.height-lipgloss.Height(header)-1)
	leftWidth, rightWidth := landscapeColumnWidths(m.width)
	filesFocused := m.resultFields.Current() == "result-files"
	actionsFocused := m.resultFields.Current() == "result-actions"
	results := m.resultList
	results.SetSize(rightWidth-4, lowerHeight-3)
	leftContent := importNoteStyle.Render(fmt.Sprintf("%d verified files · %s", len(m.processing.verified), formatBytes(bytes))) + "\n" +
		results.View(filesFocused, importSectionStyle, importNoteStyle)
	filesPane := fieldset.ViewFocused("Verified files", leftContent, rightWidth, filesFocused)

	integrity := strings.Join([]string{
		importSectionStyle.Render("INTEGRITY"),
		"✓ Source stream hashed",
		"✓ Destination read back independently",
		"✓ SHA-256 digests matched",
		"✓ Final names published after verification",
		"",
		importSectionStyle.Render("TRANSFER"),
		fmt.Sprintf("Workers       %d", len(m.processing.workers)),
		fmt.Sprintf("Verified      %d files", len(m.processing.verified)),
		fmt.Sprintf("Skipped       %d files", m.processing.skipped),
		fmt.Sprintf("Failed        %d files", m.processing.failed),
	}, "\n")
	actions := m.controls.ViewFocusedWidth([]string{deleteStateID}, actionsFocused, leftWidth-4)
	cleanupNote := importNoteStyle.Render("Delete is the default. Retain state only for audit or diagnosis.")
	if m.actionNotice != "" {
		cleanupNote = importNoteStyle.Render(m.actionNotice)
	}
	cleanup := actions + "\n\n" + cleanupNote
	cleanupHeight := 5
	integrityHeight := max(5, lowerHeight-cleanupHeight-5)
	actionsPane := lipgloss.JoinVertical(
		lipgloss.Left,
		fieldset.View("Verification summary", fitContentHeight(integrity, integrityHeight-2, leftWidth-4), leftWidth),
		"",
		fieldset.ViewFocused("State file", fitContentHeight(cleanup, cleanupHeight-2, leftWidth-4), leftWidth, actionsFocused),
		pageactions.View(resultPageActions(), leftWidth),
	)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", lipgloss.JoinHorizontal(lipgloss.Top, actionsPane, " ", filesPane))
	if m.leaveConfirm {
		view = overlay.Place(view, m.leaveConfirmationView(), m.width, m.height)
	}
	return view
}

func (m importModel) scanView() string {
	width := min(tui.DefaultContentWidth, max(24, m.width-4))
	barWidth := max(12, width-4)
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		importStepStyle.Render("SCAN  ·  PHOTO IMPORT"),
		importHeadingStyle.Render("Reading source and destination"),
		importNoteStyle.Render("Collecting file names and counts. No files are being changed."),
		"",
		fmt.Sprintf("%-13s %s", "Source", ansi.Truncate(m.paths[sourceField], max(1, width-15), "…")),
		fmt.Sprintf("%-13s %s", "Destination", ansi.Truncate(m.paths[destinationField], max(1, width-15), "…")),
		"",
		scanProgressView(m.progress, barWidth),
		"",
		importNoteStyle.Render("Esc  Cancel scan"),
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func resultPageActions() pageactions.Config {
	return pageactions.Config{
		Prev: &pageactions.Action{Title: "↻ Again  r", Destination: "New import"},
		Next: &pageactions.Action{Title: "Quit  q", Destination: "Exit dgs"},
	}
}

func (m *importModel) restartImport() {
	width, height, paths, stateFilename := m.width, m.height, m.paths, m.stateFilename
	fresh := newImportModelWithStateFile(stateFilename).(importModel)
	fresh.width, fresh.height, fresh.paths = width, height, paths
	fresh.controls.SetValue(sourceID, paths[sourceField])
	fresh.controls.SetValue(destinationID, paths[destinationField])
	*m = fresh
}

func (m *importModel) cleanupResultState() bool {
	if !m.controls.Checked(deleteStateID) {
		return true
	}
	path := filepath.Join(expandHome(m.paths[destinationField]), m.stateFilename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		m.actionNotice = "Could not delete " + path + ": " + err.Error()
		return false
	}
	m.actionNotice = "Deleted " + path
	return true
}

func (m importModel) publishedBytes() int64 {
	var bytes int64
	for _, result := range m.processing.results {
		if result.Phase == importer.PhaseComplete {
			bytes += result.Size
		}
	}
	return bytes
}

func (m importModel) resultHeaderContent() string {
	return strings.Join([]string{
		"TRANSFER COMPLETE  ·  VERIFIED BEFORE PUBLISH",
		"Source       " + m.paths[sourceField],
		"Destination  " + m.paths[destinationField],
		fmt.Sprintf("%d/%d files passed Source and Destination SHA-256 comparison", len(m.processing.verified), len(m.processing.files)),
		fmt.Sprintf("Published %s   Skipped %d   Failed %d", formatBytes(m.publishedBytes()), m.processing.skipped, m.processing.failed),
	}, "\n")
}

func scanProgressView(percent float64, width int) string {
	width = max(8, width)
	filled := int(float64(width-2) * percent)
	bar := strings.Repeat("━", filled) + strings.Repeat("─", max(0, width-2-filled))
	return importSectionStyle.Render("▐"+bar) + importNoteStyle.Render("▌")
}

func (m importModel) processingView() string {
	if m.width < 80 || m.height < 16 {
		content := lipgloss.JoinVertical(
			lipgloss.Left,
			importStepStyle.Render("PROCESSING  ·  PHOTO IMPORT"),
			importHeadingStyle.Render("More space required"),
			importNoteStyle.Render("Use at least 80 columns and 16 workspace rows for the landscape processing view."),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	total := len(m.processing.files)
	verified := len(m.processing.verified)
	percent := m.processingPercent()
	state := "RUNNING"
	if m.processing.paused {
		state = "PAUSED"
	}
	if m.processingComplete() {
		state = "COMPLETE"
	}
	progressInfo := fmt.Sprintf("%3d%% │ %d/%d verified", percent, verified, total)
	progressWidth := max(8, m.width-lipgloss.Width(progressInfo)-7)
	headerContent := strings.Join([]string{
		fmt.Sprintf("%s  ·  SHA-256 VERIFIED BEFORE PUBLISH", state),
		fmt.Sprintf("Files %d/%d verified   Workers %d   Active %d   Pending %d   Skipped %d   Failed %d", verified, total, len(m.processing.workers), m.activeWorkerCount(), max(0, total-verified-m.processing.skipped-m.processing.failed-m.activeWorkerCount()), m.processing.skipped, m.processing.failed),
		compactProgressBar(percent, progressWidth) + " " + progressInfo,
	}, "\n")
	header := fieldset.View("Import progress", headerContent, m.width)

	lowerHeight := max(8, m.height-lipgloss.Height(header)-1)
	leftWidth, rightWidth := landscapeColumnWidths(m.width)
	workers := fieldset.View("Workers", fitContentHeight(m.workerRows(rightWidth-4, lowerHeight-2), lowerHeight-2, rightWidth-4), rightWidth)
	leftTopHeight := max(4, lowerHeight/2)
	leftBottomHeight := max(4, lowerHeight-leftTopHeight-5)
	navigation := pageactions.Config{Prev: &pageactions.Action{Destination: "Parameters"}}
	if m.processingComplete() {
		navigation.Next = &pageactions.Action{Destination: "Result"}
	}
	queuePane := lipgloss.JoinVertical(
		lipgloss.Left,
		fieldset.View("Next files", fitContentHeight(m.pendingRows(leftWidth-4), leftTopHeight-2, leftWidth-4), leftWidth),
		"",
		fieldset.View("Recent results", fitContentHeight(m.recentRows(leftWidth-4), leftBottomHeight-2, leftWidth-4), leftWidth),
		pageactions.View(navigation, leftWidth),
	)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", lipgloss.JoinHorizontal(lipgloss.Top, queuePane, " ", workers))
	if m.leaveConfirm {
		view = overlay.Place(view, m.leaveConfirmationView(), m.width, m.height)
	}
	return view
}

func (m importModel) processingPercent() int {
	if len(m.processing.files) == 0 {
		return 100
	}
	units := len(m.processing.verified) * 100
	for _, worker := range m.processing.workers {
		switch worker.phase {
		case workerCopying:
			units += worker.progress * 60 / 100
		case workerVerifying:
			units += 60 + worker.progress*35/100
		case workerPublishing:
			units += 100
		}
	}
	return min(100, units/len(m.processing.files))
}

func (m importModel) activeWorkerCount() int {
	count := 0
	for _, worker := range m.processing.workers {
		if worker.phase != workerDone && worker.phase != workerIdle {
			count++
		}
	}
	return count
}

func (m importModel) workerRows(width, height int) string {
	rowsPerWorker := 4
	visible := max(1, height/rowsPerWorker)
	maxOffset := max(0, len(m.processing.workers)-visible)
	offset := min(maxOffset, max(0, m.processing.workerOffset))
	end := min(len(m.processing.workers), offset+visible)
	lines := make([]string, 0, visible*rowsPerWorker)
	if offset > 0 {
		lines = append(lines, importNoteStyle.Render("↑ more workers"))
	}
	for _, worker := range m.processing.workers[offset:end] {
		if worker.phase == workerDone {
			lines = append(lines,
				fmt.Sprintf("%02d  %-10s", worker.number, "IDLE"),
				importNoteStyle.Render("    Waiting for work"),
				"",
				"",
			)
			continue
		}
		fileSize := formatBytes(worker.file.size)
		filePrefix := fmt.Sprintf("%02d  %-10s  ", worker.number, workerPhaseLabel(worker.phase))
		fileWidth := max(8, width-lipgloss.Width(filePrefix)-lipgloss.Width(" · "+fileSize))
		name := ansi.Truncate(filepath.Base(worker.file.path), fileWidth, "...")
		sourceHash, destinationHash := "computing", "waiting"
		if worker.phase >= workerVerifying {
			sourceHash = shortHash(worker.sourceHash, "ready")
		}
		if worker.phase == workerVerifying {
			destinationHash = "computing"
		}
		if worker.phase == workerPublishing {
			destinationHash = shortHash(worker.destinationHash, "MATCH")
		}
		barWidth := max(8, width-11)
		lines = append(lines,
			filePrefix+name+" · "+fileSize,
			"    "+workerSteps(worker.phase),
			importNoteStyle.Render(fmt.Sprintf("    Source %s  │  Destination %s", sourceHash, destinationHash)),
			fmt.Sprintf("    %s %3d%%", compactProgressBar(worker.progress, barWidth), worker.progress),
		)
	}
	if end < len(m.processing.workers) && len(lines) < height {
		lines = append(lines, importNoteStyle.Render("↓ more workers"))
	}
	return strings.Join(lines, "\n")
}

func workerSteps(phase workerPhase) string {
	current := 0
	switch phase {
	case workerVerifying:
		current = 1
	case workerPublishing:
		current = 2
	case workerDone:
		current = 3
	}
	return stepper.View([]string{"COPY", "VERIFY", "PUBLISH"}, current, 999)
}

func (m importModel) leaveConfirmationView() string {
	return m.leaveDialog.View(m.width)
}

func (m *importModel) beginLeaveConfirmation(exits bool) {
	m.leaveWasPaused = m.processing.paused
	if !m.processing.paused && !m.processingComplete() {
		m.processing.paused = true
		m.transferController.Pause()
	}
	m.leaveConfirm = true
	m.leaveExits = exits
	m.leaveDialog = m.newLeaveDialog()
}

func (m importModel) newLeaveDialog() confirm.Model {
	if m.stage == resultStage {
		detail := "The import-state file will be retained."
		if m.controls.Checked(deleteStateID) {
			detail = "The selected " + m.stateFilename + " file will be deleted before exit."
		}
		return confirm.New(confirm.Config{
			Title:        "EXIT DGS?",
			Message:      "The verified import is complete.",
			Detail:       detail,
			ConfirmLabel: "Yes",
			CancelLabel:  "No",
		})
	}
	title := "RETURN TO PARAMETERS?"
	message := "Processing will stop at its current state."
	if m.leaveExits {
		title = "EXIT DGS?"
		message = "Processing will stop before the application exits."
	}
	return confirm.New(confirm.Config{
		Title:        title,
		Message:      message,
		Detail:       "Verified files stay published; the current partial file stays unpublished.",
		ConfirmLabel: "Yes",
		CancelLabel:  "No",
	})
}

func workerPhaseLabel(phase workerPhase) string {
	switch phase {
	case workerCopying:
		return "COPYING"
	case workerVerifying:
		return "VERIFYING"
	case workerPublishing:
		return "PUBLISHING"
	default:
		return "IDLE"
	}
}

func (m importModel) pendingRows(width int) string {
	active := make(map[int]struct{}, len(m.processing.workers))
	for _, worker := range m.processing.workers {
		if worker.jobIndex >= 0 {
			active[worker.jobIndex] = struct{}{}
		}
	}
	lines := make([]string, 0, len(m.processing.files))
	for index := range m.processing.files {
		if _, done := m.processing.finished[index]; done {
			continue
		}
		if _, running := active[index]; running {
			continue
		}
		lines = append(lines, fmt.Sprintf("%3d  %s", index+1, ansi.Truncate(filepath.Base(m.processing.files[index].path), max(1, width-5), "...")))
	}
	if len(lines) == 0 {
		return importNoteStyle.Render("No pending files")
	}
	return strings.Join(lines, "\n")
}

func shortHash(hash, fallback string) string {
	if hash == "" {
		return fallback
	}
	return hash[:min(8, len(hash))]
}

func (m importModel) recentRows(width int) string {
	if len(m.processing.verified) == 0 {
		return importNoteStyle.Render("Waiting for verification")
	}
	start := max(0, len(m.processing.verified)-6)
	lines := make([]string, 0, len(m.processing.verified)-start)
	for index := len(m.processing.verified) - 1; index >= start; index-- {
		lines = append(lines, "OK  "+ansi.Truncate(filepath.Base(m.processing.verified[index].path), max(1, width-4), "..."))
	}
	return strings.Join(lines, "\n")
}

func compactProgressBar(percent, width int) string {
	width = max(8, width)
	filled := width * min(100, max(0, percent)) / 100
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func (m importModel) pickerView() string {
	width, height := m.modalSize()
	label := "FILE EXPLORER · SOURCE"
	if m.pickerFor == destinationField {
		label = "FILE EXPLORER · DESTINATION"
	}
	filter := importFilterStyle.Render(m.picker.FilterLabel())
	headerWidth := max(1, width-4)
	label = ansi.Truncate(label, max(1, headerWidth-lipgloss.Width(filter)-1), "…")
	header := importTitleStyle.Render(label) +
		strings.Repeat(" ", max(1, headerWidth-lipgloss.Width(label)-lipgloss.Width(filter))) +
		filter
	selected := ansi.Truncate(displayPath(m.picker.SelectedPath()), max(1, width-4), "…")
	if notice := m.picker.Notice(); notice != "" {
		selected = ansi.Truncate(notice, max(1, width-4), "…")
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		importNoteStyle.Render(selected),
		"",
		m.picker.View(),
		importNoteStyle.Render(m.picker.Hint()),
	)
	return importModalStyle.Width(width - 2).Height(height - 2).MaxWidth(width).MaxHeight(height).Render(body)
}

func (m importModel) setupView() string {
	contentWidth := min(tui.DefaultContentWidth, max(24, m.width-4))
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		importStepStyle.Render("SETUP  ·  PHOTO IMPORT"),
		importHeadingStyle.Render("Choose directories"),
		importNoteStyle.Render("Select the source and destination to scan before configuring the import."),
	)
	directoryControls := "\n" + m.controls.View([]string{sourceID, destinationID}) + "\n"
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		"",
		fieldset.View("Directories", directoryControls, contentWidth),
	)
	availableHeight := max(1, m.height-4)
	return lipgloss.Place(contentWidth, availableHeight, lipgloss.Left, lipgloss.Center, content)
}

func (m importModel) parameterView() string {
	if m.width < 63 {
		content := lipgloss.JoinVertical(
			lipgloss.Left,
			importStepStyle.Render("PARAMETERS  ·  PHOTO IMPORT"),
			importHeadingStyle.Render("More width required"),
			importNoteStyle.Render(ansi.Truncate("Widen the terminal to at least 63 columns for the scan and parameter panes.", max(1, m.width-4), "…")),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	layout := m.parameterLayout()
	rightWidth, leftWidth := layout.rightWidth, layout.leftWidth
	sourceFocused := m.parameterFields.Current() == "source-results"
	destinationFocused := m.parameterFields.Current() == "destination-results"
	parametersFocused := m.parameterFields.Current() == "parameters"
	summaryFocused := m.parameterFields.Current() == "summary"
	sourceHeight, destinationHeight := layout.sourceHeight, layout.destinationHeight
	sourceList, destinationList := m.sourceList, m.destinationList
	sourceList.SetSize(rightWidth-4, max(1, sourceHeight-4))
	destinationList.SetSize(rightWidth-4, max(1, destinationHeight-4))
	sourceContent := directoryRootLine(m.paths[sourceField], rightWidth-4) + "\n" + importNoteStyle.Render(inventorySummary(m.filteredSource())) + "\n" + sourceList.View(sourceFocused, importSectionStyle, importNoteStyle)
	destinationContent := directoryRootLine(m.paths[destinationField], rightWidth-4) + "\n" + importNoteStyle.Render(inventorySummary(m.scan.destination)) + "\n" + destinationList.View(destinationFocused, importSectionStyle, importNoteStyle)
	sourceContent = fitContentHeight(sourceContent, sourceHeight-2, rightWidth-4)
	destinationContent = fitContentHeight(destinationContent, destinationHeight-2, rightWidth-4)
	filesPane := lipgloss.JoinVertical(
		lipgloss.Left,
		fieldset.ViewFocused("Source", sourceContent, rightWidth, sourceFocused),
		resultFlowDivider(rightWidth),
		fieldset.ViewFocused("Destination", destinationContent, rightWidth, destinationFocused),
	)
	summaryContent := m.importSummaryView()
	parameterHeight := layout.parameterHeight
	parameterContent := m.parametersView(leftWidth-4, parametersFocused)
	parameterContent = fitContentHeight(parameterContent, parameterHeight-2, leftWidth-4)
	controlsPane := lipgloss.JoinVertical(
		lipgloss.Left,
		fieldset.ViewFocused("Parameters", parameterContent, leftWidth, parametersFocused),
		"",
		fieldset.ViewFocused("Import summary", summaryContent, leftWidth, summaryFocused),
		pageactions.View(pageactions.Config{
			Prev: &pageactions.Action{Destination: "Directories"},
			Next: &pageactions.Action{Destination: "Processing"},
		}, leftWidth),
	)
	view := lipgloss.JoinHorizontal(lipgloss.Top, controlsPane, " ", filesPane)
	if m.menu.IsOpen() {
		x, y := m.menu.Position()
		view = overlay.PlaceAt(view, m.menu.View(), x, y, m.width, m.height)
	}
	return view
}

type parameterLayout struct {
	leftWidth, rightWidth           int
	sourceHeight, destinationHeight int
	parameterHeight, summaryHeight  int
}

func (m importModel) parameterLayout() parameterLayout {
	leftWidth, rightWidth := landscapeColumnWidths(m.width)
	sourceHeight := max(3, (m.height-1)/2)
	destinationHeight := max(3, m.height-1-sourceHeight)
	summaryHeight := lipgloss.Height(m.importSummaryView()) + 2
	parameterHeight := max(3, m.height-summaryHeight-5)
	return parameterLayout{
		leftWidth: leftWidth, rightWidth: rightWidth,
		sourceHeight: sourceHeight, destinationHeight: destinationHeight,
		parameterHeight: parameterHeight, summaryHeight: summaryHeight,
	}
}

func landscapeColumnWidths(width int) (int, int) {
	usable := max(2, width-1)
	left := min(90, max(1, usable/3))
	return left, max(1, usable-left)
}

func directoryRootLine(path string, width int) string {
	return importNoteStyle.Render(ansi.Truncate("Root  "+path, max(1, width), "…"))
}

func resultFlowDivider(width int) string {
	width = max(9, width)
	arrows := " ▼  ▼  ▼ "
	left := (width - lipgloss.Width(arrows)) / 2
	right := width - left - lipgloss.Width(arrows)
	return importSectionStyle.Render(strings.Repeat("━", left) + arrows + strings.Repeat("━", right))
}

func (m importModel) importSummaryView() string {
	plan := m.buildPlan()
	action := strings.ToUpper(plan.operation[:1]) + plan.operation[1:]
	return strings.Join([]string{
		fmt.Sprintf("Eligible       %s", fileCount(plan.eligible)),
		fmt.Sprintf("Duplicates     %s", fileCount(plan.duplicates)),
		fmt.Sprintf("Skipped        %s", fileCount(plan.skipped)),
		fmt.Sprintf("Will %-9s %s", strings.ToLower(action), fileCount(plan.processed)),
		fmt.Sprintf("Workers        %d", max(1, m.controls.IntValue(parallelID))),
	}, "\n")
}

func (m importModel) buildPlan() importPlan {
	destinationNames := make(map[string]struct{}, len(m.scan.destination))
	for _, file := range m.scan.destination {
		destinationNames[strings.ToLower(filepath.Base(file.path))] = struct{}{}
	}
	plan := importPlan{operation: strings.ToLower(m.controls.Value(operationID))}
	for _, file := range m.filteredSource() {
		plan.eligible++
		if _, exists := destinationNames[strings.ToLower(filepath.Base(file.path))]; exists {
			plan.duplicates++
		}
	}
	plan.processed = plan.eligible
	if m.controls.Value(duplicatesID) == "Skip" {
		plan.skipped = plan.duplicates
		plan.processed -= plan.skipped
	}
	return plan
}

func fileCount(count int) string {
	if count == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", count)
}

func inventorySummary(files []scannedFile) string {
	var bytes int64
	for _, file := range files {
		bytes += file.size
	}
	return fileCount(len(files)) + " · " + formatBytes(bytes)
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
}

func (m importModel) parametersView(width int, focused bool) string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		importSectionStyle.Render("IMPORT"),
		m.controls.ViewFocusedWidth([]string{operationID, extensionsID, duplicatesID, parallelID}, focused, width),
	)
}

func fitContentHeight(content string, height, width int) string {
	height = max(1, height)
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", max(0, width)))
	}
	return strings.Join(lines, "\n")
}

func (m importModel) Status() tui.Status {
	if m.leaveConfirm {
		left := "CONFIRM · RETURN"
		if m.leaveExits {
			left = "CONFIRM · EXIT"
		}
		return m.withWorkflow(tui.Status{Left: left, Right: "tab Switch  enter Select  esc Continue"})
	}
	if m.stage == scanStage {
		return m.withWorkflow(tui.Status{Left: "SCAN · READING", Right: "esc Cancel"})
	}
	if m.stage == processingStage {
		left := "PROCESSING · RUNNING"
		right := "↑↓ Workers  p Pause  esc Back  q Quit"
		if m.processing.paused {
			left = "PROCESSING · PAUSED"
			right = "↑/↓ Workers  p Resume  esc Back  q Quit"
		}
		if m.processingComplete() {
			left = "PROCESSING · COMPLETE"
			right = "↑↓ Workers  n Next  esc Prev  q Quit"
		}
		return m.withWorkflow(tui.Status{Left: left, Right: right})
	}
	if m.stage == resultStage {
		return m.withWorkflow(tui.Status{Left: "RESULT · VERIFIED", Right: "alt+h/l Focus  ↑↓ Browse  space Toggle  r Again  esc/q Quit"})
	}
	if m.picking {
		if m.picker.HasDialog() {
			return m.withWorkflow(tui.Status{Left: "BROWSE"})
		}
		return m.withWorkflow(tui.Status{
			Left:  "BROWSE",
			Right: m.picker.Hint(),
		})
	}
	if m.help {
		return m.withWorkflow(tui.Status{Left: "HELP", Right: "? Close  esc Commands  q Quit"})
	}
	if m.controls.IsActive() {
		if m.controls.CapturesText() {
			return m.withWorkflow(tui.Status{Left: "EDIT", Right: "↵ Apply  esc Cancel"})
		}
		return m.withWorkflow(tui.Status{Left: "SELECT", Right: "↑/k ↓/j Choose  ↵ Apply  esc Cancel"})
	}
	if m.stage == parameterStage {
		right := "click / alt+hjkl Focus  space Preview  right-click Menu"
		if m.actionNotice != "" {
			right = m.actionNotice
		}
		return m.withWorkflow(tui.Status{Left: "PARAMETERS", Right: right})
	}
	return m.withWorkflow(tui.Status{
		Left:  "READY",
		Right: "arrows/hjkl  n Next  ↵ Select  ? Help",
	})
}

func (m importModel) withWorkflow(status tui.Status) tui.Status {
	current := 0
	switch m.stage {
	case scanStage, parameterStage:
		current = 1
	case processingStage:
		current = 2
	case resultStage:
		current = 3
	}
	status.Center = stepper.View([]string{"Directories", "Parameters", "Processing", "Result"}, current, max(1, m.width*48/100))
	return status
}

func scanStatus(summary scanSummary) string {
	return fmt.Sprintf("%d source · %d destination", len(summary.source), len(summary.destination))
}

// CapturesShellKey keeps Esc inside the temporary picker so it cancels the
// picker instead of leaving Photo Import.
func (m importModel) CapturesShellKey(key string) bool {
	if m.leaveConfirm || m.stage == processingStage {
		return key == "esc" || key == "q" || key == "ctrl+c"
	}
	if m.stage == resultStage {
		return key == "esc" || key == "q" || key == "ctrl+c"
	}
	if m.menu.IsOpen() {
		return key == "esc" || key == "q"
	}
	if m.stage == scanStage || m.stage == parameterStage {
		return key == "esc"
	}
	if m.picking {
		return key == "esc" || (key == "q" && m.picker.CapturesText())
	}
	return m.controls.IsActive() && (key == "esc" || (key == "q" && m.controls.CapturesText()))
}

func (m importModel) CommandPath() []string {
	switch m.stage {
	case scanStage, parameterStage:
		return []string{"scan"}
	case processingStage:
		return []string{"processing"}
	case resultStage:
		return []string{"result"}
	default:
		return []string{"setup"}
	}
}
