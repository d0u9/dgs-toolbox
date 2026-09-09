package capture

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/apps/capture/indexschema"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/divider"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/overlay"
	"dgs-toolbox/internal/tui/scrolllist"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	scanTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(
		lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"},
	)
	scanMutedStyle = lipgloss.NewStyle().Foreground(
		lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"},
	)
	scanFocusedPathStyle = lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}).
				Background(lipgloss.AdaptiveColor{Light: "#DDF3F0", Dark: "#173F3B"})
	scanFilterStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#ECFEFF", Dark: "#134E4A"}).
			Background(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}).
			Padding(0, 1)
	scanModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}).
			Background(lipgloss.AdaptiveColor{Light: "#EAF4F4", Dark: "#243447"})
)

const (
	rootField               = "capture-root"
	capturesField           = "captures"
	centerField             = "preview"
	captureInfoField        = "capture-info"
	fileInfoField           = "file-info"
	rootPathID              = "root-path"
	columnGutter            = 1
	rootHeight              = 3
	maxPreviewBytes         = 1024 * 1024
	capturePropertyKeyWidth = 12
)

type capturesLoadedMsg struct {
	root     string
	captures []captureEntry
	err      error
}

type captureEntry struct {
	path  string
	name  string
	files []string
	index indexschema.Index
}

type previewLoadedMsg struct {
	path       string
	requestID  uint64
	content    string
	properties []property
	err        error
	centered   bool
}

type previewRequestMsg struct {
	path      string
	width     int
	height    int
	jsonHint  bool
	requestID uint64
}

type filePropertiesLoadedMsg struct {
	path       string
	requestID  uint64
	properties []property
	err        error
}

type model struct {
	width          int
	height         int
	root           string
	indexFile      string
	controls       form.Model
	picking        bool
	picker         fileexplorer.Model
	captures       scrolllist.Model
	entries        []captureEntry
	expanded       map[string]bool
	captureCount   int
	fields         datafield.Navigator
	loadError      string
	pendingGG      bool
	preview        viewport.Model
	previewPath    string
	fileProperties []property
	captureInfo    viewport.Model
	fileInfo       viewport.Model
	captureCursor  int
	fileCursor     int
	previewRequest uint64
	previewIsImage bool
	imagePreviewed bool
	previewNotice  string
}

func newModel() model {
	return newModelWithSettings("", "index.json")
}

func newModelWithSettings(root, indexFile string) model {
	if root == "" {
		root, _ = os.Getwd()
	}
	if root == "" {
		root = "."
	}
	root = expandHome(root)
	if indexFile == "" {
		indexFile = "index.json"
	}
	preview := viewport.New(20, 10)
	preview.MouseWheelEnabled = false
	preview.SetHorizontalStep(4)
	preview.SetContent("· Select a file")
	captureInfo := viewport.New(20, 10)
	fileInfo := viewport.New(20, 8)
	captureInfo.MouseWheelEnabled = false
	fileInfo.MouseWheelEnabled = false
	m := model{
		width:     80,
		height:    22,
		root:      root,
		indexFile: indexFile,
		controls: form.New(form.Field{
			ID: rootPathID, Kind: form.Path, Label: "Root", Value: displayPath(root),
		}),
		captures:      scrolllist.New(),
		expanded:      make(map[string]bool),
		preview:       preview,
		previewNotice: "· Select a file",
		captureInfo:   captureInfo,
		fileInfo:      fileInfo,
		fields: datafield.New(
			datafield.Field{ID: capturesField, Row: 0, Col: 0},
			datafield.Field{ID: rootField, Row: 1, Col: 0},
			datafield.Field{ID: centerField, Row: 0, Col: 1},
			datafield.Field{ID: captureInfoField, Row: 0, Col: 2},
			datafield.Field{ID: fileInfoField, Row: 1, Col: 2},
		),
	}
	m.fields.Set(capturesField)
	m.refreshDetails(true)
	return m
}

