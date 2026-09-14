package organizer

import (
	"fmt"
	"strconv"
	"strings"

	"dgs-toolbox/internal/apps/capture/indexschema"
)

// Position is where a Capture was, as the one answer every Action reads: the
// location note, its map links and a reminder at the place all ask here, so
// they cannot disagree about which point they mean.
type Position struct {
	Latitude  float64
	Longitude float64
	// Sourced is true when the position is not the index's own: a workflow file
	// named where the workflow keeps it, or the reader typed one. The index's
	// altitude and place then describe some other point, and are not used.
	Sourced bool
}

// Position resolves the Capture's position. What was typed in FIELDS wins, as
// it does for any field; then a source the workflow file names; then the
// index.
//
// A workflow that names a source has said the index is not where it keeps its
// position — the index is where the phone was, and the payload is the place
// the Capture is about — so when that source does not resolve the position is
// missing and asked for, rather than quietly taken from the index after all.
func (c Context) Position() (Position, bool) {
	if value, ok := c.Enrichment[FieldCoordinates]; ok && !isEmpty(value) {
		position, ok := parsePosition(value)
		position.Sourced = true
		return position, ok
	}
	if paths, ok := c.sourcePaths(FieldCoordinates); ok {
		for _, source := range paths {
			value, ok := rawValueOf(c.Settings, c.Capture.Index, strings.TrimSpace(source))
			if !ok {
				continue
			}
			if position, ok := parsePosition(value); ok {
				position.Sourced = true
				return position, true
			}
		}
		return Position{Sourced: true}, false
	}
	return indexPosition(c.Capture.Index)
}

func indexPosition(index indexschema.Index) (Position, bool) {
	if index.Coordinates == nil {
		return Position{}, false
	}
	return Position{Latitude: index.Coordinates.Latitude, Longitude: index.Coordinates.Longitude}, true
}

// positionSourced reports whether the index's position has been set aside for
// this Capture, which is also whether its altitude and place still apply.
func (c Context) positionSourced() bool {
	position, _ := c.Position()
	return position.Sourced
}

// place is the index's descriptive location, or nothing when the position came
// from elsewhere: a city named for where the phone was is wrong about a place
// the payload names.
func (c Context) place() indexschema.Place {
	if c.positionSourced() {
		return indexschema.Place{}
	}
	return c.Capture.Index.CapturePlace()
}

// sourcePaths is the sources configured for a field, the Capture's own
// workflow first and AnyWorkflow for one that has none.
func (c Context) sourcePaths(field FieldID) ([]string, bool) {
	paths, ok := c.Settings.Sources[c.Capture.Workflow()][field]
	if !ok {
		paths, ok = c.Settings.Sources[AnyWorkflow][field]
	}
	return paths, ok && len(paths) > 0
}

// rawValueOf reads a source without flattening it into a field value, because
// a position is often kept as an object and a field value has no room for one.
func rawValueOf(settings Settings, index indexschema.Index, source string) (any, bool) {
	if IsTemplateSource(source) {
		return composed(settings, index, source)
	}
	return payloadAt(index, source)
}

// parsePosition accepts the spellings a workflow keeps a position in: one
// string, "lat, lng", or an object with latitude and longitude — spelled out
// or as lat and lng/lon — whose halves may be numbers or numeric strings. A
// value that is none of these, or is off the globe, is not a position.
//
// Coordinates are taken as WGS-84, which is what Apple and the index use;
// nothing is converted.
func parsePosition(value any) (Position, bool) {
	var latitude, longitude any
	switch v := value.(type) {
	case string:
		halves := strings.Split(v, ",")
		if len(halves) != 2 {
			return Position{}, false
		}
		latitude, longitude = halves[0], halves[1]
	case map[string]any:
		latitude = firstKey(v, "latitude", "lat")
		longitude = firstKey(v, "longitude", "lng", "lon")
	default:
		return Position{}, false
	}
	lat, ok := number(latitude)
	if !ok || lat < -90 || lat > 90 {
		return Position{}, false
	}
	lng, ok := number(longitude)
	if !ok || lng < -180 || lng > 180 {
		return Position{}, false
	}
	return Position{Latitude: lat, Longitude: lng}, true
}

func firstKey(object map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value
		}
	}
	return nil
}

func number(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	}
	return 0, false
}

// positionField answers the three position fields from Position, so latitude
// and longitude can never come from a different point than the pair.
func (c Context) positionField(field FieldID) (any, bool) {
	position, ok := c.Position()
	if !ok {
		return nil, false
	}
	switch field {
	case FieldLatitude:
		return position.Latitude, true
	case FieldLongitude:
		return position.Longitude, true
	}
	return fmt.Sprintf("%v, %v", position.Latitude, position.Longitude), true
}

func isPositionField(field FieldID) bool {
	return field == FieldCoordinates || field == FieldLatitude || field == FieldLongitude
}
