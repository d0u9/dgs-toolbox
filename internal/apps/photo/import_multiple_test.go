package photo

import (
	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/config"
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func multipleImportModel(t *testing.T) importModel {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "card")
	targets := []string{filepath.Join(root, "library-a"), filepath.Join(root, "library-b")}
	for _, path := range append([]string{source}, targets...) {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "a.JPG"), testJPEG(t, "2026:10:10 11:12:13"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "a.DNG"), []byte("synthetic RAW"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newImportModelWithSettings(".dgs-state", source, targets[0]).(importModel)
	m.extraDestinations = []string{targets[1]}
	m.rebuildDirectoryControls()
	scan := m.scanAllTargetsCmd(0)().(scanDoneMsg)
	m.scan = scan.summary
	m.targetScans = scan.targets
	m.configureExtensions()
	m.stage = parameterStage
	m.width = 180
	m.height = 40
	m.controls.SetValue(parallelID, "2")
	return m
}
func runMultipleTUI(t *testing.T, m importModel) (importModel, int) {
	t.Helper()
	updated, cmd := m.startProcessing()
	m = updated.(importModel)
	if cmd == nil {
		t.Fatal("Processing refused", m.actionNotice)
	}
	copied := 0
	for !m.processing.complete {
		if cmd == nil {
			t.Fatal("Processing stopped without result")
		}
		msg := cmd()
		if event, ok := msg.(multipleTransferEventMsg); ok && event.event.Phase == importer.PhaseCopying {
			copied++
		}
		updated, cmd = m.Update(msg)
		m = updated.(importModel)
	}
	return m, copied
}
func organizeMultipleTUI(t *testing.T, m importModel) importModel {
	t.Helper()
	m.stage = postprocessStage
	updated, cmd := m.runPostProcessing()
	m = updated.(importModel)
	for m.postprocess.running {
		if cmd == nil {
			t.Fatal("organization stopped")
		}
		updated, cmd = m.Update(cmd())
		m = updated.(importModel)
		if m.postprocessPrompt {
			t.Fatal("unexpected existing-folder prompt")
		}
	}
	return m
}
func TestTUIMultipleDestinationControlsAndInventory(t *testing.T) {
	m := multipleImportModel(t)
	m.stage = setupStage
	primary := m.paths[destinationField]
	m.controls.SetFocusID(addTargetID)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(importModel)
	if !m.picking || !m.addingDestination {
		t.Fatal("Add did not open Explorer")
	}
	if !m.acceptPickedPath(t.TempDir()) {
		t.Fatal(m.actionNotice)
	}
	m.picking = false
	if len(m.destinationRoots()) != 3 || m.selectedTarget != 2 || m.paths[destinationField] != primary {
		t.Fatal("Add replaced existing target")
	}
	m.controls.SetFocusID(removeTargetID)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(importModel)
	if len(m.destinationRoots()) != 2 {
		t.Fatal("Remove failed")
	}
	m.controls.SetFocusID(destinationID)
	m.selectedTarget = 0
	m.rebuildDirectoryControls()
	if !strings.Contains(m.View(), "Destination 2") || strings.Contains(m.View(), "Target ") {
		t.Fatal("destinations hidden")
	}
	roots := m.destinationRoots()
	if err := os.WriteFile(filepath.Join(roots[1], "existing.JPG"), []byte("synthetic existing photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	scan := m.scanAllTargetsCmd(0)().(scanDoneMsg)
	m.targetScans = scan.targets
	m.scan = scan.summary
	m.stage = parameterStage
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = updated.(importModel)
	if m.selectedTarget != 1 || len(m.scan.destination) != 1 {
		t.Fatal("inventory did not switch")
	}
	for _, root := range roots {
		if _, err := os.Stat(filepath.Join(root, m.stateFilename)); !os.IsNotExist(err) {
			t.Fatal("scan wrote state")
		}
	}
}
func TestTUIMultipleCopyOrganizeAndIncrementalRestart(t *testing.T) {
	m := multipleImportModel(t)
	m, copied := runMultipleTUI(t, m)
	if copied == 0 || len(m.processing.verified) != 2 || len(m.targetResults) != 2 {
		t.Fatal("copies incomplete")
	}
	m = organizeMultipleTUI(t, m)
	if len(m.postprocess.result.Files) != 4 {
		t.Fatal("lost organization outcomes")
	}
	for _, root := range m.destinationRoots() {
		for _, name := range []string{"a.JPG", "a.DNG"} {
			if _, err := os.Stat(filepath.Join(root, "20261010", name)); err != nil {
				t.Fatal(err)
			}
		}
		state, err := importer.LoadState(filepath.Join(root, m.stateFilename))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range state.Files {
			if !strings.Contains(file.Destination, "20261010") {
				t.Fatal("state path not updated")
			}
		}
	}
	m.openResult()
	if !strings.Contains(m.View(), "Destination 2/2") {
		t.Fatal("result omitted target")
	}
	m.restartImport()
	if len(m.destinationRoots()) != 2 {
		t.Fatal("Again lost destinations")
	}
	scan := m.scanAllTargetsCmd(0)().(scanDoneMsg)
	m.scan = scan.summary
	m.targetScans = scan.targets
	m.configureExtensions()
	m.stage = parameterStage
	m, copied = runMultipleTUI(t, m)
	if copied != 0 || m.processing.existing != 2 || m.publishedBytes() != 0 {
		t.Fatal("organized copies imported again")
	}
	m.controls.SetChecked(deleteStateID, true)
	if !m.cleanupResultState() {
		t.Fatal(m.actionNotice)
	}
	for _, root := range m.destinationRoots() {
		if _, err := os.Stat(filepath.Join(root, m.stateFilename)); !os.IsNotExist(err) {
			t.Fatal("state retained")
		}
	}
}
func TestTUIMultipleMoveFailureKeepsSourceAndState(t *testing.T) {
	m := multipleImportModel(t)
	if err := os.WriteFile(filepath.Join(m.destinationRoots()[1], "a.JPG"), []byte("different photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.controls.SetValue(operationID, "Move")
	m, _ = runMultipleTUI(t, m)
	// The second target holds another photo under the same name: a conflict,
	// counted as in a single-target run, which still keeps state.
	if m.processing.conflicts != 1 || m.processing.failed != 0 || m.controls.Checked(deleteStateID) {
		t.Fatal("failed target looked complete", m.processing.conflicts, m.processing.failed)
	}
	if _, err := os.Stat(filepath.Join(m.paths[sourceField], "a.JPG")); err != nil {
		t.Fatal("source deleted", err)
	}
	if _, err := os.Stat(filepath.Join(m.paths[sourceField], "a.DNG")); !os.IsNotExist(err) {
		t.Fatal("fully copied RAW not moved")
	}
	m.openResult()
	if !strings.Contains(m.View(), "failed") {
		t.Fatal("missing-copy failure hidden")
	}
}
func TestTUIMultipleCleanupRefusesReplacedRoot(t *testing.T) {
	m := multipleImportModel(t)
	m, _ = runMultipleTUI(t, m)
	second := m.destinationRoots()[1]
	if err := os.Rename(second, second+"-detached"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(second, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(second, m.stateFilename)
	if err := os.WriteFile(path, []byte("synthetic unrelated state"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.controls.SetChecked(deleteStateID, true)
	if m.cleanupResultState() {
		t.Fatal("replacement root accepted")
	}
	for _, path := range []string{path, filepath.Join(m.destinationRoots()[0], m.stateFilename)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("state removed before root checks", err)
		}
	}
}
func TestTUIMultipleSuccessfulMoveCanOrganizeWithoutSource(t *testing.T) {
	m := multipleImportModel(t)
	m.controls.SetValue(operationID, "Move")
	m, _ = runMultipleTUI(t, m)
	if m.processing.failed != 0 || len(m.processing.verified) != 2 {
		t.Fatal("Move incomplete")
	}
	for _, name := range []string{"a.JPG", "a.DNG"} {
		if _, err := os.Stat(filepath.Join(m.paths[sourceField], name)); !os.IsNotExist(err) {
			t.Fatal("source retained")
		}
	}
	m = organizeMultipleTUI(t, m)
	for _, root := range m.destinationRoots() {
		for _, name := range []string{"a.JPG", "a.DNG"} {
			if _, err := os.Stat(filepath.Join(root, "20261010", name)); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestTUIMultipleRejectsAliasBeforeProcessingWrites(t *testing.T) {
	m := multipleImportModel(t)
	m.extraDestinations = []string{m.paths[destinationField]}
	updated, cmd := m.startProcessing()
	m = updated.(importModel)
	if cmd != nil || m.stage != parameterStage || !strings.Contains(m.actionNotice, "alias") {
		t.Fatal("alias accepted", m.actionNotice)
	}
	entries, err := os.ReadDir(m.paths[destinationField])
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid targets wrote files")
	}
}
func TestTUIMultipleVisibleDestinationRowsEditIndependently(t *testing.T) {
	m := multipleImportModel(t)
	m.stage = setupStage
	primary := m.paths[destinationField]
	m.controls.SetFocusID(destinationID)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(importModel)
	if m.controls.FocusedID() != destinationControlID(1) || m.selectedTarget != 1 {
		t.Fatal("second row unreachable")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(importModel)
	if !m.picking {
		t.Fatal("row did not open Explorer")
	}
	replacement := t.TempDir()
	if !m.acceptPickedPath(replacement) {
		t.Fatal(m.actionNotice)
	}
	m.picking = false
	if m.paths[destinationField] != primary || m.extraDestinations[0] != replacement {
		t.Fatal("first path replaced")
	}
	if m.controls.Value(parallelID) != "2" {
		t.Fatal("parameters lost")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(importModel)
	if len(m.destinationRoots()) != 1 || m.paths[destinationField] != primary || strings.Contains(m.setupView(), "Destination 2") {
		t.Fatal("stale row after removal")
	}
}
func TestTUIMultipleDirectoryButtonsShareRowAndMouseGeometry(t *testing.T) {
	m := multipleImportModel(t)
	m.stage = setupStage
	found := false
	for _, line := range strings.Split(ansi.Strip(m.setupView()), "\n") {
		if strings.Contains(line, "Add destination") && strings.Contains(line, "Remove destination") {
			found = true
		}
	}
	if !found {
		t.Fatal("buttons not on same row")
	}
	width := 100
	pathHeight := lipgloss.Height(m.controls.View(m.directoryPathIDs()))
	top := ((m.height - 4) - (11 + pathHeight)) / 2
	left := (m.width - width) / 2
	_, addWidth, removeWidth := m.directoryButtons(width - 4)
	addStart := width - 4 - removeWidth - 2 - addWidth
	updated, _ := m.clickSetup(left+2+addStart+1, top+7+pathHeight+1)
	m = updated.(importModel)
	if !m.picking || !m.addingDestination {
		t.Fatal("Add click missed")
	}
	if !m.acceptPickedPath(t.TempDir()) {
		t.Fatal(m.actionNotice)
	}
	m.picking = false
	pathHeight = lipgloss.Height(m.controls.View(m.directoryPathIDs()))
	top = ((m.height - 4) - (11 + pathHeight)) / 2
	_, _, removeWidth = m.directoryButtons(width - 4)
	updated, _ = m.clickSetup(left+2+width-4-removeWidth+1, top+7+pathHeight+1)
	m = updated.(importModel)
	if len(m.destinationRoots()) != 2 {
		t.Fatal("Remove click missed")
	}
}
func TestTUIMultipleRejectsDuplicateDestinationAtSelection(t *testing.T) {
	for _, mode := range []string{"add", "edit", "alias"} {
		t.Run(mode, func(t *testing.T) {
			m := multipleImportModel(t)
			m.pickerFor = destinationField
			m.selectedTarget = 1
			m.addingDestination = mode != "edit"
			candidate := m.paths[destinationField]
			if mode == "alias" {
				alias := filepath.Join(t.TempDir(), "library-link")
				if err := os.Symlink(candidate, alias); err != nil {
					t.Fatal(err)
				}
				candidate = alias
			}
			before := strings.Join(m.destinationRoots(), "|")
			if m.acceptPickedPath(candidate) {
				t.Fatal("duplicate accepted")
			}
			if strings.Join(m.destinationRoots(), "|") != before {
				t.Fatal("directories changed on rejection")
			}
			if !strings.Contains(m.actionNotice, "Destination 1") {
				t.Fatal("missing conflicting-row notice")
			}
			if !m.acceptPickedPath(t.TempDir()) {
				t.Fatal("valid selection refused", m.actionNotice)
			}
		})
	}
}

func TestMultipleProgressPreservesInterleavedTargets(t *testing.T) {
	m := multipleImportModel(t)
	m.processing = processingState{files: m.filteredSource(), finished: make(map[int]importer.Phase)}
	m.targetProcessing = make([]processingState, 2)
	for i := range m.targetProcessing {
		m.targetProcessing[i] = processingState{files: m.processing.files, finished: make(map[int]importer.Phase), workers: []processingWorker{{number: i + 1, jobIndex: -1}}}
	}
	send := func(target int, phase importer.Phase) {
		m.applyMultipleEvent(multipleTransferEventMsg{target: target, event: importer.Event{Worker: 1, Index: 0, Phase: phase, Bytes: 5, Total: 10}})
	}
	send(0, importer.PhaseCopying)
	send(1, importer.PhaseVerifying)
	if m.targetProcessing[0].workers[0].phase != workerCopying || m.targetProcessing[1].workers[0].phase != workerVerifying {
		t.Fatal("interleaved events lost target progress")
	}
	if m.activeWorkerCount() != 1 || m.processingPercent() >= 100 {
		t.Fatal("parallel targets inflated overall progress")
	}
	send(0, importer.PhaseComplete)
	if len(m.processing.verified) != 0 {
		t.Fatal("verified before all destinations completed")
	}
	send(1, importer.PhaseComplete)
	send(0, importer.PhaseComplete)
	if len(m.processing.verified) != 1 || len(m.targetProcessing[0].verified) != 1 {
		t.Fatal("completion lost or counted more than once")
	}
}
func TestPhotoConfigInitializesAllDestinations(t *testing.T) {
	configured := config.Config{Photo: config.Photo{Import: config.PhotoImport{Source: "/card", Dest: []string{"/library-a", "/library-b"}}}}
	m := New().Commands[0].NewWithConfig(configured).(importModel)
	roots := m.destinationRoots()
	if len(roots) != 2 || roots[0] != "/library-a" || roots[1] != "/library-b" {
		t.Fatalf("roots = %q", roots)
	}
	if m.controls.Value(destinationControlID(1)) != "/library-b" {
		t.Fatal("second configured destination missing from form")
	}
	m.extraDestinations[0] = "/changed"
	if configured.Photo.Import.Dest[1] != "/library-b" {
		t.Fatal("session mutated configuration")
	}
	empty := New().Commands[0].NewWithConfig(config.Config{}).(importModel)
	if len(empty.destinationRoots()) != 1 || empty.paths[destinationField] == "" {
		t.Fatal("missing destinations did not retain default")
	}
}

// Skipping one root's existing date folders declines that root only: the
// next root is still organized, and outcomes already gathered are kept.
func TestTUIMultipleSkipExistingFoldersSkipsOnlyThatTarget(t *testing.T) {
	m := multipleImportModel(t)
	m, _ = runMultipleTUI(t, m)
	first, second := m.destinationRoots()[0], m.destinationRoots()[1]
	if err := os.Mkdir(filepath.Join(first, "20261010"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.stage = postprocessStage
	updated, cmd := m.runPostProcessing()
	m = updated.(importModel)
	prompts := 0
	for m.postprocess.running || m.postprocessPrompt {
		if m.postprocessPrompt {
			prompts++
			updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = updated.(importModel)
			continue
		}
		if cmd == nil {
			t.Fatal("organization stopped")
		}
		updated, cmd = m.Update(cmd())
		m = updated.(importModel)
	}
	if prompts != 1 || m.postprocess.skipped || m.stage != postprocessStage {
		t.Fatal("skip ended organization for every target", prompts, m.postprocess.skipped, m.stage)
	}
	for _, name := range []string{"a.JPG", "a.DNG"} {
		if _, err := os.Stat(filepath.Join(first, name)); err != nil {
			t.Fatal("skipped target was organized", err)
		}
		if _, err := os.Stat(filepath.Join(second, "20261010", name)); err != nil {
			t.Fatal("later target not organized", err)
		}
	}
	if len(m.postprocess.result.Files) != 2 {
		t.Fatal("organized target outcomes lost", len(m.postprocess.result.Files))
	}
}

// A Move into several targets counts each source's overall outcome, which the
// header shows while sources are rechecked before deletion.
func TestTUIMultipleMoveCountsDecidedSources(t *testing.T) {
	m := multipleImportModel(t)
	m.controls.SetValue(operationID, "Move")
	m, _ = runMultipleTUI(t, m)
	if !m.processing.moving || m.processing.decided != len(m.processing.files) {
		t.Fatal("source decisions not counted", m.processing.decided)
	}
	m.processing.complete = false
	if !strings.Contains(m.processingStateLine("RUNNING"), "checking copies before deleting sources 2/2") {
		t.Fatal(m.processingStateLine("RUNNING"))
	}
}

// Returning to one destination after a multi-target batch drops the
// per-target view, so the single-target workers are the ones drawn.
func TestTUISingleRunAfterMultipleDropsTargetView(t *testing.T) {
	m := multipleImportModel(t)
	m, _ = runMultipleTUI(t, m)
	m.selectedTarget = 1
	m.removeSelectedTarget()
	m.stage = parameterStage
	m.processing = processingState{}
	updated, _ := m.startProcessing()
	m = updated.(importModel)
	if m.targetProcessing != nil || m.targetResults != nil || m.progressWorkerCount() != len(m.processing.workers) {
		t.Fatal("stale multi-target view kept")
	}
	if m.transferCancel != nil {
		m.transferCancel()
	}
}

// Cleanup keeps every state while one records an earlier source-kept Move.
func TestTUIMultipleCleanupKeepsEarlierSourceKeptState(t *testing.T) {
	m := multipleImportModel(t)
	m, _ = runMultipleTUI(t, m)
	path := filepath.Join(m.destinationRoots()[1], m.stateFilename)
	state, err := importer.LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	state.Files = append(state.Files, importer.FileResult{Index: -1, Source: "/card/earlier.JPG", Destination: filepath.Join(m.destinationRoots()[1], "earlier.JPG"), Phase: importer.PhaseSourceKept, SourceHash: "x", DestinationHash: "x"})
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	m.controls.SetChecked(deleteStateID, true)
	if m.cleanupResultState() {
		t.Fatal("cleanup removed state with an unfinished earlier Move")
	}
	for _, root := range m.destinationRoots() {
		if _, err := os.Stat(filepath.Join(root, m.stateFilename)); err != nil {
			t.Fatal("state removed", err)
		}
	}
}
