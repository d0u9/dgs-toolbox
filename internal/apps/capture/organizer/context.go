package organizer

import (
	"fmt"
	"strings"

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
	if value, ok := c.Enrichment[field]; ok && !isEmpty(value) {
		return value, true
	}
	// A configured source comes before the compiled-in one so a workflow can
	// correct what the toolbox assumed as well as name what it never knew.
	if value, ok := sourced(c.Settings, c.Capture, field); ok {
		return value, true
	}
	if value, ok := resolve(c.Capture, field); ok {
		return value, true
	}
	return derive(c.Capture, field)
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
	paths, ok := settings.Sources[capture.Workflow()][field]
	if !ok {
		paths = settings.Sources[AnyWorkflow][field]
	}
	for _, path := range paths {
		if value, ok := valueAt(capture.Index, strings.TrimSpace(path)); ok {
			return value, true
		}
	}
	return nil, false
}

// AnyWorkflow is the workflow a source block answers for when the Capture's own
// workflow has none of its own.
const AnyWorkflow = "*"

// PayloadRoot is the one thing a source path may start from.
const PayloadRoot = "payload"

// valueAt walks a dotted path into the payload. A path that does not resolve is
// not an error here: the field is simply still missing, and the reader is asked
// for it the way any other missing field is.
func valueAt(index indexschema.Index, path string) (any, bool) {
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
	return payloadValue(current)
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
func resolve(capture Capture, field FieldID) (any, bool) {
	index := capture.Index
	switch field {
	case FieldCreatedAt:
		return present(index.CreatedAt)
	case FieldCity:
		return present(index.CapturePlace().City)
	case FieldRegion:
		return present(index.CapturePlace().Region)
	case FieldCountry:
		return present(index.CapturePlace().Country)
	case FieldLatitude:
		if index.Coordinates == nil {
			return nil, false
		}
		return index.Coordinates.Latitude, true
	case FieldLongitude:
		if index.Coordinates == nil {
			return nil, false
		}
		return index.Coordinates.Longitude, true
	}
	return nil, false
}

// derive computes a field the Capture does not carry outright. A derived value
// is a starting point rather than the Capture's own data, so FromCapture
// reports false for it and the user may replace it.
func derive(capture Capture, field FieldID) (any, bool) {
	if field != FieldPlaceName {
		return nil, false
	}
	return composePlaceName(capture.Index.CapturePlace())
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
	_, ok := resolve(c.Capture, field)
	return ok
}