func (m model) Init() tea.Cmd {
	return loadCaptures(m.root, m.indexFile)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeComponents()
		if m.picking {
			m.sizePicker()
		}
		if m.previewPath != "" && (!m.previewIsImage || m.imagePreviewed) {
			if m.previewIsImage {
				cmd := m.renderSelectedImage()
				return m, cmd
			}
			return m, m.loadSelectedPreview()
		}
		return m, nil
	case capturesLoadedMsg:
		if msg.root != m.root {
			return m, nil
		}
		m.loadError = ""
		if msg.err != nil {
			m.loadError = msg.err.Error()
			m.captureCount = 0
			m.entries = nil
			m.captures.SetItems(nil)
		} else {
			m.entries = msg.captures
			m.captureCount = len(msg.captures)
			m.rebuildCaptureItems()
		}
		m.refreshDetails(true)
		return m, m.loadSelectedPreview()
	case previewLoadedMsg:
		if msg.path != m.previewPath || (msg.requestID != 0 && msg.requestID != m.previewRequest) {
			return m, nil
		}
		if msg.err != nil {
			m.setPreviewNotice("! " + msg.err.Error())
		} else if msg.centered {
			m.setPreviewNotice(msg.content)
		} else {
			m.previewNotice = ""
			m.preview.SetContent(msg.content)
		}
		m.fileProperties = msg.properties
		m.fileCursor = firstProperty(m.fileProperties)
		m.refreshDetails(false)
		m.fileInfo.GotoTop()
		m.preview.GotoTop()
		m.preview.SetXOffset(0)
		return m, nil
	case previewRequestMsg:
		if msg.path != m.previewPath || msg.requestID != m.previewRequest {
			return m, nil
		}
		return m, loadFilePreviewRequest(msg.path, msg.width, msg.height, msg.jsonHint, msg.requestID)
	case filePropertiesLoadedMsg:
		if msg.path != m.previewPath || msg.requestID != m.previewRequest {
			return m, nil
		}
		if msg.err == nil {
			m.fileProperties = msg.properties
			m.fileCursor = firstProperty(m.fileProperties)
			m.refreshDetails(false)
		}
		return m, nil
	}

	if m.picking {
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

func (m model) View() string {
	if m.width < 48 || m.height < 8 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, scanMutedStyle.Render("Resize terminal for Capture Scan"))
	}

	leftWidth, centerWidth, rightWidth := m.columnWidths()
	left := m.leftColumn(leftWidth)
	previewContent := viewportWithMarkers(m.preview, centerWidth-4)
	if m.previewNotice != "" {
		previewContent = lipgloss.Place(max(1, centerWidth-4), max(1, m.height-2), lipgloss.Center, lipgloss.Center, scanMutedStyle.Render(m.previewNotice))
	}
	center := fieldset.ViewFocused(m.previewLegend(), previewContent, centerWidth, m.fields.Current() == centerField)
	right := m.rightColumn(rightWidth)
	workspace := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", center, " ", right)
	if m.picking {
		return overlay.Place(workspace, m.pickerView(), m.width, m.height)
	}
	return workspace
}

func (m model) Status() tui.Status {
	if m.picking {
		if m.picker.HasDialog() {
			return tui.Status{Left: "BROWSE", Center: "CAPTURE ROOT"}
		}
		return tui.Status{Left: "BROWSE", Center: "CAPTURE ROOT", Right: m.picker.Hint()}
	}
	center := m.statusValue()
	switch m.fields.Current() {
	case rootField:
		return tui.Status{Left: "SCAN", Center: center, Right: "↵ Browse  R Refresh  tab Next  alt+hjkl Focus"}
	case capturesField:
		return tui.Status{Left: "SCAN", Center: center, Right: "↑/k ↓/j Move  o Open  O Close  w Close all  space Quick Look  R Refresh"}
	case centerField:
		return tui.Status{Left: "PREVIEW", Center: center, Right: "↑/k ↓/j Scroll  h/l Pan  R Refresh  alt+hjkl Focus"}
	case captureInfoField:
		return tui.Status{Left: "CAPTURE INFO", Center: center, Right: "↑/k ↓/j Scroll  alt+hjkl Focus"}
	case fileInfoField:
		return tui.Status{Left: "FILE INFO", Center: center, Right: "↑/k ↓/j Scroll  alt+hjkl Focus"}
	default:
		return tui.Status{Left: "SCAN", Center: center, Right: "tab Next  alt+hjkl Focus"}
	}
}

func (m model) CapturesShellKey(key string) bool {
	return m.picking && (key == "esc" || (key == "q" && m.picker.CapturesText()))
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "R" {
		m.pendingGG = false
		return m, loadCaptures(m.root, m.indexFile)
	}
	if m.fields.Move(key) {
		m.refreshDetails(false)
		m.pendingGG = false
		return m, nil
	}
	if key == "tab" || key == "shift+tab" {
		m.cycleSelectableField(key == "shift+tab")
		m.refreshDetails(false)
		m.pendingGG = false
		return m, nil
	}

	switch m.fields.Current() {
	case rootField:
		m.pendingGG = false
		if key == "enter" {
			return m.openPicker()
		}
	case capturesField:
		return m.updateCaptureList(key)
	case centerField:
		return m.updatePreview(key)
	case captureInfoField:
		return m.updateInfoViewport(key, true)
	case fileInfoField:
		return m.updateInfoViewport(key, false)
	default:
		m.pendingGG = false
	}
	return m, nil
}

