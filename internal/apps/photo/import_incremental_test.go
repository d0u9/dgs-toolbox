package photo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/apps/photo/postprocess"
	"dgs-toolbox/internal/verifiedcopy"

	"github.com/charmbracelet/lipgloss"
)

func TestIncrementalParameterControlsNeverOverflowShortWorkspace(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = parameterStage
	model.width, model.height = 160, 18
	view := model.View()
	if !strings.Contains(view, "More height required") || lipgloss.Width(view) != 160 || lipgloss.Height(view) != 18 {
		t.Fatal(view)
	}
}

func TestIncrementalSummaryDoesNotGuessCopyCount(t *testing.T) {
	model := newImportModel().(importModel)
	if !model.controls.Checked(incrementalID) {
		t.Fatal("incremental is not enabled initially")
	}
	model.scan = scanSummary{source: []scannedFile{{path: "old.JPG", size: 4}, {path: "new.JPG", size: 9}}, destination: []scannedFile{{path: "20261003/old.JPG", size: 4}}}
	plan := model.buildPlan()
	if !plan.incremental || plan.skipped != 0 || plan.checks != 1 {
		t.Fatalf("plan = %#v", plan)
	}
	summary := model.importSummaryView(80)
	if strings.Contains(summary, "Will copy") || !strings.Contains(summary, "To check       1 file") {
		t.Fatal(summary)
	}
	model.controls.SetFocusID(incrementalID)
	model.controls.HandleInteraction(" ")
	if summary := model.importSummaryView(80); !strings.Contains(summary, "Will copy") {
		t.Fatal(summary)
	}
}

func TestIncrementalBlocksIncompleteDestinationAndOverlappingRoots(t *testing.T) {
	model := newImportModel().(importModel)
	model.stage = parameterStage
	model.scan.destinationErrors = []string{"unreadable folder"}
	updated, cmd := model.startProcessing()
	model = updated.(importModel)
	if cmd != nil || model.stage != parameterStage || !strings.Contains(model.actionNotice, "Destination scan incomplete") {
		t.Fatal(model.actionNotice)
	}
	model.scan.destinationErrors = nil
	root := t.TempDir()
	model.paths = [pathFieldCount]string{root, filepath.Join(root, "out")}
	updated, cmd = model.startProcessing()
	model = updated.(importModel)
	if cmd != nil || !strings.Contains(model.actionNotice, "overlap") {
		t.Fatal(model.actionNotice)
	}
}

func TestIncrementalResultSeparatesExistingFromPublishedBytes(t *testing.T) {
	model := newImportModel().(importModel)
	model.processing.files = []scannedFile{{path: "old.JPG"}, {path: "new.JPG"}, {path: "conflict.JPG"}}
	model.applyTransferResult(importer.Result{Files: []importer.FileResult{
		{Index: 0, Source: "card/old.JPG", Destination: "library/20261003/old.JPG", Size: 10, Phase: importer.PhaseAlreadyPresent, Existing: true},
		{Index: 1, Source: "card/new.JPG", Destination: "library/new.JPG", Size: 20, Phase: importer.PhaseComplete},
		{Index: 2, Source: "card/conflict.JPG", Destination: "library/conflict.JPG", Size: 30, Phase: importer.PhaseSkipped},
	}})
	if model.publishedBytes() != 20 || model.processing.existing != 1 || len(model.processing.verified) != 2 || model.processing.skipped != 1 {
		t.Fatalf("counts = %#v", model.processing)
	}
	model.openResult()
	model.width, model.height = 160, 30
	if view := model.View(); !strings.Contains(view, "already present") || !strings.Contains(view, "conflict skipped") || !strings.Contains(view, "Published 20 B") {
		t.Fatal(view)
	}
}

func TestIncrementalAllExistingDoesNotOfferRenameTargets(t *testing.T) {
	model := newImportModel().(importModel)
	model.processing.results = []importer.FileResult{{Source: "card/old.JPG", Destination: "library/20261003/old.JPG", Phase: importer.PhaseAlreadyPresent, Existing: true}}
	plan := awaitOrganizationPlan(t, model)
	if len(plan.Files) != 0 {
		t.Fatal("old file entered rename list")
	}
}

