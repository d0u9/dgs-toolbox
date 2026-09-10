package organizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testEntryTemplate is what these tests write with. They use their own rather
// than the shipped default, so retuning the default entry format — a matter of
// taste, and the reason it is a template at all — does not fail tests about
// where and when an entry is written.
const testEntryTemplate = `- {{.When}} {{.ID}}
{{- range .ContentLines}}
  - {{.}}
{{- end}}
  - {{.Latitude}}, {{.Longitude}}
  - address: {{.Address}}`

func testSettings(t *testing.T) Settings {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, DailyEntryTemplate), []byte(testEntryTemplate+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.TemplateDir = dir
	return settings
}

// testVaultSettings points at a vault with a daily note template in it. The
// paths are configured rather than discovered, so a test says where things go
// the same way a reader does.
func testVaultSettings(t *testing.T, vault string) Settings {
	t.Helper()
	settings := testSettings(t)
	settings.ObsidianVault = vault
	settings.DailyNote = "00 Daily Log/{{.Date}}.md"
	writeNoteTemplate(t, settings.TemplateDir, "---\nDate: {{.Date}}\n---\n")
	return settings
}

func writeNoteTemplate(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, DailyNoteTemplate), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func vaultContext(t *testing.T, capture Capture, content string) (Context, string) {
	t.Helper()
	vault := t.TempDir()
	ctx := NewContext(capture, map[FieldID]any{FieldContent: content}).WithSettings(testVaultSettings(t, vault))
	return ctx, vault
}

func dailyPlan(t *testing.T, ctx Context) ActionPlan {
	t.Helper()
	recipe, ok := LookupRecipe("obsidian_daily")
	if !ok {
		t.Fatal("no daily recipe")
	}
	plans := Build(ctx, recipe, recipe.Actions)
	if len(plans) != 1 {
		t.Fatalf("got %d plans, want the daily append", len(plans))
	}
	return plans[0]
}

func TestDailyAppendCreatesTheNoteAndItsSection(t *testing.T) {
	ctx, vault := vaultContext(t, beenHere(), "Coffee under the bridge")
	plan := dailyPlan(t, ctx)
	if plan.Target != "00 Daily Log/2026-09-09.md" {
		t.Fatalf("target = %q", plan.Target)
	}

	results := Execute(ctx, []ActionPlan{plan})
	if !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	note, err := os.ReadFile(filepath.Join(vault, plan.Target))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nDate: 2026-09-09\n---\n\n# Captured\n\n" +
		"- 2026-09-09 21:31:22 +10 ^dgs-20260909213122900-4620\n" +
		"  - Coffee under the bridge\n" +
		"  - -33.76910, 151.08200\n" +
		"  - address: Epping, NSW\n"
	if string(note) != want {
		t.Fatalf("note = %q, want %q", note, want)
	}
}

// The note is the record of what was written, so running twice adds nothing.
func TestDailyAppendWritesAnEntryOnce(t *testing.T) {
	ctx, vault := vaultContext(t, beenHere(), "Coffee under the bridge")
	plan := dailyPlan(t, ctx)

	Execute(ctx, []ActionPlan{plan})
	before, _ := os.ReadFile(filepath.Join(vault, plan.Target))
	results := Execute(ctx, []ActionPlan{plan})
	after, _ := os.ReadFile(filepath.Join(vault, plan.Target))

	if string(before) != string(after) {
		t.Fatalf("the second run changed the note:\n%s", after)
	}
	if !results[0].Skipped || !results[0].Executed {
		t.Fatalf("a second run should succeed as a skip: %+v", results[0])
	}

	// Deleting the entry by hand is enough to have it written again.
	os.WriteFile(filepath.Join(vault, plan.Target), []byte("# Captured\n"), 0o644)
	Execute(ctx, []ActionPlan{plan})
	restored, _ := os.ReadFile(filepath.Join(vault, plan.Target))
	if !strings.Contains(string(restored), "Coffee under the bridge") {
		t.Fatalf("the entry was not written again:\n%s", restored)
	}
}

