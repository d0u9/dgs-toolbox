package timeline

import (
	"regexp"
	"strings"
)

// Document is a timeline read into its days, for a reader that shows or
// searches the list rather than rewriting it.
type Document struct {
	// Days are in the order written, which is newest first on a tidy list.
	Days []Day `json:"days"`
}

// Day is one marker and the entries under it.
type Day struct {
	Date string `json:"date"`
	// Marker is the marker's line as written.
	Marker string `json:"marker"`
	// Line is the marker's line number in the note, counting from 0.
	Line    int     `json:"line"`
	Entries []Entry `json:"entries"`
}

// Entry is one item of a day: its first line, and the lines indented under it.
type Entry struct {
	// Raw is the first line as written.
	Raw string `json:"raw"`
	// Line is Raw's line number in the note, counting from 0.
	Line int `json:"line"`
	// Time is the clock as "HH:MM:SS", empty when the entry has none.
	Time string `json:"time,omitempty"`
	// Offset is the UTC offset as written after the clock: "+10", "+9.5".
	Offset string `json:"offset,omitempty"`
	// Text is what happened: the first line without its clock, separator and
	// block id.
	Text string `json:"text"`
	// ID is the block id closing the first line, without its caret.
	ID string `json:"id,omitempty"`
	// Details are the lines under the entry, without indentation or bullet.
	Details []string `json:"details,omitempty"`
}

var (
	// entryHead splits a first line into its clock, offset and the rest.
	entryHead = regexp.MustCompile("^- `?(\\d{1,2}:\\d{2}(?::\\d{2})?)(?:\\s+([+-]\\d{1,2}(?:[.:]\\d+)?))?`?\\s*(.*)$")
	// blockID is an Obsidian block id closing a line.
	blockID = regexp.MustCompile(`\s\^([A-Za-z0-9-]+)\s*$`)
	// separator is what stands between the clock and the text.
	separator = regexp.MustCompile(`^[·—–:-]\s*`)
	// detailBullet is the list marker opening an indented line.
	detailBullet = regexp.MustCompile(`^[-*+]\s+`)
)

// Parse reads a timeline: every day, every entry under it. What the note opens
// with and anything before the first marker are not part of the list and are
// left out. Lines are numbered in the whole note, so a reader can point back
// at the line it came from.
func Parse(content string) Document {
	lines := strings.Split(content, "\n")
	start := strings.Count(content[:len(content)-len(bodyOf(content))], "\n")

	document := Document{Days: []Day{}}
	var day *Day
	var entry *Entry
	flush := func() {
		if entry != nil && day != nil {
			day.Entries = append(day.Entries, *entry)
		}
		entry = nil
	}
	for index := start; index < len(lines); index++ {
		line := strings.TrimRight(lines[index], " \t\r")
		if match := markerLine.FindStringSubmatch(line); match != nil {
			flush()
			if day != nil {
				document.Days = append(document.Days, *day)
			}
			day = &Day{Date: match[1], Marker: line, Line: index, Entries: []Entry{}}
			continue
		}
		if day == nil || strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			flush()
			parsed := parseEntry(line)
			parsed.Line = index
			entry = &parsed
			continue
		}
		if entry != nil {
			entry.Details = append(entry.Details, detailBullet.ReplaceAllString(strings.TrimSpace(line), ""))
		}
	}
	flush()
	if day != nil {
		document.Days = append(document.Days, *day)
	}
	return document
}

func parseEntry(line string) Entry {
	entry := Entry{Raw: line}
	rest := strings.TrimPrefix(line, "- ")
	if match := blockID.FindStringSubmatchIndex(rest); match != nil {
		entry.ID = rest[match[2]:match[3]]
		rest = rest[:match[0]]
	}
	if match := entryHead.FindStringSubmatch("- " + rest); match != nil {
		entry.Time = clockKey("- " + match[1])
		entry.Offset = match[2]
		rest = match[3]
	}
	entry.Text = strings.TrimSpace(separator.ReplaceAllString(strings.TrimSpace(rest), ""))
	return entry
}

// bodyOf is the content after what introduces the note.
func bodyOf(content string) string {
	_, body := splitPreamble(content)
	return body
}
