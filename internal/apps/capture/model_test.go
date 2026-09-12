package capture

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/apps/capture/indexschema"
	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestScanIsTheInitialCaptureSession(t *testing.T) {
	m := newModel()
	if got := m.fields.Current(); got != capturesField {
		t.Fatalf("initial DataField focus = %q, want %q", got, capturesField)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = updated.(model)

	view := m.View()
	for _, want := range []string{"CAPTURE ROOT", "CAPTURES", "PREVIEW", "CAPTURE INFO", "FILE INFO"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Scan workspace is missing %q:\n%s", want, view)
		}
	}
	lines := strings.Split(ansi.Strip(view), "\n")
	if len(lines) < rootHeight+1 || !strings.Contains(lines[len(lines)-rootHeight], "CAPTURE ROOT") {
		t.Fatalf("Capture Root is not the compact three-row field below Captures:\n%s", view)
	}
	if !strings.Contains(lines[0], "CAPTURES") {
		t.Fatalf("Captures is not the top field of the left column:\n%s", view)
	}
	left, _, _ := m.columnWidths()
	if got := lipgloss.Height(m.leftColumn(left)); got != m.height {
		t.Fatalf("left column height = %d, want %d", got, m.height)
	}
}

func TestSelectingExpandedIndexLoadsPreview(t *testing.T) {
	root := t.TempDir()
	capturePath := filepath.Join(root, "alpha")
	if err := os.Mkdir(capturePath, 0o755); err != nil {
		t.Fatal(err)
	}
	index := []byte(`{"schema":"v1","source":{"app":"Shortcut","workflow":"note","device":{"os":"iOS","systemVersion":"26.4.2","name":"Phone"}},"coordinates":{"altitude":0,"longitude":0,"latitude":0},"id":"capture-1","payload":{},"createdAt":"2026-09-09T16:34:35.556+10:00"}`)
	if err := os.WriteFile(filepath.Join(capturePath, "index.json"), index, 0o600); err != nil {
		t.Fatal(err)
	}
	m := newModelWithSettings(root, "index.json")
	updated, _ := m.Update(loadCaptures(root, "index.json")())
	m = updated.(model)
	updated, _ = m.updateCaptureList("o")
	m = updated.(model)
	updated, cmd := m.updateCaptureList("down")
	m = updated.(model)
	if cmd == nil || m.previewPath != filepath.Join(capturePath, "index.json") {
		t.Fatalf("index selection did not start preview: path=%q cmd=%v", m.previewPath, cmd != nil)
	}
	updated, loadCmd := m.Update(cmd())
	m = updated.(model)
	if loadCmd == nil {
		t.Fatal("settled preview request did not start loading")
	}
	updated, _ = m.Update(loadCmd())
	m = updated.(model)
	if m.preview.TotalLineCount() < 2 || !strings.Contains(m.previewLegend(), "index.json") {
		t.Fatalf("index preview was not beautified and displayed: %q", m.preview.View())
	}
}

func TestStalePreviewRequestIsIgnoredWhileBrowsing(t *testing.T) {
	m := newModel()
	m.previewPath = "/capture/new.jpg"
	m.previewRequest = 2
	updated, cmd := m.Update(previewRequestMsg{path: "/capture/old.jpg", width: 80, height: 40, requestID: 1})
	m = updated.(model)
	if cmd != nil || m.previewPath != "/capture/new.jpg" {
		t.Fatal("stale preview request was not ignored")
	}
}

