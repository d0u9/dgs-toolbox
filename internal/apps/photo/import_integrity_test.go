package photo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/apps/photo/postprocess"
	"dgs-toolbox/internal/config"
)

func TestCLIImportNeverOverwritesPhotosWithState(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	writeTree(t, source, "new.JPG")
	targets := []string{filepath.Join(root, "library-a"), filepath.Join(root, "library-b")}
	flags := importCLIFlags(targets)
	for _, target := range targets {
		if err := os.WriteFile(filepath.Join(target, "old.JPG"), []byte("old synthetic photograph"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	settings := config.Default()
	settings.Photo.Import.StateFile = "old.JPG"
	var out bytes.Buffer
	if err := importDirectoriesContext(context.Background(), &out, []string{source}, flags, settings); err == nil {
		t.Fatal("state collision silently succeeded")
	}
	for _, target := range targets {
		if data, err := os.ReadFile(filepath.Join(target, "old.JPG")); err != nil || string(data) != "old synthetic photograph" {
			t.Fatal("existing photo overwritten", err)
		}
	}
}

func TestPostprocessFailureRetainsActualTransferLocationsAndState(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "multiple"}[multiple], func(t *testing.T) {
			m := newImportModel().(importModel)
			m.stage = postprocessStage
			m.controls.SetChecked(deleteStateID, true)
			m.processing.results = []importer.FileResult{{Source: "card/a.JPG", Destination: "library/a.JPG", Phase: importer.PhaseComplete}}
			if multiple {
				m.extraDestinations = []string{"backup"}
				m.targetResults = []importer.Result{{Files: append([]importer.FileResult(nil), m.processing.results...)}}
			}
			updated, _ := m.Update(postprocessDoneMsg{result: postprocess.Result{Files: []postprocess.Outcome{{Original: "library/a.JPG", Final: "library/20261010/a.JPG", Status: postprocess.Failed, Error: "directory sync failed"}}}})
			m = updated.(importModel)
			if m.controls.Checked(deleteStateID) {
				t.Fatal("failed organization selected state deletion")
			}
			if m.processing.results[0].Destination != "library/20261010/a.JPG" {
				t.Fatal("lost actual transfer location")
			}
			if multiple && m.targetResults[0].Files[0].Destination != "library/20261010/a.JPG" {
				t.Fatal("lost target location")
			}
		})
	}
}

func TestSingleTUIMoveRejectsDirectoryLinkAfterScan(t *testing.T) {
	root := t.TempDir()
	source, dest, outside := filepath.Join(root, "card"), filepath.Join(root, "library"), filepath.Join(root, "outside")
	writeTree(t, source, "DCIM/a.JPG")
	writeTree(t, outside, "a.JPG")
	m := newImportModel().(importModel)
	m.paths = [pathFieldCount]string{source, dest}
	m.stage = parameterStage
	m.scan = scanDirectories(m.paths)
	m.configureExtensions()
	m.controls.SetChecked(incrementalID, false)
	m.controls.SetValue(operationID, "Move")
	if err := os.Rename(filepath.Join(source, "DCIM"), filepath.Join(source, "DCIM-original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "DCIM")); err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.startProcessing()
	m = updated.(importModel)
	for !m.processingComplete() {
		if cmd == nil {
			t.Fatal("processing stopped", m.actionNotice)
		}
		updated, cmd = m.Update(cmd())
		m = updated.(importModel)
	}
	if m.processing.failed != 1 {
		t.Fatal("source link accepted", m.processing.failed)
	}
	if _, err := os.Stat(filepath.Join(outside, "a.JPG")); err != nil {
		t.Fatal("external photo deleted", err)
	}
}
