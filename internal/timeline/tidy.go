package timeline

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TidyInput is a whole timeline: the running note and every year's archive.
type TidyInput struct {
	// Note is the running note.
	Note string `json:"note"`
	// Archives are the archive files there are, one per year, Text being the
	// whole file.
	Archives []Year `json:"archives"`
	// Current is the year that stays in the running note.
	Current string `json:"current"`
	// Weekdays names the days in markers, Sunday first.
	Weekdays []string `json:"weekdays"`
	// Offset is written on an entry whose offset is nowhere to be found: not
	// on it, nor on another entry of its day. Normally the device's.
	Offset string `json:"offset"`
	// Header starts an archive that does not exist yet.
	Header Header `json:"header"`
	// Undated is the marker text for entries no date can be found for.
	Undated string `json:"undated"`
}

// TidyResult is the timeline rewritten.
type TidyResult struct {
	Note string `json:"note"`
	// Archives are every archive given and every one started, by year, whole
	// files; one whose entries all left holds only its introduction.
	Archives []Year    `json:"archives"`
	Stats    TidyStats `json:"stats"`
}

// TidyStats says what tidying did, for the reader to check before trusting it.
type TidyStats struct {
	// Read is the entries read, Files the files they came from.
	Read  int `json:"read"`
	Files int `json:"files"`
	// Duplicates are entries read more than once and kept once.
	Duplicates int `json:"duplicates"`
	// Reformatted are entries whose first line was rewritten.
	Reformatted int `json:"reformatted"`
	// GuessedOffset are entries given an offset from their day or Offset.
	GuessedOffset int `json:"guessedOffset"`
	// Untimed are entries with no clock, kept as written at the end of their day.
	Untimed int `json:"untimed"`
	// Undated are entries before any date, kept under the Undated marker.
	Undated int `json:"undated"`
	// Kept is the entries left in the running note; Archived counts by year.
	Kept     int            `json:"kept"`
	Archived map[string]int `json:"archived"`
}

