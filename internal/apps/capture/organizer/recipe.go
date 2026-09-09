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
	// Source is where the Recipe came from: BuiltinSource, or the path of the
	// file that defined it. A reader asking why a Recipe behaves as it does
	// needs to know which one to open.
	Source  string
	Match   Match
	Fields  []FieldRequirement
	Actions []ActionID
}

// matches reports whether this Recipe is a candidate for the Capture.
func (r Recipe) matches(capture Capture) bool {
	if len(r.Match.Workflows) > 0 && !contains(r.Match.Workflows, capture.Workflow()) {
		return false
	}
	if len(r.Match.RequiresAny) == 0 {
		return true
	}
	ctx := NewContext(capture, nil)
	for _, field := range r.Match.RequiresAny {
		if _, ok := ctx.Get(field); ok {
			return true
		}
	}
	return false
}

// DefaultEnabled returns the Action set a Recipe starts with: all of them.
func (r Recipe) DefaultEnabled() map[ActionID]bool {
	enabled := make(map[ActionID]bool, len(r.Actions))
	for _, id := range r.Actions {
		enabled[id] = true
	}
	return enabled
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
