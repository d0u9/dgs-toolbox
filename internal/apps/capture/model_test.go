package capture

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/capture/indexschema"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestScanIsTheInitialCaptureSession(t *testing.T) {
	m := newModel()
	if got := m.fields.Current(); got != capturesField {
		t.Fatalf("initial DataField focus = %q, want %q", got, capturesField)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = updated.(model)

	tabs := m.Tabs()
	if len(tabs) != 1 || tabs[0] != (tui.Tab{Label: "Scan", Active: true}) {
		t.Fatalf("tabs = %#v", tabs)
	}
	view := m.View()
	for _, want := range []string{"CAPTURE ROOT", "CAPTURES", "PREVIEW", "CAPTURE INFO", "FILE INFO"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Scan workspace is missing %q:\n%s", want, view)
		}
	}
	lines := strings.Split(ansi.Strip(view), "\n")
	if len(lines) < 4 || !strings.Contains(lines[3], "CAPTURES") {
		t.Fatalf("Capture Root is not the compact three-row field:\n%s", view)
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
	index := []byte(`{"schema":"v1","source":{"app":"Shortcut","device":{"os":"iOS","systemVesion":"26.4.2","name":"Phone"}},"position":{"altitude":0,"longitude":0,"latitude":0},"id":"capture-1","isDone":"true","payload":{},"createdAt":"2026-09-09T16:34:35.556+10:00","type":"note","dir":"capture-1"}`)
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
	valid := []byte(`{"schema":"v1","source":{"app":"Shortcut","device":{"os":"iOS","systemVesion":"26.4.2","name":"Phone"}},"position":{"altitude":0,"longitude":0,"latitude":0},"id":"capture-1","isDone":"true","payload":{},"attachments":[{"kind":"null"},{"kind":"Image","name":"photo.jpg"},{"kind":"Image","name":"missing.jpg"}],"createdAt":"2026-09-09T16:34:35.556+10:00","type":"note","dir":"capture-1"}`)
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
	if !ok || selected.Label != "▸ alpha/" {
		t.Fatalf("selected Capture = %#v, %v", selected, ok)
	}
	updated, _ = m.updateCaptureList("o")
	m = updated.(model)
	selected, _ = m.captures.Selected()
	if selected.Label != "▾ alpha/" {
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
	if got := m.controls.Value(rootPathID); got != "/configured/captures" {
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
		ID: "capture-id", CreatedAt: "2026-09-09T16:34:35+10:00", Type: "photo_note", IsDone: "true",
		Source:   indexschema.Source{App: "Shortcut", Device: indexschema.Device{OS: "iOS", SystemVersion: "26.4", Name: "Phone"}},
		Position: indexschema.Position{Latitude: -33.7, Longitude: 151.1, Altitude: 93},
	}}}
	m.rebuildCaptureItems()
	props := m.captureProperties()
	if len(props) < 4 || props[0].name != "ID" || props[1].name != "Created" || props[2].name != "Type" || props[3].name != "Schema" {
		t.Fatalf("first properties = %#v", props[:min(4, len(props))])
	}
	for _, item := range props {
		if item.name == "Done" || item.name == "isDone" {
			t.Fatal("isDone should not be displayed")
		}
	}
}

func TestCaptureMapValueAppearsCompleteInStatus(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", index: indexschema.Index{
		Schema: "v1", ID: "capture-id", CreatedAt: "2026-09-09T16:34:35+10:00", Type: "photo_note",
		Position: indexschema.Position{Latitude: -33.7, Longitude: 151.1},
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

func TestAltJKCyclesPrimaryScanFieldsInRequestedOrder(t *testing.T) {
	m := newModel()
	order := []string{centerField, captureInfoField, fileInfoField, capturesField}
	for _, want := range order {
		updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}, Alt: true})
		m = updated.(model)
		if got := m.fields.Current(); got != want {
			t.Fatalf("Alt+J focus = %q, want %q", got, want)
		}
	}
	updated, _ := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}, Alt: true})
	m = updated.(model)
	if got := m.fields.Current(); got != fileInfoField {
		t.Fatalf("Alt+K focus = %q, want %q", got, fileInfoField)
	}
}

func TestInfoKeyboardSelectionPersists(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", index: indexschema.Index{Schema: "v1", ID: "id", CreatedAt: "created", Type: "type"}}}
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
	got := singleLineAddress("28 Cambridge St\nEpping NSW 2121\r\nAustralia")
	if got != "28 Cambridge St · Epping NSW 2121 · Australia" {
		t.Fatalf("single-line location = %q", got)
	}
}

func TestCaptureInfoGroupsStructuredLocation(t *testing.T) {
	m := newModel()
	m.entries = []captureEntry{{path: "/capture/alpha", name: "alpha", index: indexschema.Index{
		Schema: "v1", ID: "id", CreatedAt: "created", Type: "been_here",
		Position: indexschema.Position{Locality: "28 Cambridge St", City: "Epping", LegacyRegion: "NSW", Country: "Australia"},
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
