package organizer

import (
	"fmt"
	"strings"
	"text/template"

	"dgs-toolbox/internal/apps/capture/indexschema"
)

// Capture is one scanned Capture as the organizer sees it.
type Capture struct {
	Path  string
	Name  string
	Index indexschema.Index
}

// Workflow reports the workflow that produced the Capture.
func (c Capture) Workflow() string { return c.Index.Source.Workflow }

// Context resolves logical fields for one Capture. Recipes never read the
// Capture JSON directly; they read a Context, which reads
// enrichment -> Capture -> derived. The Capture on disk is never mutated.
type Context struct {
	Capture    Capture
	Enrichment map[FieldID]any
	// Settings are where the Actions write. They belong to the Context because
	// a target is resolved from the Capture and the settings together: the same
	// Capture organized against a different vault plans a different path.
	Settings Settings
	// Parameters are this Capture's overrides, by Action and name. They are
	// per-Capture rather than remembered across them: an override that stayed
	// on would quietly apply to Captures nobody meant it for.
	Parameters map[ActionID]map[string]string
}

// NewContext builds a Context over a Capture and the values the user supplied,
// with default folder names and no vault.
func NewContext(capture Capture, enrichment map[FieldID]any) Context {
	return Context{Capture: capture, Enrichment: enrichment, Settings: DefaultSettings()}
}

// WithSettings returns the Context reading against different settings.
func (c Context) WithSettings(settings Settings) Context {
	c.Settings = settings
	return c
}

// WithParameters returns the Context carrying one Capture's overrides.
func (c Context) WithParameters(parameters map[ActionID]map[string]string) Context {
	c.Parameters = parameters
	return c
}

// Parameter is what an Action should use: the override for this Capture when
// there is one, and the configured default otherwise.
func (c Context) Parameter(action ActionID, name string) string {
	if value, ok := c.Parameters[action][name]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	def, ok := LookupAction(action)
	if !ok {
		return ""
	}
	for _, parameter := range def.Parameters {
		if parameter.Name == name && parameter.Default != nil {
			return parameter.Default(c.Settings)
		}
	}
	return ""
}

// Get returns the value of a logical field and whether it resolved at all.
// User enrichment wins over the Capture's own data.
func (c Context) Get(field FieldID) (any, bool) {
	// The position fields have one resolver, so latitude, longitude and the
	// pair cannot be read from three different places.
	if isPositionField(field) {
		return c.positionField(field)
	}
	if value, ok := c.Enrichment[field]; ok && !isEmpty(value) {
		return value, true
	}
	// A configured source comes before the compiled-in one so a workflow can
	// correct what the toolbox assumed as well as name what it never knew.
	if value, ok := sourced(c.Settings, c.Capture, field); ok {
		return value, true
	}
	if value, ok := c.resolve(field); ok {
		return value, true
	}
	return c.derive(field)
}