func (m model) updateCaptureList(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.captures.Move(-1)
		m.pendingGG = false
		return m, m.loadSelectedPreview()
	case "down", "j":
		m.captures.Move(1)
		m.pendingGG = false
		return m, m.loadSelectedPreview()
	case "left", "h":
		m.captures.Pan(-4)
	case "right", "l":
		m.captures.Pan(4)
	case "o":
		m.toggleSelectedCapture()
		m.pendingGG = false
		return m, m.loadSelectedPreview()
	case "O":
		m.collapseSelectedCapture()
		m.pendingGG = false
		return m, m.loadSelectedPreview()
	case "w":
		m.expanded = make(map[string]bool)
		m.rebuildCaptureItems()
		m.pendingGG = false
		return m, m.loadSelectedPreview()
	case " ", "space":
		m.pendingGG = false
		return m, m.quickLookSelected()
	case "enter":
		m.pendingGG = false
		cmd := m.renderSelectedImage()
		return m, cmd
	case "R":
		m.pendingGG = false
		return m, loadCaptures(m.root, m.indexFile)
	case "G", "end":
		m.captures.Last()
		m.pendingGG = false
		return m, m.loadSelectedPreview()
	case "g":
		if m.pendingGG {
			m.captures.First()
			m.pendingGG = false
			return m, m.loadSelectedPreview()
		}
		m.pendingGG = true
		return m, nil
	default:
		m.pendingGG = false
		return m, nil
	}
	m.pendingGG = false
	return m, nil
}

func (m model) updatePreview(key string) (tea.Model, tea.Cmd) {
	if key == "enter" {
		cmd := m.renderSelectedImage()
		return m, cmd
	}
	switch key {
	case "up", "k":
		m.preview.ScrollUp(1)
	case "down", "j":
		m.preview.ScrollDown(1)
	case "left", "h":
		m.preview.ScrollLeft(4)
	case "right", "l":
		m.preview.ScrollRight(4)
	case "g", "home":
		m.preview.GotoTop()
		m.preview.SetXOffset(0)
	case "G", "end":
		m.preview.GotoBottom()
	}
	return m, nil
}

func (m model) updateInfoViewport(key string, capture bool) (tea.Model, tea.Cmd) {
	properties := m.fileProperties
	cursor := &m.fileCursor
	target := &m.fileInfo
	if capture {
		properties = m.captureProperties()
		cursor = &m.captureCursor
		target = &m.captureInfo
	}
	switch key {
	case "up", "k":
		movePropertyCursor(properties, cursor, -1)
	case "down", "j":
		movePropertyCursor(properties, cursor, 1)
	case "g", "home":
		*cursor = firstProperty(properties)
	case "G", "end":
		*cursor = lastProperty(properties)
	}
	m.refreshDetails(false)
	m.ensurePropertyVisible(target, properties, *cursor, capture)
	return m, nil
}

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.setFieldBounds()
	hit := m.fields.HitAt(msg.X, msg.Y)
	if hit == capturesField && msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		delta := -3
		if msg.Button == tea.MouseButtonWheelDown {
			delta = 3
		}
		m.captures.Scroll(delta)
		return m, m.loadSelectedPreview()
	}
	if hit == centerField && msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		if msg.Button == tea.MouseButtonWheelDown {
			m.preview.ScrollDown(3)
		} else {
			m.preview.ScrollUp(3)
		}
		return m, nil
	}
	if (hit == captureInfoField || hit == fileInfoField) && msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		target := &m.captureInfo
		if hit == fileInfoField {
			target = &m.fileInfo
		}
		if msg.Button == tea.MouseButtonWheelDown {
			target.ScrollDown(3)
		} else {
			target.ScrollUp(3)
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress || hit == "" {
		return m, nil
	}
	m.fields.FocusAt(msg.X, msg.Y)
	m.refreshDetails(false)
	m.pendingGG = false
	if hit == rootField {
		if id, used := m.controls.Click([]string{rootPathID}, msg.X-2, msg.Y-m.captureListHeight()-1); used && id == rootPathID {
			return m.openPicker()
		}
	}
	if hit == capturesField {
		if m.captures.SelectRow(msg.Y - 1) {
			return m, m.loadSelectedPreview()
		}
	}
	if hit == captureInfoField {
		m.selectCaptureInfoRow(msg.Y)
		m.refreshDetails(false)
	}
	if hit == fileInfoField {
		m.selectFileInfoRow(msg.Y)
		m.refreshDetails(false)
	}
	return m, nil
}