func TestLoadCapturesOnlyIncludesDirectoriesWithConfiguredIndexFile(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta", "invalid", "not-a-capture"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	valid := []byte(`{"schema":"v1","source":{"app":"Shortcut","workflow":"note","device":{"os":"iOS","systemVersion":"26.4.2","name":"Phone"}},"coordinates":{"altitude":0,"longitude":0,"latitude":0},"id":"capture-1","payload":{},"attachments":[{"kind":"null"},{"kind":"Image","name":"photo.jpg"},{"kind":"Image","name":"missing.jpg"}],"createdAt":"2026-09-09T16:34:35.556+10:00"}`)
	if err := os.WriteFile(filepath.Join(root, "alpha", "capture.json"), valid, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha", "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta", "index.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "invalid", "capture.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := newModelWithSettings(root, "capture.json")
	updated, _ := m.Update(loadCaptures(root, "capture.json")().(capturesLoadedMsg))
	m = updated.(model)
	if m.captureCount != 1 {
		t.Fatalf("capture count = %d, want 1", m.captureCount)
	}
	selected, ok := m.captures.Selected()
	if !ok || selected.Label != "▸   alpha/" {
		t.Fatalf("selected Capture = %#v, %v", selected, ok)
	}
	updated, _ = m.updateCaptureList("o")
	m = updated.(model)
	selected, _ = m.captures.Selected()
	if selected.Label != "▾   alpha/" {
		t.Fatalf("expanded Capture row = %#v", selected)
	}
	m.captures.Move(1)
	selected, _ = m.captures.Selected()
	if selected.Label != "    capture.json" {
		t.Fatalf("first expanded child = %#v", selected)
	}
	m.captures.Move(1)
	selected, _ = m.captures.Selected()
	if selected.Label != "    photo.jpg" {
		t.Fatalf("valid attachment child = %#v", selected)
	}
	m.captures.Move(1)
	selected, _ = m.captures.Selected()
	if selected.Label != "    photo.jpg" {
		t.Fatalf("invalid attachments should not add rows: %#v", selected)
	}
}

func TestCaptureSettingsPopulateRootControlAndIndexFilter(t *testing.T) {
	m := newModelWithSettings("/configured/captures", "capture.json")
	if m.root != "/configured/captures" || m.indexFile != "capture.json" {
		t.Fatalf("settings = %q, %q", m.root, m.indexFile)
	}
	if got := m.rootControl.Display(); got != "/configured/captures" {
		t.Fatalf("Root control = %q", got)
	}
}

func TestCaptureListRefreshRunsAnotherScan(t *testing.T) {
	m := newModelWithSettings(t.TempDir(), "index.json")
	updated, cmd := m.updateCaptureList("R")
	m = updated.(model)
	if cmd == nil {
		t.Fatal("R did not start a Capture rescan")
	}
	if _, ok := cmd().(capturesLoadedMsg); !ok {
		t.Fatalf("refresh command returned %T, want capturesLoadedMsg", cmd())
	}
}

func TestRefreshWorksOutsideCapturesField(t *testing.T) {
	m := newModel()
	m.fields.Set(centerField)
	updated, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("global R did not start a Capture-root rescan")
	}
	if _, ok := cmd().(capturesLoadedMsg); !ok {
		t.Fatal("global refresh returned an unexpected result")
	}
}

func TestSpaceProvidesMacQuickLookForSelectedItem(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Quick Look is macOS-specific")
	}
	m := newModel()
	m.captures.SetItems([]scrolllist.Item{{ID: "file:/tmp/index.json", Label: "index.json"}})
	_, cmd := m.updateCaptureList(" ")
	if cmd == nil {
		t.Fatal("Space did not create a Quick Look command")
	}
}

func TestScanUsesCenterWideThreeColumnLayout(t *testing.T) {
	m := newModel()
	m.width = 162 // 160 cells remain after the two one-cell gutters.
	left, center, right := m.columnWidths()
	if left != 40 || center != 80 || right != 40 {
		t.Fatalf("column widths = %d, %d, %d; want 40, 80, 40", left, center, right)
	}
}