// An existing note keeps everything it holds, and the entry lands under its own
// heading rather than at the end of the file.
func TestDailyAppendKeepsTheNoteAndWritesUnderItsSection(t *testing.T) {
	ctx, vault := vaultContext(t, beenHere(), "the new entry")
	plan := dailyPlan(t, ctx)
	path := filepath.Join(vault, plan.Target)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "---\nDate: 2026-09-09\n---\n\n# 今日活动\n\n1. 已有的内容\n\n# Captured\n\n- an earlier entry ^dgs-other\n\n# 想说的\n\n无\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if results := Execute(ctx, []ActionPlan{plan}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	note, _ := os.ReadFile(path)
	got := string(note)

	for _, want := range []string{"Date: 2026-09-09", "1. 已有的内容", "- an earlier entry ^dgs-other", "# 想说的"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the note lost %q:\n%s", want, got)
		}
	}
	lines := strings.Split(got, "\n")
	section, entry, next := -1, -1, -1
	for index, line := range lines {
		switch {
		case line == "# Captured":
			section = index
		case strings.Contains(line, "the new entry"):
			entry = index
		case line == "# 想说的":
			next = index
		}
	}
	if !(section < entry && entry < next) {
		t.Fatalf("the entry is not inside the DGS section:\n%s", got)
	}
	if strings.TrimSpace(lines[entry-1]) == "" {
		t.Fatalf("the entry should follow the section's last line, not a gap:\n%s", got)
	}
}

// A note with no DGS heading gains one at the end rather than having its
// entries mixed into somebody else's section.
func TestDailyAppendAddsTheSectionWhenTheNoteHasNone(t *testing.T) {
	ctx, vault := vaultContext(t, beenHere(), "the new entry")
	plan := dailyPlan(t, ctx)
	path := filepath.Join(vault, plan.Target)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("# 今日活动\n\n1. 已有的内容\n"), 0o644)

	Execute(ctx, []ActionPlan{plan})
	note, _ := os.ReadFile(path)
	want := "# 今日活动\n\n1. 已有的内容\n\n# Captured\n\n" +
		"- 2026-09-09 21:31:22 +10 ^dgs-20260909213122900-4620\n" +
		"  - the new entry\n" +
		"  - -33.76910, 151.08200\n" +
		"  - address: Epping, NSW\n"
	if string(note) != want {
		t.Fatalf("note = %q, want %q", note, want)
	}
}

// Without a vault the Action refuses rather than guessing a directory.
func TestDailyAppendRefusesWithoutAVault(t *testing.T) {
	settings := testVaultSettings(t, t.TempDir())
	settings.ObsidianVault = ""
	ctx := NewContext(beenHere(), map[FieldID]any{FieldContent: "x"}).WithSettings(settings)
	plan := dailyPlan(t, ctx)
	results := Execute(ctx, []ActionPlan{plan})
	if !errorsIs(results[0].Err, ErrNoVault) {
		t.Fatalf("err = %v, want ErrNoVault", results[0].Err)
	}
}

// An Action that has only been declared refuses rather than reporting success.
func TestExecuteRefusesAnUnimplementedAction(t *testing.T) {
	ctx, _ := vaultContext(t, beenHere(), "x")
	results := Execute(ctx, []ActionPlan{{Action: ActionAppleNoteCreate}})
	if !errorsIs(results[0].Err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", results[0].Err)
	}
	if Executed(results) {
		t.Fatal("an unimplemented action must not count as executed")
	}
}

func errorsIs(err, target error) bool {
	return err != nil && strings.Contains(err.Error(), target.Error())
}