func TestIncrementalTUIProcessesOldAndNewFilesEndToEnd(t *testing.T) {
	root := t.TempDir()
	source, dest := filepath.Join(root, "card"), filepath.Join(root, "library")
	for _, folder := range []string{source, filepath.Join(dest, "20261003")} {
		if err := os.MkdirAll(folder, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{filepath.Join(source, "old.JPG"): "old photo", filepath.Join(source, "new.JPG"): "new camera photograph", filepath.Join(dest, "20261003", "old.JPG"): "old photo"} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	model := newImportModel().(importModel)
	model.paths = [pathFieldCount]string{source, dest}
	model.stage = parameterStage
	model.scan = scanDirectories(model.paths)
	model.configureExtensions()
	updated, cmd := model.startProcessing()
	model = updated.(importModel)
	for !model.processingComplete() {
		if cmd == nil {
			t.Fatal("processing lost its update command")
		}
		updated, cmd = model.Update(cmd())
		model = updated.(importModel)
	}
	if model.processing.failed != 0 || model.processing.existing != 1 || model.publishedBytes() != 21 {
		t.Fatalf("counts = %#v", model.processing)
	}
	if _, err := os.Stat(filepath.Join(dest, "old.JPG")); !os.IsNotExist(err) {
		t.Fatal("organized old file recopied")
	}
}

func TestDestinationScanIncludesSelectedRootLinkAndExcludesArtifacts(t *testing.T) {
	root := t.TempDir()
	source, dest := filepath.Join(root, "card"), filepath.Join(root, "library")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dest, "20261003"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"20261003/old.JPG", ".custom-state", "..custom-state.1.tmp", ".old.JPG.1.dgs-part"} {
		if err := os.WriteFile(filepath.Join(dest, name), []byte("photo"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "chosen-library")
	if err := os.Symlink(dest, alias); err != nil {
		t.Fatal(err)
	}
	summary := scanDirectories([pathFieldCount]string{source, alias}, ".custom-state")
	if len(summary.destinationErrors) != 0 || len(summary.destination) != 1 || summary.destination[0].path != "20261003/old.JPG" {
		t.Fatalf("scan = %#v", summary)
	}
}

func TestExistingFileLeftUnorganizedByAnEarlierRunIsOrganized(t *testing.T) {
	model := newImportModel().(importModel)
	model.paths = [pathFieldCount]string{"/card", "/library"}
	model.processing.results = []importer.FileResult{
		// An interrupted run copied this one; it still mirrors its Source folder.
		{Source: "/card/DCIM/IMG_0001.JPG", Destination: "/library/DCIM/IMG_0001.JPG", Phase: importer.PhaseComplete, Existing: true},
		// Keep both renamed this one, in the same folder.
		{Source: "/card/DCIM/IMG_0002.JPG", Destination: "/library/DCIM/IMG_0002-2.JPG", Phase: importer.PhaseAlreadyPresent, Existing: true},
		// Already in a date folder: context only.
		{Source: "/card/DCIM/IMG_0003.JPG", Destination: "/library/20261003/IMG_0003.JPG", Phase: importer.PhaseAlreadyPresent, Existing: true},
	}
	var organize []string
	for _, result := range model.processing.results {
		if unorganized("/card", "/library", result) {
			organize = append(organize, filepath.Base(result.Destination))
		}
	}
	if strings.Join(organize, ",") != "IMG_0001.JPG,IMG_0002-2.JPG" {
		t.Fatal(organize)
	}
}

func TestUnorganizedExistingFileKeepsItsDigestForPostProcessing(t *testing.T) {
	for _, phase := range []importer.Phase{importer.PhaseAlreadyPresent, importer.PhaseComplete} {
		t.Run(string(phase), func(t *testing.T) {
			root := t.TempDir()
			sourceRoot, destinationRoot := filepath.Join(root, "card"), filepath.Join(root, "library")
			path := filepath.Join(destinationRoot, "DCIM", "IMG_0001.JPG")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("old photo"), 0o644); err != nil {
				t.Fatal(err)
			}
			checked, err := verifiedcopy.Check(context.Background(), destinationRoot, path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			model := newImportModel().(importModel)
			model.paths = [pathFieldCount]string{sourceRoot, destinationRoot}
			model.processing.results = []importer.FileResult{{
				Source: filepath.Join(sourceRoot, "DCIM", "IMG_0001.JPG"), Destination: path,
				Phase: phase, Existing: true, DestinationHash: checked.Digest, Size: checked.Size,
			}}
			if err := os.WriteFile(path, []byte("new photo"), 0o644); err != nil {
				t.Fatal(err)
			}
			plan := awaitOrganizationPlan(t, model)
			if len(plan.Files) != 1 || plan.Files[0].Status != postprocess.Failed || !strings.Contains(plan.Files[0].Error, "verified content") {
				t.Fatalf("verification evidence lost: %#v", plan.Files)
			}
		})
	}
}