func TestJSONPreviewIsBeautified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	if err := os.WriteFile(path, []byte(`{"schema":"v1","payload":{"text":"棒"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	message := loadJSONPreview(path)().(previewLoadedMsg)
	if message.err != nil {
		t.Fatal(message.err)
	}
	for _, want := range []string{"2 │   \"schema\": \"v1\"", "3 │   \"payload\": {", "4 │     \"text\": \"棒\""} {
		if !strings.Contains(message.content, want) {
			t.Fatalf("beautified JSON is missing %q:\n%s", want, message.content)
		}
	}
}

func TestCaptureCollapseKeys(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", files: []string{"index.json", "photo.jpg"}}}
	m.expanded["/capture/alpha"] = true
	m.rebuildCaptureItems()
	m.captures.Move(2)

	updated, _ := m.updateCaptureList("O")
	m = updated.(model)
	selected, _ := m.captures.Selected()
	if selected.ID != "capture:/capture/alpha" || m.expanded["/capture/alpha"] {
		t.Fatalf("Shift+O did not collapse and select current capture: selected=%q expanded=%v", selected.ID, m.expanded)
	}

	m.expanded["/capture/alpha"] = true
	m.rebuildCaptureItems()
	updated, _ = m.updateCaptureList("w")
	m = updated.(model)
	if m.expanded["/capture/alpha"] {
		t.Fatal("w did not collapse all captures")
	}
}

func TestCaptureInfoUsesCompactRequestedOrder(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", index: indexschema.Index{
		ID: "capture-id", CreatedAt: "2026-09-09T16:34:35+10:00", Schema: "v1",
		Source:      indexschema.Source{App: "Shortcut", Workflow: "photo_note", Device: indexschema.Device{OS: "iOS", SystemVersion: "26.4", Name: "Phone"}},
		Coordinates: &indexschema.Coordinates{Latitude: -33.7, Longitude: 151.1, Altitude: 93},
	}}}
	m.rebuildCaptureItems()
	props := m.captureProperties()
	if len(props) < 6 || props[0].name != "ID" || props[1].name != "Created" || props[2].name != "Schema" || props[3].name != "Source" {
		t.Fatalf("first properties = %#v", props[:min(4, len(props))])
	}
	// Workflow is part of the Source group, and undescribed producer fields
	// are never presented.
	workflow := false
	for index, item := range props {
		switch item.name {
		case "Workflow":
			if item.indent != 1 || item.value != "photo_note" || props[index-1].name != "App" {
				t.Fatalf("Workflow is not the second Source row: %#v", props[:6])
			}
			workflow = true
		case "Type", "Done", "isDone", "dir":
			t.Fatalf("%q should not be displayed", item.name)
		}
	}
	if !workflow {
		t.Fatalf("Workflow row is missing: %#v", props)
	}
}

func TestCaptureMapValueAppearsCompleteInStatus(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", index: indexschema.Index{
		Schema: "v1", ID: "capture-id", CreatedAt: "2026-09-09T16:34:35+10:00", Coordinates: &indexschema.Coordinates{Latitude: -33.7, Longitude: 151.1},
	}}}
	m.rebuildCaptureItems()
	m.fields.Set(captureInfoField)
	properties := m.captureProperties()
	m.captureCursor = lastProperty(properties)
	status := m.Status()
	if !strings.Contains(status.Center, "https://uri.amap.com/marker?position=151.100000,-33.700000") {
		t.Fatalf("status center does not contain complete map URL: %q", status.Center)
	}
}

func TestJSONFileInfoDoesNotExposeDocumentProperties(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	if err := os.WriteFile(path, []byte(`{"secret":"not-file-metadata"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	properties, _, err := inspectFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range properties {
		if item.name == "secret" || item.value == "not-file-metadata" {
			t.Fatalf("JSON document content leaked into File Info: %#v", properties)
		}
	}
}

func TestAltNavigationFollowsFieldPositions(t *testing.T) {
	cases := []struct {
		from string
		key  tea.KeyType
		want string
	}{
		{capturesField, tea.KeyDown, rootField},
		{rootField, tea.KeyUp, capturesField},
		{capturesField, tea.KeyRight, centerField},
		{rootField, tea.KeyRight, centerField},
		{centerField, tea.KeyRight, captureInfoField},
		{captureInfoField, tea.KeyDown, fileInfoField},
		{fileInfoField, tea.KeyLeft, centerField},
	}
	for _, tc := range cases {
		m := newModel()
		m.fields.Set(tc.from)
		updated, _ := m.updateKey(tea.KeyMsg{Type: tc.key, Alt: true})
		m = updated.(model)
		if got := m.fields.Current(); got != tc.want {
			t.Errorf("alt+%v from %s = %q, want %q", tc.key, tc.from, got, tc.want)
		}
	}
}

func TestInfoKeyboardSelectionPersists(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", index: indexschema.Index{Schema: "v1", ID: "id", CreatedAt: "created", Source: indexschema.Source{Workflow: "note"}}}}
	m.rebuildCaptureItems()
	m.fields.Set(captureInfoField)
	m.captureCursor = 0
	updated, _ := m.updateInfoViewport("down", true)
	m = updated.(model)
	if m.captureCursor != 1 {
		t.Fatalf("Capture Info cursor = %d, want 1", m.captureCursor)
	}

	m.fileProperties = []property{{name: "Name", value: "a"}, {name: "Type", value: "b"}}
	m.fileCursor = 0
	updated, _ = m.updateInfoViewport("down", false)
	m = updated.(model)
	if m.fileCursor != 1 {
		t.Fatalf("File Info cursor = %d, want 1", m.fileCursor)
	}
}

func TestImageRequiresEnterBeforeRendering(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", files: []string{"photo.jpg"}}}
	m.expanded["/capture/alpha"] = true
	m.rebuildCaptureItems()
	m.captures.Move(1)
	metadataCmd := m.loadSelectedPreview()
	if metadataCmd == nil || !m.previewIsImage || m.imagePreviewed {
		t.Fatal("selecting image should load metadata without rendering")
	}
	if got := m.previewNotice; !strings.Contains(got, "Press Enter") {
		t.Fatalf("image prompt = %q", got)
	}
	updated, renderCmd := m.updateCaptureList("enter")
	m = updated.(model)
	if renderCmd == nil || !m.imagePreviewed {
		t.Fatal("Enter did not start image rendering")
	}
}

func TestLocationFlattensMultilineAddress(t *testing.T) {
	got := singleLine("28 Cambridge St\nEpping NSW 2121\r\nAustralia")
	if got != "28 Cambridge St · Epping NSW 2121 · Australia" {
		t.Fatalf("single-line location = %q", got)
	}
}

// A Capture without coordinates shows no GPS row and no map links; one
// without a place shows no Location group.
func TestCaptureInfoOmitsAbsentLocation(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", index: indexschema.Index{
		Schema: "v1", ID: "id", CreatedAt: "created", Place: &indexschema.Place{City: "Canberra", Country: "Australia"},
	}}}
	m.rebuildCaptureItems()
	for _, item := range m.captureProperties() {
		if item.name == "GPS" || item.fixed {
			t.Fatalf("a Capture without coordinates must not show %q: %#v", item.name, item)
		}
	}

	m.entries[0].index.Place = nil
	m.entries[0].index.Coordinates = &indexschema.Coordinates{Latitude: -33.7, Longitude: 151.1, Altitude: 93}
	properties := m.captureProperties()
	gps, maps := false, 0
	for _, item := range properties {
		if item.name == "GPS" {
			gps = true
		}
		if item.name == "Location" {
			t.Fatalf("a Capture without a place must not show a Location group: %#v", properties)
		}
		if item.fixed {
			maps++
		}
	}
	if !gps || maps != 3 {
		t.Fatalf("coordinates must still show GPS and three map links: gps=%t maps=%d", gps, maps)
	}
}

