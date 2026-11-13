// Package entrytemplate writes one entry of a note — a day's log, a timeline —
// from a template and what is known about the moment it records.
//
// It is shared, like internal/timeline: dgs capture renders with it natively
// and the Obsidian plugin through the timeline engine's WebAssembly build, so
// an entry reads the same whichever of them wrote it. It takes plain values and
// reads no files; where a template comes from is the caller's business.
//
// Templates are Go text/template rather than a shape invented here: a standard
// engine brings conditionals, loops over pictures, and errors reported at
// parse time instead of a private language that has to grow one feature at a
// time.
package entrytemplate

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"
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

// Position is where an entry was written, in degrees, with the altitude in
// metres when it is known. An altitude belongs to the position it was
// measured with: a caller that took the position from somewhere else leaves
// it nil.
type Position struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Altitude  *float64 `json:"altitude,omitempty"`
}

// Place is a position's address in its structured parts.
type Place struct {
	Country  string `json:"country,omitempty"`
	Region   string `json:"region,omitempty"`
	City     string `json:"city,omitempty"`
	Locality string `json:"locality,omitempty"`
}

// Attachment is what a template can say about one of the entry's files.
type Attachment struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Input is everything an entry may be written from. Each field is optional
// except CreatedAt; a template line whose fields are all empty is dropped.
type Input struct {
	// CreatedAt is when the entry records, RFC 3339 with the offset it was
	// recorded in. The entry belongs to that wall clock, not to the one of
	// wherever it is being written.
	CreatedAt string `json:"createdAt"`
	// ID is an Obsidian block reference, "^dgs-…", for a writer that has to
	// recognise its entry again. Empty writes none.
	ID       string    `json:"id,omitempty"`
	Content  string    `json:"content,omitempty"`
	Position *Position `json:"position,omitempty"`
	Place    Place     `json:"place,omitzero"`
	// PlaceName is the place's composed name, most specific first.
	PlaceName   string       `json:"placeName,omitempty"`
	Workflow    string       `json:"workflow,omitempty"`
	App         string       `json:"app,omitempty"`
	Device      string       `json:"device,omitempty"`
	Capture     string       `json:"capture,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	// Images are the pictures written beside the note for this entry, by the
	// filename the vault knows them by, for "![[{{.}}]]".
	Images []string `json:"images,omitempty"`
}

// Mappings are tables translating a value into how this vault writes it, by
// table name: {"country": {"Australia": "澳大利亚"}}.
type Mappings map[string]map[string]string

// Data is what a template may name. It is a declared struct rather than a map
// so an unknown field is an error the reader sees when the template is
// executed, not a blank in a note discovered later.
type Data struct {
	// When is the full timestamp with the offset recorded.
	When Value
	Date Value
	Time Value
	// ID is the Obsidian block reference identifying this entry.
	ID Value
	// Content is the entry's own text, and ContentLines is the same text with
	// blank lines dropped, for a template writing one item per line.
	Content      Value
	ContentLines []string
	// Latitude and Longitude are separate rather than one position string, so
	// a template decides the order and the punctuation between them.
	Latitude  Value
	Longitude Value
	// Clock is the time of day recorded, and Offset the zone it was recorded
	// in, written as hours: "+10", "+9.5". Separate from When so a template
	// can put them where it wants.
	Clock  Value
	Offset Value
	// copyLink and mapLinks are what the CopyLink and MapLinks methods render.
	// They are methods rather than values because which services an entry
	// carries and which vault command a coordinate links to are decisions about
	// how the line reads, and the line is the template's.
	copyLink func(choice string) string
	mapLinks func(services string) string
	// Altitude is metres above sea level, rounded.
	Altitude Value
	// Address is the place from its structured parts, coarsest first.
	Address  Value
	Place    Value
	Workflow Value
	App      Value
	// Device is the device the entry was taken on.
	Device      Value
	Capture     Value
	Attachments []Attachment
	Images      []string
}

// CopyLink is the position, shown as it is read and linked to the vault command
// that copies it. The command is named by the template because it is that
// vault's command: "{{.CopyLink "Copy Coordinates (lng, lat)"}}". Called with
// nothing, the position is written unlinked.
func (d Data) CopyLink(choice ...string) Value {
	if d.copyLink == nil {
		return ""
	}
	return Value(d.copyLink(strings.Join(choice, "")))
}

// MapLinks is the map services' links, joined. The services are named by the
// template, by name or short name, in the order they are written:
// "{{.MapLinks "Apple, 高德"}}". Called with nothing, every service is written.
func (d Data) MapLinks(services ...string) Value {
	if d.mapLinks == nil {
		return ""
	}
	return Value(d.mapLinks(strings.Join(services, ", ")))
}

// NewData collects what a template may write about one entry.
func NewData(in Input) Data {
	content := strings.TrimSpace(in.Content)
	latitude, longitude := Coordinates(in.Position)
	data := Data{
		When:         Value(FormatTimestamp(in.CreatedAt)),
		Date:         Value(DayOf(in.CreatedAt)),
		Time:         Value(TimeOf(in.CreatedAt)),
		ID:           Value(in.ID),
		Content:      Value(content),
		ContentLines: contentLines(content),
		Latitude:     Value(latitude),
		Longitude:    Value(longitude),
		Clock:        Value(clockOf(in.CreatedAt)),
		Offset:       Value(offsetOf(in.CreatedAt)),
		copyLink:     func(choice string) string { return CopyLink(latitude, longitude, choice) },
		mapLinks:     func(services string) string { return MapLinksLine(latitude, longitude, Label(in.Place), services) },
		Altitude:     Value(altitude(in.Position)),
		Address:      Value(Address(in.Place)),
		Place:        Value(in.PlaceName),
		Workflow:     Value(in.Workflow),
		App:          Value(in.App),
		Device:       Value(in.Device),
		Capture:      Value(in.Capture),
		Images:       in.Images,
	}
	for _, attachment := range in.Attachments {
		if attachment.Kind == "null" || attachment.Name == "" {
			continue
		}
		data.Attachments = append(data.Attachments, attachment)
	}
	return data
}

// Funcs are the functions every template may call. They are bound to the
// mappings, which one of them reads.
func Funcs(mappings Mappings) template.FuncMap {
	return template.FuncMap{
		"indent": func(spaces int, value string) string {
			pad := strings.Repeat(" ", spaces)
			return strings.ReplaceAll(value, "\n", "\n"+pad)
		},
		"join": strings.Join,
		"trim": strings.TrimSpace,
		// firstLine and afterFirstLine split a text into a title and a body,
		// for a workflow that asks for one text and means both.
		"firstLine":      FirstLine,
		"afterFirstLine": AfterFirstLine,
		"default":        func(fallback, value string) string { return orDefault(value, fallback) },
		// mapped translates a value through a named table. A value the table
		// does not mention comes back as it was: a mapping says how some names
		// are written in this vault, not which names are allowed, and dropping
		// the rest would lose data the entry actually carried.
		"mapped": func(table string, value Value) Value {
			if replacement, ok := mappings[table][string(value)]; ok {
				return Value(replacement)
			}
			return value
		},
	}
}

// Parse reads a template, named for its errors.
func Parse(name, text string, mappings Mappings) (*template.Template, error) {
	parsed, err := template.New(name).Funcs(Funcs(mappings)).Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	return parsed, nil
}

// Render parses a template and writes one entry through it.
func Render(name, text string, in Input, mappings Mappings) ([]string, error) {
	parsed, err := Parse(name, text, mappings)
	if err != nil {
		return nil, err
	}
	lines, err := Execute(parsed, NewData(in))
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	return lines, nil
}

// Execute writes one entry through a parsed template and drops the lines that
// came out with nothing on them. Pruning rather than making every template
// write its own conditionals: the common case is a line per value, and a line
// whose value the entry does not have should simply not be there.
func Execute(parsed *template.Template, data Data) ([]string, error) {
	var out strings.Builder
	if err := parsed.Execute(&out, data); err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(out.String(), "\n") {
		text, named, empty := StripMarkers(line)
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

// StripMarkers removes the markers a Value prints itself with, reporting how
// many placeholders the line held and how many of them were empty.
func StripMarkers(line string) (text string, named, empty int) {
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

// Coordinates are a position's two halves to five decimals: enough to
// identify a doorway, short enough to read. No position has neither half, so
// a template naming both drops the line rather than writing half of one.
func Coordinates(position *Position) (latitude, longitude string) {
	if position == nil {
		return "", ""
	}
	return fmt.Sprintf("%.5f", position.Latitude), fmt.Sprintf("%.5f", position.Longitude)
}

// altitude is metres above sea level, rounded: a phone's altitude is accurate
// to nothing like a metre, and the decimals only make the line harder to read.
func altitude(position *Position) string {
	if position == nil || position.Altitude == nil {
		return ""
	}
	return fmt.Sprintf("%.0fm", *position.Altitude)
}

// Address names the place from its structured parts, coarsest first — country,
// region, city, locality. That is the order the vault's own entries use, and it
// reads as a place being narrowed down rather than as an address on an
// envelope.
func Address(place Place) string {
	parts := make([]string, 0, 4)
	for _, part := range []string{place.Country, place.Region, place.City, place.Locality} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, ", ")
}

// Label is what a map pin is named: the most specific part of the address.
// The whole of it in a URL is too long to read and no more accurate, since the
// position comes from the coordinates.
func Label(place Place) string {
	for _, part := range []string{place.Locality, place.City, place.Region, place.Country} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// contentLines splits a text one line per item, dropping blank ones. A note
// written across several lines is several things worth reading, and a blank
// line between them is spacing rather than content.
func contentLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// FirstLine is the first line of a text that has anything on it, trimmed:
// blank lines before it are spacing rather than a title.
func FirstLine(text string) string {
	title, _ := splitFirstLine(text)
	return title
}

// AfterFirstLine is everything after that line, with the blank lines at either
// end removed and the blank lines inside kept, since they separate paragraphs.
func AfterFirstLine(text string) string {
	_, body := splitFirstLine(text)
	return body
}

func splitFirstLine(text string) (first, rest string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start == len(lines) {
		return "", ""
	}
	body := lines[start+1:]
	for len(body) > 0 && strings.TrimSpace(body[0]) == "" {
		body = body[1:]
	}
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	return strings.TrimSpace(lines[start]), strings.Join(body, "\n")
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// DayOf is the date part of an RFC 3339 timestamp, as written, or nothing.
func DayOf(createdAt string) string {
	if len(createdAt) < 10 {
		return ""
	}
	day := createdAt[:10]
	if strings.Count(day, "-") != 2 {
		return ""
	}
	return day
}

// TimeOf takes the wall clock recorded, to the minute, without converting it:
// the entry belongs to the day it was taken, in the offset it was taken in.
func TimeOf(createdAt string) string {
	if len(createdAt) < 16 || createdAt[10] != 'T' {
		return ""
	}
	return createdAt[11:16]
}

// clockOf is the time of day recorded, to the second: a timeline's entries a
// minute apart should not read as one.
func clockOf(createdAt string) string {
	if len(createdAt) < 19 || createdAt[10] != 'T' {
		return ""
	}
	return createdAt[11:19]
}

// offsetOf writes the zone as hours — "+10", "+9.5", "+5.75" — rather than as
// "+10:00". It is shorter, and how many hours away it was is the thing a reader
// is actually asking when they see it.
func offsetOf(createdAt string) string {
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return ""
	}
	_, seconds := created.Zone()
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	hours := strconv.FormatFloat(float64(seconds)/3600, 'f', -1, 64)
	return sign + hours
}

// FormatTimestamp renders an RFC 3339 timestamp as a compact wall clock keeping
// the offset it was written with: 2026-09-09 21:31:22 +10. Whole-hour offsets
// drop their minutes and UTC reads as Z. A value that is not RFC 3339 is
// returned unchanged, so a malformed one stays visible rather than being
// hidden behind a blank.
func FormatTimestamp(value string) string {
	if value == "" {
		return ""
	}
	created, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return created.Format("2006-01-02 15:04:05") + " " + timestampOffset(created)
}

// FormatTimestampDay is FormatTimestamp with the day of the week after the
// date, in English as the TUI is: 2026-09-09 Wed 21:31:22 +10.
func FormatTimestampDay(value string) string {
	created, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return FormatTimestamp(value)
	}
	return created.Format("2006-01-02 Mon 15:04:05") + " " + timestampOffset(created)
}

func timestampOffset(created time.Time) string {
	_, seconds := created.Zone()
	if seconds == 0 {
		return "Z"
	}
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	hours, minutes := seconds/3600, (seconds%3600)/60
	if minutes == 0 {
		return fmt.Sprintf("%s%d", sign, hours)
	}
	return fmt.Sprintf("%s%d:%02d", sign, hours, minutes)
}
