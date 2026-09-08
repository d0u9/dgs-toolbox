package photo

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/contextmenu"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/divider"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/overlay"
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
	classifyID    = "classify"
	scanID        = "scan"
	buttonID      = "button"
)

var (
	setupIDs     = []string{sourceID, destinationID, scanID}
	parameterIDs = []string{operationID, extensionsID, duplicatesID, classifyID, buttonID}
)

type importStage int

const (
	setupStage importStage = iota
	scanStage
	parameterStage
	reviewStage
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
	sourceList           scrolllist.Model
	destinationList      scrolllist.Model
	menu                 contextmenu.Model
	menuTarget           string
	pendingListG         bool
	actionNotice         string
	extensionsConfigured bool
}

func newImportModel() tui.CommandModel {
	paths := [pathFieldCount]string{
		demoPath("src"),
		demoPath("dst"),
	}
	return importModel{
		paths: paths,
		controls: form.New(
			form.Field{ID: sourceID, Kind: form.Path, Label: "Source", Value: paths[sourceField]},
			form.Field{ID: destinationID, Kind: form.Path, Label: "Destination", Value: paths[destinationField]},
			form.Field{ID: operationID, Kind: form.Radio, Label: "Operation", Value: "Copy", Options: []string{"Copy", "Move"}},
			form.Field{ID: extensionsID, Kind: form.MultiCheckbox, Label: "Extensions", SelectAll: true},
			form.Field{ID: duplicatesID, Kind: form.Option, Label: "Duplicates", Value: "Skip", Options: []string{"Skip", "Replace", "Keep both"}},
			form.Field{ID: classifyID, Kind: form.Checkbox, Label: "Organize by EXIF date", Checked: true},
			form.Field{ID: scanID, Kind: form.Button, Label: "Scan directories"},
			form.Field{ID: buttonID, Kind: form.Button, Label: "Review import"},
		),
		parameterFields: datafield.New(
			datafield.Field{ID: "source-results", Row: 0, Col: 0},
			datafield.Field{ID: "destination-results", Row: 1, Col: 0},
			datafield.Field{ID: "parameters", Row: 0, Col: 1},
			datafield.Field{ID: "summary", Row: 1, Col: 1},
		),
		sourceList:      scrolllist.New(),
		destinationList: scrolllist.New(),
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
		var cmd tea.Cmd
		m.picker, _, cmd = m.picker.Update(msg)
		return m, cmd
	}
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.stage == setupStage {
		return m.clickSetup(msg.X, msg.Y)
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
	m.selectHoveredResult(hit, msg.Y)
	if m.parameterFields.FocusAt(msg.X, msg.Y) {
		m.pendingListG = false
		if m.parameterFields.Current() == "parameters" {
			return m.clickParameters(msg.X, msg.Y)
		}
	}
	return m, nil
}

func (m importModel) clickSetup(x, y int) (tea.Model, tea.Cmd) {
	contentWidth := min(tui.DefaultContentWidth, max(24, m.width-4))
	topPadding := 2
	if m.height < 28 {
		topPadding = 1
	}
	if m.height < 23 {
		topPadding = 0
	}
	left := max(0, (m.width-contentWidth)/2)
	if id, ok := m.controls.Click([]string{sourceID, destinationID}, x-left-2, y-topPadding-6); ok {
		if id == sourceID {
			return m.openPicker(sourceField)
		}
		if id == destinationID {
			return m.openPicker(destinationField)
		}
	}
	if id, ok := m.controls.Click([]string{scanID}, x-left, y-topPadding-10); ok && id == scanID {
		return m.startScan()
	}
	return m, nil
}

func (m importModel) clickParameters(x, y int) (tea.Model, tea.Cmd) {
	layout := m.parameterLayout()
	localX := x - layout.leftWidth - 3
	importIDs := []string{operationID, extensionsID, duplicatesID}
	importHeight := lipgloss.Height(m.controls.ViewFocused(importIDs, true))
	if _, ok := m.controls.Click(importIDs, localX, y-2); ok {
		m.syncResultLists()
		return m, nil
	}
	classifyY := importHeight + 5
	if _, ok := m.controls.Click([]string{classifyID}, localX, y-classifyY); ok {
		return m, nil
	}
	buttonY := 1 + lipgloss.Height(m.parametersView(layout.rightWidth-4, true)) + 2
	if id, ok := m.controls.Click([]string{buttonID}, localX, y-buttonY); ok && id == buttonID {
		m.review()
	}
	return m, nil
}

func (m *importModel) registerParameterBounds() {
	layout := m.parameterLayout()
	m.parameterFields.SetBounds("source-results", datafield.Bounds{X: 0, Y: 0, Width: layout.leftWidth, Height: layout.sourceHeight})
	m.parameterFields.SetBounds("destination-results", datafield.Bounds{X: 0, Y: layout.sourceHeight + 1, Width: layout.leftWidth, Height: layout.destinationHeight})
	m.parameterFields.SetBounds("parameters", datafield.Bounds{X: layout.leftWidth + 1, Y: 0, Width: layout.rightWidth, Height: layout.parameterHeight})
	m.parameterFields.SetBounds("summary", datafield.Bounds{X: layout.leftWidth + 1, Y: layout.parameterHeight + 1, Width: layout.rightWidth, Height: layout.summaryHeight})
}

func (m *importModel) scrollResultList(id string, delta int) {
	layout := m.parameterLayout()
	switch id {
	case "source-results":
		m.sourceList.SetSize(max(1, layout.leftWidth-4), max(1, layout.sourceHeight-3))
		m.sourceList.Scroll(delta)
	case "destination-results":
		m.destinationList.SetSize(max(1, layout.leftWidth-4), max(1, layout.destinationHeight-3))
		m.destinationList.Scroll(delta)
	}
}

func (m importModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
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
			m.controls.SetFocusID(scanID)
		}
		return m, nil
	}
	if m.stage == reviewStage {
		if key == "esc" {
			m.stage = parameterStage
		}
		return m, nil
	}
	if m.stage == parameterStage && key == "esc" {
		m.stage = setupStage
		m.controls.SetFocusID(scanID)
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
		case scanID:
			return m.startScan()
		case buttonID:
			m.review()
		}
	case form.PrimaryActionKey:
		m.help = false
		if m.stage == setupStage {
			return m.startScan()
		}
		m.review()
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
	rightWidth := max(50, m.width*2/5)
	rightWidth = min(rightWidth, max(50, m.width-14))
	leftWidth := max(12, m.width-rightWidth-1)
	return max(1, leftWidth-4)
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
	list.SetSize(max(1, layout.leftWidth-4), max(1, height-3))
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
		return m.sourceList.SelectRow(mouseY - 2)
	case "destination-results":
		m.sizeResultList(&m.destinationList, id)
		return m.destinationList.SelectRow(mouseY - layout.sourceHeight - 3)
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