// String returns a resolved field as a string, empty when it does not resolve.
func (c Context) String(field FieldID) string {
	value, ok := c.Get(field)
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// sourced reads a field from where the configuration says this workflow keeps
// it. The path is into the Capture's own payload, which is the half of the
// index a workflow defines; everything else in the index has one meaning and
// needs no telling.
func sourced(settings Settings, capture Capture, field FieldID) (any, bool) {
	// A workflow's own entry answers for it; AnyWorkflow answers for the ones
	// that have none. The specific wins whole rather than per path: a workflow
	// that says where it keeps a field has said it, and falling through to a
	// general list afterwards would read a key it did not name.
	paths, _ := Context{Capture: capture, Settings: settings}.sourcePaths(field)
	for _, source := range paths {
		if value, ok := valueOf(settings, capture.Index, strings.TrimSpace(source)); ok {
			return value, true
		}
	}
	return nil, false
}

// valueOf reads one source: a payload key, or a template over the payload when
// a field is several of them put together.
func valueOf(settings Settings, index indexschema.Index, source string) (any, bool) {
	if IsTemplateSource(source) {
		return composed(settings, index, source)
	}
	value, ok := payloadAt(index, source)
	if !ok {
		return nil, false
	}
	return payloadValue(value)
}

// IsTemplateSource reports whether a source composes a value rather than
// pointing at one. The two are told apart by the placeholders themselves: a
// payload key has none, and anything that does is a template.
func IsTemplateSource(source string) bool { return strings.Contains(source, "{{") }

// composed renders a source template over the payload, so a field kept as
// several keys is one field by the time an Action asks for it:
//
//	content: "{{.title}}{{with .body}}\n\n{{.}}{{end}}"
//
// The payload is the data, because that is the half of the index the workflow
// defines and the only half whose shape it is describing. A template that names
// a key the Capture does not carry leaves the field missing rather than failing
// the run — the same answer a path that resolves to nothing gives, and the
// reason optional pieces are written with "with" — so a Capture that happens
// not to carry one is asked about rather than refused.
func composed(settings Settings, index indexschema.Index, source string) (any, bool) {
	parsed, err := ParseSource(settings, source)
	if err != nil {
		return nil, false
	}
	var out strings.Builder
	if err := parsed.Execute(&out, index.Payload); err != nil {
		return nil, false
	}
	// A key named without a guard, on a Capture that does not carry it, renders
	// as the sentinel the template package writes for exactly that. It must not
	// reach a note, and it is not an error either: it is this Capture missing a
	// piece the source expected, so the field stays missing and is asked for.
	rendered := strings.TrimSpace(out.String())
	if strings.Contains(rendered, missingValue) {
		return nil, false
	}
	return payloadValue(rendered)
}

// missingValue is what text/template writes where a map has no such key. The
// alternative, refusing to render at all, would take "with" down with it — and
// guarding an optional piece with "with" is exactly how a source says a piece
// is optional.
const missingValue = "<no value>"

// ParseSource parses a source template. It is exported so a workflow file can
// be refused when it is loaded rather than rendering to nothing months later:
// a template that does not parse is a mistake, where a key that is absent is
// only a Capture that did not carry it.
func ParseSource(settings Settings, source string) (*template.Template, error) {
	return template.New("source").
		Funcs(templateFuncs(settings)).
		Option("missingkey=zero").
		Parse(source)
}

// AnyWorkflow is the workflow a source block answers for when the Capture's own
// workflow has none of its own.
const AnyWorkflow = "*"

// PayloadRoot is the one thing a source path may start from.
const PayloadRoot = "payload"

// payloadAt walks a dotted path into the payload and returns what is there as
// JSON decoded it. A path that does not resolve is not an error here: the field
// is simply still missing, and the reader is asked for it the way any other
// missing field is.
func payloadAt(index indexschema.Index, path string) (any, bool) {
	segments := strings.Split(path, ".")
	if len(segments) < 2 || segments[0] != PayloadRoot {
		return nil, false
	}
	var current any = index.Payload
	for _, segment := range segments[1:] {
		holder, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = holder[segment]
		if !ok {
			return nil, false
		}
	}
	return current, current != nil
}

// payloadValue turns what JSON decoded into what a field is. A list of strings
// is kept as one — tags are a field like any other — and anything else that is
// not a string is rendered, so a number written where text was expected is
// still the value the Capture carried rather than nothing at all.
func payloadValue(value any) (any, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, false
		}
		return v, true
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			items = append(items, text)
		}
		if len(items) == 0 {
			return nil, false
		}
		return items, true
	case bool:
		return fmt.Sprint(v), true
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%f", v), "0"), "."), true
	}
	return nil, false
}

// resolve reads a logical field out of the Capture itself. Only the fields the
// index gives one meaning: everything in the payload is the workflow's own
// shape, and where it keeps a field is said in that workflow's file rather than
// guessed at here.
//
// The place is read through c.place, which is empty once the position has come
// from somewhere else: the index's city is where the phone was.
func (c Context) resolve(field FieldID) (any, bool) {
	switch field {
	case FieldCreatedAt:
		return present(c.Capture.Index.CreatedAt)
	case FieldCity:
		return present(c.place().City)
	case FieldRegion:
		return present(c.place().Region)
	case FieldCountry:
		return present(c.place().Country)
	}
	return nil, false
}

// derive computes a field the Capture does not carry outright. A derived value
// is a starting point rather than the Capture's own data, so FromCapture
// reports false for it and the user may replace it.
func (c Context) derive(field FieldID) (any, bool) {
	if field != FieldPlaceName {
		return nil, false
	}
	return composePlaceName(c.place())
}

// composePlaceName names a place from its structured parts, because the index
// has no name field and no free-form address to borrow one from. The two most
// specific parts present are used: "Chuo, Osaka" rather than the full
// locality-to-country chain, which reads as an address instead of a name.
func composePlaceName(place indexschema.Place) (any, bool) {
	var parts []string
	for _, part := range []string{place.Locality, place.City, place.Region, place.Country} {
		if strings.TrimSpace(part) == "" {
			continue
		}
		parts = append(parts, strings.Join(strings.Fields(part), " "))
		if len(parts) == 2 {
			break
		}
	}
	if len(parts) == 0 {
		return nil, false
	}
	return strings.Join(parts, ", "), true
}

func payloadString(index indexschema.Index, key string) (any, bool) {
	value, ok := index.Payload[key]
	if !ok {
		return nil, false
	}
	s, ok := value.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil, false
	}
	return s, true
}

func present(value string) (any, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, false
	}
	return value, true
}

// FromCapture reports whether a field is the Capture's own data rather than
// enrichment or a derived starting point. Such a value is not the user's to
// edit; a derived one is.
func (c Context) FromCapture(field FieldID) bool {
	if isPositionField(field) {
		position, ok := c.Position()
		return ok && !position.Sourced
	}
	_, ok := c.resolve(field)
	return ok
}
