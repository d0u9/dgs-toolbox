package timeline

import (
	"regexp"
	"strings"
)

// entryTime reads the clock an entry's first line opens with, backticked or
// not, seconds or not: "- `17:00:05 +10` · …", "- 17:00 +10 · …".
var entryTime = regexp.MustCompile("^- `?(\\d{1,2}):(\\d{2})(?::(\\d{2}))?")

// Insert puts the entry under its day: into the day already on the list, or
// under a new marker placed among the days by date. Days are newest first, and
// so is a day's entries: the entry goes above the first one earlier than it,
// and to the top of its day when it has no clock. Nothing else is moved or
// reformatted — the list is read the way it was written, and a day out of
// order stays where it is.
//
// An entry whose first line is already in its day is not added again, and
// inserted is false. A Capture's own mark is the caller's to check: it may be
// anywhere in the note, or in another file.
func Insert(content, date, marker, entry string) (updated string, inserted bool) {
	preamble, body := splitPreamble(content)
	lines := strings.Split(strings.TrimRight(strings.TrimLeft(body, "\n"), "\n"), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		lines = nil
	}
	added := strings.Split(strings.TrimRight(entry, "\n"), "\n")

	at, ok := dayAt(lines, date)
	switch {
	case ok:
		end := dayEnd(lines, at)
		key := clockKey(added[0])
		position := end
		for index := at + 1; index < end; index++ {
			if !isEntryStart(lines[index]) {
				continue
			}
			if strings.TrimRight(lines[index], " \t") == strings.TrimRight(added[0], " \t") {
				return content, false
			}
			if position == end && (key == "" || clockKey(lines[index]) != "" && clockKey(lines[index]) < key) {
				position = index
			}
		}
		lines = splice(lines, position, added)
	default:
		block := append([]string{marker}, added...)
		lines = splice(lines, newDayAt(lines, date), block)
	}
	return withPreamble(preamble, strings.Join(lines, "\n")+"\n"), true
}

// dayAt is the line of the date's marker.
func dayAt(lines []string, date string) (int, bool) {
	for index, line := range lines {
		if match := markerLine.FindStringSubmatch(strings.TrimRight(line, " ")); match != nil && match[1] == date {
			return index, true
		}
	}
	return 0, false
}

// dayEnd is the line after a day's last non-blank line, so an entry added at
// the end of a day stays clear of blank lines and the next marker.
func dayEnd(lines []string, marker int) int {
	end := len(lines)
	for index := marker + 1; index < len(lines); index++ {
		if markerLine.MatchString(strings.TrimRight(lines[index], " ")) {
			end = index
			break
		}
	}
	for end > marker+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// newDayAt is where a day not yet on the list goes: above the first day older
// than it, after the last day when every one is newer, and at the top of a
// list with no days at all.
func newDayAt(lines []string, date string) int {
	last := -1
	for index, line := range lines {
		match := markerLine.FindStringSubmatch(strings.TrimRight(line, " "))
		if match == nil {
			continue
		}
		if match[1] < date {
			return index
		}
		last = index
	}
	if last < 0 {
		return 0
	}
	return dayEnd(lines, last)
}

func isEntryStart(line string) bool {
	return strings.HasPrefix(line, "- ") && !markerLine.MatchString(strings.TrimRight(line, " "))
}

// clockKey is the entry's clock as "HH:MM:SS", which sorts as text, or empty
// when the entry has none.
func clockKey(line string) string {
	match := entryTime.FindStringSubmatch(line)
	if match == nil {
		return ""
	}
	hour, seconds := match[1], match[3]
	if len(hour) == 1 {
		hour = "0" + hour
	}
	if seconds == "" {
		seconds = "00"
	}
	return hour + ":" + match[2] + ":" + seconds
}

func splice(lines []string, at int, added []string) []string {
	out := make([]string, 0, len(lines)+len(added))
	out = append(out, lines[:at]...)
	out = append(out, added...)
	return append(out, lines[at:]...)
}
