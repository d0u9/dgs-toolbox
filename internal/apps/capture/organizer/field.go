// Package organizer is the data abstraction underneath Capture organizing: it
// turns a Capture into candidate Recipes, reports what those Recipes still
// need, and builds the Action Plan they expand into. It holds every domain
// judgement and carries no TUI dependency; see docs/apps/capture/organizer.md.
package organizer

// FieldID is a logical field, never a path into the Capture JSON. The same
// logical field is sourced differently per workflow, so callers ask for
// FieldContent rather than for payload.note.
type FieldID string

const (
	FieldContent   FieldID = "content"
	FieldTitle     FieldID = "title"
	FieldCreatedAt FieldID = "createdAt"
	FieldPlaceName FieldID = "place.name"
	FieldCity      FieldID = "place.city"
	FieldRegion    FieldID = "place.region"
	FieldCountry   FieldID = "place.country"
	// FieldCoordinates is the position itself, which is the field a Recipe
	// means when it asks whether the Capture has one at all. Latitude and
	// longitude are the same question asked twice — the index carries them as
	// one object, so neither can be present without the other — and a Recipe
	// naming one of them to mean "has a position" reads as though the other
	// could be missing.
	FieldCoordinates FieldID = "coordinates"
	FieldLatitude    FieldID = "coordinates.latitude"
	FieldLongitude   FieldID = "coordinates.longitude"
	FieldTags        FieldID = "tags"
	FieldDueAt       FieldID = "due_at"
	FieldStartAt     FieldID = "start_at"
	FieldAllDay      FieldID = "all_day"
)

// InputType tells a caller how to ask for a field. A bare field name is not
// enough: the caller must know to ask for a datetime rather than free text.
type InputType string

const (
	InputText        InputType = "text"
	InputMultiline   InputType = "multiline"
	InputDateTime    InputType = "datetime"
	InputMultiSelect InputType = "multi_select"
)

// Predicate gates a conditional requirement, such as a Calendar Action needing
// start_at only when all_day is false.
type Predicate func(Context) bool

// Validator reports whether a resolved value satisfies a requirement. A nil
// Validator accepts any present, non-empty value.
type Validator func(value any) bool

// FieldRequirement carries enough to drive an input surface on its own. Action
// records which Action declared it, so a caller can say which Action is
// blocking; it is empty for requirements the Recipe declares itself.
type FieldRequirement struct {
	Field    FieldID
	Label    string
	Required bool
	Input    InputType
	When     Predicate
	Validate Validator
	Action   ActionID
}

// satisfied reports whether ctx already carries an acceptable value.
func (r FieldRequirement) satisfied(ctx Context) bool {
	value, ok := ctx.Get(r.Field)
	if !ok {
		return false
	}
	if r.Validate != nil {
		return r.Validate(value)
	}
	return !isEmpty(value)
}

func isEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []string:
		return len(v) == 0
	default:
		return false
	}
}

func text(field FieldID, label string) FieldRequirement {
	return FieldRequirement{Field: field, Label: label, Required: true, Input: InputText}
}

func multiline(field FieldID, label string) FieldRequirement {
	return FieldRequirement{Field: field, Label: label, Required: true, Input: InputMultiline}
}

func optional(req FieldRequirement) FieldRequirement {
	req.Required = false
	return req
}
