package organizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/capture/indexschema"
)

// These tests describe how a template is rendered, so they supply their own:
// the shipped default is a matter of taste and is covered on its own below.
func templated(t *testing.T, capture Capture) Context {
	t.Helper()
	return NewContext(capture, nil).WithSettings(testSettings(t))
}

func mustEntry(t *testing.T, ctx Context) string {
	t.Helper()
	lines, err := dailyEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}

func TestDailyEntryWritesWhatTheCaptureKnows(t *testing.T) {
	full := beenHere()
	full.Index.Payload = map[string]any{"note": "Coffee under the bridge"}
	want := strings.Join([]string{
		"- 2026-09-09 21:31:22 +10 ^dgs-20260909213122900-4620",
		"  - Coffee under the bridge",
		"  - -33.76910, 151.08200",
		"  - address: NSW, Epping",
	}, "\n")
	if got := mustEntry(t, templated(t, full)); got != want {
		t.Fatalf("entry =\n%s\nwant\n%s", got, want)
	}
}

// A line whose value the Capture does not have is dropped, label and all.
func TestDailyEntryDropsLinesWithNothingOnThem(t *testing.T) {
	bare := placeless()
	bare.Index.Coordinates = nil
	bare.Index.Payload = map[string]any{"note": "just the note"}

	want := "- 2026-09-09 21:31:22 +10 ^dgs-20260909213122900-4620\n  - just the note"
	if got := mustEntry(t, templated(t, bare)); got != want {
		t.Fatalf("entry =\n%s\nwant\n%s", got, want)
	}
	// Without content there is nothing worth writing at all.
	if entry, err := dailyEntry(templated(t, beenHere())); err != nil || entry != nil {
		t.Fatalf("entry = %q, err = %v, want nothing for a capture with no content", entry, err)
	}
}

// Every line of a note becomes an item of its own, and blank lines are spacing
// rather than content.
func TestDailyEntryWritesOneItemPerLine(t *testing.T) {
	capture := beenHere()
	capture.Index.Payload = map[string]any{"note": "first line\n\nsecond line\n   \nthird"}
	got := mustEntry(t, templated(t, capture))
	for _, want := range []string{"  - first line\n", "  - second line\n", "  - third\n"} {
		if !strings.Contains(got+"\n", want) {
			t.Fatalf("entry =\n%s\nwant an item per line", got)
		}
	}
	if strings.Contains(got, "  - \n") {
		t.Fatalf("a blank line became an item:\n%s", got)
	}
}

// A template in the configured directory replaces the compiled-in one by name,
// and the rest keep working.
func TestLoadTemplatePrefersTheConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, DailyEntryTemplate), []byte("- {{.Time}} {{.Workflow}}\n{{- range .Attachments}}\n  - ![[{{.Name}}]]\n{{- end}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	capture := beenHere()
	capture.Index.Payload = map[string]any{"note": "Coffee"}
	capture.Index.Attachments = []indexschema.Attachment{{Kind: "image", Name: "street.jpg"}, {Kind: "null"}}
	settings := DefaultSettings()
	settings.TemplateDir = dir
	settings.Sources = starterSources(t)

	want := "- 21:31 been_here\n  - ![[street.jpg]]"
	if got := mustEntry(t, NewContext(capture, nil).WithSettings(settings)); got != want {
		t.Fatalf("entry =\n%s\nwant\n%s", got, want)
	}

	// A directory that does not hold the template falls back to the built-in.
	settings.TemplateDir = t.TempDir()
	if got := mustEntry(t, NewContext(capture, nil).WithSettings(settings)); !strings.Contains(got, captureMark(capture)) {
		t.Fatalf("entry =\n%s\nwant the built-in template", got)
	}
}

// A template that names a field the data does not have fails when it is read,
// rather than writing a blank into a note.
func TestLoadTemplateReportsAnUnknownField(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, DailyEntryTemplate), []byte("- {{.Nope}}\n"), 0o600)
	settings := DefaultSettings()
	settings.TemplateDir = dir
	if _, err := dailyEntry(NewContext(beenHere(), map[FieldID]any{FieldContent: "x"}).WithSettings(settings)); err == nil {
		t.Fatal("an unknown field was accepted")
	}
}

