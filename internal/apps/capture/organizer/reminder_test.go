package organizer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/desktop/reminders"
)

type fakeReminders struct {
	sent   []reminders.Request
	result reminders.Result
	err    error
}

func (f *fakeReminders) Create(_ context.Context, request reminders.Request) (reminders.Result, error) {
	f.sent = append(f.sent, request)
	return f.result, f.err
}

// withReminders stands a fake in for the helper, so no test writes into the
// reader's Reminders.
func withReminders(t *testing.T, fake *fakeReminders) {
	t.Helper()
	previous := reminderClient
	reminderClient = func() (reminderCreator, error) { return fake, nil }
	t.Cleanup(func() { reminderClient = previous })
}

func runAction(ctx Context, id ActionID) Result {
	def, _ := LookupAction(id)
	plan := ActionPlan{Action: id, Target: def.Target(ctx), Missing: nil}
	return Execute(ctx, []ActionPlan{plan})[0]
}

func TestPlaceReminder(t *testing.T) {
	fake := &fakeReminders{result: reminders.Result{ID: "r1"}}
	withReminders(t, fake)
	ctx := NewContext(beenHere(), map[FieldID]any{FieldTitle: "Buy tea", FieldContent: "the green one"})

	result := runAction(ctx, ActionReminderAtPlace)
	if result.Err != nil || !result.Executed || result.Skipped {
		t.Fatalf("result = %+v", result)
	}
	want := reminders.Request{
		Title: "Buy tea",
		Notes: "the green one",
		Mark:  "^dgs-20260909213122900-4620",
		Location: &reminders.Location{
			Title:     "Epping, NSW",
			Latitude:  -33.7691,
			Longitude: 151.082,
			Radius:    DefaultReminderRadius,
			Proximity: reminders.Arrive,
		},
	}
	if len(fake.sent) != 1 {
		t.Fatalf("sent %d requests", len(fake.sent))
	}
	got := fake.sent[0]
	if got.Title != want.Title || got.Notes != want.Notes || got.Mark != want.Mark || got.List != "" || got.Due != "" ||
		got.Location == nil || *got.Location != *want.Location {
		t.Errorf("sent %+v %+v, want %+v %+v", got, got.Location, want, want.Location)
	}
	if result.Target != "Buy tea · arrive -33.76910, 151.08200" {
		t.Errorf("target = %q", result.Target)
	}
}

// The payload's position is the one reminded at, and the index's place does
// not name it.
func TestPlaceReminderUsesTheSourcedPosition(t *testing.T) {
	fake := &fakeReminders{}
	withReminders(t, fake)
	ctx := NewContext(visited("31.2304, 121.4737"), map[FieldID]any{FieldTitle: "Revisit"}).
		WithSettings(sourcing("payload.place"))
	if result := runAction(ctx, ActionReminderAtPlace); result.Err != nil {
		t.Fatal(result.Err)
	}
	location := fake.sent[0].Location
	if location.Latitude != 31.2304 || location.Longitude != 121.4737 || location.Title != "" {
		t.Errorf("location = %+v", location)
	}
}

func TestPlaceReminderParameters(t *testing.T) {
	fake := &fakeReminders{}
	withReminders(t, fake)
	settings := DefaultSettings()
	settings.ReminderList, settings.ReminderRadius = "Errands", 300
	ctx := NewContext(beenHere(), map[FieldID]any{FieldTitle: "Leave"}).WithSettings(settings)

	if result := runAction(ctx, ActionReminderAtPlace); result.Err != nil {
		t.Fatal(result.Err)
	}
	if got := fake.sent[0]; got.List != "Errands" || got.Location.Radius != 300 {
		t.Errorf("configured: list %q, radius %v", got.List, got.Location.Radius)
	}

	ctx = ctx.WithParameters(map[ActionID]map[string]string{
		ActionReminderAtPlace: {ParameterProximity: "leave", ParameterRadius: "75", ParameterList: "Home"},
	})
	result := runAction(ctx, ActionReminderAtPlace)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if got := fake.sent[1]; got.List != "Home" || got.Location.Radius != 75 || got.Location.Proximity != reminders.Leave {
		t.Errorf("overridden: %+v %+v", got, got.Location)
	}
	if result.Target != "Home · Leave · leave -33.76910, 151.08200" {
		t.Errorf("target = %q", result.Target)
	}
}