func TestCaptureInfoGroupsStructuredLocation(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", index: indexschema.Index{
		Schema: "v1", ID: "id", CreatedAt: "created", Place: &indexschema.Place{Locality: "28 Cambridge St", City: "Epping", Region: "NSW", Country: "Australia"},
	}}}
	m.rebuildCaptureItems()
	properties := m.captureProperties()
	want := []string{"Location", "Locality", "City", "Region", "Country"}
	position := 0
	for _, item := range properties {
		if position < len(want) && item.name == want[position] {
			position++
		}
	}
	if position != len(want) {
		t.Fatalf("structured Location group is incomplete: %#v", properties)
	}
}

// Scan answers whether a Capture has been organized: the organizer's record is
// a file in the tree, the row is marked, and Capture Info reports the pass.
func TestScanShowsTheOrganizeRecord(t *testing.T) {
	root := routeTestRoot(t, "alpha", "beta")
	run := organizer.Run{
		OrganizedAt: "2026-09-09T21:40:12+10:00",
		Recipe:      "obsidian_location_daily",
		RecipeName:  "Location + Daily",
		Actions:     []organizer.RecordedAction{{Action: organizer.ActionDailyAppend, Target: "Daily/2026-09-09.md"}},
	}
	if _, err := organizer.AppendRun(filepath.Join(root, "alpha"), run); err != nil {
		t.Fatal(err)
	}

	m := newModelWithSettings(root, "index.json")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = updated.(model)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(model)

	organizedRow, ok := m.captures.Selected()
	if !ok || !strings.Contains(organizedRow.Label, "● alpha") {
		t.Fatalf("organized capture row = %q", organizedRow.Label)
	}
	m.captures.Move(1)
	if next, _ := m.captures.Selected(); strings.Contains(next.Label, "●") {
		t.Fatalf("unorganized capture row = %q, want no marker", next.Label)
	}

	// The record is a real file in the directory and belongs in the tree.
	m.captures.First()
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = updated.(model)
	if view := ansi.Strip(m.View()); !strings.Contains(view, organizer.RecordFilename) {
		t.Fatalf("the organize record is missing from the tree:\n%s", view)
	}

	m.captures.First()
	m.refreshDetails(false)
	info := ""
	for _, property := range m.captureProperties() {
		info += property.name + " " + property.value + "\n"
	}
	for _, want := range []string{"Organized", "Recipe Location + Daily", "When 2026-09-09T21:40:12+10:00", "obsidian.daily.append · Daily/2026-09-09.md"} {
		if !strings.Contains(info, want) {
			t.Fatalf("capture info is missing %q:\n%s", want, info)
		}
	}
}

