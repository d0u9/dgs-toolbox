package organizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func locationSettings(t *testing.T, vault string) Settings {
	t.Helper()
	settings := testSettings(t)
	// The day's name comes from the weekday table this version ships, the way
	// it does for a reader who has run --init.
	settings.Mappings = starterTables(t)
	settings.ObsidianVault = vault
	settings.LocationNote = "88 Inbox/06 Locations.md"
	settings.LocationArchive = "88 Inbox/06 Locations"
	// Which services an entry carries and which vault command a coordinate
	// links to are the template's decisions, so the test's template makes them.
	writeLocationTemplate(t, settings.TemplateDir, `- `+"`"+`{{.Clock}} {{.Offset}}`+"`"+` · {{.Content}} {{.ID}}
    - {{.Address}}
    - {{.CopyLink "Copy Coordinates (lng, lat)"}}
    - {{.MapLinks "Apple, 高德, Google"}}`)
	return settings
}

func writeLocationTemplate(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, LocationEntryTemplate), []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func locationPlan(t *testing.T, ctx Context) ActionPlan {
	t.Helper()
	recipe, ok := starterSet(t).Lookup("obsidian_location")
	if !ok {
		t.Fatal("no location recipe")
	}
	plans := Build(ctx, recipe, recipe.Actions)
	if len(plans) != 1 {
		t.Fatalf("got %d plans, want the location append", len(plans))
	}
	return plans[0]
}

func locationContext(t *testing.T, vault, content string) Context {
	t.Helper()
	capture := beenHere()
	capture.Index.Place.Country = "Australia"
	capture.Index.Place.City = "Sydney"
	capture.Index.Place.Locality = "Epping"
	return NewContext(capture, map[FieldID]any{FieldContent: content}).
		WithSettings(locationSettings(t, vault))
}