func (m *importModel) review() {
	m.stage = reviewStage
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
	width := min(72, m.width-4)
	height := min(22, m.height-4)
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
	if m.stage == reviewStage {
		return m.reviewView()
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
	if m.picking {
		return overlay.Place(workspace, m.pickerView(), m.width, m.height)
	}
	return workspace
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
		scanProgressView(m.progress, barWidth),
		"",
		importNoteStyle.Render("Esc  Cancel scan"),
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func scanProgressView(percent float64, width int) string {
	width = max(8, width)
	filled := int(float64(width-2) * percent)
	bar := strings.Repeat("━", filled) + strings.Repeat("─", max(0, width-2-filled))
	return importSectionStyle.Render("▐"+bar) + importNoteStyle.Render("▌")
}

func (m importModel) reviewView() string {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		importStepStyle.Render("REVIEW  ·  PHOTO IMPORT"),
		importHeadingStyle.Render("Review import plan"),
		importNoteStyle.Render("Preview only — no files have been scanned, copied, or moved."),
		"",
		importNoteStyle.Render("Esc  Back to setup"),
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
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
	topPadding := 2
	if m.height < 28 {
		topPadding = 1
	}
	if m.height < 23 {
		topPadding = 0
	}
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		importStepStyle.Render("SETUP  ·  PHOTO IMPORT"),
		importHeadingStyle.Render("Choose directories"),
		importNoteStyle.Render("Select the source and destination to scan before configuring the import."),
	)
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		"",
		fieldset.View("Directories", m.controls.View([]string{sourceID, destinationID}), contentWidth),
		"",
		m.controls.View([]string{scanID}),
	)
	return strings.Repeat("\n", topPadding) + content
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
	sourceList.SetSize(leftWidth-4, max(1, sourceHeight-3))
	destinationList.SetSize(leftWidth-4, max(1, destinationHeight-3))
	sourceContent := importNoteStyle.Render(inventorySummary(m.filteredSource())) + "\n" + sourceList.View(sourceFocused, importSectionStyle, importNoteStyle)
	destinationContent := importNoteStyle.Render(inventorySummary(m.scan.destination)) + "\n" + destinationList.View(destinationFocused, importSectionStyle, importNoteStyle)
	left := lipgloss.JoinVertical(
		lipgloss.Left,
		fieldset.ViewFocused("Source", sourceContent, leftWidth, sourceFocused),
		resultFlowDivider(leftWidth),
		fieldset.ViewFocused("Destination", destinationContent, leftWidth, destinationFocused),
	)
	summaryContent := m.importSummaryView()
	parameterHeight := layout.parameterHeight
	parameterContent := m.parametersView(rightWidth-4, parametersFocused) + "\n\n" +
		m.controls.ViewFocusedWidth([]string{buttonID}, parametersFocused, rightWidth-4)
	parameterContent = fitContentHeight(parameterContent, parameterHeight-2, rightWidth-4)
	right := lipgloss.JoinVertical(
		lipgloss.Left,
		fieldset.ViewFocused("Parameters", parameterContent, rightWidth, parametersFocused),
		"",
		fieldset.ViewFocused("Import summary", summaryContent, rightWidth, summaryFocused),
	)
	view := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
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
	rightWidth := max(50, m.width*2/5)
	rightWidth = min(rightWidth, max(50, m.width-14))
	leftWidth := max(12, m.width-rightWidth-1)
	sourceHeight := max(3, (m.height-1)/2)
	destinationHeight := max(3, m.height-1-sourceHeight)
	summaryHeight := lipgloss.Height(m.importSummaryView()) + 2
	parameterHeight := max(3, m.height-summaryHeight-1)
	return parameterLayout{
		leftWidth: leftWidth, rightWidth: rightWidth,
		sourceHeight: sourceHeight, destinationHeight: destinationHeight,
		parameterHeight: parameterHeight, summaryHeight: summaryHeight,
	}
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
	classification := "No"
	if m.controls.Checked(classifyID) {
		classification = "EXIF date folders"
	}
	return strings.Join([]string{
		fmt.Sprintf("Eligible       %s", fileCount(plan.eligible)),
		fmt.Sprintf("Duplicates     %s", fileCount(plan.duplicates)),
		fmt.Sprintf("Skipped        %s", fileCount(plan.skipped)),
		fmt.Sprintf("Will %-9s %s", strings.ToLower(action), fileCount(plan.processed)),
		"Classification " + classification,
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
		m.controls.ViewFocusedWidth([]string{operationID, extensionsID, duplicatesID}, focused, width),
		"",
		divider.Anchored(width),
		importSectionStyle.Render("CLASSIFICATION"),
		m.controls.ViewFocusedWidth([]string{classifyID}, focused, width),
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
	if m.stage == scanStage {
		return m.withWorkflow(tui.Status{Left: "SCANNING", Right: "esc Cancel"})
	}
	if m.stage == reviewStage {
		return m.withWorkflow(tui.Status{Left: "REVIEW", Right: "esc Back"})
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
	case reviewStage:
		current = 2
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
	if m.menu.IsOpen() {
		return key == "esc" || key == "q"
	}
	if m.stage == scanStage || m.stage == parameterStage || m.stage == reviewStage {
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
	case reviewStage:
		return []string{"review"}
	default:
		return []string{"setup"}
	}
}