// A JSON preview is offered in two shapes: the file as written, and its
// structure. Both are rendered when the file is read, so t switches between
// them without a reload.
func TestScanTogglesJSONPreviewBetweenSourceAndTree(t *testing.T) {
	root := routeTestRoot(t, "alpha")
	m := newModelWithSettings(root, "index.json")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m = updated.(model)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(model)
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = updated.(model)
	m.captures.Move(1)
	m = settlePreview(t, m)

	if !m.previewIsJSON {
		t.Fatal("the index was not recognised as JSON")
	}
	if !strings.Contains(m.previewLegend(), "SOURCE") {
		t.Fatalf("legend = %q, want the shape named", m.previewLegend())
	}
	source := ansi.Strip(m.View())
	if !strings.Contains(source, `"schema": "v1"`) {
		t.Fatalf("source view does not show the file as written:\n%s", source)
	}

	// The switch belongs to the preview: it does nothing from another field.
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updated.(model)
	if !strings.Contains(m.previewLegend(), "SOURCE") {
		t.Fatalf("t outside the preview changed the shape: %q", m.previewLegend())
	}
	// The hint is checked for the switch itself rather than for a bare "t",
	// which any word ending in one would match.
	if strings.Contains(m.Status().Right, "t Tree") || strings.Contains(m.Status().Right, "t Source") {
		t.Fatalf("the CAPTURES hint offers the shape switch: %q", m.Status().Right)
	}

	m.fields.Set(centerField)
	if !strings.Contains(m.Status().Right, "t Tree") {
		t.Fatalf("the preview hint does not offer the switch: %q", m.Status().Right)
	}
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updated.(model)
	if !strings.Contains(m.previewLegend(), "TREE") {
		t.Fatalf("legend = %q after toggling", m.previewLegend())
	}
	if !strings.Contains(m.Status().Right, "t Source") {
		t.Fatalf("the tree hint does not offer the way back: %q", m.Status().Right)
	}
	tree := ansi.Strip(m.View())
	if !strings.Contains(tree, "schema : v1") || !strings.Contains(tree, "coordinates : {") {
		t.Fatalf("tree view does not show the structure:\n%s", tree)
	}
	if strings.Contains(tree, `"schema": "v1"`) {
		t.Fatalf("tree view still shows the source:\n%s", tree)
	}

	// The choice is remembered, so a reader who wants structure keeps it for
	// the next file.
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updated.(model)
	if !strings.Contains(m.previewLegend(), "SOURCE") {
		t.Fatalf("legend = %q after toggling back", m.previewLegend())
	}
}

// settlePreview runs the preview load chain, which hands the request off before
// reading the file.
func settlePreview(t *testing.T, m model) model {
	t.Helper()
	updated, cmd := m.Update(m.loadSelectedPreview()())
	m = updated.(model)
	for range 4 {
		if cmd == nil {
			break
		}
		updated, cmd = m.Update(cmd())
		m = updated.(model)
	}
	return m
}

