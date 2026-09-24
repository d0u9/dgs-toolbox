package organizer

import "strings"

// WorkflowProperty is the daily note property listing the workflows of the
// Captures written into it, so a vault can find the days that have any.
const WorkflowProperty = "capture_workflows"

// addToListProperty adds value to the list property key in the note's
// frontmatter, leaving every other line as it was. A value already listed
// changes nothing. Only this key is rewritten: a vault's frontmatter is the
// reader's, and reformatting the rest of it would be a change nobody asked for.
// A note without frontmatter gains one holding only this key.
func addToListProperty(note, key, value string) string {
	lines := strings.Split(note, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "---\n" + key + ":\n  - " + value + "\n---\n\n" + note
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}
	if end < 0 {
		// An opening line with no closing one is not frontmatter a reader
		// meant; it is left alone rather than guessed at.
		return note
	}

	start := -1
	for index := 1; index < end; index++ {
		if name, _, ok := strings.Cut(lines[index], ":"); ok && name == key {
			start = index
			break
		}
	}
	if start < 0 {
		updated := append([]string{}, lines[:end]...)
		updated = append(updated, key+":", "  - "+value)
		updated = append(updated, lines[end:]...)
		return strings.Join(updated, "\n")
	}

	// The key's value is either written after it — empty, one scalar or a
	// flow list — or as a block list on the indented lines that follow.
	var values []string
	_, inline, _ := strings.Cut(lines[start], ":")
	inline = strings.TrimSpace(inline)
	if strings.HasPrefix(inline, "[") && strings.HasSuffix(inline, "]") {
		for _, item := range strings.Split(inline[1:len(inline)-1], ",") {
			values = appendValue(values, item)
		}
	} else {
		values = appendValue(values, inline)
	}
	stop := start + 1
	for ; stop < end; stop++ {
		trimmed := strings.TrimSpace(lines[stop])
		if !strings.HasPrefix(trimmed, "- ") && trimmed != "-" {
			break
		}
		values = appendValue(values, strings.TrimPrefix(trimmed, "-"))
	}
	for _, existing := range values {
		if existing == value {
			return note
		}
	}
	values = append(values, value)

	block := []string{key + ":"}
	for _, item := range values {
		block = append(block, "  - "+item)
	}
	updated := append([]string{}, lines[:start]...)
	updated = append(updated, block...)
	updated = append(updated, lines[stop:]...)
	return strings.Join(updated, "\n")
}

// appendValue adds one written YAML scalar, unquoted, skipping an empty one.
func appendValue(values []string, item string) []string {
	item = strings.TrimSpace(item)
	if len(item) >= 2 && (item[0] == '"' || item[0] == '\'') && item[len(item)-1] == item[0] {
		item = item[1 : len(item)-1]
	}
	if item == "" {
		return values
	}
	return append(values, item)
}
