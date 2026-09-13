package photo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestPhotoAppInjectsConfiguredImportStateFilename(t *testing.T) {
	app := New()
	model := app.Commands[0].NewWithConfig(config.Config{
		Photo: config.Photo{Import: config.PhotoImport{
			StateFile:   ".custom-photo-state",
			Source:      "~/Pictures/camera",
			Destination: "/Volumes/Photos/import",
		}},
	}).(importModel)
	if model.stateFilename != ".custom-photo-state" {
		t.Fatalf("state filename = %q", model.stateFilename)
	}
	if model.paths[sourceField] != "~/Pictures/camera" || model.paths[destinationField] != "/Volumes/Photos/import" {
		t.Fatalf("configured paths = %#v", model.paths)
	}
}

func TestImportStartsWithDirectoriesOnly(t *testing.T) {
	model := newImportModel()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 126, Height: 30})
	model = updated.(tui.CommandModel)

	view := model.View()
	for _, text := range []string{
		"SETUP  ·  PHOTO IMPORT",
		"Choose directories",
		"Select the source and destination to scan",
		"Directories",
		"Source",
		"testdata/photo-import/src",
		"Destination",
		"testdata/photo-import/dst",
		"Next →",
	} {
		if !strings.Contains(view, text) {
			t.Errorf("setup view does not contain %q:\n%s", text, view)
		}
	}
	for _, hidden := range []string{"Operation", "Extensions", "Duplicates", "Review import"} {
		if strings.Contains(view, hidden) {
			t.Errorf("setup view unexpectedly contains %q:\n%s", hidden, view)
		}
	}
	lines := strings.Split(view, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "" || strings.TrimSpace(lines[1]) != "" {
		t.Errorf("large workspace does not give the page header two rows of breathing room:\n%s", view)
	}
	if strings.Contains(view, "──›") || strings.Contains(view, "● ─") {
		t.Errorf("setup view still contains the animated data-flow line:\n%s", view)
	}
	if strings.Contains(view, "Archive") {
		t.Errorf("setup view still uses Archive:\n%s", view)
	}
	if got := lipgloss.Width(view); got != 126 {
		t.Errorf("view width = %d, want 126", got)
	}
	if got := lipgloss.Height(view); got != 30 {
		t.Errorf("view height = %d, want 30", got)
	}

	status := model.Status()
	if status.Left != "READY" || !strings.Contains(status.Center, "Directories") || !strings.Contains(status.Center, "Post-processing") {
		t.Errorf("status = %#v", status)
	}
}

func TestDirectoryPickerIsAWorkspaceOverlay(t *testing.T) {
	model := newImportModel().(importModel)
	model.paths[sourceField] = t.TempDir()
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if cmd != nil {
		updated, _ = model.Update(cmd())
		model = updated.(importModel)
	}

	view := model.View()
	if got := lipgloss.Width(view); got != model.width {
		t.Fatalf("overlay width = %d, want %d", got, model.width)
	}
	if got := lipgloss.Height(view); got != model.height {
		t.Fatalf("overlay height = %d, want %d", got, model.height)
	}
	for _, want := range []string{"FILE EXPLORER · SOURCE", "╭", "╯"} {
		if !strings.Contains(view, want) {
			t.Fatalf("workspace does not contain %q in floating picker:\n%s", want, view)
		}
	}
}

func TestPathFieldOpensDirectoryPickerAndSelectsDirectory(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "selected")
	if err := os.Mkdir(want, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "not-selectable.txt"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}

	model := newImportModel().(importModel)
	model.paths[sourceField] = root
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if !model.picking || !model.CapturesShellKey("esc") {
		t.Fatal("source did not open the directory picker")
	}
	if cmd == nil {
		t.Fatal("picker did not request its initial directory listing")
	}
	updated, _ = model.Update(cmd())
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if model.picking {
		t.Fatal("Enter did not close the picker after choosing a directory")
	}
	if got := model.paths[sourceField]; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if !strings.Contains(model.Status().Center, "Directories") {
		t.Fatal("choosing a path changed scan state")
	}
}