var (
	// tidyMarkerItem is a day's marker, the weekday optional, as it has been
	// written over the years.
	tidyMarkerItem = regexp.MustCompile(`^\s*[-*+]\s+\*(\d{4}-\d{2}-\d{2})(?:\s+[^*]*)?\*\s*$`)
	// tidyMarkerHeading is a day written as a heading, the way it once was.
	tidyMarkerHeading = regexp.MustCompile(`^#{1,6}\s+(\d{4}-\d{2}-\d{2})\b`)
	// tidyItem is an entry: a bullet or a number at the start of the line.
	tidyItem = regexp.MustCompile(`^(?:[-*+]|\d+[.)])\s+(.*)$`)
	// tidyFenced is a backticked head and what follows it.
	tidyFenced = regexp.MustCompile("^`([^`]+)`\\s*(.*)$")
	// tidyDated is a head opening with its own date.
	tidyDated = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})[T\s]+(.*)$`)
	// tidyClock is a clock, its seconds and offset optional, and the rest.
	tidyClock = regexp.MustCompile(`^(\d{1,2}):(\d{2})(?::(\d{2}))?\s*([+-][\d:.]+)?\s*(.*)$`)
	// tidyOffset is an offset in any of the ways it has been written.
	tidyOffset = regexp.MustCompile(`^([+-])(\d{1,2}):?(\d{2})?(?:\.(\d+))?$`)
)

type tidyEntry struct {
	date, clock, offset, text, id, original string
	details                                 []string
}

// Tidy rewrites a timeline into its one shape: every entry, from the running
// note and from every archive, read in whichever way it was written —
// headings for days, numbered items, a clock without backticks, seconds or
// offset, a date inside the entry — and written back as the plugin and dgs
// capture write it, days newest first and each day's entries latest first.
// An entry read twice is kept once, the copy with the most detail lines.
// The current year goes to the running note and every other to its year's
// archive, as Archive would.
//
// Nothing is made up: an entry without a clock keeps its line as written, at
// the end of its day, and one with no date goes under the Undated marker at
// the top of the running note. A line that is neither a day, an entry nor
// under one is refused rather than dropped.
func Tidy(in TidyInput) (TidyResult, error) {
	if len(in.Weekdays) != 7 {
		return TidyResult{}, fmt.Errorf("tidy: %d weekday names, want 7", len(in.Weekdays))
	}
	result := TidyResult{Stats: TidyStats{Archived: map[string]int{}}}
	var read []tidyEntry
	sources := append([]Year{{Text: in.Note}}, in.Archives...)
	for _, source := range sources {
		if source.Year == "" && source.Text == "" {
			continue
		}
		name := "the running note"
		if source.Year != "" {
			name = "the " + source.Year + " archive"
		}
		entries, err := tidyRead(source.Text, name)
		if err != nil {
			return TidyResult{}, err
		}
		result.Stats.Files++
		read = append(read, entries...)
	}
	result.Stats.Read = len(read)

	entries := tidyDedupe(read)
	result.Stats.Duplicates = len(read) - len(entries)

	dayOffset := map[string]string{}
	for _, entry := range entries {
		if entry.date != "" && entry.offset != "" && dayOffset[entry.date] == "" {
			dayOffset[entry.date] = entry.offset
		}
	}
	byYear := map[string][]tidyEntry{}
	var kept, undated []tidyEntry
	for index := range entries {
		entry := &entries[index]
		if entry.clock != "" && entry.offset == "" {
			entry.offset = dayOffset[entry.date]
			if entry.offset == "" {
				entry.offset = in.Offset
			}
			result.Stats.GuessedOffset++
		}
		if entry.clock == "" {
			result.Stats.Untimed++
		} else if entry.line() != "- "+entry.original {
			result.Stats.Reformatted++
		}
		switch {
		case entry.date == "":
			undated = append(undated, *entry)
		case entry.date[:4] == in.Current:
			kept = append(kept, *entry)
		default:
			byYear[entry.date[:4]] = append(byYear[entry.date[:4]], *entry)
		}
	}
	result.Stats.Undated = len(undated)
	result.Stats.Kept = len(kept)

	var blocks []string
	if len(undated) > 0 {
		blocks = append(blocks, fmt.Sprintf("- *%s*\n%s", in.Undated, tidyLines(undated)))
	}
	if body := tidyTimeline(kept, in.Weekdays); body != "" {
		blocks = append(blocks, body)
	}
	preamble, _ := splitPreamble(in.Note)
	result.Note = tidyFile(preamble, strings.Join(blocks, "\n"))

	years := map[string]bool{}
	for _, archive := range in.Archives {
		years[archive.Year] = true
	}
	for year := range byYear {
		years[year] = true
	}
	var ordered []string
	result.Archives = []Year{} // JSON [] rather than null, for the plugin to loop over
	for year := range years {
		ordered = append(ordered, year)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ordered)))
	for _, year := range ordered {
		body := tidyTimeline(byYear[year], in.Weekdays)
		result.Stats.Archived[year] = len(byYear[year])
		preamble := ""
		for _, archive := range in.Archives {
			if archive.Year == year {
				preamble, _ = splitPreamble(archive.Text)
			}
		}
		if preamble == "" {
			preamble = strings.TrimRight(in.Header.Render(year), "\n")
		}
		result.Archives = append(result.Archives, Year{Year: year, Text: tidyFile(preamble, body)})
	}
	return result, nil
}

// tidyRead reads one file's entries.
func tidyRead(content, name string) ([]tidyEntry, error) {
	preamble, body := splitPreamble(content)
	first := 0
	if preamble != "" {
		first = strings.Count(preamble, "\n") + 1
	}
	var entries []tidyEntry
	var current *tidyEntry
	date := ""
	flush := func() {
		if current != nil {
			entries = append(entries, *current)
		}
		current = nil
	}
	for index, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if match := tidyMarkerItem.FindStringSubmatch(line); match != nil {
			flush()
			date = match[1]
			continue
		}
		if match := tidyMarkerHeading.FindStringSubmatch(line); match != nil {
			flush()
			date = match[1]
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if current == nil {
				return nil, fmt.Errorf("tidy: line %d of %s is indented under no entry: %q", first+index+1, name, line)
			}
			current.details = append(current.details, detailBullet.ReplaceAllString(strings.TrimSpace(line), ""))
			continue
		}
		match := tidyItem.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("tidy: line %d of %s is not a day or an entry: %q", first+index+1, name, line)
		}
		flush()
		entry := tidyParse(match[1], date)
		current = &entry
	}
	flush()
	return entries, nil
}

// tidyParse reads an entry's first line, without its bullet.
func tidyParse(content, date string) tidyEntry {
	entry := tidyEntry{date: date, original: strings.TrimSpace(content)}
	rest := entry.original
	if match := blockID.FindStringSubmatchIndex(rest); match != nil {
		entry.id = rest[match[2]:match[3]]
		rest = strings.TrimSpace(rest[:match[0]])
	}
	head, tail, fenced := rest, "", false
	if match := tidyFenced.FindStringSubmatch(rest); match != nil {
		head, tail, fenced = match[1], match[2], true
	}
	if match := tidyDated.FindStringSubmatch(head); match != nil {
		entry.date, head = match[1], match[2]
	}
	if match := tidyClock.FindStringSubmatch(head); match != nil {
		hour := match[1]
		if len(hour) == 1 {
			hour = "0" + hour
		}
		seconds := match[3]
		if seconds == "" {
			seconds = "00"
		}
		entry.clock = hour + ":" + match[2] + ":" + seconds
		entry.offset = tidyOffsetOf(match[4])
		if !fenced {
			tail = match[5]
		}
	} else if !fenced {
		tail = rest
	}
	entry.text = strings.TrimSpace(separator.ReplaceAllString(strings.TrimSpace(tail), ""))
	return entry
}

// tidyOffsetOf writes an offset as hours: "+10:00", "+1000" and "+10" are
// "+10", "+09:30" is "+9.5". Empty when it is not one.
func tidyOffsetOf(value string) string {
	match := tidyOffset.FindStringSubmatch(value)
	if match == nil {
		return ""
	}
	hours, _ := strconv.Atoi(match[2])
	total := float64(hours)
	switch {
	case match[4] != "":
		fraction, _ := strconv.ParseFloat("0."+match[4], 64)
		total += fraction
	case match[3] != "":
		minutes, _ := strconv.Atoi(match[3])
		total += float64(minutes) / 60
	}
	return match[1] + strconv.FormatFloat(total, 'f', -1, 64)
}

// line is the entry's first line as the timeline writes it.
func (e tidyEntry) line() string {
	if e.clock == "" {
		return "- " + e.original
	}
	line := fmt.Sprintf("- `%s %s`", e.clock, e.offset)
	if e.text != "" {
		line += " · " + e.text
	}
	if e.id != "" {
		line += " ^" + e.id
	}
	return line
}

// tidyDedupe keeps each entry once, by date, clock and text, in the order
// first read, the copy with the most details.
func tidyDedupe(entries []tidyEntry) []tidyEntry {
	at := map[string]int{}
	var kept []tidyEntry
	for _, entry := range entries {
		key := entry.date + "|" + entry.clock + "|" + entry.text
		if entry.clock == "" {
			key = entry.date + "|" + entry.original
		}
		if index, seen := at[key]; seen {
			if len(entry.details) > len(kept[index].details) {
				kept[index] = entry
			}
			continue
		}
		at[key] = len(kept)
		kept = append(kept, entry)
	}
	return kept
}

// tidyTimeline writes entries as days newest first, each day's entries latest
// first and those without a clock last, in the order read.
func tidyTimeline(entries []tidyEntry, weekdays []string) string {
	byDate := map[string][]tidyEntry{}
	var dates []string
	for _, entry := range entries {
		if _, seen := byDate[entry.date]; !seen {
			dates = append(dates, entry.date)
		}
		byDate[entry.date] = append(byDate[entry.date], entry)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	var days []string
	for _, date := range dates {
		day := byDate[date]
		sort.SliceStable(day, func(i, j int) bool {
			if day[i].clock == "" || day[j].clock == "" {
				return day[i].clock != "" && day[j].clock == ""
			}
			return day[i].clock > day[j].clock
		})
		parsed, _ := time.Parse(DateLayout, date)
		days = append(days, Marker(parsed, weekdays[parsed.Weekday()])+"\n"+tidyLines(day))
	}
	return strings.Join(days, "\n")
}

func tidyLines(entries []tidyEntry) string {
	var lines []string
	for _, entry := range entries {
		lines = append(lines, entry.line())
		for _, detail := range entry.details {
			lines = append(lines, "    - "+detail)
		}
	}
	return strings.Join(lines, "\n")
}

func tidyFile(preamble, body string) string {
	switch {
	case body == "":
		return preamble + "\n"
	case preamble == "":
		return body + "\n"
	}
	return preamble + "\n\n" + body + "\n"
}