func TestPlaceReminderRefusesBadParameters(t *testing.T) {
	fake := &fakeReminders{}
	withReminders(t, fake)
	for name, value := range map[string]string{ParameterProximity: "near", ParameterRadius: "far"} {
		ctx := NewContext(beenHere(), map[FieldID]any{FieldTitle: "x"}).
			WithParameters(map[ActionID]map[string]string{ActionReminderAtPlace: {name: value}})
		if result := runAction(ctx, ActionReminderAtPlace); result.Err == nil {
			t.Errorf("%s=%s: no error", name, value)
		}
	}
	if len(fake.sent) != 0 {
		t.Errorf("sent %d requests for bad parameters", len(fake.sent))
	}
}

func TestTimedReminder(t *testing.T) {
	fake := &fakeReminders{}
	withReminders(t, fake)
	tests := map[string]string{
		"2026-09-15T09:00:00+10:00": "2026-09-15T09:00:00+10:00",
		"2026-09-15 09:00":          time.Date(2026, 9, 15, 9, 0, 0, 0, time.Local).Format(time.RFC3339),
	}
	for due, want := range tests {
		ctx := NewContext(beenHere(), map[FieldID]any{FieldTitle: "Call", FieldDueAt: due})
		result := runAction(ctx, ActionReminderCreate)
		if result.Err != nil {
			t.Fatalf("%s: %v", due, result.Err)
		}
		got := fake.sent[len(fake.sent)-1]
		if got.Due != want || got.Location != nil {
			t.Errorf("%s: sent %+v", due, got)
		}
	}

	ctx := NewContext(beenHere(), map[FieldID]any{FieldTitle: "Call", FieldDueAt: "tomorrow"})
	if result := runAction(ctx, ActionReminderCreate); result.Err == nil || !strings.Contains(result.Err.Error(), "not a time") {
		t.Errorf("err = %v", result.Err)
	}
}

func TestReminderRequirements(t *testing.T) {
	def, _ := LookupAction(ActionReminderAtPlace)
	if got := fieldIDs(def.Required); !equalFields(got, []FieldID{FieldTitle, FieldCoordinates}) {
		t.Errorf("at_place requires %v", got)
	}
	capture := beenHere()
	capture.Index.Coordinates = nil
	ctx := NewContext(capture, map[FieldID]any{FieldTitle: "x"})
	if def.Target(ctx) != "" {
		t.Errorf("target = %q without a position", def.Target(ctx))
	}
}

func TestReminderSkippedAndFailed(t *testing.T) {
	fake := &fakeReminders{result: reminders.Result{Skipped: "already a reminder in Inbox"}}
	withReminders(t, fake)
	ctx := NewContext(beenHere(), map[FieldID]any{FieldTitle: "x"})
	if result := runAction(ctx, ActionReminderAtPlace); !result.Skipped || result.Reason != "already a reminder in Inbox" {
		t.Errorf("result = %+v", result)
	}

	fake.result, fake.err = reminders.Result{}, errors.New("reminders access was denied")
	if result := runAction(ctx, ActionReminderAtPlace); result.Executed || result.Err == nil {
		t.Errorf("result = %+v", result)
	}

	reminderClient = func() (reminderCreator, error) { return nil, reminders.ErrNoHelper }
	if result := runAction(ctx, ActionReminderAtPlace); !errors.Is(result.Err, reminders.ErrNoHelper) {
		t.Errorf("err = %v", result.Err)
	}
}
