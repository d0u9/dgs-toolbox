package capture

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/config"
)

// starterConfig is an installation that has been given the Recipes this
// version ships, which is what a reader has after --export-recipes. Writing
// them is how the tests get them, so the exporter is exercised too.
func starterConfig(t *testing.T) config.Config {
	t.Helper()
	global := config.Config{ConfigDir: t.TempDir()}
	if err := writeStarterRecipes(io.Discard, global); err != nil {
		t.Fatal(err)
	}
	return global
}

// The report is written from what loaded, so it cannot drift from what Route
// offers: every Recipe and every Action it names must appear.
func TestRecipeReportCoversTheWholeRegistry(t *testing.T) {
	global := starterConfig(t)
	var out strings.Builder
	if err := writeRecipeReport(&out, global); err != nil {
		t.Fatal(err)
	}
	report := out.String()

	recipes := loadRecipes(global).Set.All()
	for _, recipe := range recipes {
		if !strings.Contains(report, string(recipe.ID)) {
			t.Errorf("report omits recipe %q", recipe.ID)
		}
		for _, id := range recipe.Actions {
			if !strings.Contains(report, string(id)) {
				t.Errorf("report omits action %q of recipe %q", id, recipe.ID)
			}
		}
	}
	if !strings.Contains(report, "RECIPES  7") {
		t.Errorf("report does not count the recipes:\n%s", report)
	}
	if !strings.Contains(report, "obsidian_daily.yaml") {
		t.Errorf("report does not say which file a recipe came from:\n%s", report)
	}
}

// A requirement is written by field id, and the ids exist nowhere else a reader
// can look them up, so the report lists them with their input type.
func TestRecipeReportNamesFieldsAndTheirInputTypes(t *testing.T) {
	var out strings.Builder
	if err := writeRecipeReport(&out, starterConfig(t)); err != nil {
		t.Fatal(err)
	}
	report := out.String()

	for _, want := range []string{
		"content*",                // required, marked
		"tags",                    // optional, unmarked
		"start_at* (conditional)", // required only when the predicate holds
		"content                  multiline",
		"due_at                   datetime",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report is missing %q:\n%s", want, report)
		}
	}
}

// The Actions document is a catalogue someone writes a Recipe against, so an
// Action the binary carries and the document does not name is a trap: the
// Recipe they write from it is missing something they had no way to know
// about. The document is not read for what an Action does — only for whether
// it is named at all.
func TestActionsDocumentNamesEveryAction(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "apps", "capture", "actions.md")
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range organizer.Actions() {
		if !strings.Contains(string(document), string(def.ID)) {
			t.Fatalf("docs/apps/capture/actions.md does not name %q — add it there in the same change", def.ID)
		}
	}
}
