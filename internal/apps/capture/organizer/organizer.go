package organizer

// Selection is what the user has decided about one Capture: which Recipe,
// which of its Actions are enabled, and what they supplied. The enabled set is
// per-Capture state rather than a property of the Recipe, so it is passed into
// MissingFields and Build rather than read off the Recipe.
type Selection struct {
	Recipe     RecipeID
	Enabled    map[ActionID]bool
	Enrichment map[FieldID]any
}

// NewSelection starts a Selection on a Recipe with its default Action set.
func NewSelection(recipe Recipe) Selection {
	return Selection{
		Recipe:     recipe.ID,
		Enabled:    recipe.DefaultEnabled(),
		Enrichment: make(map[FieldID]any),
	}
}

// Toggle flips one Action on or off for this Capture.
func (s Selection) Toggle(id ActionID) {
	s.Enabled[id] = !s.Enabled[id]
}

// EnabledActions lists the enabled Actions in the order the Recipe lists them.
func (s Selection) EnabledActions(recipe Recipe) []ActionID {
	var enabled []ActionID
	for _, id := range recipe.Actions {
		if s.Enabled[id] {
			enabled = append(enabled, id)
		}
	}
	return enabled
}

// Set records one enriched value, or clears it when the value is empty.
func (s Selection) Set(field FieldID, value any) {
	if isEmpty(value) {
		delete(s.Enrichment, field)
		return
	}
	s.Enrichment[field] = value
}

// FindRecipes returns the Recipes offered for a Capture. It narrows the
// candidates only; it never chooses one, not even when exactly one matches.
func FindRecipes(capture Capture) []Recipe {
	var candidates []Recipe
	for _, recipe := range builtinRecipes {
		if recipe.matches(capture) {
			candidates = append(candidates, recipe)
		}
	}
	return candidates
}

// MissingFields returns the requirements the Context does not yet satisfy,
// attributed to the Action that declared them so a caller can say which Action
// is blocking. A requirement whose When evaluates false is skipped entirely,
// and a disabled Action contributes nothing at all.
func MissingFields(ctx Context, recipe Recipe, enabled []ActionID) []FieldRequirement {
	var missing []FieldRequirement
	for _, req := range recipe.RequiredFields(enabled) {
		if !req.Required {
			continue
		}
		if req.When != nil && !req.When(ctx) {
			continue
		}
		if req.satisfied(ctx) {
			continue
		}
		missing = append(missing, req)
	}
	return missing
}

// Build expands a Recipe into one ActionPlan per enabled Action, in Recipe
// order. An Action with unmet requirements still appears, with its target
// unresolved, so the blockage stays visible instead of being dropped.
func Build(ctx Context, recipe Recipe, enabled []ActionID) []ActionPlan {
	missing := MissingFields(ctx, recipe, enabled)
	var plans []ActionPlan
	for _, id := range recipe.Actions {
		if !contains(enabled, id) {
			continue
		}
		def, ok := LookupAction(id)
		if !ok {
			continue
		}
		plan := ActionPlan{Action: id, Label: def.Label, Missing: missingFor(missing, id)}
		if def.Target != nil {
			plan.Target = def.Target(ctx)
		}
		if id == ActionDailyAppend {
			plan.Content = ctx.String(FieldContent)
		}
		plans = append(plans, plan)
	}
	return plans
}

// Ready reports whether a Capture can be executed: it has at least one enabled
// Action and nothing is missing. An empty plan counts as blocked, never ready.
func Ready(ctx Context, recipe Recipe, enabled []ActionID) bool {
	if len(enabled) == 0 {
		return false
	}
	return len(MissingFields(ctx, recipe, enabled)) == 0
}

// missingFor picks out the requirements attributed to one Action, plus the
// Recipe's own, which belong to no Action and block every one of them.
func missingFor(missing []FieldRequirement, id ActionID) []FieldRequirement {
	var out []FieldRequirement
	for _, req := range missing {
		if req.Action == id || req.Action == "" {
			out = append(out, req)
		}
	}
	return out
}
