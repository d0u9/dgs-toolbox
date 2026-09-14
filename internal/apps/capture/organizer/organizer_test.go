package organizer

import (
	"testing"

	"dgs-toolbox/internal/apps/capture/indexschema"
)

// beenHere is the worked example from docs/apps/capture/organizer.md: a
// Capture that knows where it is but not what the place is called.
func beenHere() Capture {
	return Capture{
		Path: "/captures/20260909213122900-4620",
		Name: "20260909213122900-4620",
		Index: indexschema.Index{
			Schema:    "v1",
			ID:        "20260909213122900-4620",
			CreatedAt: "2026-09-09T21:31:22.900+10:00",
			Source:    indexschema.Source{App: "Shortcut", Workflow: "been_here"},
			Coordinates: &indexschema.Coordinates{
				Latitude:  -33.7691,
				Longitude: 151.082,
			},
			Place: &indexschema.Place{City: "Epping", Region: "NSW"},
		},
	}
}

// placeless is a been_here Capture that knows where it is only by number, so
// nothing can be composed into a place name.
func placeless() Capture {
	capture := beenHere()
	capture.Index.Place = nil
	return capture
}

func fieldIDs(reqs []FieldRequirement) []FieldID {
	ids := make([]FieldID, 0, len(reqs))
	for _, req := range reqs {
		ids = append(ids, req.Field)
	}
	return ids
}

func equalFields(got []FieldID, want []FieldID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func locationDaily(t *testing.T) Recipe {
	t.Helper()
	recipe, ok := starterSet(t).Lookup("obsidian_location_daily")
	if !ok {
		t.Fatal("recipe obsidian_location_daily not found")
	}
	return recipe
}

// The shipped Recipes narrow to the right candidates, in the order their files
// sort in — a directory has no order of its own, and filename order is the one
// a reader can see and change.
func TestFindNarrowsCandidates(t *testing.T) {
	tests := []struct {
		name    string
		capture Capture
		want    []RecipeID
	}{
		{
			name:    "been_here offers the location recipes",
			capture: beenHere(),
			want:    []RecipeID{"apple_reminder_place", "obsidian_daily", "obsidian_location", "obsidian_location_daily"},
		},
		{
			name: "quick_mark offers a different set entirely",
			capture: Capture{Index: indexschema.Index{
				Source: indexschema.Source{Workflow: "quick_mark"},
			}},
			want: []RecipeID{"apple_calendar", "apple_note", "apple_reminder", "obsidian_daily"},
		},
		{
			name: "a location recipe needs a location, so been_here without one drops them",
			capture: Capture{Index: indexschema.Index{
				Source: indexschema.Source{Workflow: "been_here"},
			}},
			want: []RecipeID{"obsidian_daily"},
		},
		{
			name: "an unknown workflow is offered nothing",
			capture: Capture{Index: indexschema.Index{
				Source: indexschema.Source{Workflow: "voice_memo"},
			}},
			want: nil,
		},
	}

	set := starterSet(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := set.Find(tc.capture, starterSettings(t))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d candidates, want %d", len(got), len(tc.want))
			}
			for i, recipe := range got {
				if recipe.ID != tc.want[i] {
					t.Errorf("candidate %d = %q, want %q", i, recipe.ID, tc.want[i])
				}
			}
		})
	}
}

func TestContextReadsEnrichmentBeforeCapture(t *testing.T) {
	capture := beenHere()
	ctx := NewContext(capture, map[FieldID]any{
		FieldPlaceName: "Epping Station",
		FieldCity:      "Sydney",
	})

	if got := ctx.String(FieldPlaceName); got != "Epping Station" {
		t.Errorf("place.name = %q, want the enriched value", got)
	}
	if got := ctx.String(FieldCity); got != "Sydney" {
		t.Errorf("place.city = %q, want the enriched value to win", got)
	}
	if got := ctx.String(FieldRegion); got != "NSW" {
		t.Errorf("place.region = %q, want the capture value", got)
	}
	if capture.Index.CapturePlace().City != "Epping" {
		t.Error("enrichment mutated the capture")
	}
}

// The workflows this toolbox ships shortcuts for are described in files like
// any other, so their readings are exercised through those files.
func TestContentIsSourcedPerWorkflow(t *testing.T) {
	tests := []struct {
		workflow string
		payload  map[string]any
		want     string
	}{
		{workflow: "been_here", payload: map[string]any{"note": "note text"}, want: "note text"},
		{workflow: "photo_note", payload: map[string]any{"text": "photo text"}, want: "photo text"},
		{workflow: "quick_mark", payload: map[string]any{"mark": "mark text"}, want: "mark text"},
		{workflow: "been_here", payload: map[string]any{"text": "wrong key"}, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.workflow, func(t *testing.T) {
			capture := Capture{Index: indexschema.Index{
				Source:  indexschema.Source{Workflow: tc.workflow},
				Payload: tc.payload,
			}}
			ctx := NewContext(capture, nil).WithSettings(starterSettings(t))
			if got := ctx.String(FieldContent); got != tc.want {
				t.Errorf("content = %q, want %q", got, tc.want)
			}
		})
	}
}