func (m model) openPicker() (tea.Model, tea.Cmd) {
	width, height := m.modalSize()
	m.picker = fileexplorer.New(m.root, width-2, height-6, fileexplorer.WithFilter(fileexplorer.Directories()))
	m.picking = true
	return m, m.picker.Init()
}

func (m model) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" && !m.picker.HasDialog() {
		m.picking = false
		return m, nil
	}
	picker, selected, cmd := m.picker.Update(msg)
	m.picker = picker
	if selected == "" {
		return m, cmd
	}
	m.root = selected
	m.controls.SetValue(rootPathID, displayPath(selected))
	m.entries = nil
	m.expanded = make(map[string]bool)
	m.captures.SetItems(nil)
	m.captureCount = 0
	m.clearPreview("· Select a file")
	m.loadError = ""
	m.picking = false
	return m, tea.Batch(cmd, loadCaptures(selected, m.indexFile))
}

func (m *model) cycleSelectableField(reverse bool) {
	order := []string{capturesField, rootField, centerField, captureInfoField, fileInfoField}
	current := 0
	for i, id := range order {
		if m.fields.Current() == id {
			current = i
			break
		}
	}
	step := 1
	if reverse {
		step = -1
	}
	m.fields.Set(order[(current+step+len(order))%len(order)])
}

func (m *model) resizeComponents() {
	if m.width < 48 || m.height < 8 {
		return
	}
	left, center, right := m.columnWidths()
	m.captures.SetSize(max(1, left-4), max(1, m.captureListHeight()-2))
	m.preview.Width = max(1, center-4)
	m.preview.Height = max(1, m.height-2)
	top := m.rightTopHeight()
	m.captureInfo.Width = max(1, right-4)
	m.captureInfo.Height = max(1, top-2-m.captureMapHeight())
	m.fileInfo.Width = max(1, right-4)
	m.fileInfo.Height = max(1, m.height-top-2)
	m.refreshDetails(false)
	m.setFieldBounds()
}

func (m *model) setFieldBounds() {
	left, center, right := m.columnWidths()
	listHeight := m.captureListHeight()
	m.fields.SetBounds(capturesField, datafield.Bounds{X: 0, Y: 0, Width: left, Height: listHeight})
	m.fields.SetBounds(rootField, datafield.Bounds{X: 0, Y: listHeight, Width: left, Height: rootHeight})
	m.fields.SetBounds(centerField, datafield.Bounds{X: left + columnGutter, Y: 0, Width: center, Height: m.height})
	rightX := left + center + 2*columnGutter
	top := m.rightTopHeight()
	m.fields.SetBounds(captureInfoField, datafield.Bounds{X: rightX, Y: 0, Width: right, Height: top})
	m.fields.SetBounds(fileInfoField, datafield.Bounds{X: rightX, Y: top, Width: right, Height: m.height - top})
}

func (m model) leftColumn(width int) string {
	root := fieldset.ViewFocused(
		"CAPTURE ROOT",
		m.rootPathView(width-4),
		width,
		m.fields.Current() == rootField,
	)
	listHeight := m.captureListHeight()
	captures := m.captures
	captures.SetSize(max(1, width-4), max(1, listHeight-2))
	content := fitHeight(captures.View(m.fields.Current() == capturesField, scanTitleStyle, scanMutedStyle), listHeight-2, width-4)
	if m.loadError != "" {
		content = fitHeight(scanMutedStyle.Render("! "+m.loadError), listHeight-2, width-4)
	}
	list := fieldset.ViewFocused("CAPTURES", content, width, m.fields.Current() == capturesField)
	return list + "\n" + root
}

func (m model) rootPathView(width int) string {
	marker := "  "
	style := lipgloss.NewStyle()
	if m.fields.Current() == rootField {
		marker = "› "
		style = scanFocusedPathStyle
	}
	path := ansi.Truncate(m.controls.Value(rootPathID), max(1, width-2), "…")
	return style.Width(max(1, width)).MaxWidth(max(1, width)).Render(marker + path)
}

