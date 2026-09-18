// Package text holds the small string-layout helpers more than one command
// page needs: pluralising a count, wrapping to a width, padding content to
// an exact height. None of it knows about a specific command's data or
// styling — a page's own colors and labels stay in the page.
package text

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Plural is "1 noun" or "N nouns", with the one irregular case ("-ty" ->
// "-ties") every caller of this so far has needed.
func Plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	if strings.HasSuffix(noun, "ty") {
		return fmt.Sprintf("%d %sies", count, strings.TrimSuffix(noun, "y"))
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// Wrapped breaks text into lines of at most width cells, breaking inside a
// word when it has to — a key or a path has nowhere else to break.
func Wrapped(s string, width int) []string {
	return strings.Split(ansi.Wrap(s, max(1, width), ""), "\n")
}

// Fit pads or truncates content to exactly height lines of width cells, for
// a column whose box must stay a fixed size regardless of how much its
// content has to say.
func Fit(content string, height, width int) string {
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), max(1, height))]
	for len(lines) < max(1, height) {
		lines = append(lines, strings.Repeat(" ", max(1, width)))
	}
	return strings.Join(lines, "\n")
}
