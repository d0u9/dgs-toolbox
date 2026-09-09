package organizer

// builtinRecipes are compiled in rather than configured: user defined Recipes
// come later, and until then these are what Route offers. been_here is the
// workflow the model was built against; photo_note and quick_mark are here to
// exercise it against Recipes whose Actions require different fields.
var builtinRecipes = withBuiltinSource([]Recipe{
	{
		ID:    "obsidian_location",
		Name:  "Location",
		Match: Match{Workflows: []string{"been_here"}, RequiresAny: []FieldID{FieldLatitude, FieldCity}},
		Actions: []ActionID{
			ActionLocationUpsert,
			ActionCaptureArchive,
		},
	},
	{
		ID:    "obsidian_location_daily",
		Name:  "Location + Daily",
		Match: Match{Workflows: []string{"been_here"}, RequiresAny: []FieldID{FieldLatitude, FieldCity}},
		Fields: []FieldRequirement{
			optional(FieldRequirement{Field: FieldTags, Label: "Tags", Input: InputMultiSelect}),
		},
		Actions: []ActionID{
			ActionLocationUpsert,
			ActionDailyAppend,
			ActionCaptureArchive,
		},
	},
	{
		ID:      "obsidian_daily",
		Name:    "Daily",
		Match:   Match{Workflows: []string{"been_here", "photo_note", "quick_mark"}},
		Actions: []ActionID{ActionDailyAppend, ActionCaptureArchive},
	},
	{
		ID:      "photo_location_daily",
		Name:    "Photo + Location",
		Match:   Match{Workflows: []string{"photo_note"}, RequiresAny: []FieldID{FieldLatitude, FieldCity}},
		Actions: []ActionID{ActionLocationUpsert, ActionDailyAppend, ActionCaptureArchive},
	},
	{
		ID:      "apple_note",
		Name:    "Apple Note",
		Match:   Match{Workflows: []string{"photo_note", "quick_mark"}},
		Actions: []ActionID{ActionAppleNoteCreate, ActionCaptureArchive},
	},
	{
		ID:      "apple_reminder",
		Name:    "Reminder",
		Match:   Match{Workflows: []string{"quick_mark"}},
		Actions: []ActionID{ActionReminderCreate, ActionCaptureArchive},
	},
	{
		ID:    "apple_calendar",
		Name:  "Calendar",
		Match: Match{Workflows: []string{"quick_mark"}},
		Fields: []FieldRequirement{
			optional(FieldRequirement{Field: FieldAllDay, Label: "All day", Input: InputText}),
		},
		Actions: []ActionID{ActionCalendarCreate, ActionCaptureArchive},
	},
	{
		ID:      "archive_only",
		Name:    "Archive",
		Match:   Match{},
		Actions: []ActionID{ActionCaptureArchive},
	},
})

// withBuiltinSource labels the compiled-in Recipes so a reader can tell them
// apart from the ones a file defined.
func withBuiltinSource(recipes []Recipe) []Recipe {
	for index := range recipes {
		recipes[index].Source = BuiltinSource
	}
	return recipes
}

// Recipes returns every known Recipe, in a stable order.
func Recipes() []Recipe { return Builtin().All() }

// LookupRecipe returns a Recipe by id.
func LookupRecipe(id RecipeID) (Recipe, bool) {
	for _, recipe := range builtinRecipes {
		if recipe.ID == id {
			return recipe, true
		}
	}
	return Recipe{}, false
}