func (m model) columnWidths() (left, center, right int) {
	available := m.width - 2*columnGutter
	left = min(90, max(12, available/4))
	right = min(90, max(12, available/4))
	center = available - left - right
	return left, center, right
}

func (m model) captureListHeight() int {
	return m.height - rootHeight
}

func (m model) rightTopHeight() int { return max(7, m.height*3/5) }

func (m model) rightColumn(width int) string {
	topHeight := m.rightTopHeight()
	innerWidth := width - 4
	captureHeight := topHeight - 2
	mapProperties := m.captureMapProperties()
	mapHeight := m.captureMapHeight()
	scrollHeight := max(1, captureHeight-mapHeight)
	capture := fitHeight(viewportWithMarkers(m.captureInfo, innerWidth), scrollHeight, innerWidth)
	if mapHeight > 0 {
		capture += "\n" + divider.Anchored(innerWidth) + "\n" + renderProperties(mapProperties, innerWidth, m.captureMapCursor(), m.fields.Current() == captureInfoField, capturePropertyKeyWidth)
	}
	capture = fitHeight(capture, captureHeight, innerWidth)
	fileHeight := m.height - topHeight
	file := fitHeight(viewportWithMarkers(m.fileInfo, width-4), fileHeight-2, width-4)
	return fieldset.ViewFocused("CAPTURE INFO", capture, width, m.fields.Current() == captureInfoField) + "\n" +
		fieldset.ViewFocused("FILE INFO", file, width, m.fields.Current() == fileInfoField)
}

func (m model) columnRightWidth() int {
	_, _, right := m.columnWidths()
	return right
}

func (m *model) refreshDetails(reset bool) {
	width := max(1, m.columnRightWidth()-4)
	m.captureInfo.Height = max(1, m.rightTopHeight()-2-m.captureMapHeight())
	if reset {
		m.captureCursor = firstProperty(m.captureProperties())
		m.fileCursor = firstProperty(m.fileProperties)
	}
	m.captureInfo.SetContent(renderProperties(m.captureScrollableProperties(), width, m.captureScrollableCursor(), m.fields.Current() == captureInfoField, capturePropertyKeyWidth))
	m.fileInfo.SetContent(renderProperties(m.fileProperties, width, m.fileCursor, m.fields.Current() == fileInfoField, 0))
	if reset {
		m.captureInfo.GotoTop()
		m.fileInfo.GotoTop()
	}
}

func (m model) captureScrollableProperties() []property {
	var result []property
	for _, item := range m.captureProperties() {
		if !item.fixed {
			result = append(result, item)
		}
	}
	return result
}

func (m model) captureMapProperties() []property {
	var result []property
	for _, item := range m.captureProperties() {
		if item.fixed {
			result = append(result, item)
		}
	}
	return result
}

func (m model) captureMapHeight() int {
	if len(m.captureMapProperties()) == 0 {
		return 0
	}
	return 1 + len(m.captureMapProperties())
}

func (m model) captureScrollableCursor() int {
	position := 0
	for index, item := range m.captureProperties() {
		if item.fixed {
			continue
		}
		if index == m.captureCursor {
			return position
		}
		position++
	}
	return -1
}

func (m model) captureMapCursor() int {
	position := 0
	for index, item := range m.captureProperties() {
		if !item.fixed {
			continue
		}
		if index == m.captureCursor {
			return position
		}
		position++
	}
	return -1
}

func firstProperty(properties []property) int {
	for index, item := range properties {
		if item.value != "" {
			return index
		}
	}
	return 0
}

func lastProperty(properties []property) int {
	for index := len(properties) - 1; index >= 0; index-- {
		if properties[index].value != "" {
			return index
		}
	}
	return 0
}

func movePropertyCursor(properties []property, cursor *int, delta int) {
	if len(properties) == 0 {
		*cursor = 0
		return
	}
	for next := *cursor + delta; next >= 0 && next < len(properties); next += delta {
		if properties[next].value != "" {
			*cursor = next
			return
		}
	}
}

func (m *model) ensurePropertyVisible(target *viewport.Model, properties []property, cursor int, capture bool) {
	row := cursor
	if capture {
		row = m.captureScrollableCursor()
	}
	if row < 0 {
		return
	}
	if row < target.YOffset {
		target.SetYOffset(row)
	}
	if row >= target.YOffset+target.Height {
		target.SetYOffset(row - target.Height + 1)
	}
}

