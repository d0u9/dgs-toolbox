package organizer

// Set is the Recipes available to a session: whatever was loaded from the
// Recipe directory. Recipes are per-session state rather than package state, so
// a caller with a different Recipe directory does not disturb another's.
type Set struct {
	recipes []Recipe
}

// NewSet builds a Set from an explicit list, for callers that construct one.
func NewSet(recipes []Recipe) Set { return Set{recipes: append([]Recipe(nil), recipes...)} }

// With layers Recipes over this Set: one whose id matches replaces the Recipe
// it shadows in place, keeping the order stable, and one with a new id is
// appended. Overriding rather than merging means a user file is the whole
// Recipe, not a patch on a built-in that has to be read alongside it.
func (s Set) With(recipes []Recipe) Set {
	merged := append([]Recipe(nil), s.recipes...)
	for _, recipe := range recipes {
		replaced := false
		for index, existing := range merged {
			if existing.ID == recipe.ID {
				merged[index] = recipe
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, recipe)
		}
	}
	return Set{recipes: merged}
}

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
func (s Set) Find(capture Capture) []Recipe {
	var candidates []Recipe
	for _, recipe := range s.recipes {
		if !recipe.Disabled && recipe.matches(capture) {
			candidates = append(candidates, recipe)
		}
	}
	return candidates
}
