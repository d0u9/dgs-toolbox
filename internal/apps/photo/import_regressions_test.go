package photo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/apps/photo/postprocess"

	tea "github.com/charmbracelet/bubbletea"
)

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, name := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func scannedPaths(files []scannedFile) string {
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.path
	}
	return strings.Join(paths, ",")
}

// AppleDouble companions carry a photo's extension and trashed photos sit in
// .Trashes; neither is a camera file to import.
func TestSourceScanLeavesOutHiddenAndTrashEntries(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "card"), filepath.Join(root, "library")
	writeTree(t, source, "DCIM/IMG_0001.JPG", "DCIM/._IMG_0001.JPG", ".Trashes/501/IMG_0002.JPG", "#recycle/IMG_0003.JPG", ".hidden/IMG_0004.JPG")
	summary := scanDirectories([pathFieldCount]string{source, destination})
	if got := scannedPaths(summary.source); got != "DCIM/IMG_0001.JPG" {
		t.Fatalf("source = %s", got)
	}
	if len(summary.sourceErrors) != 0 || len(summary.sourceSkipped) != 4 {
		t.Fatalf("skipped = %v errors = %v", summary.sourceSkipped, summary.sourceErrors)
	}
}

func TestDestinationScanIgnoresUnreadableSystemFolders(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "card"), filepath.Join(root, "library")
	writeTree(t, source, "DCIM/IMG_0001.JPG")
	writeTree(t, destination, "20261003/IMG_0001.JPG", ".Trashes/x", "#recycle/IMG_0009.JPG", ".hidden-library/IMG_0005.JPG")
	trash := filepath.Join(destination, ".Trashes")
	if err := os.Chmod(trash, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(trash, 0o755)
	summary := scanDirectories([pathFieldCount]string{source, destination})
	if len(summary.destinationErrors) != 0 {
		t.Fatalf("errors = %v", summary.destinationErrors)
	}
	// Other hidden library folders still count: only named system folders
	// are left out of the destination.
	if got := scannedPaths(summary.destination); got != ".hidden-library/IMG_0005.JPG,20261003/IMG_0001.JPG" {
		t.Fatalf("destination = %s", got)
	}
}

func TestOverlappingRootsAreRefusedWithoutIncremental(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "DCIM/IMG_0001.JPG")
	model := newImportModel().(importModel)
	model.controls.SetChecked(incrementalID, false)
	model.paths = [pathFieldCount]string{root, filepath.Join(root, "backup")}
	model.stage = parameterStage
	model.scan = scanDirectories(model.paths)
	updated, cmd := model.startProcessing()
	model = updated.(importModel)
	if cmd != nil || model.stage != parameterStage || !strings.Contains(model.actionNotice, "overlap") {
		t.Fatalf("stage=%v notice=%q", model.stage, model.actionNotice)
	}
}

func TestReturningToParametersRescans(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = processingStage
	model.processing.complete = true
	model.beginLeaveConfirmation(false)
	updated, cmd := model.updateKey(keyMsg("tab"))
	model = updated.(importModel)
	updated, cmd = model.updateKey(keyMsg("enter"))
	model = updated.(importModel)
	if model.stage != parameterStage || !model.refreshing || cmd == nil {
		t.Fatalf("stage=%v refreshing=%v cmd=%v", model.stage, model.refreshing, cmd != nil)
	}
	if _, ok := cmd().(scanDoneMsg); !ok {
		t.Fatal("return did not re-read the folders")
	}
}

func TestUnfinishedBatchKeepsStateByDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		file importer.FileResult
		keep bool
	}{
		{"complete", importer.FileResult{Phase: importer.PhaseComplete}, false},
		{"failed", importer.FileResult{Phase: importer.PhaseFailed}, true},
		{"source kept", importer.FileResult{Phase: importer.PhaseSourceKept}, true},
		{"not imported", importer.FileResult{Phase: importer.PhaseSkipped, Warning: importer.ConflictNotImported}, true},
		{"filename skip", importer.FileResult{Phase: importer.PhaseSkipped}, false},
	} {
		model := newImportModel().(importModel)
		model.processing.files = []scannedFile{{path: "IMG_0001.JPG"}}
		model.applyTransferResult(importer.Result{Files: []importer.FileResult{tc.file}})
		if model.controls.Checked(deleteStateID) == tc.keep {
			t.Errorf("%s: delete state = %v", tc.name, model.controls.Checked(deleteStateID))
		}
	}
}