// The shipped default is checked for what it must always do rather than for its
// exact text: how an entry reads is a matter of taste and is meant to be
// retuned, but it must parse, and it must carry what makes an entry an entry.
func TestBuiltinDailyEntryTemplateRenders(t *testing.T) {
	capture := beenHere()
	capture.Index.Payload = map[string]any{"note": "Coffee under the bridge"}
	ctx := NewContext(capture, nil).WithSettings(starterSettings(t))

	entry, err := dailyEntry(ctx)
	if err != nil {
		t.Fatalf("the shipped template does not render: %v", err)
	}
	got := strings.Join(entry, "\n")
	for _, want := range []string{captureMark(capture), "Coffee under the bridge", "-33.76910", "151.08200", "NSW, Epping"} {
		if !strings.Contains(got, want) {
			t.Errorf("the shipped entry is missing %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, "- ") {
		t.Errorf("an entry must start as a list item:\n%s", got)
	}
}

// A line written as a literal heading stays, while a line holding only a label
// and a placeholder that resolved to nothing goes: the difference is whether
// the blank was written there or came from the Capture.
func TestRenderEntryKeepsLiteralLinesAndDropsEmptyPlaceholders(t *testing.T) {
	dir := t.TempDir()
	body := "- {{.When}} {{.ID}}\n  - Content:\n{{- range .ContentLines}}\n    - {{.}}\n{{- end}}\n  - altitude: {{.Altitude}}\n  - address: {{.Address}}"
	if err := os.WriteFile(filepath.Join(dir, DailyEntryTemplate), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.TemplateDir = dir
	settings.Sources = starterSources(t)

	bare := placeless()
	bare.Index.Coordinates = nil
	bare.Index.Payload = map[string]any{"note": "only text"}
	got := mustEntry(t, NewContext(bare, nil).WithSettings(settings))

	if !strings.Contains(got, "  - Content:") {
		t.Errorf("a literal heading was dropped:\n%s", got)
	}
	for _, gone := range []string{"altitude:", "address:"} {
		if strings.Contains(got, gone) {
			t.Errorf("a label with no value was kept:\n%s", got)
		}
	}
}

// Altitude is metres above sea level, rounded, and a Capture with no position
// has none.
func TestEntryAltitude(t *testing.T) {
	capture := beenHere()
	capture.Index.Coordinates.Altitude = 12.44
	if got := string(entryData(NewContext(capture, nil)).Altitude); got != "12m" {
		t.Errorf("altitude = %q", got)
	}
	capture.Index.Coordinates = nil
	if got := string(entryData(NewContext(capture, nil)).Altitude); got != "" {
		t.Errorf("altitude = %q, want none without a position", got)
	}
}

// A value can be translated on its way into a note. The engine can do it inline
// for a case or two; a table belongs in the configuration, where it is data
// rather than something buried in a template.
func TestMappedTranslatesAValueThroughATable(t *testing.T) {
	settings := DefaultSettings()
	settings.TemplateDir = t.TempDir()
	settings.Mappings = map[string]map[string]string{
		"country": {"Australia": "🇦🇺_Australia"},
	}
	capture := beenHere()
	capture.Index.Place.Country = "Australia"
	ctx := NewContext(capture, nil).WithSettings(settings)
	day, _ := captureDay(ctx)

	render := func(body string) string {
		t.Helper()
		out, err := renderNote(settings, "test", body, noteData(ctx, day))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	if got := render(`{{mapped "country" .Country}}`); got != "🇦🇺_Australia" {
		t.Errorf("mapped = %q", got)
	}
	// A value the table does not mention comes back as it was: a mapping says
	// how some names are written here, not which names are allowed.
	if got := render(`{{mapped "country" .Region}}`); got != "NSW" {
		t.Errorf("unmapped = %q, want the value unchanged", got)
	}
	if got := render(`{{mapped "nothing" .Country}}`); got != "Australia" {
		t.Errorf("unknown table = %q, want the value unchanged", got)
	}
	// The engine can also do it inline, without a table.
	if got := render(`{{if eq .Country "Australia"}}AU{{else}}{{.Country}}{{end}}`); got != "AU" {
		t.Errorf("inline = %q", got)
	}
}
