// Package timeline keeps a Markdown note that grows at the top: entries newest
// first, grouped under a marker for their day, with the years that have rolled
// over moved out into one file per year. It works on text only — reading and
// writing the files is the caller's — so any list of this shape, a list of
// places or a family's timeline, is the same few calls.
package timeline

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The marker's shape is fixed here rather than left to a template because
// archiving reads the year back out of it: a marker a reader could reshape is
// one the archive could no longer recognise. Only the day's name is the
// caller's, so a list written in another language keeps the shape.
const (
	// DateLayout is the date in a marker.
	DateLayout = "2006-01-02"
	// YearLayout names a year's archive file.
	YearLayout = "2006"
)

// markerLine matches a day's marker and captures its date; the year is the
// date's first four characters.
var markerLine = regexp.MustCompile(`^- \*(\d{4}-\d{2}-\d{2}) .+\*$`)

// Marker is the day's tick on the timeline: "- *2026-09-24 周四*". The italics
// are what a vault's stylesheet recognises, and what Archive matches on.
func Marker(day time.Time, weekday string) string {
	return fmt.Sprintf("- *%s %s*", day.Format(DateLayout), weekday)
}

// Header introduces a year's archive file, naming the note its entries came
// from so a reader opening a year on its own knows what they are looking at.
type Header struct {
	// CSSClass is written into the frontmatter for the vault's stylesheet.
	CSSClass string `json:"cssclass"`
	// Title names the list in the callout: "2026 年的<Title>".
	Title string `json:"title"`
	// Source is the running note's filename.
	Source string `json:"source"`
}

// Render is the header of the archive for one year.
func (h Header) Render(year string) string {
	return fmt.Sprintf("---\ncssclasses:\n  - %s\n---\n\n", h.CSSClass) +
		fmt.Sprintf("> [!note]- %s 年的%s\n", year, h.Title) +
		fmt.Sprintf("> 从 %s 归档过来的，格式一样，新的在最上面。\n", h.Source)
}

// Year is the entries of one year leaving the running list, markers included,
// newest first.
type Year struct {
	Year string `json:"year"`
	Text string `json:"text"`
}

// Archive takes every day not in the current year out of the list. It returns
// what is left and what left, one Year per year in the order first met. With
// nothing to move, the content comes back unchanged, so the caller writes no
// archive at all.
func Archive(content, current string) (remaining string, moved []Year) {
	preamble, body := splitPreamble(content)
	lead, days := splitByMarker(body)
	var kept []string
	if lead != "" {
		kept = append(kept, lead)
	}
	index := map[string]int{}
	for _, day := range days {
		if day.Year == current {
			kept = append(kept, day.Text)
			continue
		}
		at, seen := index[day.Year]
		if !seen {
			index[day.Year] = len(moved)
			moved = append(moved, day)
			continue
		}
		moved[at].Text += "\n" + day.Text
	}
	if len(moved) == 0 {
		return content, nil
	}
	return withPreamble(preamble, strings.Join(kept, "\n")+"\n"), moved
}

// Merge puts what is leaving the running list at the top of a year's archive,
// below its introduction. An archive that does not exist yet is started from
// the header.
func Merge(archive string, header Header, moving Year) string {
	if archive == "" {
		return header.Render(moving.Year) + "\n" + moving.Text + "\n"
	}
	preamble, body := splitPreamble(archive)
	rest := strings.TrimLeft(body, "\n")
	merged := moving.Text + "\n"
	if rest != "" {
		merged = moving.Text + "\n" + rest
	}
	return withPreamble(preamble, merged)
}

func withPreamble(preamble, body string) string {
	if preamble != "" {
		return preamble + "\n\n" + body
	}
	return body
}

// splitPreamble separates what introduces the note from what it records: the
// frontmatter, and a callout following it.
func splitPreamble(content string) (preamble, body string) {
	lines := strings.Split(content, "\n")
	index := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for at := 1; at < len(lines); at++ {
			if strings.TrimSpace(lines[at]) == "---" {
				index = at + 1
				break
			}
		}
	}
	for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
		index++
	}
	for index < len(lines) && strings.HasPrefix(lines[index], ">") {
		index++
	}
	end := index
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[:end], "\n"), strings.Join(lines[end:], "\n")
}

func trimLeadingBlank(lines []string) []string {
	index := 0
	for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
		index++
	}
	return lines[index:]
}

// splitByMarker cuts the body into days. Anything before the first marker —
// normally nothing — comes back as lead, so archiving never loses it.
func splitByMarker(body string) (lead string, days []Year) {
	var current *Year
	var before []string
	for _, line := range strings.Split(body, "\n") {
		if match := markerLine.FindStringSubmatch(strings.TrimRight(line, " ")); match != nil {
			if current != nil {
				days = append(days, *current)
			}
			current = &Year{Year: match[1][:4], Text: line}
			continue
		}
		if current == nil {
			before = append(before, line)
			continue
		}
		current.Text += "\n" + line
	}
	if current != nil {
		days = append(days, *current)
	}
	for index := range days {
		days[index].Text = strings.TrimRight(days[index].Text, " \n\t")
	}
	return strings.Trim(strings.Join(before, "\n"), " \n\t"), days
}
