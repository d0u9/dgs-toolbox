package entrytemplate

import (
	"fmt"
	"strings"
	"text/template"
	"time"
)

// DailyNote is the template a day's log is created from. dgs capture creates
// a missing log with it, and the Obsidian plugin renders it for Templater's
// daily log template, so a log reads the same whoever made it. Its compiled-in
// copy is the shape the vault's logs already have; what neither writer knows
// when the log is made — the weather — is left empty for the backfill.
const DailyNote = "daily-note.md"

// NoteInput is what a daily note is made from: the day it is for, when it is
// made, and where, when the maker knows. Obsidian makes a log before it knows
// where this device is, so its place is empty and the backfill fills it.
type NoteInput struct {
	// Date is the day, YYYY-MM-DD.
	Date string `json:"date"`
	// Now is when the note is made, RFC 3339: a note written today for last
	// week records both.
	Now      string    `json:"now"`
	Place    Place     `json:"place,omitzero"`
	Position *Position `json:"position,omitempty"`
}

// NoteData is what a daily note's template may name. Nothing in it needs
// asking: the date is the date, and where it was is what the maker brings.
// What genuinely cannot be answered yet is left for later.
type NoteData struct {
	Date    Value
	Year    Value
	Month   Value
	Day     Value
	Weekday Value
	// DayOfYear is zero-padded to three digits, as the vault's logs write it.
	DayOfYear Value
	Now       Value
	Country   Value
	Region    Value
	City      Value
	Locality  Value
	// Place is the address, coarsest first, as an entry writes it.
	Place     Value
	Latitude  Value
	Longitude Value
}

// NewNoteData collects what a daily note's template may write for one day.
// An empty date is the zero day, which is what a template check renders with.
func NewNoteData(in NoteInput) (NoteData, error) {
	var day time.Time
	if in.Date != "" {
		parsed, err := time.Parse("2006-01-02", in.Date)
		if err != nil {
			return NoteData{}, fmt.Errorf("note date %q: %w", in.Date, err)
		}
		day = parsed
	}
	latitude, longitude := Coordinates(in.Position)
	return NoteData{
		Date:      Value(day.Format("2006-01-02")),
		Year:      Value(day.Format("2006")),
		Month:     Value(day.Format("01")),
		Day:       Value(day.Format("02")),
		Weekday:   Value(day.Format("Monday")),
		DayOfYear: Value(fmt.Sprintf("%03d", day.YearDay())),
		Now:       Value(in.Now),
		Country:   Value(strings.TrimSpace(in.Place.Country)),
		Region:    Value(strings.TrimSpace(in.Place.Region)),
		City:      Value(strings.TrimSpace(in.Place.City)),
		Locality:  Value(strings.TrimSpace(in.Place.Locality)),
		Place:     Value(Address(in.Place)),
		Latitude:  Value(latitude),
		Longitude: Value(longitude),
	}, nil
}

// ExecuteNote fills a parsed note template. Unlike an entry, nothing is
// pruned: a note is a document the reader will edit, and dropping lines out of
// it because a value was unknown would leave them wondering what was removed.
func ExecuteNote(parsed *template.Template, data NoteData) (string, error) {
	var out strings.Builder
	if err := parsed.Execute(&out, data); err != nil {
		return "", err
	}
	rendered, _, _ := StripMarkers(out.String())
	return rendered, nil
}

// RenderNote parses a note template and fills it for one day.
func RenderNote(name, text string, in NoteInput, mappings Mappings) (string, error) {
	data, err := NewNoteData(in)
	if err != nil {
		return "", err
	}
	parsed, err := Parse(name, text, mappings)
	if err != nil {
		return "", err
	}
	rendered, err := ExecuteNote(parsed, data)
	if err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return rendered, nil
}
