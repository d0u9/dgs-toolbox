package organizer

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// Templates are Go text/template rather than a shape invented here. The entry
// for a daily note is only the first of them — a location note and, later, an
// Apple note need the same thing — and a standard engine brings conditionals,
// loops over a Capture's attachments, and errors reported at parse time instead
// of a private language that has to grow one feature at a time.
//
// The defaults are compiled in, so the tool works before anything is
// configured. A configured template directory overrides them by filename, which
// is how a template becomes the reader's without becoming a prerequisite.
//
//go:embed templates/*.md
var builtinTemplates embed.FS

const (
	// DailyEntryTemplate is the template one Capture is written with. It has a
	// compiled-in default, so the tool writes sensibly before anything is
	// configured.
	DailyEntryTemplate = "daily-entry.md"
	// DailyNoteTemplate is the template a missing daily note is created from.
	// It has no default: what a reader's note should hold is not something to
	// invent, and an empty note among templated ones is one they have to
	// repair by hand later.
	DailyNoteTemplate = "daily-note.md"
)

// Value is a template field. It prints itself wrapped in markers the renderer
// strips afterwards, which is how a line knows whether the blank on it came
// from a placeholder that resolved to nothing or was written there on purpose:
// a literal "- Content:" heading is kept, while "- address:" whose address is
// unknown is dropped. Its underlying kind is still a string, so {{if .Place}}
// and {{range}} behave as a template author expects.
type Value string

const (
	valueOpen  = "\x01"
	valueClose = "\x02"
)

func (v Value) String() string { return valueOpen + string(v) + valueClose }

// EntryData is what a template may name. It is a declared struct rather than a
// map so an unknown field is an error the reader sees when the template is
// parsed, not a blank in a note discovered later.
type EntryData struct {
	// When is the full timestamp with the offset the Capture recorded.
	When Value
	Date Value
	Time Value
	// ID is the Obsidian block reference identifying this entry.
	ID Value
	// Content is the Capture's own text, and ContentLines is the same text with
	// blank lines dropped, for a template writing one item per line.
	Content      Value
	ContentLines []string
	Coordinates  Value
	// Altitude is metres above sea level, as the Capture recorded it. It is
	// separate from Coordinates because a template may want the position
	// without it.
	Altitude Value
	// Address is the place from its structured parts, most specific first.
	Address     Value
	Place       Value
	Workflow    Value
	App         Value
	Capture     Value
	Attachments []Attachment
}

// Attachment is what a template can say about one of a Capture's files.
type Attachment struct {
	Name string
	Kind string
}

var templateFuncs = template.FuncMap{
	"indent": func(spaces int, value string) string {
		pad := strings.Repeat(" ", spaces)
		return strings.ReplaceAll(value, "\n", "\n"+pad)
	},
	"join":    strings.Join,
	"trim":    strings.TrimSpace,
	"default": func(fallback, value string) string { return orDefault(value, fallback) },
}

// entryData collects what a template may write about one Capture.
func entryData(ctx Context) EntryData {
	created := ctx.String(FieldCreatedAt)
	content := strings.TrimSpace(ctx.String(FieldContent))
	data := EntryData{
		When:         Value(FormatTimestamp(created)),
		Date:         Value(dayOf(created)),
		Time:         Value(timeOf(created)),
		ID:           Value(captureMark(ctx.Capture)),
		Content:      Value(content),
		ContentLines: contentLines(content),
		Coordinates:  Value(coordinates(ctx)),
		Altitude:     Value(altitude(ctx)),
		Address:      Value(address(ctx)),
		Place:        Value(ctx.String(FieldPlaceName)),
		Workflow:     Value(ctx.Capture.Workflow()),
		App:          Value(ctx.Capture.Index.Source.App),
		Capture:      Value(ctx.Capture.Name),
	}
	for _, attachment := range ctx.Capture.Index.Attachments {
		if attachment.Kind == "null" || attachment.Name == "" {
			continue
		}
		data.Attachments = append(data.Attachments, Attachment{Name: attachment.Name, Kind: attachment.Kind})
	}
	return data
}

// contentLines splits a Capture's text one line per item, dropping blank ones.
// A note written across several lines is several things worth reading, and a
// blank line between them is spacing rather than content.
func contentLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// address names the place from its structured parts, most specific first, so an
// entry reads as a place rather than as coordinates a person has to decode.
func address(ctx Context) string {
	place := ctx.Capture.Index.CapturePlace()
	parts := make([]string, 0, 4)
	for _, part := range []string{place.Locality, place.City, place.Region, place.Country} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, ", ")
}

// coordinates is the position as Capture Info shows it, to five decimals:
// enough to identify a doorway, short enough to read.
func coordinates(ctx Context) string {
	latitude, hasLatitude := ctx.Get(FieldLatitude)
	longitude, hasLongitude := ctx.Get(FieldLongitude)
	if !hasLatitude || !hasLongitude {
		return ""
	}
	return fmt.Sprintf("%.5f, %.5f", asFloat(latitude), asFloat(longitude))
}

