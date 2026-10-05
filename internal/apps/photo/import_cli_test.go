package photo

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/config"
)

func importCLIFlags(targets []string) map[string]string {
	for _, target := range targets {
		_ = os.MkdirAll(target, 0o755)
	}
	encoded, _ := json.Marshal(targets)
	return map[string]string{"dest": string(encoded), "ext": "null", "workers": "2", "duplicates": "skip", "incremental": "true"}
}

func TestCLIImportOrganizesAndFindsEarlierCopiesInEveryTarget(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	targets := []string{filepath.Join(root, "library-a"), filepath.Join(root, "library-b")}
	writeTree(t, source, "DCIM/IMG_0001.JPG", "DCIM/IMG_0001.DNG", "DCIM/._IMG_0001.JPG")
	jpg := filepath.Join(source, "DCIM", "IMG_0001.JPG")
	if err := os.WriteFile(jpg, testJPEG(t, "2026:10:10 11:12:13"), 0o644); err != nil {
		t.Fatal(err)
	}
	flags := importCLIFlags(targets)
	flags["organize"] = "true"
	var output bytes.Buffer
	run := func() {
		t.Helper()
		output.Reset()
		if err := importDirectoriesContext(context.Background(), &output, []string{source}, flags, config.Default()); err != nil {
			t.Fatal(err, output.String())
		}
	}
	run()
	for _, target := range targets {
		for _, name := range []string{"IMG_0001.JPG", "IMG_0001.DNG"} {
			if _, err := os.Stat(filepath.Join(target, "20261010", name)); err != nil {
				t.Fatal("shot not organized", err)
			}
		}
	}
	// RAW arrives after the JPG was organized; the existing JPG supplies date
	// context, and the RAW itself need not have a parsable metadata container.
	for _, target := range targets {
		if err := os.Remove(filepath.Join(target, "20261010", "IMG_0001.DNG")); err != nil {
			t.Fatal(err)
		}
	}
	run()
	for _, target := range targets {
		if _, err := os.Stat(filepath.Join(target, "20261010", "IMG_0001.DNG")); err != nil {
			t.Fatal("late RAW lost its JPG date context", err)
		}
	}
	run()
	if strings.Count(output.String(), "already-present") != 4 || !strings.Contains(output.String(), "Imported into every destination: 2/2 files") {
		t.Fatal(output.String())
	}
}

