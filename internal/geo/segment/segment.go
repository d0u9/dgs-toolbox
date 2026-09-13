// Package segment divides a track into the pieces it is saved as: cuts are
// point indices where one segment ends and the next begins, and every stop is
// a candidate cut. It reads no files and draws nothing.
package segment

import (
	"sort"

	"dgs-toolbox/internal/geo/stops"
)

// Range is one segment: the points first to last of a line, both included.
type Range struct {
	First int `json:"first"`
	Last  int `json:"last"`
}

// Name is a segment's name, given by the point it starts at. A segment
// without one is named by whoever shows it.
type Name struct {
	Start int    `json:"start"`
	Name  string `json:"name"`
}

// Params are a track's cuts and the names given to its segments.
type Params struct {
	Cuts  []int  `json:"cuts"`
	Names []Name `json:"names"`
}

// Active reports whether the params record anything.
func (p Params) Active() bool { return len(p.Cuts) > 0 || len(p.Names) > 0 }

// Clean is the cuts sorted, without repeats, and only those strictly inside a
// line of n points: a cut at either end would make an empty segment.
func Clean(cuts []int, n int) []int {
	var inside []int
	for _, cut := range cuts {
		if cut > 0 && cut < n-1 {
			inside = append(inside, cut)
		}
	}
	sort.Ints(inside)
	out := inside[:0]
	for i, cut := range inside {
		if i == 0 || cut != inside[i-1] {
			out = append(out, cut)
		}
	}
	return out
}

// Split is the segments of a line of n points. Neighbouring segments share
// the point at their cut, so none of the track is lost between them.
func Split(n int, cuts []int) []Range {
	if n == 0 {
		return nil
	}
	var ranges []Range
	first := 0
	for _, cut := range Clean(cuts, n) {
		ranges = append(ranges, Range{First: first, Last: cut})
		first = cut
	}
	return append(ranges, Range{First: first, Last: n - 1})
}

// Candidates are the proposed cuts: the point recorded midway through each
// stop, so the rest is split between the segment arriving and the one leaving.
func Candidates(found []stops.Stop) []int {
	cuts := make([]int, len(found))
	for i, stop := range found {
		cuts[i] = (stop.First + stop.Last) / 2
	}
	return cuts
}

// NameOf is the name given to the segment starting at start, or "".
func NameOf(names []Name, start int) string {
	for _, name := range names {
		if name.Start == start {
			return name.Name
		}
	}
	return ""
}