// altitude is metres above sea level, rounded: a Capture's altitude is accurate
// to nothing like a metre, and the decimals only make the line harder to read.
// A Capture with no coordinates has none.
func altitude(ctx Context) string {
	position := ctx.Capture.Index.Coordinates
	if position == nil {
		return ""
	}
	return fmt.Sprintf("%.0fm", position.Altitude)
}

func asFloat(value any) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case float32:
		return float64(number)
	}
	return 0
}

// LoadTemplate reads a template by name, preferring the configured directory
// over the compiled-in default so a reader can replace one without having to
// supply all of them.
func LoadTemplate(dir, name string) (*template.Template, error) {
	text, err := templateText(dir, name)
	if err != nil {
		return nil, err
	}
	parsed, err := template.New(name).Funcs(templateFuncs).Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	return parsed, nil
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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
	data, err := builtinTemplates.ReadFile("templates/" + name)
	if err != nil {
		return "", fmt.Errorf("no template named %s", name)
	}
	return string(data), nil
}

// renderEntry writes one Capture through a template and drops the lines that
// came out with nothing on them. Pruning rather than making every template
// write its own conditionals: the common case is a line per value, and a line
// whose value the Capture does not have should simply not be there.
func renderEntry(parsed *template.Template, data EntryData) ([]string, error) {
	var out strings.Builder
	if err := parsed.Execute(&out, data); err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(out.String(), "\n") {
		text, named, empty := stripMarkers(line)
		if named > 0 && named == empty {
			// Every placeholder on this line resolved to nothing, so the line
			// is a label with no value rather than something to write.
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		lines = append(lines, strings.TrimRight(text, " "))
	}
	return lines, nil
}

// stripMarkers removes the markers a Value prints itself with, reporting how
// many placeholders the line held and how many of them were empty.
func stripMarkers(line string) (text string, named, empty int) {
	var out strings.Builder
	for {
		start := strings.Index(line, valueOpen)
		if start < 0 {
			out.WriteString(line)
			return out.String(), named, empty
		}
		end := strings.Index(line[start:], valueClose)
		if end < 0 {
			out.WriteString(line)
			return out.String(), named, empty
		}
		value := line[start+len(valueOpen) : start+end]
		named++
		if value == "" {
			empty++
		}
		out.WriteString(line[:start])
		out.WriteString(value)
		line = line[start+end+len(valueClose):]
	}
}

// NoteData is what a daily note's own template may name: the day it is for, and
// what the Capture that prompted it knows about where it was taken.
//
// It exists because a vault's own daily template is usually written for a
// plugin that asks the reader questions as it runs — country, region, weather —
// and none of that can be answered without a person. Half of it need not be
// asked at all: the date is the date, and the Capture already carries where it
// was. What is genuinely unanswerable is left to the reader to fill in later.
type NoteData struct {
	Date      Value
	Year      Value
	Month     Value
	Day       Value
	Weekday   Value
	DayOfYear Value
	// Country, Region, City and Locality come from the Capture that prompted
	// the note.
	Country  Value
	Region   Value
	City     Value
	Locality Value
	// Place is the composed name, most specific first.
	Place Value
}

// noteData collects what a daily note's template may write for one day.
func noteData(ctx Context, day time.Time) NoteData {
	place := ctx.Capture.Index.CapturePlace()
	return NoteData{
		Date:      Value(day.Format("2006-01-02")),
		Year:      Value(day.Format("2006")),
		Month:     Value(day.Format("01")),
		Day:       Value(day.Format("02")),
		Weekday:   Value(day.Format("Monday")),
		DayOfYear: Value(fmt.Sprintf("%d", day.YearDay())),
		Country:   Value(strings.TrimSpace(place.Country)),
		Region:    Value(strings.TrimSpace(place.Region)),
		City:      Value(strings.TrimSpace(place.City)),
		Locality:  Value(strings.TrimSpace(place.Locality)),
		Place:     Value(address(ctx)),
	}
}

// LoadNoteTemplate reads the template a daily note is created from. Unlike the
// entry template it has no compiled-in default, so a missing one names the file
// that was expected rather than falling back to something invented here.
func LoadNoteTemplate(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", ErrNoDailyTemplate
	}
	path := filepath.Join(dir, DailyNoteTemplate)
	text, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: expected %s", ErrNoDailyTemplate, path)
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(text), nil
}

// renderNote fills a template read from a file. Unlike an entry, nothing is
// pruned: a note is a document the reader will edit, and dropping lines out of
// it because a value was unknown would leave them wondering what was removed.
func renderNote(name, text string, data NoteData) (string, error) {
	parsed, err := template.New(name).Funcs(templateFuncs).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	var out strings.Builder
	if err := parsed.Execute(&out, data); err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	rendered, _, _ := stripMarkers(out.String())
	return rendered, nil
}
