package organizer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"dgs-toolbox/internal/entrytemplate"
)

// Templates are Go text/template rather than a shape invented here. The entry
// for a daily note is only the first of them — a location note and, later, an
// Apple note need the same thing — and a standard engine brings conditionals,
// loops over a Capture's attachments, and errors reported at parse time instead
// of a private language that has to grow one feature at a time.
//
// The defaults are compiled in, so the tool works before anything is
// configured. The vault's entry template folder overrides them by filename,
// which is how a template becomes the reader's without becoming a prerequisite.
// They live in internal/entrytemplate, so the Obsidian plugin falls back to
// the same ones through the timeline engine.

const (
	// DailyEntryTemplate is the template one Capture is written with. It has a
	// compiled-in default, so the tool writes sensibly before anything is
	// configured.
	DailyEntryTemplate = "daily-entry.md"
	// DailyNoteTemplate is the template a missing daily note is created from.
	// Its compiled-in default is the shape the Obsidian side creates a log
	// with, so a log reads the same whichever side made it.
	DailyNoteTemplate = entrytemplate.DailyNote
	// LocationEntryTemplate is one Capture's entry in the running list of
	// places. It has a compiled-in default too.
	LocationEntryTemplate = "location-entry.md"
	// TimelineEntryTemplate is one Capture's entry on the family timeline.
	// Without one, the timeline is written with the location entry's template,
	// so the two lists read alike until a reader says otherwise.
	TimelineEntryTemplate = "timeline-entry.md"
)

// Value, EntryData and Attachment are the entry template's own, from
// internal/entrytemplate: the Obsidian plugin renders with the same package
// through the timeline engine, so an entry reads the same whoever wrote it.
// What lives here is only how a Capture becomes that package's plain input.
type (
	Value      = entrytemplate.Value
	EntryData  = entrytemplate.Data
	Attachment = entrytemplate.Attachment
)

// templateFuncs are bound to the settings the template is rendered against,
// because one of them reads the configured mappings.
func templateFuncs(settings Settings) template.FuncMap {
	return entrytemplate.Funcs(settings.Mappings)
}

// entryData collects what a template may write about one Capture.
func entryData(ctx Context) EntryData {
	return entrytemplate.NewData(entryInput(ctx))
}

// entryInput is the Capture as the entry template takes it. The position is
// the resolved one — typed, sourced or the index's — while its altitude and
// address are the index's only when the position is too: a city named for
// where the phone was is wrong about a place the payload names.
func entryInput(ctx Context) entrytemplate.Input {
	in := entrytemplate.Input{
		CreatedAt: ctx.String(FieldCreatedAt),
		ID:        captureMark(ctx.Capture),
		Content:   ctx.String(FieldContent),
		Place:     placeOf(ctx),
		PlaceName: ctx.String(FieldPlaceName),
		Workflow:  ctx.Capture.Workflow(),
		App:       ctx.Capture.Index.Source.App,
		Device:    ctx.Capture.Index.Source.Device.Name,
		Capture:   ctx.Capture.Name,
	}
	if position, ok := ctx.Position(); ok {
		in.Position = &entrytemplate.Position{Latitude: position.Latitude, Longitude: position.Longitude}
		if index := ctx.Capture.Index.Coordinates; index != nil && !position.Sourced {
			metres := index.Altitude
			in.Position.Altitude = &metres
		}
	}
	for _, attachment := range ctx.Capture.Index.Attachments {
		in.Attachments = append(in.Attachments, Attachment{Name: attachment.Name, Kind: attachment.Kind})
	}
	return in
}

// placeOf is the Capture's place in the entry template's terms.
func placeOf(ctx Context) entrytemplate.Place {
	place := ctx.place()
	return entrytemplate.Place{Country: place.Country, Region: place.Region, City: place.City, Locality: place.Locality}
}

// address names the Capture's place, coarsest first.
func address(ctx Context) string { return entrytemplate.Address(placeOf(ctx)) }

// coordinate is one half of the resolved position, to five decimals.
func coordinate(ctx Context, field FieldID) string {
	position, ok := ctx.Position()
	if !ok {
		return ""
	}
	latitude, longitude := entrytemplate.Coordinates(&entrytemplate.Position{Latitude: position.Latitude, Longitude: position.Longitude})
	if field == FieldLongitude {
		return longitude
	}
	return latitude
}