// The entry is the shape the vault already uses: a date tick, the time with its
// offset, and the place on three lines under it.
func TestLocationEntryFollowsTheVaultsShape(t *testing.T) {
	vault := t.TempDir()
	ctx := locationContext(t, vault, "在家调试")
	if results := Execute(ctx, []ActionPlan{locationPlan(t, ctx)}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}

	note, err := os.ReadFile(filepath.Join(vault, "88 Inbox/06 Locations.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimRight(string(note), "\n"), "\n")
	want := []string{
		"- *2026-09-09 周三*",
		"- `21:31:22 +10` · 在家调试 ^dgs-20260909213122900-4620",
		"    - Australia, NSW, Sydney, Epping",
		"    - [(-33.76910, 151.08200)](obsidian://quickadd?choice=Copy%20Coordinates%20(lng%2C%20lat)&value-coordinates=-33.76910%2C%20151.08200)",
		"    - [Apple](https://maps.apple.com/place?coordinate=-33.76910,151.08200&name=Epping) · " +
			"[高德](https://uri.amap.com/marker?position=151.08200,-33.76910&coordinate=wgs84&name=Epping) · " +
			"[Google](https://www.google.com/maps/search/?api=1&query=-33.76910,151.08200)",
	}
	for index, line := range want {
		if index >= len(got) || got[index] != line {
			t.Fatalf("line %d =\n%q\nwant\n%q", index, strings.Join(got, "\n"), line)
		}
	}
	// The marker and its entries are one list: a blank line between them would
	// break the list, and the timeline's rule with it.
	if strings.Contains(string(note), "周三*\n\n") {
		t.Fatalf("a blank line separates the marker from its entries:\n%s", note)
	}
}

// The newest entry goes on top, under the day's marker, and what is already
// there is kept — including the callout that introduces the note.
func TestLocationEntryGoesOnTopBelowThePreamble(t *testing.T) {
	vault := t.TempDir()
	path := filepath.Join(vault, "88 Inbox/06 Locations.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "---\ncssclasses:\n  - locations\n---\n\n> [!note]- 位置速记\n> 说明。\n\n" +
		"- *2026-09-09 周三*\n- `09:00:00 +10` · 更早的一条 ^dgs-other\n" +
		"- *2026-09-08 周二*\n- `08:00:00 +10` · 前一天 ^dgs-older\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := locationContext(t, vault, "新的一条")
	Execute(ctx, []ActionPlan{locationPlan(t, ctx)})
	note, _ := os.ReadFile(path)
	got := string(note)

	for _, want := range []string{"cssclasses", "> [!note]- 位置速记", "更早的一条", "前一天"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the note lost %q:\n%s", want, got)
		}
	}
	lines := strings.Split(got, "\n")
	marker, fresh, older := -1, -1, -1
	for index, line := range lines {
		switch {
		case line == "- *2026-09-09 周三*":
			if marker < 0 {
				marker = index
			}
		case strings.Contains(line, "新的一条"):
			fresh = index
		case strings.Contains(line, "更早的一条"):
			older = index
		}
	}
	if !(marker < fresh && fresh < older) {
		t.Fatalf("the new entry is not at the top of its day:\n%s", got)
	}
	if strings.Count(got, "- *2026-09-09 周三*") != 1 {
		t.Fatalf("the day's marker was written twice:\n%s", got)
	}
}

// The note is the record of what it holds, so a Capture already in it is not
// written again.
func TestLocationEntryIsWrittenOnce(t *testing.T) {
	vault := t.TempDir()
	ctx := locationContext(t, vault, "只写一次")
	plan := locationPlan(t, ctx)

	Execute(ctx, []ActionPlan{plan})
	before, _ := os.ReadFile(filepath.Join(vault, "88 Inbox/06 Locations.md"))
	results := Execute(ctx, []ActionPlan{plan})
	after, _ := os.ReadFile(filepath.Join(vault, "88 Inbox/06 Locations.md"))

	if string(before) != string(after) {
		t.Fatalf("the second run changed the note:\n%s", after)
	}
	if !results[0].Skipped || !results[0].Executed {
		t.Fatalf("a second run should succeed as a skip: %+v", results[0])
	}
}

// A year that has rolled over moves into the archive on the way past, so the
// running list stays the size of one year however long it has been kept.
func TestLocationArchivesPastYears(t *testing.T) {
	vault := t.TempDir()
	path := filepath.Join(vault, "88 Inbox/06 Locations.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "---\ncssclasses:\n  - locations\n---\n\n> [!note]- 位置速记\n\n" +
		"- *2026-01-02 周五*\n- `10:00:00 +10` · 今年更早 ^dgs-a\n" +
		"- *2025-12-31 周三*\n- `23:00:00 +10` · 去年 ^dgs-b\n" +
		"- *2024-05-05 周日*\n- `12:00:00 +10` · 前年 ^dgs-c\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := locationContext(t, vault, "今天")
	Execute(ctx, []ActionPlan{locationPlan(t, ctx)})

	note, _ := os.ReadFile(path)
	if strings.Contains(string(note), "去年") || strings.Contains(string(note), "前年") {
		t.Fatalf("past years were left in the running list:\n%s", note)
	}
	for _, want := range []string{"今天", "今年更早", "cssclasses", "位置速记"} {
		if !strings.Contains(string(note), want) {
			t.Fatalf("the running list lost %q:\n%s", want, note)
		}
	}

	// One file per year, each with its own heading.
	for year, want := range map[string]string{"2025": "去年", "2024": "前年"} {
		archived, err := os.ReadFile(filepath.Join(vault, "88 Inbox/06 Locations", year+".md"))
		if err != nil {
			t.Fatalf("no archive for %s: %v", year, err)
		}
		if !strings.Contains(string(archived), want) {
			t.Fatalf("the %s archive is missing %q:\n%s", year, want, archived)
		}
		if !strings.Contains(string(archived), year+" 年的位置速记") {
			t.Fatalf("the %s archive has no heading:\n%s", year, archived)
		}
		// It names where the entries came from, as the vault's own script does.
		if !strings.Contains(string(archived), "从 06 Locations.md 归档过来的") {
			t.Fatalf("the %s archive does not name its source:\n%s", year, archived)
		}
	}
}

// Nothing has rolled over, so not a single extra write happens.
func TestLocationDoesNotTouchTheArchiveWithinOneYear(t *testing.T) {
	vault := t.TempDir()
	ctx := locationContext(t, vault, "第一条")
	Execute(ctx, []ActionPlan{locationPlan(t, ctx)})

	second := beenHere()
	second.Index.ID = "second"
	next := NewContext(second, map[FieldID]any{FieldContent: "第二条"}).WithSettings(ctx.Settings)
	Execute(next, []ActionPlan{locationPlan(t, next)})

	if _, err := os.Stat(filepath.Join(vault, "88 Inbox/06 Locations")); !os.IsNotExist(err) {
		t.Fatal("an archive was written for a year that has not rolled over")
	}
}

// What is not configured is an error rather than a guess.
func TestLocationRefusesWithoutANote(t *testing.T) {
	settings := locationSettings(t, t.TempDir())
	settings.LocationNote = ""
	ctx := NewContext(beenHere(), map[FieldID]any{FieldContent: "x"}).WithSettings(settings)
	results := Execute(ctx, []ActionPlan{{Action: ActionLocationAppend}})
	if !errorsIs(results[0].Err, ErrNoLocationNote) {
		t.Fatalf("err = %v, want ErrNoLocationNote", results[0].Err)
	}
}

// Organizing happens long after capturing, so a Capture from a year that has
// already rolled over is the ordinary case rather than an edge: it belongs in
// that year's archive, and what counts as this year is the clock's answer
// rather than the Capture's.
func TestLocationBackfillGoesStraightToItsYear(t *testing.T) {
	vault := t.TempDir()
	path := filepath.Join(vault, "88 Inbox/06 Locations.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	running := "- *" + time.Now().Format("2006-01-02") + " 周一*\n- `10:00:00 +10` · 今年的 ^dgs-now\n"
	if err := os.WriteFile(path, []byte(running), 0o644); err != nil {
		t.Fatal(err)
	}

	old := beenHere()
	old.Index.ID = "backfill"
	old.Index.CreatedAt = "2024-03-03T09:00:00+10:00"
	ctx := NewContext(old, map[FieldID]any{FieldContent: "补录"}).WithSettings(locationSettings(t, vault))
	if results := Execute(ctx, []ActionPlan{locationPlan(t, ctx)}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}

	// It went to its own year, and this year's list was not touched.
	archived, err := os.ReadFile(filepath.Join(vault, "88 Inbox/06 Locations", "2024.md"))
	if err != nil {
		t.Fatalf("the backfilled entry did not reach its archive: %v", err)
	}
	if !strings.Contains(string(archived), "补录") || !strings.Contains(string(archived), "2024 年的位置速记") {
		t.Fatalf("archive = %s", archived)
	}
	note, _ := os.ReadFile(path)
	if string(note) != running {
		t.Fatalf("this year's list was rewritten:\n%s", note)
	}

	// Running it again writes nothing: the archive is the record of what it
	// holds, as the running list is.
	results := Execute(ctx, []ActionPlan{locationPlan(t, ctx)})
	if !results[0].Skipped {
		t.Fatalf("the backfilled entry was written twice: %+v", results[0])
	}
}

// Without an archive there is nowhere else to put it, so everything stays in
// the one list.
func TestLocationKeepsEverythingWhenNothingIsArchived(t *testing.T) {
	vault := t.TempDir()
	settings := locationSettings(t, vault)
	settings.LocationArchive = ""
	old := beenHere()
	old.Index.CreatedAt = "2024-03-03T09:00:00+10:00"
	ctx := NewContext(old, map[FieldID]any{FieldContent: "补录"}).WithSettings(settings)

	Execute(ctx, []ActionPlan{locationPlan(t, ctx)})
	note, err := os.ReadFile(filepath.Join(vault, "88 Inbox/06 Locations.md"))
	if err != nil || !strings.Contains(string(note), "补录") {
		t.Fatalf("note = %s, err = %v", note, err)
	}
}