func TestNotImportedConflictIsShownApart(t *testing.T) {
	model := newImportModel().(importModel)
	model.width, model.height = 160, 30
	model.processing.files = []scannedFile{{path: "IMG_0001.JPG"}}
	model.applyTransferResult(importer.Result{Files: []importer.FileResult{{Source: "/card/IMG_0001.JPG", Destination: "/library/IMG_0001.JPG", Phase: importer.PhaseSkipped, Warning: importer.ConflictNotImported}}})
	model.postprocess = postprocessState{skipped: true}
	model.openResult()
	view := model.View()
	if !strings.Contains(view, "Not imported (same name, different content) 1") || !strings.Contains(view, "different content: not imported") {
		t.Fatal(view)
	}
}

func TestProcessingShowsInventoryBeforeWorkersMove(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = processingStage
	model.width, model.height = 160, 30
	model.processing = processingState{files: []scannedFile{{path: "IMG_0001.JPG"}}, finished: map[int]importer.Phase{}, workers: []processingWorker{{number: 1, jobIndex: -1}}}
	model.applyTransferEvent(importer.Event{Index: -1, Phase: importer.PhaseInventory, Bytes: 1200})
	if view := model.View(); !strings.Contains(view, "destination inventory 1200 files") {
		t.Fatal(view)
	}
	if model.processing.workers[0].jobIndex != -1 {
		t.Fatal("inventory event occupied a worker")
	}
}

func TestOrganizeTreeSkipsRecycleBins(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "IMG_0001.JPG", "#recycle/IMG_0002.JPG", "@eaDir/IMG_0001.JPG/SYNOPHOTO_THUMB_M.jpg")
	files, err := postprocess.TreeFiles(root)
	if err != nil || len(files) != 1 || filepath.Base(files[0].Path) != "IMG_0001.JPG" {
		t.Fatalf("files = %v, %v", files, err)
	}
}

func TestSummaryWarnsWhenDestinationWillBeCreated(t *testing.T) {
	root := t.TempDir()
	writeTree(t, filepath.Join(root, "card"), "DCIM/IMG_0001.JPG")
	model := newImportModel().(importModel)
	model.paths = [pathFieldCount]string{filepath.Join(root, "card"), filepath.Join(root, "nas", "photos")}
	model.scan = scanDirectories(model.paths)
	model.configureExtensions()
	if summary := model.importSummaryView(200); !strings.Contains(summary, "will be created") {
		t.Fatal(summary)
	}
	if err := os.MkdirAll(model.paths[destinationField], 0o755); err != nil {
		t.Fatal(err)
	}
	model.scan = scanDirectories(model.paths)
	if summary := model.importSummaryView(200); strings.Contains(summary, "will be created") {
		t.Fatal(summary)
	}
}

func TestEarlierSourceKeptMovesKeepStateByDefault(t *testing.T) {
	destination := t.TempDir()
	model := newImportModel().(importModel)
	model.paths[destinationField] = destination
	// A card imported before this one could not have its source deleted; the
	// state file still carries that record for a later run to finish.
	state := `{"version": 2, "hash_algorithm": "SHA-256", "files": [
		{"index": 0, "source": "/card-b/IMG_0002.JPG", "destination": "` + filepath.ToSlash(filepath.Join(destination, "IMG_0002.JPG")) + `", "phase": "complete"},
		{"index": -1, "source": "/card-a/IMG_0001.JPG", "destination": "` + filepath.ToSlash(filepath.Join(destination, "IMG_0001.JPG")) + `", "phase": "source-kept", "destination_hash": "00"}]}`
	if err := os.WriteFile(filepath.Join(destination, model.stateFilename), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	model.processing.files = []scannedFile{{path: "IMG_0002.JPG"}}
	model.applyTransferResult(importer.Result{Files: []importer.FileResult{{Phase: importer.PhaseComplete}}})
	if model.controls.Checked(deleteStateID) {
		t.Fatal("deleting state by default would forget an earlier unfinished Move")
	}
	if !strings.Contains(model.publishedLine(), "Earlier batches source kept 1") {
		t.Fatal(model.publishedLine())
	}
}
