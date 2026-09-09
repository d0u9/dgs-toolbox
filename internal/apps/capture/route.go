package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/scrolllist"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	routeCapturesField     = "route-captures"
	routeSummaryField      = "route-summary"
	routeDestinationsField = "route-destinations"
	routePlanField         = "route-plan"
	routePropertyKeyWidth  = 12
)

// routeModel is the Route session: it assigns each scanned Capture to one of
// the configured destinations. This session only records the plan; nothing is
// moved on disk.
type routeModel struct {
	width        int
	height       int
	root         string
	indexFile    string
	entries      []captureEntry
	captures     scrolllist.Model
	destinations []config.CaptureDestination
	targets      scrolllist.Model
	assignments  map[string]string
	summary      viewport.Model
	plan         viewport.Model
	fields       datafield.Navigator
	loadError    string
	pendingGG    bool
}

func newRouteModel(root, indexFile string, destinations []config.CaptureDestination) routeModel {
	summary := viewport.New(20, 10)
	plan := viewport.New(20, 8)
	summary.MouseWheelEnabled = false
	plan.MouseWheelEnabled = false
	m := routeModel{
		width:        80,
		height:       22,
		root:         root,
		indexFile:    indexFile,
		captures:     scrolllist.New(),
		destinations: destinations,
		targets:      scrolllist.New(),
		assignments:  make(map[string]string),
		summary:      summary,
		plan:         plan,
		fields: datafield.New(
			datafield.Field{ID: routeCapturesField, Row: 0, Col: 0},
			datafield.Field{ID: routeSummaryField, Row: 0, Col: 1},
			datafield.Field{ID: routeDestinationsField, Row: 0, Col: 2},
			datafield.Field{ID: routePlanField, Row: 1, Col: 2},
		),
	}
	m.fields.Set(routeCapturesField)
	m.rebuildDestinationItems()
	m.refresh()
	return m
}

func (m routeModel) Init() tea.Cmd { return nil }

func (m routeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeComponents()
		return m, nil
	case capturesLoadedMsg:
		m.root = msg.root
		m.loadError = ""
		if msg.err != nil {
			m.loadError = msg.err.Error()
			m.entries = nil
		} else {
			m.entries = msg.captures
			m.pruneAssignments()
		}
		m.rebuildCaptureItems()
		m.refresh()
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m routeModel) View() string {
	if m.width < 48 || m.height < 8 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, scanMutedStyle.Render("Resize terminal for Capture Route"))
	}
	left, center, right := m.columnWidths()
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.capturesColumn(left), " ", m.summaryColumn(center), " ", m.rightColumn(right))
}

func (m routeModel) Status() tui.Status {
	center := m.statusValue()
	switch m.fields.Current() {
	case routeCapturesField:
		return tui.Status{Left: "ROUTE", Center: center, Right: "↑/k ↓/j Move  ↵ Choose destination  u Unassign  R Refresh"}
	case routeSummaryField:
		return tui.Status{Left: "CAPTURE", Center: center, Right: "↑/k ↓/j Scroll  alt+hjkl Focus"}
	case routeDestinationsField:
		return tui.Status{Left: "DESTINATIONS", Center: center, Right: "↑/k ↓/j Move  ↵ Assign  alt+hjkl Focus"}
	case routePlanField:
		return tui.Status{Left: "ROUTE PLAN", Center: center, Right: "↑/k ↓/j Scroll  alt+hjkl Focus"}
	default:
		return tui.Status{Left: "ROUTE", Center: center, Right: "tab Next  alt+hjkl Focus"}
	}
}

func (m routeModel) statusValue() string {
	switch m.fields.Current() {
	case routeDestinationsField:
		if destination, ok := m.selectedDestination(); ok {
			return destination.Name + " · " + displayPath(destination.Path)
		}
		return "No destinations configured"
	case routeCapturesField, routeSummaryField:
		if entry, ok := m.selectedCapture(); ok {
			if target, assigned := m.assignments[entry.path]; assigned {
				return entry.path + " → " + displayPath(target)
			}
			return entry.path
		}
	}
	routed, total := m.routedCount(), len(m.entries)
	return fmt.Sprintf("%d/%d ROUTED · %d DESTINATIONS", routed, total, len(m.destinations))
}

func (m routeModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "R" {
		m.pendingGG = false
		return m, loadCaptures(m.root, m.indexFile)
	}
	if key == "alt+j" || key == "alt+down" || key == "alt+k" || key == "alt+up" {
		m.cycleFields(key == "alt+j" || key == "alt+down")
		m.refresh()
		return m, nil
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
	case routeCapturesField:
		return m.updateCaptures(key)
	case routeDestinationsField:
		return m.updateDestinations(key)
	case routeSummaryField:
		m.summary = scrollViewport(m.summary, key)
	case routePlanField:
		m.plan = scrollViewport(m.plan, key)
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
		m.fields.Set(routeDestinationsField)
	case "u", "backspace":
		if entry, ok := m.selectedCapture(); ok {
			delete(m.assignments, entry.path)
			m.rebuildCaptureItems()
		}
	}
	m.pendingGG = false
	m.refresh()
	return m, nil
}