// The tree draws itself for one width and one focus state, so a resize or a
// focus change has to redraw it: a row painted for a wider column runs over the
// fieldset border, and a cursor painted for a focused field stays lit after the
// focus has left it.
func TestScanJSONTreeRedrawsOnResizeAndFocusChange(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	root := routeTestRoot(t, "alpha")
	m := newModelWithSettings(root, "index.json")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m = updated.(model)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(model)
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = updated.(model)
	m.captures.Move(1)
	m = settlePreview(t, m)
	m.fields.Set(centerField)
	updated, _ = m.updatePreview("t")
	m = updated.(model)
	updated, _ = m.updatePreview("down")
	m = updated.(model)

	// Narrower: every row must still fit its column. A resize reloads the
	// preview, because an image preview is rendered for a size.
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = settlePreview(t, updated.(model))
	for index, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got != 120 {
			t.Fatalf("line %d is %d cells wide, want 120:\n%s", index, got, ansi.Strip(line))
		}
	}

	// Read the preview's own content: the CAPTURES list marks its selection
	// with the same style, and cutting a column out of the composed view
	// re-opens whatever style was active at the cut.
	marker := selectionSGR(t)
	if !strings.Contains(m.preview.View(), marker) {
		t.Fatalf("the tree cursor is not marked while the preview has focus:\n%s", ansi.Strip(m.View()))
	}
	m.fields.Set(capturesField)
	m.refreshDetails(false)
	if strings.Contains(m.preview.View(), marker) {
		t.Fatal("the tree cursor stays lit after the focus has left the preview")
	}
}