func (m *model) selectCaptureInfoRow(y int) {
	innerHeight := m.rightTopHeight() - 2
	mapHeight := m.captureMapHeight()
	scrollHeight := max(1, innerHeight-mapHeight)
	contentRow := y - 1
	properties := m.captureProperties()
	if mapHeight > 0 && contentRow > scrollHeight {
		mapRow := contentRow - scrollHeight - 1
		fixedIndex := 0
		for index, item := range properties {
			if !item.fixed {
				continue
			}
			if fixedIndex == mapRow {
				m.captureCursor = index
				return
			}
			fixedIndex++
		}
		return
	}
	targetRow := m.captureInfo.YOffset + contentRow
	scrollRow := 0
	for index, item := range properties {
		if item.fixed {
			continue
		}
		if scrollRow == targetRow && item.value != "" {
			m.captureCursor = index
			return
		}
		scrollRow++
	}
}

func (m *model) selectFileInfoRow(y int) {
	contentRow := y - m.rightTopHeight() - 1
	index := m.fileInfo.YOffset + contentRow
	if index >= 0 && index < len(m.fileProperties) && m.fileProperties[index].value != "" {
		m.fileCursor = index
	}
}

func (m model) statusValue() string {
	switch m.fields.Current() {
	case rootField:
		return m.root
	case capturesField:
		if selected, ok := m.captures.Selected(); ok {
			for _, prefix := range []string{"capture:", "file:"} {
				if strings.HasPrefix(selected.ID, prefix) {
					return strings.TrimPrefix(selected.ID, prefix)
				}
			}
		}
	case centerField:
		return m.previewPath
	case captureInfoField:
		properties := m.captureProperties()
		if m.captureCursor >= 0 && m.captureCursor < len(properties) {
			return properties[m.captureCursor].value
		}
	case fileInfoField:
		if m.fileCursor >= 0 && m.fileCursor < len(m.fileProperties) {
			return m.fileProperties[m.fileCursor].value
		}
	}
	return fmt.Sprintf("%d CAPTURES · %s", m.captureCount, m.indexFile)
}

func (m model) modalSize() (int, int) {
	return max(20, min(96, m.width-4)), max(8, min(30, m.height-2))
}

func (m *model) sizePicker() {
	width, height := m.modalSize()
	m.picker.SetSize(width-2, height-6)
}

func (m model) pickerView() string {
	width, height := m.modalSize()
	label := "FILE EXPLORER · CAPTURE ROOT"
	filter := scanFilterStyle.Render(m.picker.FilterLabel())
	headerWidth := max(1, width-4)
	label = ansi.Truncate(label, max(1, headerWidth-lipgloss.Width(filter)-1), "…")
	header := scanTitleStyle.Render(label) + strings.Repeat(" ", max(1, headerWidth-lipgloss.Width(label)-lipgloss.Width(filter))) + filter
	selected := ansi.Truncate(displayPath(m.picker.SelectedPath()), headerWidth, "…")
	if notice := m.picker.Notice(); notice != "" {
		selected = ansi.Truncate(notice, headerWidth, "…")
	}
	body := lipgloss.JoinVertical(lipgloss.Left, header, scanMutedStyle.Render(selected), "", m.picker.View(), scanMutedStyle.Render(m.picker.Hint()))
	return scanModalStyle.Width(width - 2).Height(height - 2).MaxWidth(width).MaxHeight(height).Render(body)
}

func (m model) previewLegend() string {
	if m.previewPath == "" {
		return "PREVIEW"
	}
	return "PREVIEW · " + filepath.Base(m.previewPath)
}

func (m *model) clearPreview(message string) {
	m.previewRequest++
	m.previewPath = ""
	m.previewIsImage = false
	m.imagePreviewed = false
	m.fileProperties = nil
	m.refreshDetails(false)
	m.setPreviewNotice(message)
	m.preview.GotoTop()
	m.preview.SetXOffset(0)
}

func (m *model) loadSelectedPreview() tea.Cmd {
	m.captureInfo.GotoTop()
	m.captureCursor = firstProperty(m.captureProperties())
	m.fileProperties = nil
	m.fileCursor = 0
	m.refreshDetails(false)
	selected, ok := m.captures.Selected()
	if !ok || !strings.HasPrefix(selected.ID, "file:") {
		m.clearPreview("· Select a file")
		return nil
	}
	path := strings.TrimPrefix(selected.ID, "file:")
	m.previewRequest++
	requestID := m.previewRequest
	m.previewPath = path
	m.previewIsImage = isInlineImagePath(path)
	m.imagePreviewed = false
	m.preview.GotoTop()
	m.preview.SetXOffset(0)
	if m.previewIsImage {
		m.setPreviewNotice("Press Enter to preview")
		return loadFileProperties(path, requestID)
	}
	m.setPreviewNotice("Loading preview…")
	width, height := max(1, m.preview.Width), max(1, m.preview.Height)
	jsonHint := filepath.Base(path) == m.indexFile
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return previewRequestMsg{path: path, width: width, height: height, jsonHint: jsonHint, requestID: requestID}
	})
}