func TestCLIImportExtensionSelectionAndFailedMove(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	targets := []string{filepath.Join(root, "library-a"), filepath.Join(root, "library-b")}
	writeTree(t, source, "DCIM/a.JPG", "DCIM/b.RAW")
	writeTree(t, targets[1], "DCIM/a.JPG")
	if err := os.WriteFile(filepath.Join(targets[1], "DCIM", "a.JPG"), []byte("different photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	flags := importCLIFlags(targets)
	flags["ext"], flags["move"] = `[".jpg"]`, "true"
	var output bytes.Buffer
	if err := importDirectoriesContext(context.Background(), &output, []string{source}, flags, config.Default()); err == nil {
		t.Fatal("incomplete Move reported success")
	}
	if !strings.Contains(output.String(), "failed") || !strings.Contains(output.String(), "not imported") {
		t.Fatal(output.String())
	}
	for _, name := range []string{"a.JPG", "b.RAW"} {
		if _, err := os.Stat(filepath.Join(source, "DCIM", name)); err != nil {
			t.Fatal("source lost", err)
		}
	}
	if _, err := os.Stat(filepath.Join(targets[0], "DCIM", "b.RAW")); !os.IsNotExist(err) {
		t.Fatal("excluded extension copied", err)
	}
}

func TestCLIImportRejectsBadOptionsBeforeWrites(t *testing.T) {
	for _, option := range []string{"workers", "duplicates", "dest", "ext"} {
		t.Run(option, func(t *testing.T) {
			root := t.TempDir()
			source, target := filepath.Join(root, "card"), filepath.Join(root, "library")
			writeTree(t, source, "a.JPG")
			flags := importCLIFlags([]string{target})
			flags[option] = map[string]string{"workers": "9", "duplicates": "replace", "dest": "[]", "ext": `[""]`}[option]
			var output bytes.Buffer
			if err := importDirectoriesContext(context.Background(), &output, []string{source}, flags, config.Default()); err == nil {
				t.Fatal("invalid options accepted")
			}
			if entries, _ := os.ReadDir(target); len(entries) != 0 {
				t.Fatal("invalid command wrote destination", entries)
			}
		})
	}
}

func TestCLIImportRejectsEmptySource(t *testing.T) {
	target := filepath.Join(t.TempDir(), "library")
	var output bytes.Buffer
	if err := importDirectoriesContext(context.Background(), &output, []string{""}, importCLIFlags([]string{target}), config.Default()); err == nil {
		t.Fatal("empty source silently selected the working directory")
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatal("empty source wrote destination", entries)
	}
}

func TestCLIImportDryRunDoesNotWriteOrDelete(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	targets := []string{filepath.Join(root, "library-a"), filepath.Join(root, "library-b")}
	writeTree(t, source, "DCIM/a.JPG", "DCIM/b.DNG")
	flags := importCLIFlags(targets)
	flags["dry-run"], flags["move"], flags["organize"], flags["delete-state"] = "true", "true", "true", "true"
	flags["ext"] = `["JPG"]`
	output, err := runImportCLI(t, source, flags)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "DCIM/a.JPG") || strings.Contains(output, "DCIM/b.DNG") || !strings.Contains(output, "Selected: 1 files") {
		t.Fatal(output)
	}
	for _, target := range targets {
		entries, err := os.ReadDir(target)
		if err != nil || len(entries) != 0 {
			t.Fatalf("dry run wrote destination: %v %v", entries, err)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "DCIM", "a.JPG")); err != nil {
		t.Fatal("dry run deleted source", err)
	}
}

func TestCLIImportStateCleanupRequiresFullSuccess(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "conflict"}[conflict], func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "card")
			targets := []string{filepath.Join(root, "library-a"), filepath.Join(root, "library-b")}
			writeTree(t, source, "a.JPG")
			flags := importCLIFlags(targets)
			flags["delete-state"] = "true"
			if conflict {
				if err := os.WriteFile(filepath.Join(targets[1], "a.JPG"), []byte("different content"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := runImportCLI(t, source, flags)
			if (err != nil) != conflict {
				t.Fatalf("unexpected result: %v", err)
			}
			for _, target := range targets {
				_, err := os.Stat(filepath.Join(target, config.Default().PhotoImportStateFile()))
				if conflict && err != nil {
					t.Fatal("unfinished state removed", err)
				}
				if !conflict && !os.IsNotExist(err) {
					t.Fatal("completed state retained", err)
				}
				if _, err := os.Stat(filepath.Join(target, "a.JPG")); err != nil {
					t.Fatal("photo lost", err)
				}
			}
		})
	}
}

func TestCLIImportCleanupKeepsEarlierUnfinishedMove(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "card"), filepath.Join(root, "library")
	writeTree(t, source, "a.JPG")
	flags := importCLIFlags([]string{target})
	flags["delete-state"] = "true"
	state := importer.State{Version: importer.StateVersion, HashAlgorithm: "SHA-256", Operation: importer.Move,
		Files: []importer.FileResult{{Index: -1, Source: filepath.Join(root, "earlier-card", "b.JPG"), Destination: filepath.Join(target, "b.JPG"), DestinationHash: "00", Phase: importer.PhaseSourceKept}}}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(target, config.Default().PhotoImportStateFile())
	if err := os.WriteFile(statePath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := runImportCLI(t, source, flags)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "State retained") {
		t.Fatal(output)
	}
	saved, err := importer.LoadState(statePath)
	if err != nil || importer.EarlierSourceKept(saved) != 1 {
		t.Fatalf("earlier Move state lost: %v", err)
	}
}
