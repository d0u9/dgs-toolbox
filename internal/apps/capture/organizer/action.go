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
	// Parameters are how the Action behaves rather than what it needs: a
	// default answers for almost every Capture, and the occasional one wants
	// something else. Declared here so a new Action arrives with its own,
	// without the screens that show them learning anything about it.
	Parameters []ParameterDefinition
	// Target resolves the concrete destination for one Capture, relative to
	// whatever root the Action writes under. It returns an empty string when
	// the inputs it needs are still missing, so an unresolved target stays
	// visible instead of being invented.
	Target func(Context) string
	// Run carries the Action out, reporting whether there was nothing left to
	// do. A nil Run is an Action that has been declared but not implemented: it
	// plans and explains itself, and refuses to run rather than silently doing
	// nothing.
	Run func(Context, ActionPlan) (skipped bool, err error)
}

// ParameterDefinition is one knob of an Action. Default reads the configured
// value, so the layers stay ordered: what the Action ships with, what the
// configuration says, and what this one Capture was given.
type ParameterDefinition struct {
	Name    string
	Label   string
	Input   InputType
	Default func(Settings) string
}

// ParameterSection is the heading a Capture is written under. It is the same
// name in the Action, the record, and a Recipe file.
const ParameterSection = "section"

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
			return path.Join(ctx.Settings.locations(), name+".md")
		},
	},
	ActionDailyAppend: {
		ID:    ActionDailyAppend,
		Label: "Daily note",
		Effects: []string{
			"Appends the Capture under the note's DGS heading as one nested entry, adding that heading when it has none",
			"Creates the note itself when the day has none",
			"Writes nothing the second time: an entry carries the Capture's id and is added once",
		},
		Required: []FieldRequirement{
			{Field: FieldCreatedAt, Label: "Created", Required: true, Input: InputText},
			multiline(FieldContent, "Note"),
		},
		Parameters: []ParameterDefinition{{
			Name:    ParameterSection,
			Label:   "Section",
			Input:   InputText,
			Default: func(settings Settings) string { return settings.section() },
		}},
		Target: func(ctx Context) string {
			name, err := dailyNoteName(ctx)
			if err != nil || name == "" {
				return ""
			}
			return path.Join(ctx.Settings.daily(), name)
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

// The implementations are wired here rather than in the literal above: an
// Action reads the registry to resolve its own parameters, and Go rejects a
// variable whose initializer refers to a function that refers back to it.
func init() {
	implementations := map[ActionID]func(Context, ActionPlan) (bool, error){
		ActionDailyAppend: appendToDailyNote,
	}
	for id, run := range implementations {
		def := actionDefinitions[id]
		def.Run = run
		actionDefinitions[id] = def
	}
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