func (m *model) renderSelectedImage() tea.Cmd {
	if !m.previewIsImage || m.previewPath == "" {
		return nil
	}
	m.previewRequest++
	m.imagePreviewed = true
	m.setPreviewNotice("Loading image…")
	m.preview.GotoTop()
	return loadFilePreviewRequest(m.previewPath, max(1, m.preview.Width), max(1, m.preview.Height), false, m.previewRequest)
}

func (m *model) setPreviewNotice(message string) {
	m.previewNotice = message
	m.preview.SetContent("")
}

func loadFileProperties(path string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		properties, _, err := inspectFile(path)
		return filePropertiesLoadedMsg{path: path, requestID: requestID, properties: properties, err: err}
	}
}

func (m model) quickLookSelected() tea.Cmd {
	selected, ok := m.captures.Selected()
	if !ok || runtime.GOOS != "darwin" {
		return nil
	}
	path := ""
	for _, prefix := range []string{"capture:", "file:"} {
		if strings.HasPrefix(selected.ID, prefix) {
			path = strings.TrimPrefix(selected.ID, prefix)
			break
		}
	}
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		return exec.Command("qlmanage", "-p", path).Start()
	}
}

func (m *model) toggleSelectedCapture() {
	selected, ok := m.captures.Selected()
	if !ok || !strings.HasPrefix(selected.ID, "capture:") {
		return
	}
	path := strings.TrimPrefix(selected.ID, "capture:")
	m.expanded[path] = !m.expanded[path]
	m.rebuildCaptureItems()
}

func (m *model) collapseSelectedCapture() {
	selected, ok := m.captures.Selected()
	if !ok {
		return
	}
	path := ""
	if strings.HasPrefix(selected.ID, "capture:") {
		path = strings.TrimPrefix(selected.ID, "capture:")
	} else if strings.HasPrefix(selected.ID, "file:") {
		path = filepath.Dir(strings.TrimPrefix(selected.ID, "file:"))
	}
	if path == "" || !m.expanded[path] {
		return
	}
	m.expanded[path] = false
	m.rebuildCaptureItems()
	m.captures.SelectID("capture:" + path)
}

func (m model) selectedCapture() (captureEntry, bool) {
	selected, ok := m.captures.Selected()
	if !ok {
		return captureEntry{}, false
	}
	path := ""
	if strings.HasPrefix(selected.ID, "capture:") {
		path = strings.TrimPrefix(selected.ID, "capture:")
	}
	if strings.HasPrefix(selected.ID, "file:") {
		path = filepath.Dir(strings.TrimPrefix(selected.ID, "file:"))
	}
	for _, entry := range m.entries {
		if entry.path == path {
			return entry, true
		}
	}
	return captureEntry{}, false
}

