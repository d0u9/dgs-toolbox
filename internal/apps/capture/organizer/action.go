package organizer

import (
	"fmt"
	"path"
	"strings"
)

// ActionID identifies one concrete execution unit. A Recipe expands into
// Actions; a Recipe is never itself an Action.
type ActionID string

const (
	ActionLocationUpsert  ActionID = "obsidian.location.upsert"
	ActionDailyAppend     ActionID = "obsidian.daily.append"
	ActionCaptureArchive  ActionID = "capture.archive"
	ActionAppleNoteCreate ActionID = "apple.notes.create"
	ActionReminderCreate  ActionID = "apple.reminders.create"
	ActionCalendarCreate  ActionID = "apple.calendar.create"
)

// ActionDefinition says what an Action is: its label, what it requires, and how
// it names the thing it will write. It says nothing about any one Capture.
type ActionDefinition struct {
	ID    ActionID
	Label string
	// Effects say what this Action does outside the Capture, in the terms a
	// reader needs before running it: what it creates, what it changes, and
	// what it does not undo. Declared here rather than written into a screen,
	// so the TUI and the generated reference say the same thing.
	Effects  []string
	Required []FieldRequirement
	// Target resolves the concrete destination for one Capture. It returns an
	// empty string when the inputs it needs are still missing, so an
	// unresolved target stays visible instead of being invented.
	Target func(Context) string
}

// ActionPlan is what one enabled Action will do to one Capture. Definition is
// the what; Plan is the where and the with-what.
type ActionPlan struct {
	Action  ActionID
	Label   string
	Target  string
	Content string
	Missing []FieldRequirement
}

// Ready reports whether every requirement of this Action is met.
func (p ActionPlan) Ready() bool { return len(p.Missing) == 0 }

var actionDefinitions = map[ActionID]ActionDefinition{
	ActionLocationUpsert: {
		ID:      ActionLocationUpsert,
		Label:   "Location note",
		Effects: []string{"Creates Locations/<place name>.md, or updates it in place when it exists"},
		Required: []FieldRequirement{
			text(FieldPlaceName, "Place name"),
			{Field: FieldLatitude, Label: "Latitude", Required: true, Input: InputText},
			{Field: FieldLongitude, Label: "Longitude", Required: true, Input: InputText},
		},
		Target: func(ctx Context) string {
			name := ctx.String(FieldPlaceName)
			if name == "" {
				return ""
			}
			return path.Join("Locations", name+".md")
		},
	},
	ActionDailyAppend: {
		ID:      ActionDailyAppend,
		Label:   "Daily note",
		Effects: []string{"Appends one entry to the note for the day the Capture was taken", "Creates that note when the day has none"},
		Required: []FieldRequirement{
			{Field: FieldCreatedAt, Label: "Created", Required: true, Input: InputText},
			multiline(FieldContent, "Note"),
		},
		Target: func(ctx Context) string {
			day := dayOf(ctx.String(FieldCreatedAt))
			if day == "" {
				return ""
			}
			return path.Join("Daily", day+".md")
		},
	},
	// The Apple Actions exist so the model can be exercised against more than
	// one workflow. Each declares its requirements and names its target; none
	// of them talks to an Apple API yet, and that detail must not shape the
	// model when it arrives.
	ActionAppleNoteCreate: {
		ID:      ActionAppleNoteCreate,
		Label:   "Apple note",
		Effects: []string{"Creates a new note in Apple Notes", "Never edits an existing note, so running twice makes two"},
		Required: []FieldRequirement{
			text(FieldTitle, "Title"),
			multiline(FieldContent, "Body"),
		},
		Target: func(ctx Context) string { return ctx.String(FieldTitle) },
	},
	ActionReminderCreate: {
		ID:      ActionReminderCreate,
		Label:   "Reminder",
		Effects: []string{"Creates a reminder due at the given time", "Never edits an existing reminder, so running twice makes two"},
		Required: []FieldRequirement{
			text(FieldTitle, "Title"),
			{Field: FieldDueAt, Label: "Due", Required: true, Input: InputDateTime},
		},
		Target: func(ctx Context) string {
			title, due := ctx.String(FieldTitle), ctx.String(FieldDueAt)
			if title == "" || due == "" {
				return ""
			}
			return title + " · " + due
		},
	},
	ActionCalendarCreate: {
		ID:      ActionCalendarCreate,
		Label:   "Calendar event",
		Effects: []string{"Creates a calendar event", "Never edits an existing event, so running twice makes two"},
		Required: []FieldRequirement{
			text(FieldTitle, "Title"),
			// An all-day event needs no start time, so the requirement is
			// conditional rather than always asked for.
			{
				Field:    FieldStartAt,
				Label:    "Start",
				Required: true,
				Input:    InputDateTime,
				When:     func(ctx Context) bool { return ctx.String(FieldAllDay) != "true" },
			},
		},
		Target: func(ctx Context) string { return ctx.String(FieldTitle) },
	},
}

// LookupAction returns the definition of an Action.
func LookupAction(id ActionID) (ActionDefinition, bool) {
	def, ok := actionDefinitions[id]
	return def, ok
}

// dayOf takes the date half of an RFC 3339 timestamp without converting it to
// local time, matching how Capture renders timestamps elsewhere.
func dayOf(createdAt string) string {
	if len(createdAt) < 10 {
		return ""
	}
	day := createdAt[:10]
	if strings.Count(day, "-") != 2 {
		return ""
	}
	return day
}

func (p ActionPlan) String() string {
	return fmt.Sprintf("%s -> %s", p.Action, p.Target)
}
