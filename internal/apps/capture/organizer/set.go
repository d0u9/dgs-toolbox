package organizer

// Set is the Recipes available to a session: whatever was loaded from the
// Recipe directory. Recipes are per-session state rather than package state, so
// a caller with a different Recipe directory does not disturb another's.
type Set struct {
	recipes []Recipe
}

// NewSet builds a Set from an explicit list, for callers that construct one.
func NewSet(recipes []Recipe) Set { return Set{recipes: append([]Recipe(nil), recipes...)} }

// All returns every Recipe in the Set, in a stable order.
func (s Set) All() []Recipe { return append([]Recipe(nil), s.recipes...) }

// Lookup returns a Recipe by id.
func (s Set) Lookup(id RecipeID) (Recipe, bool) {
	for _, recipe := range s.recipes {
		if recipe.ID == id {
			return recipe, true
		}
	}
	return Recipe{}, false
}

// Find returns the Recipes offered for a Capture. It narrows the candidates
// only; it never chooses one, not even when exactly one matches. A disabled
// Recipe is not a candidate: it stays in the Set so the report can say it is
// there and switched off, which is the answer to "where did it go?".
func (s Set) Find(capture Capture, settings Settings) []Recipe {
	// The Context carries no enrichment: whether a Recipe is a candidate is a
	// question about the Capture as it arrived, asked before anyone has typed
	// anything, and an answer that changed as they typed would move the list
	// under the cursor.
	ctx := NewContext(capture, nil).WithSettings(settings)
	var candidates []Recipe
	for _, recipe := range s.recipes {
		if !recipe.Disabled && recipe.matches(ctx) {
			candidates = append(candidates, recipe)
		}
	}
	return candidates
}