func (m routeModel) updateDestinations(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.targets.Move(-1)
	case "down", "j":
		m.targets.Move(1)
	case "G", "end":
		m.targets.Last()
	case "g":
		if m.pendingGG {
			m.targets.First()
			m.pendingGG = false
			m.refresh()
			return m, nil
		}
		m.pendingGG = true
		return m, nil
	case "enter":
		entry, hasCapture := m.selectedCapture()
		destination, hasDestination := m.selectedDestination()
		if hasCapture && hasDestination {
			m.assignments[entry.path] = destination.Path
			m.rebuildCaptureItems()
			m.captures.Move(1)
			m.fields.Set(routeCapturesField)
		}
	case "esc":
		m.fields.Set(routeCapturesField)
	}
	m.pendingGG = false
	m.refresh()
	return m, nil
}

func scrollViewport(view viewport.Model, key string) viewport.Model {
	switch key {
	case "up", "k":
		view.ScrollUp(1)
	case "down", "j":
		view.ScrollDown(1)
	case "g", "home":
		view.GotoTop()
	case "G", "end":
		view.GotoBottom()
	}
	return view
}

func (m *routeModel) cycleFields(forward bool) {
	order := []string{routeCapturesField, routeSummaryField, routeDestinationsField, routePlanField}
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

func (m routeModel) selectedDestination() (config.CaptureDestination, bool) {
	item, ok := m.targets.Selected()
	if !ok {
		return config.CaptureDestination{}, false
	}
	for _, destination := range m.destinations {
		if destination.Path == strings.TrimPrefix(item.ID, "destination:") {
			return destination, true
		}
	}
	return config.CaptureDestination{}, false
}

func (m *routeModel) pruneAssignments() {
	known := make(map[string]bool, len(m.entries))
	for _, entry := range m.entries {
		known[entry.path] = true
	}
	for path := range m.assignments {
		if !known[path] {
			delete(m.assignments, path)
		}
	}
}

func (m *routeModel) rebuildCaptureItems() {
	selected := ""
	if item, ok := m.captures.Selected(); ok {
		selected = item.ID
	}
	items := make([]scrolllist.Item, 0, len(m.entries))
	for _, entry := range m.entries {
		marker := "○ "
		suffix := ""
		if target, ok := m.assignments[entry.path]; ok {
			marker = "● "
			suffix = "  → " + m.destinationName(target)
		}
		items = append(items, scrolllist.Item{
			ID:    "capture:" + entry.path,
			Label: marker + entry.name + string(os.PathSeparator) + suffix,
		})
	}
	m.captures.SetItems(items)
	if selected != "" {
		m.captures.SelectID(selected)
	}
}

func (m *routeModel) rebuildDestinationItems() {
	items := make([]scrolllist.Item, 0, len(m.destinations))
	for _, destination := range m.destinations {
		items = append(items, scrolllist.Item{
			ID:    "destination:" + destination.Path,
			Label: destination.Name + "  " + displayPath(destination.Path),
		})
	}
	m.targets.SetItems(items)
}

func (m routeModel) destinationName(path string) string {
	for _, destination := range m.destinations {
		if destination.Path == path {
			return destination.Name
		}
	}
	return filepath.Base(path)
}

func (m routeModel) routedCount() int {
	count := 0
	for _, entry := range m.entries {
		if _, ok := m.assignments[entry.path]; ok {
			count++
		}
	}
	return count
}

func (m routeModel) columnWidths() (left, center, right int) {
	available := m.width - 2*columnGutter
	left = min(90, max(12, available/4))
	right = min(90, max(12, available/4))
	center = available - left - right
	return left, center, right
}

func (m routeModel) destinationsHeight() int { return max(7, m.height*3/5) }

func (m *routeModel) resizeComponents() {
	if m.width < 48 || m.height < 8 {
		return
	}
	left, center, right := m.columnWidths()
	m.captures.SetSize(max(1, left-4), max(1, m.height-2))
	m.summary.Width = max(1, center-4)
	m.summary.Height = max(1, m.height-2)
	top := m.destinationsHeight()
	m.targets.SetSize(max(1, right-4), max(1, top-2))
	m.plan.Width = max(1, right-4)
	m.plan.Height = max(1, m.height-top-2)
	m.setFieldBounds()
	m.refresh()
}

func (m *routeModel) setFieldBounds() {
	left, center, right := m.columnWidths()
	m.fields.SetBounds(routeCapturesField, datafield.Bounds{X: 0, Y: 0, Width: left, Height: m.height})
	m.fields.SetBounds(routeSummaryField, datafield.Bounds{X: left + columnGutter, Y: 0, Width: center, Height: m.height})
	rightX := left + center + 2*columnGutter
	top := m.destinationsHeight()
	m.fields.SetBounds(routeDestinationsField, datafield.Bounds{X: rightX, Y: 0, Width: right, Height: top})
	m.fields.SetBounds(routePlanField, datafield.Bounds{X: rightX, Y: top, Width: right, Height: m.height - top})
}

func (m *routeModel) refresh() {
	_, center, right := m.columnWidths()
	m.summary.SetContent(renderProperties(m.summaryProperties(), max(1, center-4), -1, false, routePropertyKeyWidth))
	m.plan.SetContent(m.planContent(max(1, right-4)))
}

func (m routeModel) summaryProperties() []property {
	entry, ok := m.selectedCapture()
	if !ok {
		return nil
	}
	index := entry.index
	destination := "· Unassigned"
	if target, assigned := m.assignments[entry.path]; assigned {
		destination = m.destinationName(target) + " · " + displayPath(target)
	}
	attachments := 0
	for _, attachment := range index.Attachments {
		if attachment.Kind != "null" {
			attachments++
		}
	}
	return []property{
		{name: "Capture", value: entry.name},
		{name: "ID", value: index.ID},
		{name: "Created", value: index.CreatedAt},
		{name: "Workflow", value: index.Source.Workflow},
		{name: "Files", value: fmt.Sprintf("%d", len(entry.files))},
		{name: "Attachments", value: fmt.Sprintf("%d", attachments)},
		{name: "Route", section: true},
		{name: "Destination", value: destination, indent: 1},
	}
}

func (m routeModel) planContent(width int) string {
	if len(m.destinations) == 0 {
		return scanMutedStyle.Render("· No destinations configured")
	}
	counts := make(map[string]int, len(m.destinations))
	for _, target := range m.assignments {
		counts[target]++
	}
	properties := make([]property, 0, len(m.destinations)+2)
	for _, destination := range m.destinations {
		properties = append(properties, property{name: destination.Name, value: fmt.Sprintf("%d", counts[destination.Path])})
	}
	properties = append(properties,
		property{separator: true},
		property{name: "Unassigned", value: fmt.Sprintf("%d", len(m.entries)-m.routedCount())},
	)
	return renderProperties(properties, width, -1, false, 0)
}

func (m routeModel) capturesColumn(width int) string {
	captures := m.captures
	captures.SetSize(max(1, width-4), max(1, m.height-2))
	content := fitHeight(captures.View(m.fields.Current() == routeCapturesField, scanTitleStyle, scanMutedStyle), m.height-2, width-4)
	if m.loadError != "" {
		content = fitHeight(scanMutedStyle.Render("! "+m.loadError), m.height-2, width-4)
	} else if len(m.entries) == 0 {
		content = fitHeight(scanMutedStyle.Render("· No captures found"), m.height-2, width-4)
	}
	return fieldset.ViewFocused("CAPTURES", content, width, m.fields.Current() == routeCapturesField)
}

func (m routeModel) summaryColumn(width int) string {
	content := viewportWithMarkers(m.summary, width-4)
	if _, ok := m.selectedCapture(); !ok {
		content = lipgloss.Place(max(1, width-4), max(1, m.height-2), lipgloss.Center, lipgloss.Center, scanMutedStyle.Render("· Select a capture"))
	}
	return fieldset.ViewFocused("CAPTURE", fitHeight(content, m.height-2, width-4), width, m.fields.Current() == routeSummaryField)
}

func (m routeModel) rightColumn(width int) string {
	top := m.destinationsHeight()
	targets := m.targets
	targets.SetSize(max(1, width-4), max(1, top-2))
	destinations := fitHeight(targets.View(m.fields.Current() == routeDestinationsField, scanTitleStyle, scanMutedStyle), top-2, width-4)
	if len(m.destinations) == 0 {
		destinations = fitHeight(scanMutedStyle.Render("· Configure capture.route.destinations"), top-2, width-4)
	}
	plan := fitHeight(viewportWithMarkers(m.plan, width-4), m.height-top-2, width-4)
	return fieldset.ViewFocused("DESTINATIONS", destinations, width, m.fields.Current() == routeDestinationsField) + "\n" +
		fieldset.ViewFocused("ROUTE PLAN", plan, width, m.fields.Current() == routePlanField)
}