func TestChooseTreeRoot(t *testing.T) {
	root := t.TempDir()
	model := newImportModel().(importModel)
	model.paths[destinationField] = root
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if model.picking {
		t.Fatal("Enter did not choose the tree root")
	}
	if got := expandHome(model.paths[destinationField]); got != root {
		t.Fatalf("destination = %q, want %q", got, root)
	}
}

func TestImportFormSupportsSharedControlNavigation(t *testing.T) {
	model := newImportModel().(importModel)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(importModel)
	if model.controls.FocusedID() != destinationID {
		t.Fatalf("j focused %q, want destination", model.controls.FocusedID())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updated.(importModel)
	if model.controls.FocusedID() != sourceID {
		t.Fatalf("k focused %q, want source", model.controls.FocusedID())
	}
}

func TestImportControlsCanBeChanged(t *testing.T) {
	model := newImportModel().(importModel)
	model.width = 160
	model.stage = parameterStage
	model.parameterFields.Set("parameters")
	model.controls.SetFocusID(duplicatesID)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if !model.controls.IsActive() || !strings.Contains(model.View(), "Replace") {
		t.Fatal("Enter did not open the duplicate option menu")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if got := model.controls.Value(duplicatesID); got != "Replace" {
		t.Fatalf("duplicates = %q, want Replace", got)
	}

	model.controls.SetOptions(extensionsID, []string{"JPG", "DNG"}, true)
	model.extensionsConfigured = true
	model.controls.SetFocusID(extensionsID)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(importModel)
	if got := model.controls.Values(extensionsID); len(got) != 1 || got[0] != "DNG" {
		t.Fatalf("extensions = %#v, want DNG only", got)
	}
}

func TestImportNavigationMatchesParameterLayout(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = parameterStage
	model.parameterFields.Set("parameters")
	model.controls.SetFocusID(operationID)
	want := []string{extensionsID, duplicatesID, parallelID, operationID}
	for _, wantID := range want {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(importModel)
		if got := model.controls.FocusedID(); got != wantID {
			t.Fatalf("focused %q, want %q", got, wantID)
		}
	}
}

func TestProcessingUsesConfiguredWorkersAndLandscapeLayout(t *testing.T) {
	root := t.TempDir()
	sourceRoot, destinationRoot := filepath.Join(root, "src"), filepath.Join(root, "dst")
	for _, name := range []string{"one.JPG", "two.DNG", "three.JPG", "four.DNG"} {
		if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceRoot, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	model := newImportModel().(importModel)
	model.width, model.height = 140, 28
	model.stage = parameterStage
	model.paths = [pathFieldCount]string{sourceRoot, destinationRoot}
	model.scan.source = []scannedFile{{path: "one.JPG", size: 100}, {path: "two.DNG", size: 200}, {path: "three.JPG", size: 300}, {path: "four.DNG", size: 400}}
	model.configureExtensions()
	model.controls.SetValue(parallelID, "3")
	updated, cmd := model.startProcessing()
	model = updated.(importModel)
	if cmd == nil || model.stage != processingStage || len(model.processing.workers) != 3 {
		t.Fatalf("stage=%v workers=%d cmd=%v", model.stage, len(model.processing.workers), cmd != nil)
	}
	view := model.View()
	for _, want := range []string{"SHA-256 VERIFIED BEFORE PUBLISH", "Workers", "Next files", "Recent results", "["} {
		if !strings.Contains(view, want) {
			t.Errorf("processing view missing %q:\n%s", want, view)
		}
	}
	if lipgloss.Width(view) != 140 || lipgloss.Height(view) != 28 {
		t.Fatalf("processing view size = %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	for !model.processingComplete() {
		message := cmd()
		updated, cmd = model.Update(message)
		model = updated.(importModel)
	}
	if len(model.processing.verified) != 4 {
		t.Fatalf("verified=%d, want 4", len(model.processing.verified))
	}
}

func TestProcessingGuardsReturnAndExit(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = processingStage
	model.processing = processingState{
		files:   []scannedFile{{path: "one.JPG"}},
		workers: []processingWorker{{number: 1, file: scannedFile{path: "one.JPG"}, phase: workerCopying}},
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(importModel)
	if !model.leaveConfirm || model.leaveExits || model.stage != processingStage {
		t.Fatalf("Esc did not guard return: confirm=%v exits=%v stage=%v", model.leaveConfirm, model.leaveExits, model.stage)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if model.leaveConfirm || model.stage != processingStage || model.processing.paused {
		t.Fatalf("cancel did not resume: confirm=%v stage=%v paused=%v", model.leaveConfirm, model.stage, model.processing.paused)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = updated.(importModel)
	if !model.leaveConfirm || !model.leaveExits {
		t.Fatal("q did not guard application exit")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(importModel)
	model.processing.complete = true
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmed exit did not return a quit command")
	}
}

func TestProcessingStatusDistinguishesRunningPausedAndComplete(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = processingStage
	model.processing = processingState{
		files:   []scannedFile{{path: "one.JPG"}},
		workers: []processingWorker{{number: 1, file: scannedFile{path: "one.JPG"}, phase: workerCopying}},
	}
	if status := model.Status(); status.Left != "PROCESSING · RUNNING" || !strings.Contains(status.Right, "p Pause") {
		t.Fatalf("running status = %#v", status)
	}
	model.processing.paused = true
	if status := model.Status(); status.Left != "PROCESSING · PAUSED" || !strings.Contains(status.Right, "p Resume") {
		t.Fatalf("paused status = %#v", status)
	}
	model.processing.paused = false
	model.processing.verified = append(model.processing.verified, model.processing.files[0])
	model.processing.workers[0].phase = workerDone
	model.processing.complete = true
	if status := model.Status(); status.Left != "PROCESSING · COMPLETE" || strings.Contains(status.Right, "p Pause") {
		t.Fatalf("complete status = %#v", status)
	}
}

func TestCompletedProcessingOpensLandscapeResultPreview(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 140, 28
	model.stage = processingStage
	model.processing = processingState{
		files:    []scannedFile{{path: "DCIM/one.JPG", size: 2048}, {path: "DCIM/two.DNG", size: 4096}},
		verified: []scannedFile{{path: "DCIM/one.JPG", size: 2048}, {path: "DCIM/two.DNG", size: 4096}},
		workers:  []processingWorker{{number: 1, phase: workerDone}},
		complete: true,
		results:  []importer.FileResult{{Index: 0, Source: "DCIM/one.JPG", Destination: "one.JPG", Size: 2048, Phase: importer.PhaseComplete}, {Index: 1, Source: "DCIM/two.DNG", Destination: "two.DNG", Size: 4096, Phase: importer.PhaseComplete}},
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if model.stage != resultStage || !model.controls.Checked(deleteStateID) {
		t.Fatalf("result stage=%v delete-state=%v", model.stage, model.controls.Checked(deleteStateID))
	}
	view := model.View()
	for _, want := range []string{"Import result", "Verified files", "Verification summary", "State file", "SHA-256 MATCH", "Delete .dgs-state", "TRANSFER COMPLETE", "Source", "Destination", "Published 6.0 KB", "Again", "Quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("result view missing %q:\n%s", want, view)
		}
	}
	if lipgloss.Width(view) != 140 || lipgloss.Height(view) != 28 {
		t.Fatalf("result view size = %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	if status := model.Status(); status.Left != "RESULT · VERIFIED" || !strings.Contains(status.Center, "● Result") {
		t.Fatalf("result status = %#v", status)
	}
}

func TestPostProcessingPageOffersExplicitSkip(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = processingStage
	model.processing.complete = true
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if model.stage != postprocessStage || !model.postprocessPrompt || cmd != nil || !strings.Contains(model.View(), "ORGANIZE BY CAPTURE DATE?") {
		t.Fatalf("post-process prompt stage=%v prompt=%v cmd=%v", model.stage, model.postprocessPrompt, cmd != nil)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if model.stage != resultStage || !model.postprocess.skipped {
		t.Fatalf("skip stage=%v skipped=%v", model.stage, model.postprocess.skipped)
	}
}

func TestScanViewRepeatsSelectedPaths(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 100, 24
	model.paths = [pathFieldCount]string{"/Volumes/CAMERA/DCIM", "/Volumes/PHOTOS/Import"}
	model.stage = scanStage
	view := model.View()
	for _, want := range []string{"Source", "/Volumes/CAMERA/DCIM", "Destination", "/Volumes/PHOTOS/Import"} {
		if !strings.Contains(view, want) {
			t.Fatalf("scan view missing %q:\n%s", want, view)
		}
	}
}

func TestResultAgainReturnsToDirectoriesAndKeepsPaths(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = resultStage
	model.paths = [pathFieldCount]string{"/source", "/destination"}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(importModel)
	if model.stage != setupStage || model.paths != [pathFieldCount]string{"/source", "/destination"} {
		t.Fatalf("restart stage=%v paths=%v", model.stage, model.paths)
	}
}

func TestResultConfirmedCtrlCDeletesSelectedStateFileBeforeExit(t *testing.T) {
	destination := t.TempDir()
	statePath := filepath.Join(destination, ".photo-import-state")
	if err := os.WriteFile(statePath, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := newImportModelWithStateFile(".photo-import-state").(importModel)
	model.stage = resultStage
	model.paths[destinationField] = destination
	if !strings.Contains(model.controls.View([]string{deleteStateID}), "Delete .photo-import-state") {
		t.Fatal("configured state filename is not shown on Result")
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(importModel)
	if !model.leaveConfirm || model.stage != resultStage {
		t.Fatalf("ctrl-c confirm=%v stage=%v", model.leaveConfirm, model.stage)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(importModel)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	if cmd == nil {
		t.Fatal("confirmed ctrl-c did not quit")
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file remains after confirmed exit: %v", err)
	}
}

func TestResultAgainHonorsRetainStateSelection(t *testing.T) {
	destination := t.TempDir()
	statePath := filepath.Join(destination, ".dgs-state")
	if err := os.WriteFile(statePath, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := newImportModel().(importModel)
	model.stage = resultStage
	model.paths[destinationField] = destination
	model.controls.SetFocusID(deleteStateID)
	model.controls.HandleInteraction(" ")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(importModel)
	if model.stage != setupStage {
		t.Fatalf("again stage=%v", model.stage)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("unchecked state file was removed: %v", err)
	}
}

func TestMouseClickSwitchesParameterDataFieldFocus(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 160, 30
	model.stage = parameterStage
	model.parameterFields.Set("parameters")
	layout := model.parameterLayout()

	updated, _ := model.Update(tea.MouseMsg{
		X:      layout.leftWidth + layout.middleWidth + 3,
		Y:      2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	model = updated.(importModel)
	if got := model.parameterFields.Current(); got != "destination-results" {
		t.Fatalf("click focused %q, want destination-results", got)
	}

	updated, _ = model.Update(tea.MouseMsg{
		X:      2,
		Y:      layout.parameterHeight + 2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	model = updated.(importModel)
	if got := model.parameterFields.Current(); got != "summary" {
		t.Fatalf("click focused %q, want summary", got)
	}
}

func TestMouseClickOperatesParameterControls(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 160, 30
	model.stage = parameterStage
	updated, _ := model.Update(tea.MouseMsg{
		X:      33,
		Y:      2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	model = updated.(importModel)
	if model.controls.Value(operationID) != "Move" {
		t.Fatalf("radio click selected %q, want Move", model.controls.Value(operationID))
	}
	if model.parameterFields.Current() != "parameters" {
		t.Fatal("control click did not focus Parameters")
	}
}

func TestMouseWheelScrollsHoveredListWithoutChangingFocus(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 160, 30
	model.stage = parameterStage
	model.parameterFields.Set("parameters")
	for i := 0; i < 40; i++ {
		model.scan.source = append(model.scan.source, scannedFile{path: fmt.Sprintf("photo-%02d.jpg", i)})
	}
	model.syncResultLists()
	updated, _ := model.Update(tea.MouseMsg{
		X:      model.parameterLayout().leftWidth + 2,
		Y:      3,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	model = updated.(importModel)
	if model.sourceList.Top() != 3 {
		t.Fatalf("wheel set source top to %d, want 3", model.sourceList.Top())
	}
	if got := model.parameterFields.Current(); got != "parameters" {
		t.Fatalf("wheel changed focus to %q", got)
	}
}

func TestClickSelectsNumberedRowAndSpaceProvidesQuickLookCommand(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 160, 30
	model.stage = parameterStage
	model.scan.source = []scannedFile{{path: "first.jpg"}, {path: "second.jpg"}, {path: "third.jpg"}}
	model.syncResultLists()
	updated, _ := model.Update(tea.MouseMsg{X: model.parameterLayout().leftWidth + 8, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	model = updated.(importModel)
	if model.parameterFields.Current() != "source-results" || model.sourceList.Cursor() != 1 {
		t.Fatalf("field=%q cursor=%d", model.parameterFields.Current(), model.sourceList.Cursor())
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(importModel)
	if cmd == nil || !strings.Contains(model.actionNotice, "second.jpg") {
		t.Fatal("Space did not prepare Quick Look for the selected row")
	}
}

func TestRightClickOpensReusableContextMenuWithoutChangingFocus(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 160, 30
	model.stage = parameterStage
	model.parameterFields.Set("parameters")
	model.scan.source = []scannedFile{{path: "first.jpg"}, {path: "second.jpg"}}
	model.syncResultLists()
	updated, _ := model.Update(tea.MouseMsg{X: model.parameterLayout().leftWidth + 8, Y: 4, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	model = updated.(importModel)
	if !model.menu.IsOpen() || model.sourceList.Cursor() != 1 {
		t.Fatal("right click did not select the row and open its menu")
	}
	if model.parameterFields.Current() != "parameters" {
		t.Fatal("right click unexpectedly changed data-field focus")
	}
	if !strings.Contains(model.View(), "Open with default app") {
		t.Fatal("context menu is not rendered over the workspace")
	}
}

func TestNextShortcutStartsScanAndOpensParameterScreen(t *testing.T) {
	model := newImportModel().(importModel)
	model.width = 160
	if model.controls.FocusedID() != sourceID {
		t.Fatal("test must begin on Source")
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(importModel)
	if model.stage != scanStage || model.Status().Left != "SCAN · READING" || cmd == nil {
		t.Fatal("n did not start the scan screen")
	}
	if !strings.Contains(model.View(), "Reading source and destination") || !strings.Contains(model.View(), "▐") {
		t.Fatal("scan screen does not show progress")
	}
	summary := scanDirectories(model.paths)
	updated, _ = model.Update(scanDoneMsg{generation: model.scanGeneration, summary: summary})
	model = updated.(importModel)
	if model.stage != parameterStage || !strings.Contains(model.View(), "Source") || !strings.Contains(model.View(), "Destination") || !strings.Contains(model.View(), "Parameters") {
		t.Fatal("scan completion did not open the parameter screen")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(importModel)
	if model.stage != setupStage || !strings.Contains(model.View(), "Next →") || !strings.Contains(model.View(), "Parameters") {
		t.Fatal("Esc did not return from Parameters to Setup")
	}
}

func TestScanDirectoriesCollectsBothInventories(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "one.jpg"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "existing.dng"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary := scanDirectories([pathFieldCount]string{source, destination})
	if len(summary.errors) != 0 || len(summary.source) != 1 || len(summary.destination) != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.source[0].path != "nested/one.jpg" || summary.destination[0].path != "existing.dng" {
		t.Fatalf("unexpected files: %#v", summary)
	}
}

func TestCancelledScanIgnoresLateResult(t *testing.T) {
	model := newImportModel().(importModel)
	updated, _ := model.startScan()
	model = updated.(importModel)
	generation := model.scanGeneration
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(importModel)
	updated, _ = model.Update(scanDoneMsg{generation: generation, summary: scanSummary{source: []scannedFile{{path: "late.jpg"}}}})
	model = updated.(importModel)
	if model.stage != setupStage || len(model.scan.source) != 0 {
		t.Fatal("cancelled scan accepted a late result")
	}
}

func TestParameterScreenUsesFourColumnLayout(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 160, 28
	model.stage = parameterStage
	model.paths = [pathFieldCount]string{"/Volumes/SD", "/Volumes/Photos/Import"}
	model.parameterFields.Set("parameters")
	model.scan = scanSummary{
		source:      []scannedFile{{path: "DCIM/IMG_0001.JPG", size: 10}, {path: "DCIM/IMG_0002.DNG", size: 20}},
		destination: []scannedFile{{path: "Existing/IMG_0001.JPG", size: 10}},
	}
	model.syncResultLists()
	view := model.View()
	for _, want := range []string{"Source files", "Destination files", "JPG only · no RAW", "RAW only · no JPG", "Parameters", "Import summary", "Eligible", "Duplicates", "Will copy", "Workers"} {
		if !strings.Contains(view, want) {
			t.Errorf("split view does not contain %q:\n%s", want, view)
		}
	}
	if got := lipgloss.Width(view); got != 160 {
		t.Fatalf("split view width = %d, want 160", got)
	}
	firstLine := strings.Split(view, "\n")[0]
	if !strings.HasSuffix(firstLine, "╮") {
		t.Fatalf("right pane is not flush with terminal edge: %q", firstLine)
	}
	lines := strings.Split(view, "\n")
	buttonLine := ""
	for _, line := range lines {
		if strings.Contains(line, "Directories") && strings.Contains(line, "Processing") {
			buttonLine = line
			break
		}
	}
	if buttonLine == "" || strings.Index(buttonLine, "Directories") < model.parameterLayout().leftWidth+2*model.parameterLayout().middleWidth {
		t.Fatalf("page actions are not anchored in the bottom-right pane:\n%s", view)
	}
	if !strings.Contains(view, "Root  "+model.paths[sourceField]) || !strings.Contains(view, "Root  "+model.paths[destinationField]) {
		t.Fatal("source and destination roots are not visible above their relative file lists")
	}
}

func TestAltNavigationMovesBetweenParameterDataFields(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = parameterStage
	model.parameterFields.Set("parameters")
	tests := []struct{ key, want string }{{"alt+l", "source-results"}, {"alt+l", "destination-results"}, {"alt+l", "jpeg-only-results"}, {"alt+j", "raw-only-results"}, {"alt+h", "destination-results"}, {"alt+h", "source-results"}, {"alt+h", "parameters"}, {"alt+j", "summary"}}
	for _, test := range tests {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(test.key[len(test.key)-1])}, Alt: true})
		model = updated.(importModel)
		if got := model.parameterFields.Current(); got != test.want {
			t.Fatalf("%s focused %q, want %q", test.key, got, test.want)
		}
	}
}

func TestResultListsUseVimNavigationInsteadOfMoreText(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 160, 20
	model.stage = parameterStage
	for index := range 20 {
		model.scan.source = append(model.scan.source, scannedFile{path: fmt.Sprintf("deep/path/with/many/nested/directories/file-%02d-with-an-extraordinarily-long-name.jpg", index)})
	}
	model.syncResultLists()
	model.parameterFields.Set("source-results")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	model = updated.(importModel)
	if model.sourceList.Cursor() != 19 || model.sourceList.Top() == 0 {
		t.Fatalf("G did not move to the end: cursor=%d top=%d", model.sourceList.Cursor(), model.sourceList.Top())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(importModel)
	if model.sourceList.Cursor() != 0 || model.sourceList.Top() != 0 {
		t.Fatal("gg did not return to the first file")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = updated.(importModel)
	if model.sourceList.Horizontal() == 0 {
		t.Fatal("l did not scroll a long path horizontally")
	}
	if strings.Contains(model.View(), "files more") {
		t.Fatal("result list still uses a remaining-files message")
	}
}

func TestImportSummaryRespondsToParameters(t *testing.T) {
	model := newImportModel().(importModel)
	// Duplicates compare the full relative path, because Destination mirrors the
	// Source hierarchy: "old/two.dng" does not collide with "two.DNG" at the root.
	model.scan = scanSummary{
		source:      []scannedFile{{path: "one.JPG"}, {path: "two.DNG"}, {path: "notes.txt"}},
		destination: []scannedFile{{path: "one.jpg"}, {path: "old/two.dng"}},
	}
	plan := model.buildPlan()
	if plan.eligible != 3 || plan.duplicates != 1 || plan.skipped != 1 || plan.processed != 2 {
		t.Fatalf("default plan = %#v", plan)
	}
	model.controls.SetOptions(extensionsID, []string{"DNG", "JPG", "TXT"}, true)
	model.controls.SetValues(extensionsID, []string{"DNG"})
	model.extensionsConfigured = true
	model.controls.SetValue(operationID, "Move")
	plan = model.buildPlan()
	if plan.eligible != 1 || plan.duplicates != 0 || plan.processed != 1 || plan.operation != "move" {
		t.Fatalf("filtered plan = %#v", plan)
	}
}

func TestExtensionsComeFromSourceAndFilterItsViewport(t *testing.T) {
	model := newImportModel().(importModel)
	model.scan.source = []scannedFile{{path: "one.JPG"}, {path: "two.DNG"}, {path: "notes.txt"}, {path: "three.jpg"}}
	model.scan.destination = []scannedFile{{path: "existing.PNG"}}
	model.configureExtensions()
	if got := model.controls.Values(extensionsID); len(got) != 3 || got[0] != "DNG" || got[1] != "JPG" || got[2] != "TXT" {
		t.Fatalf("source extensions = %#v", got)
	}
	model.controls.SetValues(extensionsID, []string{"JPG"})
	model.syncResultLists()
	if len(model.filteredSource()) != 2 {
		t.Fatalf("filtered source has %d files, want 2", len(model.filteredSource()))
	}
	if model.sourceList.Cursor() != 0 {
		t.Fatal("filtered viewport did not clamp selection")
	}
	model.controls.SetFocusID(extensionsID)
	model.controls.HandleInteraction("enter")
	model.controls.HandleInteraction("enter")
	model.syncResultLists()
	if len(model.filteredSource()) != 4 {
		t.Fatal("All did not restore every source extension")
	}
}

func TestParameterScreenRequestsMinimumWidthWithoutOverflow(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 60, 22
	model.stage = parameterStage
	view := model.View()
	if !strings.Contains(view, "at least 140 columns") {
		t.Fatalf("narrow view does not explain its minimum width:\n%s", view)
	}
	if got := lipgloss.Width(view); got != 60 {
		t.Fatalf("narrow view width = %d, want 60", got)
	}
}

func TestEscapeCancelsDirectoryPicker(t *testing.T) {
	model := newImportModel().(importModel)
	original := model.paths[sourceField]
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(importModel)
	if model.picking {
		t.Fatal("Esc did not close the picker")
	}
	if got := model.paths[sourceField]; got != original {
		t.Fatalf("cancelled source = %q, want %q", got, original)
	}
}

func TestExplorerDialogHintsAreNotRepeatedInStatusBar(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "delete-me"), 0o755); err != nil {
		t.Fatal(err)
	}
	model := newImportModel().(importModel)
	model.paths[sourceField] = root
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(importModel)
	updated, _ = model.Update(cmd())
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(importModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(importModel)

	if status := model.Status(); status.Right != "" {
		t.Fatalf("dialog controls repeated in status bar: %q", status.Right)
	}
	if !strings.Contains(model.View(), "y confirm") {
		t.Fatal("delete confirmation is missing from the explorer operation area")
	}
	if count := strings.Count(model.View(), "n/esc cancel"); count != 1 {
		t.Fatalf("delete cancel hint appears %d times, want once", count)
	}
}

func TestBuildJobsMirrorsSourceHierarchy(t *testing.T) {
	jobs := buildJobs("/card", "/library", []scannedFile{
		{path: "DCIM/100MSDCF/IMG_0001.JPG"},
		{path: "DCIM/101MSDCF/IMG_0001.JPG"},
		{path: "IMG_0002.JPG"},
	})
	want := []string{
		filepath.Join("/library", "DCIM", "100MSDCF", "IMG_0001.JPG"),
		filepath.Join("/library", "DCIM", "101MSDCF", "IMG_0001.JPG"),
		filepath.Join("/library", "IMG_0002.JPG"),
	}
	seen := map[string]struct{}{}
	for index, job := range jobs {
		if job.Destination != want[index] {
			t.Fatalf("job %d destination = %q, want %q", index, job.Destination, want[index])
		}
		if _, exists := seen[job.Destination]; exists {
			t.Fatalf("two jobs share destination %q", job.Destination)
		}
		seen[job.Destination] = struct{}{}
	}
}

func TestClassifySidecarsUsesDirectoryAndStem(t *testing.T) {
	files := []scannedFile{
		{path: "DCIM/IMG_0001.JPG"},
		{path: "DCIM/IMG_0001.DNG"},
		{path: "DCIM/IMG_0002.JPG"},
		{path: "DCIM/IMG_0003.NEF"},
		{path: "OTHER/IMG_0003.JPG"},
	}
	classifySidecars(files)
	if !files[0].hasSidecar || !files[1].hasSidecar {
		t.Fatal("matching JPG and RAW were not paired")
	}
	for _, index := range []int{2, 3, 4} {
		if files[index].hasSidecar {
			t.Fatalf("file %q was paired across a missing stem or directory", files[index].path)
		}
	}
}

func TestSidecarExceptionDataFieldsContainOnlyUnpairedFiles(t *testing.T) {
	model := newImportModel().(importModel)
	model.scan.source = []scannedFile{
		{path: "DCIM/IMG_0001.JPG"},
		{path: "DCIM/IMG_0001.DNG"},
		{path: "DCIM/IMG_0002.JPG"},
		{path: "DCIM/IMG_0003.NEF"},
	}
	classifySidecars(model.scan.source)
	model.syncResultLists()
	jpeg, jpegOK := model.jpegOnlyList.Selected()
	raw, rawOK := model.rawOnlyList.Selected()
	if !jpegOK || jpeg.ID != "DCIM/IMG_0002.JPG" {
		t.Fatalf("JPG-only list selected %#v, ok=%v", jpeg, jpegOK)
	}
	if !rawOK || raw.ID != "DCIM/IMG_0003.NEF" {
		t.Fatalf("RAW-only list selected %#v, ok=%v", raw, rawOK)
	}
}
