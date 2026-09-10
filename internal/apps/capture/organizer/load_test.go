package organizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRecipe(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const dailyRecipe = `
name: Work Daily
match:
  workflows: [been_here]
fields:
  - id: project
    label: Project
    required: true
actions:
  - id: obsidian.daily.append
`

func TestLoadLayersFilesOverTheBuiltins(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "work_daily.yaml", dailyRecipe)

	loaded := Load(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	if loaded.Files != 1 {
		t.Fatalf("files = %d, want 1", loaded.Files)
	}

	recipe, ok := loaded.Set.Lookup("work_daily")
	if !ok {
		t.Fatal("the recipe was not loaded under its filename")
	}
	if recipe.Name != "Work Daily" || recipe.Source != filepath.Join(dir, "work_daily.yaml") {
		t.Fatalf("recipe = %+v", recipe)
	}
	if len(recipe.Actions) != 1 || recipe.Actions[0] != ActionDailyAppend {
		t.Fatalf("actions = %v", recipe.Actions)
	}
	if len(recipe.Fields) != 1 || recipe.Fields[0].Field != "project" || !recipe.Fields[0].Required {
		t.Fatalf("fields = %+v", recipe.Fields)
	}
	// A field with no input is text, the common case, rather than an error.
	if recipe.Fields[0].Input != InputText {
		t.Fatalf("input = %q, want the text default", recipe.Fields[0].Input)
	}
	// The built-ins are still there.
	if _, ok := loaded.Set.Lookup("obsidian_location"); !ok {
		t.Fatal("loading a file dropped the built-in recipes")
	}
}

// A file with a built-in's id replaces it in place rather than appearing twice.
func TestLoadOverridesABuiltinByID(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "obsidian_location.yaml", `
name: My Location
match:
  workflows: [been_here, quick_mark]
actions:
  - id: obsidian.location.append
`)

	loaded := Load(dir)
	recipe, ok := loaded.Set.Lookup("obsidian_location")
	if !ok {
		t.Fatal("the override is missing")
	}
	if recipe.Name != "My Location" || len(recipe.Actions) != 1 {
		t.Fatalf("recipe = %+v", recipe)
	}
	count := 0
	for _, existing := range loaded.Set.All() {
		if existing.ID == "obsidian_location" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("the id appears %d times, want the built-in replaced in place", count)
	}
	if got := len(loaded.Set.All()); got != len(Builtin().All()) {
		t.Fatalf("set holds %d recipes, want %d", got, len(Builtin().All()))
	}
}

func TestLoadReportsBadFilesWithoutDroppingTheRest(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "unknown action", body: "name: X\nactions:\n  - id: obsidian.nope\n", want: `unknown action "obsidian.nope"`},
		{name: "no action", body: "name: X\n", want: "would organize nothing"},
		{name: "misspelled key", body: "name: X\nactons:\n  - id: obsidian.daily.append\n", want: "field actons not found"},
		{name: "unknown input", body: "name: X\nfields:\n  - id: due\n    input: calendar\nactions:\n  - id: obsidian.daily.append\n", want: `unknown input "calendar"`},
		{name: "malformed yaml", body: "name: [unclosed\n", want: "decode"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRecipe(t, dir, "broken.yaml", tc.body)
			writeRecipe(t, dir, "work_daily.yaml", dailyRecipe)

			loaded := Load(dir)
			if len(loaded.Failures) != 1 {
				t.Fatalf("failures = %v, want the one bad file", loaded.Failures)
			}
			if !strings.Contains(loaded.Failures[0].Err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", loaded.Failures[0].Err, tc.want)
			}
			if !strings.HasSuffix(loaded.Failures[0].Path, "broken.yaml") {
				t.Errorf("failure names %q, want the file that failed", loaded.Failures[0].Path)
			}
			// The good file beside it still loads.
			if _, ok := loaded.Set.Lookup("work_daily"); !ok {
				t.Error("one bad file dropped a good one")
			}
		})
	}
}

func TestLoadIgnoresWhatIsNotARecipe(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "notes.txt", "not a recipe")
	writeRecipe(t, dir, ".hidden.yaml", dailyRecipe)
	if err := os.Mkdir(filepath.Join(dir, "nested.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	loaded := Load(dir)
	if loaded.Files != 0 || len(loaded.Failures) != 0 {
		t.Fatalf("files = %d, failures = %v", loaded.Files, loaded.Failures)
	}
}

// A missing directory is the normal state of an installation with no Recipes.
func TestLoadTreatsAMissingDirectoryAsNoRecipes(t *testing.T) {
	for _, dir := range []string{"", filepath.Join(t.TempDir(), "absent")} {
		loaded := Load(dir)
		if len(loaded.Failures) != 0 {
			t.Fatalf("dir %q: failures = %v", dir, loaded.Failures)
		}
		if len(loaded.Set.All()) != len(Builtin().All()) {
			t.Fatalf("dir %q: set = %d recipes", dir, len(loaded.Set.All()))
		}
	}
}

// A loaded Recipe behaves like any other: it is offered by Find and its
// requirements come from its Actions.
func TestALoadedRecipeIsOfferedAndCarriesItsRequirements(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "work_daily.yaml", dailyRecipe)
	set := Load(dir).Set

	found := false
	for _, recipe := range set.Find(beenHere()) {
		if recipe.ID != "work_daily" {
			continue
		}
		found = true
		fields := fieldIDs(recipe.RequiredFields(recipe.Actions))
		if !equalFields(fields, []FieldID{FieldCreatedAt, FieldContent, "project"}) {
			t.Errorf("required = %v", fields)
		}
	}
	if !found {
		t.Error("a loaded recipe is not offered for a matching capture")
	}
}
