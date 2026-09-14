package organizer

import (
	"strings"
	"testing"
)

// visited is a been_here Capture whose payload names the place it is about,
// somewhere other than where the phone was.
func visited(payload any) Capture {
	capture := beenHere()
	capture.Index.Payload = map[string]any{"place": payload}
	capture.Index.Coordinates.Altitude = 88
	return capture
}

func sourcing(sources ...string) Settings {
	settings := DefaultSettings()
	settings.Sources = map[string]map[FieldID][]string{"been_here": {FieldCoordinates: sources}}
	return settings
}

func TestPositionFromTheIndex(t *testing.T) {
	ctx := NewContext(beenHere(), nil)
	position, ok := ctx.Position()
	if !ok || position != (Position{Latitude: -33.7691, Longitude: 151.082}) {
		t.Fatalf("position = %+v, %v", position, ok)
	}
	if !ctx.FromCapture(FieldCoordinates) {
		t.Error("the index's own position should not be the reader's to edit")
	}
}

func TestPositionSpellings(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		want    Position
		ok      bool
	}{
		{"string", "31.2304, 121.4737", Position{31.2304, 121.4737, true}, true},
		{"object", map[string]any{"latitude": 31.2304, "longitude": 121.4737}, Position{31.2304, 121.4737, true}, true},
		{"short keys", map[string]any{"lat": 31.2304, "lng": 121.4737}, Position{31.2304, 121.4737, true}, true},
		{"lon", map[string]any{"lat": 31.2304, "lon": 121.4737}, Position{31.2304, 121.4737, true}, true},
		{"numeric strings", map[string]any{"lat": " 31.2304", "lng": "121.4737 "}, Position{31.2304, 121.4737, true}, true},
		{"one half", map[string]any{"lat": 31.2304}, Position{Sourced: true}, false},
		{"off the globe", "91, 10", Position{Sourced: true}, false},
		{"not a number", "north, east", Position{Sourced: true}, false},
		{"three parts", "1, 2, 3", Position{Sourced: true}, false},
		{"a number", 31.2304, Position{Sourced: true}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := NewContext(visited(tc.payload), nil).WithSettings(sourcing("payload.place"))
			position, ok := ctx.Position()
			if ok != tc.ok || position != tc.want {
				t.Errorf("position = %+v, %v; want %+v, %v", position, ok, tc.want, tc.ok)
			}
		})
	}
}

// A workflow that says where it keeps its position has said the index is not
// it, so a Capture missing that key is asked about rather than filed under
// where the phone was.
func TestSourcedPositionDoesNotFallBackToTheIndex(t *testing.T) {
	capture := beenHere()
	capture.Index.Payload = map[string]any{}
	ctx := NewContext(capture, nil).WithSettings(sourcing("payload.place"))
	if value, ok := ctx.Get(FieldCoordinates); ok {
		t.Fatalf("coordinates = %v, want missing", value)
	}
	if got := coordinate(ctx, FieldLatitude); got != "" {
		t.Errorf("latitude = %q, want nothing", got)
	}
}

func TestSourcedPositionTriesEachSource(t *testing.T) {
	capture := visited("31.2304, 121.4737")
	ctx := NewContext(capture, nil).WithSettings(sourcing("payload.missing", "payload.place"))
	if got := ctx.String(FieldCoordinates); got != "31.2304, 121.4737" {
		t.Errorf("coordinates = %q", got)
	}
}

func TestTemplateSourcedPosition(t *testing.T) {
	capture := beenHere()
	capture.Index.Payload = map[string]any{"y": 31.2304, "x": 121.4737}
	ctx := NewContext(capture, nil).WithSettings(sourcing("{{.y}}, {{.x}}"))
	if got := ctx.String(FieldCoordinates); got != "31.2304, 121.4737" {
		t.Errorf("coordinates = %q", got)
	}
}

// Latitude, longitude and the pair are one point, whichever of them is asked.
func TestHalvesFollowThePair(t *testing.T) {
	ctx := NewContext(visited(map[string]any{"lat": 31.2304, "lng": 121.4737}), nil).
		WithSettings(sourcing("payload.place"))
	if got := coordinate(ctx, FieldLatitude); got != "31.23040" {
		t.Errorf("latitude = %q", got)
	}
	if got := coordinate(ctx, FieldLongitude); got != "121.47370" {
		t.Errorf("longitude = %q", got)
	}
	if value, _ := ctx.Get(FieldLongitude); value != 121.4737 {
		t.Errorf("longitude field = %v", value)
	}
	if ctx.FromCapture(FieldCoordinates) {
		t.Error("a sourced position is not the index's own")
	}
}

// Once the position is the payload's, the index's altitude and place describe
// somewhere else and are left out.
func TestSourcedPositionDropsTheIndexPlace(t *testing.T) {
	ctx := NewContext(visited("31.2304, 121.4737"), nil).WithSettings(sourcing("payload.place"))
	if got := altitude(ctx); got != "" {
		t.Errorf("altitude = %q", got)
	}
	if got := address(ctx); got != "" {
		t.Errorf("address = %q", got)
	}
	for _, field := range []FieldID{FieldCity, FieldRegion, FieldPlaceName} {
		if value, ok := ctx.Get(field); ok {
			t.Errorf("%s = %v, want nothing", field, value)
		}
	}
	if got := mapLinksLine(ctx, "31.23040", "121.47370", "Apple"); strings.Contains(got, "Epping") {
		t.Errorf("map links = %q, named after the index's place", got)
	}

	// The index's own position keeps all of it.
	own := NewContext(visited("31.2304, 121.4737"), nil)
	if altitude(own) != "88m" || address(own) != "NSW, Epping" || own.String(FieldCity) != "Epping" {
		t.Errorf("index place = %q, %q, %q", altitude(own), address(own), own.String(FieldCity))
	}
}

// What was typed in FIELDS wins, and is the reader's, not the index's.
func TestTypedPosition(t *testing.T) {
	capture := beenHere()
	capture.Index.Coordinates = nil
	ctx := NewContext(capture, map[FieldID]any{FieldCoordinates: "35.6812, 139.7671"})
	position, ok := ctx.Position()
	if !ok || position != (Position{35.6812, 139.7671, true}) {
		t.Fatalf("position = %+v, %v", position, ok)
	}
	if ctx.FromCapture(FieldCoordinates) {
		t.Error("a typed position is the reader's to edit")
	}
}

func TestWorkflowRefusesHalfAPosition(t *testing.T) {
	for _, field := range []FieldID{FieldLatitude, FieldLongitude} {
		path := writeWorkflow(t, t.TempDir(), "been_here.yaml", "fields:\n  "+string(field)+": payload.lat\n")
		_, _, err := LoadWorkflowFile(path)
		if err == nil || !strings.Contains(err.Error(), "source \"coordinates\" instead") {
			t.Errorf("%s: err = %v", field, err)
		}
	}
	path := writeWorkflow(t, t.TempDir(), "been_here.yaml", "fields:\n  coordinates: payload.place\n")
	if _, fields, err := LoadWorkflowFile(path); err != nil || fields[FieldCoordinates][0] != "payload.place" {
		t.Errorf("fields = %v, err = %v", fields, err)
	}
}
