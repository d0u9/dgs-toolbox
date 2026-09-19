// Shared formatting for dgs conf inspect's detail panes. The pane is one
// column of a two-column layout, so vertical space is the scarce thing: a
// field on a line of its own costs a row whether it holds two characters or
// forty. These helpers put short facts side by side and keep a line per item
// only where the items are a list worth scanning down.
package conf

import (
	"fmt"
	"sort"
	"strings"
)

// fieldSeparator joins facts that share a line. A middle dot reads as a
// break without looking like punctuation belonging to either side.
const fieldSeparator = " · "

// fields joins the non-empty strings among its arguments onto one line.
// Callers build each argument with field, so a fact with no value drops out
// and takes its separator with it.
func fields(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, fieldSeparator)
}

// field is one labelled fact, or the empty string when there is no value to
// show. The label is dimmed by being lower case and unpunctuated: a colon
// after every label turns a line of facts into a line of syntax.
func field(label, value string) string {
	if value == "" {
		return ""
	}
	if label == "" {
		return value
	}
	return label + " " + value
}

// line writes one line of joined facts, and nothing at all when every fact
// is empty — an empty line is a row spent on nothing.
func line(b *strings.Builder, parts ...string) {
	if s := fields(parts...); s != "" {
		fmt.Fprintln(b, s)
	}
}

// pairsInline renders a small map as "key value · key value", for the case
// where a map has one or two entries and a block for it would cost three
// rows to carry one fact.
func pairsInline(m map[string]string) string {
	var names []string
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+" "+m[name])
	}
	return strings.Join(parts, fieldSeparator)
}

// section writes a heading and its rows, with one blank line before it. It
// writes nothing when there are no rows: a heading over nothing is two rows
// saying there is nothing, which the absence says better.
func section(b *strings.Builder, heading string, rows []string) {
	if len(rows) == 0 {
		return
	}
	if b.Len() > 0 {
		fmt.Fprintln(b)
	}
	fmt.Fprintln(b, heading)
	for _, r := range rows {
		fmt.Fprintln(b, "  "+r)
	}
}

// columns lays out rows of cells with each column padded to its widest
// entry, so a list reads down as well as across without a fixed width
// guessed at in every caller.
func columns(rows [][]string) []string {
	widths := map[int]int{}
	for _, r := range rows {
		for i, cell := range r {
			if i < len(r)-1 && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		var b strings.Builder
		for i, cell := range r {
			if i > 0 {
				b.WriteString("  ")
			}
			if i < len(r)-1 {
				fmt.Fprintf(&b, "%-*s", widths[i], cell)
				continue
			}
			b.WriteString(cell)
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}