// A Capture is a folder, so double clicking it opens and closes it the way a
// file manager does. A single click only selects: clicking down the list must
// not leave a trail of opened folders behind it.
func TestScanDoubleClickTogglesACaptureFolder(t *testing.T) {
	m := newModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = updated.(model)
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", files: []string{"index.json"}}}
	m.rebuildCaptureItems()

	click := func(m model) model {
		updated, _ := m.updateMouse(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return updated.(model)
	}

	m = click(m)
	if m.expanded["/capture/alpha"] {
		t.Fatal("a single click opened the folder, want it only selected")
	}
	m = click(m)
	if !m.expanded["/capture/alpha"] {
		t.Fatal("a double click did not open the folder")
	}
	// The pair is spent, so the next two presses close it again rather than
	// the third press alone doing it.
	m = click(m)
	if !m.expanded["/capture/alpha"] {
		t.Fatal("a third click closed the folder, want the pair to have been spent")
	}
	m = click(m)
	if m.expanded["/capture/alpha"] {
		t.Fatal("a second double click did not close the folder")
	}
}

// Two presses far enough apart are two clicks, not one double click.
func TestScanSlowClicksDoNotToggleACaptureFolder(t *testing.T) {
	m := newModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = updated.(model)
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", files: []string{"index.json"}}}
	m.rebuildCaptureItems()

	updated, _ = m.updateMouse(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	m.lastClick.at = time.Now().Add(-time.Second)
	updated, _ = m.updateMouse(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.expanded["/capture/alpha"] {
		t.Fatal("two slow clicks opened the folder")
	}
}

// Scan is where a Capture is first looked at, so it is where an accident is
// first recognised: rejecting one there saves carrying it through Route only to
// throw it out at the end.
func TestScanRejectsTheSelectedCaptureIntoTheRejectFolder(t *testing.T) {
	root := routeTestRoot(t, "junk", "keeper")
	rejectRoot := filepath.Join(t.TempDir(), "Rejected")
	m := newModelWithReject(root, "index.json", rejectRoot)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = updated.(model)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(model)
	if !m.captures.SelectID("capture:" + filepath.Join(root, "junk")) {
		t.Fatal("the capture to reject is not listed")
	}

	updated, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(model)
	if _, err := os.Stat(filepath.Join(rejectRoot, "junk")); err != nil {
		t.Fatalf("backspace did not reject the capture: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "junk")); !os.IsNotExist(err) {
		t.Fatalf("the capture is still in the capture root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "keeper")); err != nil {
		t.Fatalf("another capture was moved: %v", err)
	}
	if m.notice != "junk → reject" {
		t.Fatalf("notice = %q", m.notice)
	}
	if cmd == nil {
		t.Fatal("the rejection did not reload the shared capture list")
	}
	// A file row belongs to its Capture, so rejecting from inside one moves the
	// Capture it is part of.
	if !m.captures.SelectID("capture:" + filepath.Join(root, "keeper")) {
		t.Fatal("the remaining capture is not listed")
	}
	updated, _ = m.updateCaptureList("o")
	m = updated.(model)
	updated, _ = m.updateCaptureList("down")
	m = updated.(model)
	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyDelete})
	m = updated.(model)
	if _, err := os.Stat(filepath.Join(rejectRoot, "keeper")); err != nil {
		t.Fatalf("delete on a file row did not reject its capture: %v", err)
	}
}

func TestScanSaysWhichKeyToSetWithNoRejectFolder(t *testing.T) {
	root := routeTestRoot(t, "junk")
	m := newModelWithReject(root, "index.json", "")
	updated, _ := m.Update(loadCaptures(root, "index.json")())
	m = updated.(model)

	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(model)
	if !m.noticeBad || !strings.Contains(m.notice, "capture.archive.reject") {
		t.Fatalf("notice = %q, want the key named", m.notice)
	}
	if _, err := os.Stat(filepath.Join(root, "junk")); err != nil {
		t.Fatalf("the capture should not have moved: %v", err)
	}
	// Backspace stays in the session whether or not it could do anything, so a
	// key held down while walking the list cannot leave the command.
	if !m.CapturesShellKey("backspace") {
		t.Fatal("backspace falls through to the shell")
	}
}

// The undo belongs on the screen the mistake was made on: a Capture rejected by
// accident should be taken back without changing tabs.
func TestScanUndoesARejectionFromItsOwnPane(t *testing.T) {
	root := routeTestRoot(t, "junk")
	rejectRoot := filepath.Join(t.TempDir(), "Rejected")
	m := newModelWithReject(root, "index.json", rejectRoot)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = updated.(model)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(model)
	m.captures.SelectID("capture:" + filepath.Join(root, "junk"))

	updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(model)
	if len(m.rejected) != 1 || m.rejected[0].path != filepath.Join(rejectRoot, "junk") {
		t.Fatalf("the pane did not remember the rejection: %#v", m.rejected)
	}
	if !strings.Contains(m.View(), "REJECTED") || !strings.Contains(m.View(), "junk/") {
		t.Fatal("the rejected pane does not name the folder that was sent away")
	}

	m.fields.Set(rejectedField)
	updated, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = updated.(model)
	if _, err := os.Stat(filepath.Join(root, "junk")); err != nil {
		t.Fatalf("u did not put the capture back: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rejectRoot, "junk")); !os.IsNotExist(err) {
		t.Fatalf("the capture is still in the reject folder: %v", err)
	}
	if len(m.rejected) != 0 {
		t.Fatalf("the pane still lists a restored capture: %#v", m.rejected)
	}
	if cmd == nil {
		t.Fatal("the undo started no reload")
	}
}

// Three rows, most recent first: it is a reach back rather than a history, and
// the whole reject folder is Archive's to show.
func TestScanRejectedPaneKeepsTheLastThreeMostRecentFirst(t *testing.T) {
	root := routeTestRoot(t, "one", "two", "three", "four")
	rejectRoot := filepath.Join(t.TempDir(), "Rejected")
	m := newModelWithReject(root, "index.json", rejectRoot)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = updated.(model)
	updated, _ = m.Update(loadCaptures(root, "index.json")())
	m = updated.(model)

	for _, name := range []string{"one", "two", "three", "four"} {
		m.captures.SelectID("capture:" + filepath.Join(root, name))
		updated, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyBackspace})
		m = updated.(model)
	}
	if len(m.rejected) != 4 {
		t.Fatalf("remembered %d rejections, want every one of them", len(m.rejected))
	}
	visible := m.rejectedVisible()
	if len(visible) != 3 || visible[0].name != "four" || visible[2].name != "two" {
		t.Fatalf("visible = %v, want the last three most recent first", visible)
	}
	pane := m.rejectedPane(m.columnRightWidth())
	if strings.Contains(pane, "one/") || !strings.Contains(pane, "four/") {
		t.Fatalf("the pane shows more than it has room for:\n%s", pane)
	}
}
