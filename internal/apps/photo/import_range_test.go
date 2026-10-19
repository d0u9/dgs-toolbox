package photo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

func TestParseDateRangeForms(t *testing.T) {
	for text, want := range map[string]dateRange{
		"":                  {},
		"20261001-20261018": {"20261001", "20261018"},
		"-20261018":         {"", "20261018"},
		"20261001-":         {"20261001", ""},
		"20261001-20261001": {"20261001", "20261001"},
	} {
		if got, err := parseDateRange(text); err != nil || got != want {
			t.Errorf("%q = %+v, %v", text, got, err)
		}
	}
	for _, text := range []string{"-", "20261001", "2026-10-01", "20261301-", "20261018-20261001", "1-2", "20261001--20261002"} {
		if _, err := parseDateRange(text); err == nil {
			t.Errorf("%q accepted", text)
		}
	}
	r := dateRange{"20261001", ""}
	if !r.contains("20261001") || r.contains("20260930") || !(dateRange{"", "20261001"}).contains("20261001") {
		t.Error("ends must be inclusive")
	}
}

// rangeCard holds two shots a day apart, a RAW paired with the first, a
// RAW-only shot dated through nothing, and a video.
func rangeCard(t *testing.T) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "card")
	writeTree(t, source, "DCIM/A.DNG", "DCIM/C.DNG", "DCIM/V.MOV")
	for name, taken := range map[string]string{"A.JPG": "2026:10:01 10:00:00", "B.JPG": "2026:10:02 10:00:00"} {
		if err := os.WriteFile(filepath.Join(source, "DCIM", name), testJPEG(t, taken), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return source
}

func TestWithinRangeKeepsShotsTogetherAndSetsUndatedAside(t *testing.T) {
	source := rangeCard(t)
	scan := scanSource(source)
	days := captureDays(source, scan.source)
	in, undated := withinRange(scan.source, days, dateRange{"20261001", "20261001"})
	var names []string
	for _, file := range in {
		names = append(names, file.path)
	}
	slices.Sort(names)
	slices.Sort(undated)
	if !slices.Equal(names, []string{"DCIM/A.DNG", "DCIM/A.JPG"}) || !slices.Equal(undated, []string{"DCIM/C.DNG", "DCIM/V.MOV"}) {
		t.Fatalf("in %v undated %v", names, undated)
	}
}

func TestCLIRangeImportsOnlyThoseDays(t *testing.T) {
	source := rangeCard(t)
	library := filepath.Join(t.TempDir(), "library")
	if err := os.MkdirAll(library, 0o755); err != nil {
		t.Fatal(err)
	}
	flags := map[string]string{"dest": `["` + library + `"]`, "ext": "[]", "workers": "1", "duplicates": "skip", "incremental": "true", "range": "20261002-", tui.GivenFlags: "[]"}
	var out bytes.Buffer
	if err := importDirectoriesContext(context.Background(), &out, []string{source}, flags, config.Default()); err != nil {
		t.Fatal(err, out.String())
	}
	if !strings.Contains(out.String(), "Range 20261002-: 1 of 5 files") || !strings.Contains(out.String(), "No capture date, not imported: DCIM/V.MOV") {
		t.Fatalf("output:\n%s", out.String())
	}
	for name, want := range map[string]bool{"B.JPG": true, "A.JPG": false, "A.DNG": false, "V.MOV": false} {
		if _, err := os.Stat(filepath.Join(library, "DCIM", name)); (err == nil) != want {
			t.Errorf("%s imported = %v", name, err == nil)
		}
	}
	flags["range"] = "20261002"
	if err := importDirectoriesContext(context.Background(), &out, []string{source}, flags, config.Default()); err == nil {
		t.Fatal("malformed range accepted")
	}
}

func TestWorkspaceRangeFiltersSourceAndRefusesMalformed(t *testing.T) {
	source := rangeCard(t)
	model := newImportModelWithSettings("_dgs-state", source, t.TempDir()).(importModel)
	model.width, model.height = 150, 40
	next, _ := model.startScan()
	model = next.(importModel)
	msg := model.scanAllTargetsCmd(model.scanGeneration)()
	next, _ = model.Update(msg)
	model = next.(importModel)
	model.controls.SetValue(rangeID, "-20261001")
	if got := len(model.filteredSource()); got != 2 {
		t.Fatalf("filtered %d files, want A.JPG and A.DNG", got)
	}
	model.controls.SetValue(rangeID, "oops")
	if got := len(model.filteredSource()); got != 5 {
		t.Fatalf("malformed range filtered: %d", got)
	}
	next, _ = model.startProcessing()
	if model = next.(importModel); model.stage != parameterStage || !strings.Contains(model.actionNotice, "range") {
		t.Fatalf("malformed range started: %q", model.actionNotice)
	}
}