// undated is a been_here Capture that does not say when it was taken, which
// both Obsidian Actions need and nothing else can answer. The note cannot stand
// in for this: an entry is written whether or not anything was typed.
func undated(capture Capture) Capture {
	capture.Index.CreatedAt = ""
	return capture
}

func TestMissingFieldsFollowsTheEnabledSet(t *testing.T) {
	recipe := locationDaily(t)
	capture := undated(beenHere())

	tests := []struct {
		name       string
		enabled    []ActionID
		enrichment map[FieldID]any
		want       []FieldID
	}{
		{
			name:    "all actions enabled asks for the time the capture does not carry",
			enabled: []ActionID{ActionLocationAppend, ActionDailyAppend},
			// Both Actions want it; it is reported once per Action that does.
			want: []FieldID{FieldCreatedAt, FieldCreatedAt},
		},
		{
			name:    "the location note wants the time too, so disabling the daily one keeps it",
			enabled: []ActionID{ActionLocationAppend},
			want:    []FieldID{FieldCreatedAt},
		},
		{
			name:    "the note is optional, so a capture with no text is not held up by it",
			enabled: []ActionID{ActionLocationAppend, ActionDailyAppend},
			// Supplying the time leaves nothing: the missing note does not block.
			enrichment: map[FieldID]any{FieldCreatedAt: "2026-09-09T21:31:22+10:00"},
			want:       nil,
		},

		{
			name:    "no action enabled requires nothing",
			enabled: nil,
			want:    nil,
		},
		{
			name:       "enrichment satisfies the time",
			enabled:    []ActionID{ActionLocationAppend, ActionDailyAppend},
			enrichment: map[FieldID]any{FieldPlaceName: "Epping Station", FieldCreatedAt: "2026-09-09T21:31:22+10:00", FieldContent: "晚上再来看看"},
			want:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := NewContext(capture, tc.enrichment)
			got := fieldIDs(MissingFields(ctx, recipe, tc.enabled))
			if !equalFields(got, tc.want) {
				t.Errorf("missing = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAPlaceNameIsComposedFromTheStructuredParts(t *testing.T) {
	tests := []struct {
		name  string
		place *indexschema.Place
		want  string
	}{
		{name: "city and region", place: &indexschema.Place{City: "Epping", Region: "NSW"}, want: "Epping, NSW"},
		{name: "the two most specific parts win", place: &indexschema.Place{Locality: "Chuo", City: "Osaka", Region: "Osaka Prefecture", Country: "Japan"}, want: "Chuo, Osaka"},
		{name: "country alone", place: &indexschema.Place{Country: "Australia"}, want: "Australia"},
		{name: "no place composes nothing", place: nil, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := beenHere()
			capture.Index.Place = tc.place
			ctx := NewContext(capture, nil)
			if got := ctx.String(FieldPlaceName); got != tc.want {
				t.Errorf("place.name = %q, want %q", got, tc.want)
			}
			// A composed name is a starting point, not the Capture's own data,
			// so the user may replace it.
			if ctx.FromCapture(FieldPlaceName) {
				t.Error("a composed place name must not read as capture data")
			}
		})
	}

	ctx := NewContext(beenHere(), map[FieldID]any{FieldPlaceName: "Epping Station"})
	if got := ctx.String(FieldPlaceName); got != "Epping Station" {
		t.Errorf("place.name = %q, want the user's name to win over the composed one", got)
	}
}

func TestMissingFieldsAreAttributedToTheirAction(t *testing.T) {
	recipe := locationDaily(t)
	ctx := NewContext(undated(placeless()), nil)
	missing := MissingFields(ctx, recipe, recipe.Actions)

	// The time is wanted by both, so it is reported once for each of them and
	// each report names the Action that asked.
	seen := map[ActionID]bool{}
	for _, req := range missing {
		if req.Field != FieldCreatedAt {
			t.Errorf("%s is missing, want only the time", req.Field)
		}
		seen[req.Action] = true
	}
	for _, action := range []ActionID{ActionLocationAppend, ActionDailyAppend} {
		if !seen[action] {
			t.Errorf("nothing attributed to %q", action)
		}
	}
}

func TestRecipeFieldsStayRequiredWhateverIsEnabled(t *testing.T) {
	// The Recipe's own field belongs to no Action, so it is required whether or
	// not anything is enabled.
	recipe := Recipe{
		ID:     "with_project",
		Fields: []FieldRequirement{text("project", "Project")},
	}
	ctx := NewContext(beenHere(), nil)

	for _, enabled := range [][]ActionID{{ActionDailyAppend}, nil} {
		got := fieldIDs(MissingFields(ctx, recipe, enabled))
		if !equalFields(got, []FieldID{"project"}) {
			t.Errorf("enabled %v: missing = %v, want [project]", enabled, got)
		}
	}
}

func TestOptionalAndConditionalRequirementsDoNotBlock(t *testing.T) {
	never := Recipe{
		ID: "conditional",
		Fields: []FieldRequirement{
			optional(text(FieldTags, "Tags")),
			{
				Field:    "end_at",
				Label:    "End",
				Required: true,
				Input:    InputDateTime,
				When:     func(ctx Context) bool { return ctx.String("has_end") == "true" },
			},
		},
	}
	ctx := NewContext(beenHere(), nil)

	if got := MissingFields(ctx, never, never.Actions); len(got) != 0 {
		t.Errorf("missing = %v, want none", fieldIDs(got))
	}

	withEnd := NewContext(beenHere(), map[FieldID]any{"has_end": "true"})
	if got := fieldIDs(MissingFields(withEnd, never, never.Actions)); !equalFields(got, []FieldID{"end_at"}) {
		t.Errorf("missing = %v, want [end_at]", got)
	}
}

func TestBuildProducesThePlanFromTheWorkedExample(t *testing.T) {
	recipe := locationDaily(t)
	ctx := NewContext(beenHere(), map[FieldID]any{
		FieldPlaceName: "Epping Station",
		FieldContent:   "晚上这里的人流和光线可以再看看",
	})

	plans := Build(ctx, recipe, recipe.Actions)
	want := []struct {
		action ActionID
		target string
	}{
		// Neither target resolves until the vault says where its note lives.
		{ActionLocationAppend, ""},
		{ActionDailyAppend, ""},
	}

	if len(plans) != len(want) {
		t.Fatalf("got %d plans, want %d", len(plans), len(want))
	}
	for i, plan := range plans {
		if plan.Action != want[i].action || plan.Target != want[i].target {
			t.Errorf("plan %d = %s -> %q, want %s -> %q", i, plan.Action, plan.Target, want[i].action, want[i].target)
		}
		if !plan.Ready() {
			t.Errorf("plan %d is blocked by %v", i, fieldIDs(plan.Missing))
		}
	}
	if !Ready(ctx, recipe, recipe.Actions) {
		t.Error("capture should be ready")
	}
}

func TestBuildKeepsBlockedActionsVisible(t *testing.T) {
	recipe := locationDaily(t)
	ctx := NewContext(undated(placeless()), nil)

	plans := Build(ctx, recipe, recipe.Actions)
	if len(plans) != 2 {
		t.Fatalf("got %d plans, want 2", len(plans))
	}
	if plans[0].Ready() || plans[0].Target != "" {
		t.Errorf("location plan should be blocked with no target, got %q", plans[0].Target)
	}
	if plans[1].Target != "" {
		t.Errorf("daily target = %q, want none until a path is configured", plans[1].Target)
	}
	if plans[1].Ready() {
		t.Error("daily plan should be blocked by the missing time")
	}

}

func TestBuildOmitsDisabledActions(t *testing.T) {
	recipe := locationDaily(t)
	selection := NewSelection(recipe)
	selection.Toggle(ActionDailyAppend)
	ctx := NewContext(beenHere(), selection.Enrichment)

	enabled := selection.EnabledActions(recipe)
	plans := Build(ctx, recipe, enabled)
	for _, plan := range plans {
		if plan.Action == ActionDailyAppend {
			t.Error("disabled action must not enter the plan")
		}
	}
	if len(plans) != 1 {
		t.Errorf("got %d plans, want 1", len(plans))
	}
}

func TestAnEmptyPlanIsBlockedNotReady(t *testing.T) {
	recipe := locationDaily(t)
	ctx := NewContext(beenHere(), map[FieldID]any{
		FieldPlaceName: "Epping Station",
		FieldContent:   "note",
	})
	if Ready(ctx, recipe, nil) {
		t.Error("a capture with no enabled action must not count as ready")
	}
}

func TestSelectionDefaultsToEveryAction(t *testing.T) {
	recipe := locationDaily(t)
	selection := NewSelection(recipe)

	got := selection.EnabledActions(recipe)
	if len(got) != len(recipe.Actions) {
		t.Fatalf("got %d enabled, want %d", len(got), len(recipe.Actions))
	}
	for i, id := range got {
		if id != recipe.Actions[i] {
			t.Errorf("enabled %d = %q, want %q in recipe order", i, id, recipe.Actions[i])
		}
	}

	selection.Toggle(ActionLocationAppend)
	selection.Toggle(ActionLocationAppend)
	if len(selection.EnabledActions(recipe)) != len(recipe.Actions) {
		t.Error("toggling twice should restore the default set")
	}
}

func TestSelectionSetClearsEmptyValues(t *testing.T) {
	selection := NewSelection(locationDaily(t))
	selection.Set(FieldPlaceName, "Epping Station")
	selection.Set(FieldContent, "")

	if _, ok := selection.Enrichment[FieldContent]; ok {
		t.Error("an empty value should be cleared rather than stored")
	}
	if selection.Enrichment[FieldPlaceName] != "Epping Station" {
		t.Error("place name was not recorded")
	}
}
