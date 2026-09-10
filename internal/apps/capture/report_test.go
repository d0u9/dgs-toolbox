package capture

import (
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/config"
)

// The report is written from the registry, so it cannot drift from what Route
// offers: every Recipe and every Action it names must appear.
func TestRecipeReportCoversTheWholeRegistry(t *testing.T) {
	var out strings.Builder
	if err := writeRecipeReport(&out, config.Config{}); err != nil {
		t.Fatal(err)
	}
	report := out.String()

	recipes := organizer.Recipes()
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
	if !strings.Contains(report, organizer.BuiltinSource) {
		t.Errorf("report does not say where a recipe came from:\n%s", report)
	}
}

// A requirement is written by field id, and the ids exist nowhere else a reader
// can look them up, so the report lists them with their input type.
func TestRecipeReportNamesFieldsAndTheirInputTypes(t *testing.T) {
	var out strings.Builder
	if err := writeRecipeReport(&out, config.Config{}); err != nil {
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
