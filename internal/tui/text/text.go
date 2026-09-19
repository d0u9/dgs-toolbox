// Package text holds the small string-layout helpers more than one command
// page needs: pluralising a count, wrapping to a width, padding content to
// an exact height. None of it knows about a specific command's data or
// styling — a page's own colors and labels stay in the page.
package text

import (
	"fmt"
	"regexp"
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

// columnGap is the run of spaces that separates two columns of a tabular
// line, as opposed to the single space between words.
var columnGap = regexp.MustCompile(` {2,}`)

// Hanging wraps s to width like Wrapped, but a line that does not fit
// continues under where it began rather than at the left edge. A tabular line
// — indented, its columns separated by two or more spaces — continues under
// its last column, so a wrapped row still reads as one row of the table. Any
// other line continues under its own indent.
func Hanging(s string, width int) []string {
	width = max(1, width)
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if ansi.StringWidth(line) <= width {
			out = append(out, line)
			continue
		}
		indent := hangingIndent(line)
		if indent >= width/2 {
			indent = min(leadingSpaces(ansi.Strip(line))+2, width/2)
		}
		first, rest := wrapOnce(line, width)
		out = append(out, first)
		pad := strings.Repeat(" ", indent)
		for _, cont := range strings.Split(ansi.Wrap(rest, max(1, width-indent), ""), "\n") {
			out = append(out, pad+cont)
		}
	}
	return out
}

// hangingIndent is the column a wrapped line continues at: its last column
// when it is tabular, its own indent otherwise.
func hangingIndent(line string) int {
	plain := ansi.Strip(line)
	indent := leadingSpaces(plain)
	if indent == 0 {
		return 0
	}
	gaps := columnGap.FindAllStringIndex(strings.TrimRight(plain[indent:], " "), -1)
	if len(gaps) == 0 {
		return indent
	}
	return indent + ansi.StringWidth(plain[indent:indent+gaps[len(gaps)-1][1]])
}

func leadingSpaces(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

// wrapOnce is the first wrapped row of line and what is left after it, with
// the spaces at the break dropped.
func wrapOnce(line string, width int) (first, rest string) {
	rows := strings.SplitN(ansi.Wrap(line, width, ""), "\n", 2)
	if len(rows) < 2 {
		return rows[0], ""
	}
	return rows[0], strings.TrimLeft(rows[1], " ")
}
