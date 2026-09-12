package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Failure is one Recipe file that could not be loaded, and why. A bad file is
// reported rather than skipped: silently dropping it would leave the author
// believing a Recipe is in effect when it is not.
type Failure struct {
	Path string
	Err  error
}

func (f Failure) Error() string { return f.Path + ": " + f.Err.Error() }

// Loaded is the outcome of reading a Recipe directory: the Set to use, and
// whatever could not be read. Failures do not prevent the rest from loading,
// because a Recipe is not a precondition for running the session the way the
// configuration file is.
type Loaded struct {
	Set      Set
	Dir      string
	Files    int
	Failures []Failure
}

// Load reads every Recipe file in dir. A missing directory is not an error: it
// is an installation that has not been given its Recipes yet, and Route says so
// by offering none rather than by refusing to open.
func Load(dir string) Loaded {
	loaded := Loaded{Dir: dir}
	if dir == "" {
		return loaded
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return loaded
	}
	if err != nil {
		loaded.Failures = append(loaded.Failures, Failure{Path: dir, Err: err})
		return loaded
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isRecipeFile(entry.Name()) {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)

	var recipes []Recipe
	for _, path := range paths {
		loaded.Files++
		recipe, err := LoadFile(path)
		if err != nil {
			loaded.Failures = append(loaded.Failures, Failure{Path: path, Err: err})
			continue
		}
		recipes = append(recipes, recipe)
	}
	loaded.Set = NewSet(recipes)
	return loaded
}

func isRecipeFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml":
		return !strings.HasPrefix(name, ".")
	}
	return false
}

// LoadFile reads one Recipe. Its id comes from the filename rather than from a
// field inside it, so a file cannot claim an id that disagrees with where it
// lives, and renaming the file is how a Recipe is renamed.
func LoadFile(path string) (Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, err
	}
	var document recipeDocument
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	// A misspelled key is an error rather than a silently ignored line: the
	// author would otherwise be told nothing and see nothing change.
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return Recipe{}, fmt.Errorf("decode: %w", err)
	}
	return document.recipe(path)
}

// recipeDocument is the on-disk shape. Actions are objects rather than bare
// strings so a later "with:" can be added without rewriting existing files.
type recipeDocument struct {
	Name string `yaml:"name"`
	// Enabled is a pointer so an absent key means "on" without the file having
	// to say so: switching a Recipe off is one line added, and switching it
	// back on is one line removed.
	Enabled *bool            `yaml:"enabled"`
	Match   matchDocument    `yaml:"match"`
	Fields  []fieldDocument  `yaml:"fields"`
	Actions []actionDocument `yaml:"actions"`
}

type matchDocument struct {
	Workflows   []string `yaml:"workflows"`
	RequiresAny []string `yaml:"requires_any"`
}

type fieldDocument struct {
	ID       string `yaml:"id"`
	Label    string `yaml:"label"`
	Required bool   `yaml:"required"`
	Input    string `yaml:"input"`
}

type actionDocument struct {
	ID string `yaml:"id"`
	// Enabled says whether the Action is ticked when the Recipe is chosen. It
	// is not whether the Action is in the Recipe: an Action that is off is
	// still listed in ACTIONS and still one keystroke away, for the one wanted
	// now and then rather than every time.
	Enabled *bool `yaml:"enabled"`
}

func (d recipeDocument) recipe(path string) (Recipe, error) {
	id := RecipeID(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if id == "" {
		return Recipe{}, errors.New("the filename is the recipe id and this one is empty")
	}
	name := strings.TrimSpace(d.Name)
	if name == "" {
		name = string(id)
	}
	if len(d.Actions) == 0 {
		return Recipe{}, errors.New("a recipe with no action would organize nothing")
	}
	disabled := d.Enabled != nil && !*d.Enabled

	recipe := Recipe{ID: id, Name: name, Source: path, Disabled: disabled}
	recipe.Match.Workflows = d.Match.Workflows
	for _, field := range d.Match.RequiresAny {
		recipe.Match.RequiresAny = append(recipe.Match.RequiresAny, FieldID(field))
	}
	for _, action := range d.Actions {
		id := ActionID(strings.TrimSpace(action.ID))
		if id == "" {
			return Recipe{}, errors.New("an action needs an id")
		}
		if _, ok := LookupAction(id); !ok {
			// Naming an Action this binary does not have is a mistake worth
			// stopping for: the Recipe would silently do less than it says.
			return Recipe{}, fmt.Errorf("unknown action %q", id)
		}
		recipe.Actions = append(recipe.Actions, id)
		if action.Enabled != nil && !*action.Enabled {
			recipe.DefaultOff = append(recipe.DefaultOff, id)
		}
	}
	for _, field := range d.Fields {
		requirement, err := field.requirement()
		if err != nil {
			return Recipe{}, err
		}
		recipe.Fields = append(recipe.Fields, requirement)
	}
	return recipe, nil
}

func (d fieldDocument) requirement() (FieldRequirement, error) {
	id := FieldID(strings.TrimSpace(d.ID))
	if id == "" {
		return FieldRequirement{}, errors.New("a field needs an id")
	}
	input, err := parseInput(d.Input)
	if err != nil {
		return FieldRequirement{}, fmt.Errorf("field %q: %w", id, err)
	}
	label := strings.TrimSpace(d.Label)
	if label == "" {
		label = string(id)
	}
	return FieldRequirement{Field: id, Label: label, Required: d.Required, Input: input}, nil
}

func parseInput(value string) (InputType, error) {
	switch InputType(strings.TrimSpace(value)) {
	case "":
		return InputText, nil
	case InputText:
		return InputText, nil
	case InputMultiline:
		return InputMultiline, nil
	case InputDateTime:
		return InputDateTime, nil
	case InputMultiSelect:
		return InputMultiSelect, nil
	}
	return "", fmt.Errorf("unknown input %q", value)
}
