package timeline

import (
	"regexp"
	"strings"
)

// A daily note is not a timeline: entries go at the end of a section, under a
// heading, in the order they were written. dgs capture and the Obsidian plugin
// both append to one, so where an entry lands is decided here, once.

// InsertUnderSection puts the entry at the end of the named section, adding the
// section at the end of the note when it has none. A section that already holds
// entries keeps them: the new one goes after the last line of that section,
// before whatever heading follows it.
//
// The section is found by its text at whatever level the note gives it: a
// note whose template says "## 去过哪里" has that section, and a second one
// added at the end would split the day's places in two. The configured level
// only decides the heading written when the note has none.
func InsertUnderSection(note, section string, entry []string) string {
	heading, _ := sectionHeading(section)
	_, wanted := headingText(heading)
	block := strings.Join(entry, "\n")
	if strings.TrimSpace(note) == "" {
		return heading + "\n\n" + block + "\n"
	}
	lines := strings.Split(strings.TrimRight(note, "\n"), "\n")

	start, level := -1, 0
	for index, line := range lines {
		if found, text := headingText(line); found > 0 && text == wanted {
			start, level = index, found
			break
		}
	}
	if start < 0 {
		return strings.Join(lines, "\n") + "\n\n" + heading + "\n\n" + block + "\n"
	}

	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		// The section ends at the next heading that is not inside it: one of
		// the same level or shallower. A deeper heading is part of what this
		// section holds, so an entry still goes after it.
		if next := headingLevel(lines[index]); next > 0 && next <= level {
			end = index
			break
		}
	}
	// Trailing blank lines belong to the gap before the next heading rather
	// than to the section, so the entry goes above them.
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	// A section holding only the daily note template's placeholder, "1. 无",
	// is empty: the first entry takes the placeholder's place rather than
	// following it.
	body := lines[start+1 : end]
	if holdsOnlyPlaceholders(body) {
		lines = append(append(append([]string{}, lines[:start+1]...), ""), lines[end:]...)
		end = start + 2
	}
	updated := append([]string{}, lines[:end]...)
	if end == start+1 {
		// A blank line between the heading and the list, as one written by
		// hand has.
		updated = append(updated, "")
	}
	updated = append(updated, entry...)
	updated = append(updated, lines[end:]...)
	return strings.Join(updated, "\n") + "\n"
}

// placeholderLine is a line a template writes into a section with nothing in
// it yet: "无", "None", "N/A" or a dash, as a list item or alone.
var placeholderLine = regexp.MustCompile(`^\s*(?:\d+[.)]|[-*+])?\s*(?:无|None|N/A|-|—)\s*$`)

// holdsOnlyPlaceholders reports whether lines have something in them and all
// of it is placeholders.
func holdsOnlyPlaceholders(lines []string) bool {
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !placeholderLine.MatchString(line) {
			return false
		}
		found = true
	}
	return found
}

// sectionHeading is the heading an entry is written under, and its level. A
// configured section may carry its own hashes — "## Captured" asks for a
// second-level heading — and otherwise it is a first-level one.
func sectionHeading(section string) (heading string, level int) {
	section = strings.TrimSpace(section)
	if level := headingLevel(section); level > 0 {
		return section, level
	}
	return "# " + section, 1
}

// headingText is a heading's level and its text, without the hashes that open
// it or the ones that may close it: "## 去过哪里 ##" is level 2, "去过哪里".
// A line that is not a heading is level 0.
func headingText(line string) (level int, text string) {
	level = headingLevel(line)
	if level == 0 {
		return 0, ""
	}
	text = strings.TrimSpace(strings.TrimLeft(strings.TrimLeft(line, " "), "#"))
	if closed := strings.TrimRight(text, "#"); closed != text && (closed == "" || strings.HasSuffix(closed, " ")) {
		text = strings.TrimSpace(closed)
	}
	return level, text
}

// headingLevel is the number of leading hashes of a Markdown heading, or 0 for
// a line that is not one.
func headingLevel(line string) int {
	trimmed := strings.TrimLeft(line, " ")
	level := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
	if level == 0 || level > 6 || !strings.HasPrefix(trimmed[level:], " ") {
		return 0
	}
	return level
}