func (m model) captureProperties() []property {
	entry, ok := m.selectedCapture()
	if !ok {
		return nil
	}
	index := entry.index
	props := []property{
		{name: "ID", value: index.ID},
		{name: "Created", value: index.CreatedAt},
		{name: "Schema", value: index.Schema},
		{name: "Source", section: true},
		{name: "App", value: index.Source.App, indent: 1},
		{name: "Workflow", value: index.Source.Workflow, indent: 1},
		{name: "Device", value: index.Source.Device.Name, indent: 1},
		{name: "System", value: index.Source.Device.OS + " " + index.Source.Device.SystemVersion, indent: 1},
	}
	// A Capture may carry no coordinates at all, and its place may be known
	// without them. Absent groups are omitted rather than shown as zeroes.
	if position := index.Coordinates; position != nil {
		props = append(props, property{name: "GPS", value: fmt.Sprintf("%.5f,%.5f · %.0fm", position.Latitude, position.Longitude, position.Altitude)})
	}
	place := index.CapturePlace()
	location := []property{
		{name: "Locality", value: singleLineAddress(place.Locality), indent: 1},
		{name: "City", value: singleLineAddress(place.City), indent: 1},
		{name: "Region", value: singleLineAddress(place.Region), indent: 1},
		{name: "Country", value: singleLineAddress(place.Country), indent: 1},
	}
	hasLocation := false
	for _, item := range location {
		if item.value != "" {
			hasLocation = true
			break
		}
	}
	if hasLocation {
		props = append(props, property{name: "Location", section: true})
		for _, item := range location {
			if item.value != "" {
				props = append(props, item)
			}
		}
	} else if address := singleLineAddress(place.Address); address != "" {
		props = append(props, property{name: "Location", section: true}, property{name: "Address", value: address, indent: 1})
	}
	validAttachments := 0
	for _, attachment := range index.Attachments {
		if attachment.Kind != "null" {
			validAttachments++
		}
	}
	if validAttachments > 0 {
		props = append(props, property{separator: true}, property{name: fmt.Sprintf("Attachments (%d)", validAttachments), section: true})
	}
	number := 0
	for _, attachment := range index.Attachments {
		if attachment.Kind == "null" {
			continue
		}
		number++
		props = append(props, property{name: fmt.Sprintf("%d", number), value: attachment.Kind + " · " + attachment.Name, indent: 1})
		if attachment.SHA256 != "" {
			props = append(props, property{name: "SHA-256", value: attachment.SHA256, indent: 2})
		}
	}
	if position := index.Coordinates; position != nil {
		props = append(props,
			property{name: "Apple", value: mapURLLink(appleMapsURL(position.Latitude, position.Longitude)), fixed: true},
			property{name: "Google", value: mapURLLink(googleMapsURL(position.Latitude, position.Longitude)), fixed: true},
			property{name: "AMap", value: mapURLLink(amapURL(position.Latitude, position.Longitude)), fixed: true},
		)
	}
	return props
}

func singleLineAddress(address string) string {
	parts := strings.FieldsFunc(address, func(r rune) bool { return r == '\n' || r == '\r' })
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		line := strings.Join(strings.Fields(part), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, " · ")
}

func (m *model) rebuildCaptureItems() {
	items := make([]scrolllist.Item, 0, len(m.entries))
	for _, capture := range m.entries {
		marker := "▸ "
		if m.expanded[capture.path] {
			marker = "▾ "
		}
		items = append(items, scrolllist.Item{
			ID: "capture:" + capture.path, Label: marker + capture.name + string(os.PathSeparator),
		})
		if !m.expanded[capture.path] {
			continue
		}
		for _, name := range capture.files {
			items = append(items, scrolllist.Item{
				ID: "file:" + filepath.Join(capture.path, name), Label: "    " + name,
			})
		}
	}
	m.captures.SetItems(items)
}

func loadCaptures(root, indexFile string) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(root)
		if err != nil {
			return capturesLoadedMsg{root: root, err: err}
		}
		captures := make([]captureEntry, 0)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifest := filepath.Join(root, entry.Name(), indexFile)
			info, err := os.Lstat(manifest)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			index, err := indexschema.ReadFile(manifest)
			if err != nil {
				continue
			}
			capturePath := filepath.Join(root, entry.Name())
			captures = append(captures, captureEntry{
				path: capturePath, name: entry.Name(), files: captureFiles(capturePath, indexFile, index.Attachments), index: index,
			})
		}
		sort.Slice(captures, func(i, j int) bool { return strings.ToLower(captures[i].name) < strings.ToLower(captures[j].name) })
		return capturesLoadedMsg{root: root, captures: captures}
	}
}

func captureFiles(capturePath, indexFile string, attachments []indexschema.Attachment) []string {
	files := []string{indexFile}
	seen := map[string]struct{}{filepath.Clean(indexFile): struct{}{}}
	for _, attachment := range attachments {
		if attachment.Kind == "null" || attachment.Name == "" {
			continue
		}
		name := filepath.Clean(attachment.Name)
		if filepath.IsAbs(name) || filepath.Base(name) != name || name == "." || name == ".." {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		info, err := os.Lstat(filepath.Join(capturePath, name))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		seen[name] = struct{}{}
		files = append(files, name)
	}
	return files
}

func displayPath(path string) string {
	cleaned := filepath.Clean(path)
	home, err := os.UserHomeDir()
	if err == nil && (cleaned == home || strings.HasPrefix(cleaned, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(cleaned, home)
	}
	return cleaned
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

func blankContent(width, height int) string {
	return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", max(1, width))+"\n", max(1, height)), "\n")
}

func fitHeight(content string, height, width int) string {
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), max(1, height))]
	for len(lines) < max(1, height) {
		lines = append(lines, strings.Repeat(" ", max(1, width)))
	}
	return strings.Join(lines, "\n")
}
