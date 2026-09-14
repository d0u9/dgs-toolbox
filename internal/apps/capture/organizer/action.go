package organizer

import (
	"fmt"
	"slices"
	"strings"
)

// ActionID identifies one concrete execution unit. A Recipe expands into
// Actions; a Recipe is never itself an Action.
type ActionID string

const (
	ActionLocationAppend  ActionID = "obsidian.location.append"
	ActionDailyAppend     ActionID = "obsidian.daily.append"
	ActionCaptureArchive  ActionID = "capture.archive"
	ActionAppleNoteCreate ActionID = "apple.notes.create"
	ActionReminderCreate  ActionID = "apple.reminders.create"
	ActionReminderAtPlace ActionID = "apple.reminders.at_place"
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
	// Run carries the Action out. It returns a reason when there was nothing
	// left to do, and the empty string when it wrote: an Action that writes
	// nothing owes the reader why, and "skipped" on its own is not an answer
	// anyone can act on months later. A nil Run is an Action that has been
	// declared but not implemented: it plans and explains itself, and refuses
	// to run rather than silently doing nothing.
	Run func(Context, ActionPlan) (skipped string, err error)
	// UsesPosition says the Action writes the Capture's position somewhere,
	// whether or not it names coordinates as a requirement: the location note
	// draws it from a template. A caller offers a map for the position when an
	// enabled Action uses it, since that is when a wrong one does harm.
	UsesPosition bool
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
	ActionLocationAppend: {
		ID:           ActionLocationAppend,
		Label:        "Location note",
		UsesPosition: true,
		Effects: []string{
			"Adds the Capture to the top of the running list of places, under its day",
			"Moves any year that has rolled over into the archive as it goes",
			"Writes nothing the second time: an entry carries the Capture's id and is added once",
		},
		// The note is optional: a place is worth recording whether or not
		// anything was written about it.
		Required: []FieldRequirement{
			{Field: FieldCreatedAt, Label: "Created", Required: true, Input: InputText},
			optional(multiline(FieldContent, "Note")),
		},
		Target: func(ctx Context) string { return ctx.Settings.LocationNote },
	},
	ActionDailyAppend: {
		ID:    ActionDailyAppend,
		Label: "Daily note",
		Effects: []string{
			"Appends the Capture under the configured section as one nested entry, adding that heading when the note has none",
			"Creates the note from the configured template when the day has none",
			"Writes nothing the second time: an entry carries the Capture's id and is added once",
		},
		Required: []FieldRequirement{
			{Field: FieldCreatedAt, Label: "Created", Required: true, Input: InputText},
			optional(multiline(FieldContent, "Note")),
		},
		Parameters: []ParameterDefinition{{
			Name:    ParameterSection,
			Label:   "Section",
			Input:   InputText,
			Default: func(settings Settings) string { return settings.section() },
		}},
		Target: func(ctx Context) string {
			target, err := dailyNotePath(ctx)
			if err != nil {
				return ""
			}
			return target
		},
	},
	// The Apple note and calendar Actions exist so the model can be exercised
	// against more than one workflow. Each declares its requirements and names
	// its target; neither talks to an Apple API yet. The reminder Actions below
	// do, through EventKit.
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
		ID:    ActionReminderCreate,
		Label: "Reminder",
		Effects: []string{
			"Creates a reminder in Apple Reminders, due at the given time, with the Capture's text as its notes",
			"Writes nothing the second time: the reminder's notes carry the Capture's id, and a list already holding it is left alone",
			"Never edits or deletes a reminder; deleting it is how it is created again",
		},
		Required: []FieldRequirement{
			text(FieldTitle, "Title"),
			{Field: FieldDueAt, Label: "Due", Required: true, Input: InputDateTime},
		},
		Parameters: []ParameterDefinition{reminderListParameter()},
	},
	// A reminder at a place is its own Action rather than a switch on the timed
	// one: what it needs is different — a position, not a time — and a Recipe
	// names an Action without setting its parameters, so a switch would have to
	// be flipped by hand every time.
	ActionReminderAtPlace: {
		ID:           ActionReminderAtPlace,
		Label:        "Reminder at place",
		UsesPosition: true,
		Effects: []string{
			"Creates a reminder in Apple Reminders that fires on arriving at, or leaving, the Capture's position",
			"Writes nothing the second time: the reminder's notes carry the Capture's id, and a list already holding it is left alone",
			"Fires on a device that has location access for Reminders, usually the phone, once iCloud has synced it",
		},
		Required: []FieldRequirement{
			text(FieldTitle, "Title"),
			{Field: FieldCoordinates, Label: "Position", Required: true, Input: InputText},
		},
		Parameters: []ParameterDefinition{
			{
				Name:    ParameterProximity,
				Label:   "When (arrive/leave)",
				Input:   InputText,
				Default: func(Settings) string { return "arrive" },
			},
			{
				Name:    ParameterRadius,
				Label:   "Radius (m)",
				Input:   InputText,
				Default: func(settings Settings) string { return settings.reminderRadius() },
			},
			reminderListParameter(),
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
	implementations := map[ActionID]func(Context, ActionPlan) (string, error){
		ActionDailyAppend:     appendToDailyNote,
		ActionLocationAppend:  appendToLocationNote,
		ActionReminderCreate:  createReminder,
		ActionReminderAtPlace: createPlaceReminder,
	}
	for id, run := range implementations {
		def := actionDefinitions[id]
		def.Run = run
		actionDefinitions[id] = def
	}
	// The reminders name their list in the target, which is a parameter, so
	// their targets read the registry too.
	targets := map[ActionID]func(Context) string{
		ActionReminderCreate: func(ctx Context) string {
			return reminderTarget(ctx, ActionReminderCreate, ctx.String(FieldDueAt))
		},
		ActionReminderAtPlace: func(ctx Context) string {
			position, ok := ctx.Position()
			if !ok {
				return ""
			}
			when := fmt.Sprintf("%s %.5f, %.5f", ctx.Parameter(ActionReminderAtPlace, ParameterProximity), position.Latitude, position.Longitude)
			return reminderTarget(ctx, ActionReminderAtPlace, when)
		},
	}
	for id, target := range targets {
		def := actionDefinitions[id]
		def.Target = target
		actionDefinitions[id] = def
	}
}

// ActionOrder is the order Actions are listed in: what the toolbox does first,
// the Obsidian ones and then the reminders, and the Apple ones that do nothing
// yet last, rather than alphabetical, which would file them among each other.
var ActionOrder = []ActionID{
	ActionDailyAppend, ActionLocationAppend,
	ActionReminderCreate, ActionReminderAtPlace,
	ActionAppleNoteCreate, ActionCalendarCreate,
}

// reminderListParameter is the list a reminder is written into. Empty is the
// list Reminders files new reminders in.
func reminderListParameter() ParameterDefinition {
	return ParameterDefinition{
		Name:    ParameterList,
		Label:   "List",
		Input:   InputText,
		Default: func(settings Settings) string { return settings.ReminderList },
	}
}

// Actions is every Action this binary carries, in ActionOrder. A caller
// listing them reads this rather than the map, so what is documented and what
// is offered cannot fall out of step.
func Actions() []ActionDefinition {
	definitions := make([]ActionDefinition, 0, len(actionDefinitions))
	for _, id := range ActionOrder {
		if def, ok := actionDefinitions[id]; ok {
			definitions = append(definitions, def)
		}
	}
	// An Action added to the registry and not to the order is still listed:
	// leaving it out would make this the hand-kept list it exists to replace.
	for id, def := range actionDefinitions {
		if !slices.Contains(ActionOrder, id) {
			definitions = append(definitions, def)
		}
	}
	return definitions
}

// UsesPosition reports whether any of the enabled Actions of a Recipe writes
// the Capture's position.
func UsesPosition(recipe Recipe, enabled []ActionID) bool {
	for _, id := range recipe.Actions {
		if def, ok := actionDefinitions[id]; ok && def.UsesPosition && slices.Contains(enabled, id) {
			return true
		}
	}
	return false
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
