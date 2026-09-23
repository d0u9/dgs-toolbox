// Package pagerange reads and writes the page lists a split scan is described
// with: "1-3", "2-5", "6", "7-10". It is the notation a person types and the
// notation a sidecar stores, so a hand-edited sidecar and the intake page mean
// the same thing by the same text.
//
// Ranges may overlap. A covering letter that belongs to two documents is a
// real case, so nothing here treats an overlap as an error.
package pagerange

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Range is the pages From to To, both inclusive, counting from 1.
type Range struct {
	From int
	To   int
}

// String writes one range: "6" for a single page, "1-3" otherwise.
func (r Range) String() string {
	if r.From == r.To {
		return strconv.Itoa(r.From)
	}
	return fmt.Sprintf("%d-%d", r.From, r.To)
}

// Parse reads a comma-separated list such as "1-3, 6". Empty text is no
// ranges at all. The ranges are returned in the order written, because the
// order a person lists pages in can be the reading order, and normalising it
// away would lose that.
func Parse(text string) ([]Range, error) {
	var out []Range
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		from, to, isRange := strings.Cut(part, "-")
		first, err := page(from, part)
		if err != nil {
			return nil, err
		}
		last := first
		if isRange {
			if last, err = page(to, part); err != nil {
				return nil, err
			}
		}
		if last < first {
			return nil, fmt.Errorf("page range %q runs backwards", part)
		}
		out = append(out, Range{From: first, To: last})
	}
	return out, nil
}

func page(text, part string) (int, error) {
	number, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || number < 1 {
		return 0, fmt.Errorf("page range %q: pages count from 1", part)
	}
	return number, nil
}

// Format writes ranges back as Parse reads them.
func Format(ranges []Range) string {
	parts := make([]string, len(ranges))
	for i, r := range ranges {
		parts[i] = r.String()
	}
	return strings.Join(parts, ",")
}

// Within reports the first range that reaches past the last page, if any.
func Within(ranges []Range, last int) error {
	for _, r := range ranges {
		if r.To > last {
			return fmt.Errorf("page range %s is past the last page, %d", r, last)
		}
	}
	return nil
}

// Pages lists every page the ranges name, once each, in order.
func Pages(ranges ...[]Range) []int {
	seen := map[int]struct{}{}
	for _, list := range ranges {
		for _, r := range list {
			for p := r.From; p <= r.To; p++ {
				seen[p] = struct{}{}
			}
		}
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// Compact turns a set of pages into the fewest ranges that name them, in
// order. It is how a set of pages built one click at a time is written down.
func Compact(pages []int) []Range {
	sorted := append([]int(nil), pages...)
	sort.Ints(sorted)
	var out []Range
	for _, p := range sorted {
		if n := len(out); n > 0 && p <= out[n-1].To+1 {
			if p > out[n-1].To {
				out[n-1].To = p
			}
			continue
		}
		out = append(out, Range{From: p, To: p})
	}
	return out
}

// Missing is every page from 1 to last that no range names.
func Missing(last int, ranges ...[]Range) []int {
	named := map[int]struct{}{}
	for _, p := range Pages(ranges...) {
		named[p] = struct{}{}
	}
	var out []int
	for p := 1; p <= last; p++ {
		if _, found := named[p]; !found {
			out = append(out, p)
		}
	}
	return out
}