// One block id is often a prefix of another — an edited "^dgs-x" becomes
// "^dgs-x-2", which contains the first — so the mark is matched as a whole word
// and an edited entry does not pass for the one it was made from.
func TestDailyAppendMatchesTheMarkAsAWholeWord(t *testing.T) {
	ctx, vault := vaultContext(t, beenHere(), "Coffee under the bridge")
	plan := dailyPlan(t, ctx)
	path := filepath.Join(vault, plan.Target)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	mark := captureMark(ctx.Capture)
	for _, edited := range []string{mark + "-2", "x" + mark, mark + "x"} {
		if err := os.WriteFile(path, []byte("# Captured\n\n- an edited entry "+edited+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		results := Execute(ctx, []ActionPlan{plan})
		if results[0].Skipped {
			t.Fatalf("%q was taken for %q", edited, mark)
		}
		note, _ := os.ReadFile(path)
		if !strings.Contains(string(note), "Coffee under the bridge") {
			t.Fatalf("the entry was not written beside %q:\n%s", edited, note)
		}
	}

	// The mark itself, on a line of its own or beside text, still counts.
	os.WriteFile(path, []byte("# Captured\n\n- something "+mark+"\n"), 0o644)
	if results := Execute(ctx, []ActionPlan{plan}); !results[0].Skipped {
		t.Fatal("the entry was written twice")
	}
}

// The section is configured, including its level: a value carrying its own
// hashes asks for that heading, and the entry goes under it rather than under a
// first-level heading invented here.
func TestDailyAppendWritesUnderTheConfiguredSection(t *testing.T) {
	tests := []struct {
		section string
		heading string
	}{
		{section: "", heading: "# Captured"},
		{section: "Captures", heading: "# Captures"},
		{section: "## Captured", heading: "## Captured"},
	}

	for _, tc := range tests {
		t.Run(tc.section, func(t *testing.T) {
			ctx, vault := vaultContext(t, beenHere(), "note text")
			settings := ctx.Settings
			settings.DailySection = tc.section
			ctx = ctx.WithSettings(settings)
			plan := dailyPlan(t, ctx)

			if results := Execute(ctx, []ActionPlan{plan}); !Executed(results) {
				t.Fatalf("results = %+v", results)
			}
			note, _ := os.ReadFile(filepath.Join(vault, plan.Target))
			if !strings.Contains(string(note), "\n"+tc.heading+"\n") {
				t.Fatalf("note = %q, want the heading %q", note, tc.heading)
			}
		})
	}
}

// A section ends at the next heading of its own level or shallower; a deeper
// one is part of what the section holds, so an entry still goes after it.
func TestDailyAppendWritesAfterASubheadingInsideItsSection(t *testing.T) {
	ctx, vault := vaultContext(t, beenHere(), "the new entry")
	plan := dailyPlan(t, ctx)
	path := filepath.Join(vault, plan.Target)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "# Captured\n\n## Morning\n\n- an earlier entry ^dgs-other\n\n# 想说的\n\n无\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	Execute(ctx, []ActionPlan{plan})
	note, _ := os.ReadFile(path)
	lines := strings.Split(string(note), "\n")
	entry, next := -1, -1
	for index, line := range lines {
		switch {
		case strings.Contains(line, "the new entry"):
			entry = index
		case line == "# 想说的":
			next = index
		}
	}
	if entry < 0 || next < 0 || entry > next {
		t.Fatalf("the entry did not land inside the section:\n%s", note)
	}
}

func firstNoteLine(note string) string {
	if index := strings.IndexByte(note, '\n'); index >= 0 {
		return note[:index]
	}
	return note
}

// Nothing is discovered: what is not configured is an error rather than a
// guess, because guessing wrongly writes a Capture into a file nobody was
// looking at.
func TestDailyAppendRefusesWithoutAConfiguredPathOrTemplate(t *testing.T) {
	vault := t.TempDir()
	base := testVaultSettings(t, vault)

	// No path: the Action cannot even say where it would write.
	settings := base
	settings.DailyNote = ""
	ctx := NewContext(beenHere(), map[FieldID]any{FieldContent: "x"}).WithSettings(settings)
	recipe, _ := LookupRecipe("obsidian_daily")
	plans := Build(ctx, recipe, recipe.Actions)
	if plans[0].Target != "" {
		t.Fatalf("target = %q, want none without a configured path", plans[0].Target)
	}
	if results := Execute(ctx, plans); !errorsIs(results[0].Err, ErrNoDailyNote) {
		t.Fatalf("err = %v, want ErrNoDailyNote", results[0].Err)
	}

	// No template: appending to a note that exists is fine, but the day
	// without one cannot be created.
	settings = base
	settings.TemplateDir = t.TempDir()
	ctx = NewContext(beenHere(), map[FieldID]any{FieldContent: "x"}).WithSettings(settings)
	plans = Build(ctx, recipe, recipe.Actions)
	if results := Execute(ctx, plans); !errorsIs(results[0].Err, ErrNoDailyTemplate) {
		t.Fatalf("err = %v, want ErrNoDailyTemplate", results[0].Err)
	}
	existing := filepath.Join(vault, plans[0].Target)
	os.MkdirAll(filepath.Dir(existing), 0o755)
	os.WriteFile(existing, []byte("# Captured\n"), 0o644)
	if results := Execute(ctx, plans); !Executed(results) {
		t.Fatalf("appending to a note that exists needs no template: %+v", results)
	}

	// A template directory without one names the file it expected.
	settings = base
	settings.TemplateDir = t.TempDir()
	ctx = NewContext(beenHere(), map[FieldID]any{FieldContent: "x"}).WithSettings(settings)
	os.RemoveAll(filepath.Join(vault, "Daily"))
	os.RemoveAll(filepath.Join(vault, "00 Daily Log"))
	results := Execute(ctx, Build(ctx, recipe, recipe.Actions))
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), DailyNoteTemplate) {
		t.Fatalf("err = %v, want it to name the template", results[0].Err)
	}
}

