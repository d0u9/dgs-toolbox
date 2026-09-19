// Package tristate draws the checkbox of a row that stands for several
// checkable things at once: a group whose members may be all, some or none
// checked. See docs/tui.md#multi-choice-values.
package tristate

// Box is the checkbox for a row standing for total things, checked of which
// are checked: "[x]" when all are, "[-]" when some are, "[ ]" otherwise. A row
// standing for nothing is never checked.
func Box(total, checked int) string {
	switch {
	case total > 0 && checked == total:
		return "[x]"
	case checked > 0:
		return "[-]"
	}
	return "[ ]"
}

// Count is how many of keys are set in checked.
func Count(keys []string, checked map[string]bool) int {
	count := 0
	for _, key := range keys {
		if checked[key] {
			count++
		}
	}
	return count
}
