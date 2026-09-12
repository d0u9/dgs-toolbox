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

func TestLoadReadsARecipeFile(t *testing.T) {
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
	// Nothing else appears from anywhere: the directory is the whole Set.
	if len(loaded.Set.All()) != 1 {
		t.Fatalf("set = %d recipes, want only the file in the directory", len(loaded.Set.All()))
	}
}

// A file with a built-in's id replaces it in place rather than appearing twice.
// The filename is the id, so a directory cannot hold the same Recipe twice —
// and a file dropped in beside the shipped copies replaces that copy rather
// than adding a second Recipe under the same name.
func TestLoadKeepsOneRecipePerID(t *testing.T) {
	dir := t.TempDir()
	for _, starter := range Starters() {
		writeRecipe(t, dir, starter.Name, string(starter.Data))
	}
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
		t.Fatal("the edited recipe is missing")
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
		t.Fatalf("the id appears %d times, want one recipe per file", count)
	}
	if got := len(loaded.Set.All()); got != len(Starters()) {
		t.Fatalf("set holds %d recipes, want the %d files in the directory", got, len(Starters()))
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
		// Nothing is compiled in, so an installation that has not been given
		// its Recipes has none — Route says so by offering none.
		if len(loaded.Set.All()) != 0 {
			t.Fatalf("dir %q: set = %d recipes, want none", dir, len(loaded.Set.All()))
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

// A Recipe is switched off by one line, and switched back on by removing it.
// The file stays, because deleting it is the other thing entirely.
func TestADisabledRecipeStaysInTheSetButIsNotOffered(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "work_daily.yaml", dailyRecipe+"enabled: false\n")

	loaded := Load(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	recipe, ok := loaded.Set.Lookup("work_daily")
	if !ok || !recipe.Disabled {
		t.Fatalf("recipe = %+v, want it loaded and disabled", recipe)
	}
	// It still knows what it is, so the report can say what has been switched
	// off rather than only that something is missing.
	if len(recipe.Actions) != 1 {
		t.Fatalf("actions = %v, want the recipe to keep its definition", recipe.Actions)
	}
	for _, candidate := range loaded.Set.Find(beenHere()) {
		if candidate.ID == "work_daily" {
			t.Fatal("a disabled recipe was still offered")
		}
	}
}

// A switched-off Recipe still has to be a Recipe: the file says what it does
// and then says not to do it, so switching it back on needs no memory of what
// it was.
func TestADisabledRecipeStillDeclaresItsActions(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "work_daily.yaml", "enabled: false\n")

	loaded := Load(dir)
	if len(loaded.Failures) != 1 {
		t.Fatalf("failures = %v, want the empty recipe refused", loaded.Failures)
	}
	if !strings.Contains(loaded.Failures[0].Err.Error(), "no action") {
		t.Fatalf("error = %v, want it to say the recipe does nothing", loaded.Failures[0].Err)
	}
}

// An Action that is off by default is still in the Recipe: it is listed in
// ACTIONS and one keystroke away, it simply is not ticked on arrival.
func TestAnActionCanBeOffByDefault(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "work_daily.yaml", `
name: Work Daily
match:
  workflows: [been_here]
actions:
  - id: obsidian.daily.append
  - id: obsidian.location.append
    enabled: false
`)

	loaded := Load(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	recipe, _ := loaded.Set.Lookup("work_daily")
	if len(recipe.Actions) != 2 {
		t.Fatalf("actions = %v, want both to stay in the recipe", recipe.Actions)
	}
	enabled := recipe.DefaultEnabled()
	if !enabled[ActionDailyAppend] || enabled[ActionLocationAppend] {
		t.Fatalf("default enabled = %v, want only the daily action ticked", enabled)
	}
	selection := NewSelection(recipe)
	if actions := selection.EnabledActions(recipe); len(actions) != 1 || actions[0] != ActionDailyAppend {
		t.Fatalf("selection = %v, want the recipe's default", actions)
	}
}

// starterSet loads the Recipes this version ships, the way a run does: written
// into a directory and read back. Tests exercise what a reader will actually
// have, and a shipped file that no longer parses fails here rather than on
// their first run.
func starterSet(t *testing.T) Set {
	t.Helper()
	dir := t.TempDir()
	for _, starter := range Starters() {
		if err := os.WriteFile(filepath.Join(dir, starter.Name), starter.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loaded := Load(dir)
	if len(loaded.Failures) > 0 {
		t.Fatalf("the shipped recipes do not load: %v", loaded.Failures)
	}
	return loaded.Set
}

// starterRecipe is one of them, by id.
func starterRecipe(t *testing.T, id RecipeID) Recipe {
	t.Helper()
	recipe, ok := starterSet(t).Lookup(id)
	if !ok {
		t.Fatalf("no shipped recipe %q", id)
	}
	return recipe
}

// Every shipped Recipe parses, names Actions this binary has, and is offered
// for at least one of the workflows it claims.
func TestShippedRecipesLoadAndMatch(t *testing.T) {
	set := starterSet(t)
	if len(set.All()) == 0 {
		t.Fatal("this version ships no recipes")
	}
	for _, recipe := range set.All() {
		if len(recipe.Actions) == 0 {
			t.Fatalf("%s names no action", recipe.ID)
		}
		for _, id := range recipe.Actions {
			if _, ok := LookupAction(id); !ok {
				t.Fatalf("%s names unknown action %q", recipe.ID, id)
			}
		}
	}
	offered := set.Find(beenHere())
	if len(offered) == 0 {
		t.Fatal("a been_here capture with a position is offered no shipped recipe")
	}
}
