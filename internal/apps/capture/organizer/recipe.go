package organizer

// RecipeID identifies one way of organizing a class of Capture.
type RecipeID string

// Match decides which Recipes are offered for a Capture. It never triggers
// execution: choosing among the candidates is always the user's decision.
type Match struct {
	Workflows   []string
	RequiresAny []FieldID
}

// Recipe has exactly three parts: what it matches, what it needs beyond its
// Actions, and the Actions it expands into.
type Recipe struct {
	ID   RecipeID
	Name string
	// Source is the path of the file that defined it. A reader asking why a
	// Recipe behaves as it does needs to know which file to open.
	Source  string
	Match   Match
	Fields  []FieldRequirement
	Actions []ActionID
	// Disabled takes a Recipe out of what is offered without taking it out of
	// the Set: a file that is switched off still exists, is still listed by the
	// report, and is turned back on by one line removed. Deleting the file is
	// the other thing, and a reader who did that to try something has lost the
	// Recipe.
	Disabled bool
	// DefaultOff are Actions the Recipe carries but does not start with. The
	// Action is still part of the Recipe and still one keystroke away in
	// ACTIONS; it simply is not ticked when the Recipe is chosen, for the one
	// that is wanted now and then rather than every time.
	DefaultOff []ActionID
}

// matches reports whether this Recipe is a candidate for the Capture. It is
// asked of a Context rather than of a Capture so that a requirement may name
// any field, including one a workflow file says where to find: a condition that
// could only be written about the index would be a second, smaller vocabulary
// to learn.
func (r Recipe) matches(ctx Context) bool {
	if len(r.Match.Workflows) > 0 && !contains(r.Match.Workflows, ctx.Capture.Workflow()) {
		return false
	}
	if len(r.Match.RequiresAny) == 0 {
		return true
	}
	for _, field := range r.Match.RequiresAny {
		if _, ok := ctx.Get(field); ok {
			return true
		}
	}
	return false
}

// DefaultEnabled returns the Action set a Recipe starts with: all of them,
// less the ones it declares off by default.
func (r Recipe) DefaultEnabled() map[ActionID]bool {
	enabled := make(map[ActionID]bool, len(r.Actions))
	for _, id := range r.Actions {
		enabled[id] = !contains(actionNames(r.DefaultOff), string(id))
	}
	return enabled
}

func actionNames(ids []ActionID) []string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, string(id))
	}
	return names
}

// RequiredFields is the union of the enabled Actions' requirements plus the
// Recipe's own declarations, in Recipe order. A Recipe never restates its
// Actions' requirements by hand, so adding one to an Action cannot silently
// desync the Recipes that use it. The Recipe's own fields belong to no Action
// and stay required whatever is enabled.
func (r Recipe) RequiredFields(enabled []ActionID) []FieldRequirement {
	var fields []FieldRequirement
	for _, id := range r.Actions {
		if !contains(enabled, id) {
			continue
		}
		def, ok := LookupAction(id)
		if !ok {
			continue
		}
		for _, req := range def.Required {
			req.Action = id
			fields = append(fields, req)
		}
	}
	return append(fields, r.Fields...)
}

func contains[T comparable](values []T, want T) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
