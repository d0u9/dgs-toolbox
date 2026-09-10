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

// resolve reads a logical field out of the Capture itself.
func resolve(capture Capture, field FieldID) (any, bool) {
	index := capture.Index
	switch field {
	case FieldContent:
		return payloadContent(index)
	case FieldTitle:
		return payloadString(index, "title")
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

// contentKeys maps a workflow to the payload key its note text lives under.
// The same logical field is sourced differently per workflow, which is exactly
// why callers never name a payload key themselves.
var contentKeys = map[string]string{
	"been_here":  "note",
	"photo_note": "text",
	"quick_mark": "mark",
}

func payloadContent(index indexschema.Index) (any, bool) {
	if key, ok := contentKeys[index.Source.Workflow]; ok {
		if value, ok := payloadString(index, key); ok {
			return value, true
		}
	}
	return payloadString(index, "note")
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
