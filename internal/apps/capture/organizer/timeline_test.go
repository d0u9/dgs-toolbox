package organizer

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func timelineSettings(t *testing.T, vault string) Settings {
	t.Helper()
	settings := testSettings(t)
	settings.Mappings = starterTables(t)
	settings.ObsidianVault = vault
	writeTimelines(t, &settings, testTimelines)
	return settings
}

func timelinePlan(t *testing.T, ctx Context) ActionPlan {
	t.Helper()
	recipe, ok := starterSet(t).Lookup("obsidian_timeline")
	if !ok {
		t.Fatal("no timeline recipe")
	}
	plans := Build(ctx, recipe, recipe.Actions)
	if len(plans) != 1 {
		t.Fatalf("got %d plans, want the timeline append", len(plans))
	}
	return plans[0]
}

// With a position, the entry carries the place the way the list of places
// does; the compiled-in template writes every map service.
func TestTimelineEntryCarriesThePlaceWhenThereIsOne(t *testing.T) {
	vault := t.TempDir()
	ctx := NewContext(beenHere(), map[FieldID]any{FieldContent: "Emma 去买了叶酸。"}).
		WithSettings(timelineSettings(t, vault))
	if results := Execute(ctx, []ActionPlan{timelinePlan(t, ctx)}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	note, err := os.ReadFile(filepath.Join(vault, "03 Family/00 Timeline/Timeline.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(note), "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("want a marker, an entry and three place lines:\n%s", note)
	}
	if lines[0] != "- *2026-09-09 周三*" ||
		lines[1] != "- `21:31:22 +10` · Emma 去买了叶酸。 ^dgs-20260909213122900-4620" {
		t.Fatalf("note =\n%s", note)
	}
	if lines[3] != "    - (-33.76910, 151.08200)" || !strings.Contains(lines[4], "maps.apple.com") {
		t.Fatalf("the place lines are missing:\n%s", note)
	}
}

// Without a position, the place lines are left out rather than written empty.
func TestTimelineEntryWithoutAPlaceIsOneLine(t *testing.T) {
	vault := t.TempDir()
	capture := beenHere()
	capture.Index.Source.Workflow = "quick_marker"
	capture.Index.Coordinates = nil
	capture.Index.Place = nil
	ctx := NewContext(capture, map[FieldID]any{FieldContent: "Emma 说她怀孕了。"}).
		WithSettings(timelineSettings(t, vault))
	if results := Execute(ctx, []ActionPlan{timelinePlan(t, ctx)}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	note, _ := os.ReadFile(filepath.Join(vault, "03 Family/00 Timeline/Timeline.md"))
	want := "- *2026-09-09 周三*\n- `21:31:22 +10` · Emma 说她怀孕了。 ^dgs-20260909213122900-4620\n"
	if string(note) != want {
		t.Fatalf("note =\n%q\nwant\n%q", note, want)
	}
}

// What happened is the entry, so a Capture nobody wrote about is not ready.
func TestTimelineNeedsContent(t *testing.T) {
	ctx := NewContext(beenHere(), nil).WithSettings(timelineSettings(t, t.TempDir()))
	plan := timelinePlan(t, ctx)
	if plan.Ready() {
		t.Fatal("a Capture without content is ready for the timeline")
	}
}

// A rolled-over year goes into the timeline's own archive, introduced as a
// timeline rather than as a list of places.
func TestTimelineBackfillGoesToItsYear(t *testing.T) {
	vault := t.TempDir()
	old := beenHere()
	old.Index.CreatedAt = "2024-03-03T09:00:00+10:00"
	ctx := NewContext(old, map[FieldID]any{FieldContent: "补录"}).WithSettings(timelineSettings(t, vault))
	if results := Execute(ctx, []ActionPlan{timelinePlan(t, ctx)}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	archived, err := os.ReadFile(filepath.Join(vault, "03 Family/00 Timeline/2024.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  - timeline\n", "2024 年的时间线", "从 Timeline.md 归档过来的", "补录"} {
		if !strings.Contains(string(archived), want) {
			t.Fatalf("archive lacks %q:\n%s", want, archived)
		}
	}
}

// Without its own template, the timeline is written the way the reader writes
// places; with one, its own wins.
func TestTimelineBorrowsTheLocationTemplate(t *testing.T) {
	vault := t.TempDir()
	settings := timelineSettings(t, vault)
	writeLocationTemplate(t, settings.TemplateDir, "- located {{.Content}} {{.ID}}")
	ctx := NewContext(beenHere(), map[FieldID]any{FieldContent: "借用"}).WithSettings(settings)
	Execute(ctx, []ActionPlan{timelinePlan(t, ctx)})
	note, _ := os.ReadFile(filepath.Join(vault, "03 Family/00 Timeline/Timeline.md"))
	if !strings.Contains(string(note), "- located 借用 ") {
		t.Fatalf("the location template was not used:\n%s", note)
	}

	if err := os.WriteFile(filepath.Join(settings.TemplateDir, TimelineEntryTemplate), []byte("- own {{.Content}} {{.ID}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := beenHere()
	other.Index.ID = "other"
	ctx = NewContext(other, map[FieldID]any{FieldContent: "自己的"}).WithSettings(settings)
	Execute(ctx, []ActionPlan{timelinePlan(t, ctx)})
	note, _ = os.ReadFile(filepath.Join(vault, "03 Family/00 Timeline/Timeline.md"))
	if !strings.Contains(string(note), "- own 自己的 ") {
		t.Fatalf("the timeline's own template was not used:\n%s", note)
	}
}

// Any name the timelines file may hold is an Action, looked up rather than
// registered; a name it may not hold is not.
func TestTimelineActionsAreLookedUp(t *testing.T) {
	for _, name := range []string{"family", "locations", "trip-2026"} {
		def, ok := LookupAction(TimelineAction(name))
		if !ok || def.Run == nil || def.Target == nil {
			t.Errorf("%s: ok = %v, definition = %+v", name, ok, def)
		}
	}
	for _, id := range []ActionID{TimelineAction("Family"), TimelineAction(""), TimelineAction("a b")} {
		if _, ok := LookupAction(id); ok {
			t.Errorf("%s was looked up", id)
		}
	}
	if slices.ContainsFunc(Actions(), func(def ActionDefinition) bool { return strings.HasPrefix(string(def.ID), TimelineActionPrefix) }) {
		t.Error("a timeline Action is listed among the registered ones")
	}
}