// altitude is the entry's altitude as a template writes it.
func altitude(ctx Context) string { return string(entryData(ctx).Altitude) }

// mapLinksLine is the map services' links on one line, the pin named by the
// most specific part of the Capture's place.
func mapLinksLine(ctx Context, latitude, longitude, services string) string {
	return entrytemplate.MapLinksLine(latitude, longitude, entrytemplate.Label(placeOf(ctx)), services)
}

// renderEntry writes one Capture through a template, dropping empty lines.
func renderEntry(parsed *template.Template, data EntryData) ([]string, error) {
	return entrytemplate.Execute(parsed, data)
}

var (
	firstLine      = entrytemplate.FirstLine
	afterFirstLine = entrytemplate.AfterFirstLine
)

// LoadTemplate reads a template by name, preferring the configured directory
// over the compiled-in default so a reader can replace one without having to
// supply all of them.
func LoadTemplate(settings Settings, name string) (*template.Template, error) {
	text, err := templateText(settings.TemplateDir, name)
	if err != nil {
		return nil, err
	}
	return entrytemplate.Parse(name, text, settings.Mappings)
}

func templateText(dir, name string) (string, error) {
	if dir != "" {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			return string(data), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read template %s: %w", name, err)
		}
	}
	text, ok := entrytemplate.Builtin(name)
	if !ok {
		return "", fmt.Errorf("no template named %s", name)
	}
	return text, nil
}

// NoteData is what a daily note's template may name, from
// internal/entrytemplate: the Obsidian plugin renders the same template with
// the same data for Templater's daily log template.
type NoteData = entrytemplate.NoteData

// noteData collects what a daily note's template may write for one day: the
// day, and where the Capture that prompted it was taken.
func noteData(ctx Context, day time.Time) NoteData {
	in := entrytemplate.NoteInput{
		Date:  day.Format("2006-01-02"),
		Now:   time.Now().Format(time.RFC3339),
		Place: placeOf(ctx),
	}
	if position, ok := ctx.Position(); ok {
		in.Position = &entrytemplate.Position{Latitude: position.Latitude, Longitude: position.Longitude}
	}
	data, _ := entrytemplate.NewNoteData(in)
	return data
}

// LoadNoteTemplate reads the template a daily note is created from: the
// vault's daily-note.md, else the compiled-in one, which is the shape the
// Obsidian side creates a log with too.
func LoadNoteTemplate(dir string) (string, error) {
	if strings.TrimSpace(dir) != "" {
		path := filepath.Join(dir, DailyNoteTemplate)
		text, err := os.ReadFile(path)
		if err == nil {
			return string(text), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
	}
	text, _ := entrytemplate.Builtin(DailyNoteTemplate)
	return text, nil
}

// CheckNoteTemplate reports whether the daily note's template parses and
// renders, without writing anything. A template is only exercised when a day
// has no note yet, which can be weeks after it was edited; this answers "is
// this file still good?" on demand.
func CheckNoteTemplate(settings Settings) error {
	text, err := LoadNoteTemplate(settings.TemplateDir)
	if err != nil {
		return err
	}
	_, err = renderNote(settings, DailyNoteTemplate, text, NoteData{})
	return err
}

// renderNote fills a note template, nothing pruned.
func renderNote(settings Settings, name, text string, data NoteData) (string, error) {
	parsed, err := entrytemplate.Parse(name, text, settings.Mappings)
	if err != nil {
		return "", err
	}
	rendered, err := entrytemplate.ExecuteNote(parsed, data)
	if err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return rendered, nil
}

// renderSection fills a heading template against the Capture. It shares the
// entry's data, because a heading names the same thing the entries under it
// describe.
func renderSection(ctx Context, section string) (string, error) {
	parsed, err := template.New("section").Funcs(templateFuncs(ctx.Settings)).Option("missingkey=error").Parse(section)
	if err != nil {
		return "", fmt.Errorf("section: %w", err)
	}
	var out strings.Builder
	if err := parsed.Execute(&out, entryData(ctx)); err != nil {
		return "", fmt.Errorf("section: %w", err)
	}
	rendered, _, _ := entrytemplate.StripMarkers(out.String())
	return rendered, nil
}