// The note's path is a template over the date, so a vault that files notes by
// year is configured rather than special-cased.
func TestDailyNotePathIsATemplateOverTheDate(t *testing.T) {
	vault := t.TempDir()
	settings := testVaultSettings(t, vault)
	settings.DailyNote = "00 Daily Log/{{.Year}}/{{.Date}}.md"
	ctx := NewContext(beenHere(), map[FieldID]any{FieldContent: "x"}).WithSettings(settings)

	recipe, _ := LookupRecipe("obsidian_daily")
	plans := Build(ctx, recipe, recipe.Actions)
	if plans[0].Target != "00 Daily Log/2026/2026-09-09.md" {
		t.Fatalf("target = %q", plans[0].Target)
	}
	if results := Execute(ctx, plans); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	if _, err := os.Stat(filepath.Join(vault, plans[0].Target)); err != nil {
		t.Fatalf("the note was not written where the path says: %v", err)
	}
}

// A created note comes from the reader's own template, filled with what the day
// and the Capture answer: the date and where it was taken need no prompting,
// and what genuinely cannot be answered is left for them to fill in.
func TestCreatedNoteComesFromTheConfiguredTemplate(t *testing.T) {
	vault := t.TempDir()
	settings := testVaultSettings(t, vault)
	writeNoteTemplate(t, settings.TemplateDir,
		"---\nDate: {{.Date}}\nDayOfTheYear: {{.DayOfYear}}\nCountry: {{.Country}}\nRegion: {{.Region}}\nWeather:\n---\n\n# 今日活动\n")
	capture := beenHere()
	capture.Index.Place.Country = "Australia"
	ctx := NewContext(capture, map[FieldID]any{FieldContent: "x"}).WithSettings(settings)

	recipe, _ := LookupRecipe("obsidian_daily")
	plans := Build(ctx, recipe, recipe.Actions)
	Execute(ctx, plans)

	note, err := os.ReadFile(filepath.Join(vault, plans[0].Target))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Date: 2026-09-09", "DayOfTheYear: 252", "Country: Australia", "Region: NSW", "Weather:\n", "# 今日活动"} {
		if !strings.Contains(string(note), want) {
			t.Errorf("the created note is missing %q:\n%s", want, note)
		}
	}
}

// The heading is a template over the Capture, so a note read months later says
// where the entries under it came from.
func TestDailySectionNamesWhereTheEntriesCameFrom(t *testing.T) {
	named := beenHere()
	named.Index.Source.Device.Name = "Phone"
	ctx, vault := vaultContext(t, named, "note text")
	plan := dailyPlan(t, ctx)
	if results := Execute(ctx, []ActionPlan{plan}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	note, _ := os.ReadFile(filepath.Join(vault, plan.Target))
	if !strings.Contains(string(note), "# Captured - Phone\n") {
		t.Fatalf("note = %q, want the device in the heading", note)
	}

	// A Capture from another device writes under its own heading.
	other := beenHere()
	other.Index.ID = "second"
	other.Index.Source.Device.Name = "Camera phone"
	next := NewContext(other, map[FieldID]any{FieldContent: "another"}).WithSettings(ctx.Settings)
	Execute(next, []ActionPlan{dailyPlan(t, next)})
	note, _ = os.ReadFile(filepath.Join(vault, plan.Target))
	for _, want := range []string{"# Captured - Phone\n", "# Captured - Camera phone\n"} {
		if !strings.Contains(string(note), want) {
			t.Fatalf("note is missing %q:\n%s", want, note)
		}
	}

	// A Capture that records no device leaves the heading on its own rather
	// than trailing a separator with nothing after it.
	nameless := beenHere()
	nameless.Index.ID = "third"
	nameless.Index.Source.Device.Name = ""
	bare := NewContext(nameless, map[FieldID]any{FieldContent: "no device"}).WithSettings(ctx.Settings)
	Execute(bare, []ActionPlan{dailyPlan(t, bare)})
	note, _ = os.ReadFile(filepath.Join(vault, plan.Target))
	if !strings.Contains(string(note), "\n# Captured\n") {
		t.Fatalf("a capture with no device did not fall back to the bare heading:\n%s", note)
	}

	// A heading with no placeholders is written as it stands.
	settings := ctx.Settings
	settings.DailySection = "DGS"
	plain := NewContext(beenHere(), map[FieldID]any{FieldContent: "x"}).WithSettings(settings)
	plain.Capture.Index.ID = "fourth"
	Execute(plain, []ActionPlan{dailyPlan(t, plain)})
	note, _ = os.ReadFile(filepath.Join(vault, plan.Target))
	if !strings.Contains(string(note), "# DGS\n") {
		t.Fatalf("a literal heading was not written as it stands:\n%s", note)
	}
}
