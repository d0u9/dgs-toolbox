// Package clean runs a track's cleaning filters in order and says, for every
// sample, whether it was kept, which rule removed it, and where it ended up.
// The source samples are never changed: the result is laid over them. It reads
// no files and draws nothing.
package clean

import (
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/hampel"
	"dgs-toolbox/internal/geo/kalman"
	"dgs-toolbox/internal/geo/spikes"
	"dgs-toolbox/internal/geo/stops"
	"dgs-toolbox/internal/geo/track"
)

// Rule names what removed a sample.
type Rule string

const (
	Kept   Rule = ""
	Manual Rule = "manual" // removed by hand
	Spike  Rule = "spike"  // spikes.Detect
	Drift  Rule = "drift"  // hampel.Detect
	Stop   Rule = "stop"   // collapsed into a stop's centre
)

// Params are every filter's switch and settings, and the removals made by
// hand. The zero value cleans nothing.
type Params struct {
	Spikes SpikesParams `json:"spikes"`
	Drift  DriftParams  `json:"drift"`
	Smooth SmoothParams `json:"smooth"`
	Stops  StopsParams  `json:"stops"`
	// Edits are the removals made by hand, oldest first: the history undo
	// walks back, and each can be restored on its own.
	Edits []Edit `json:"edits"`
	// Removed and Ranges are how sidecars before edits recorded removals by
	// hand. They are still read; Normalize turns them into edits.
	Removed []int   `json:"removed,omitempty"`
	Ranges  []Range `json:"ranges,omitempty"`
}

// Range is a run of samples removed by hand, as sidecars before edits kept it.
type Range struct {
	First int  `json:"first"`
	Last  int  `json:"last"`
	Join  bool `json:"join"`
}

// EditKind says how a removal by hand was made.
type EditKind string

const (
	EditRange EditKind = "range" // a stretch chosen on the timeline or the track
	EditStop  EditKind = "stop"  // every point of a stop
	EditLasso EditKind = "lasso" // points drawn around on the map
)

// Edit is one removal by hand. A range or stop removes the samples first to
// last; a lasso removes Points. Join says whether the samples either side are
// joined, as a straight line; otherwise the track breaks there, like a pause in
// the recording. A lasso always joins. At is when it was made, Unix
// milliseconds, for the history.
type Edit struct {
	Kind   EditKind `json:"kind"`
	First  int      `json:"first,omitempty"`
	Last   int      `json:"last,omitempty"`
	Points []int    `json:"points,omitempty"`
	Join   bool     `json:"join"`
	At     int64    `json:"at,omitempty"`
}

// Normalize moves removals recorded the old way into edits, so everything
// after reads only Edits: the single points as one lasso edit, each range as a
// range edit.
func (p Params) Normalize() Params {
	if len(p.Removed) > 0 {
		p.Edits = append([]Edit{{Kind: EditLasso, Points: p.Removed, Join: true}}, p.Edits...)
	}
	for _, r := range p.Ranges {
		p.Edits = append(p.Edits, Edit{Kind: EditRange, First: r.First, Last: r.Last, Join: r.Join})
	}
	p.Removed, p.Ranges = nil, nil
	return p
}

// indices are the samples an edit removes, within a line of n.
func (e Edit) indices(n int) []int {
	if e.Kind == EditLasso {
		return e.Points
	}
	var out []int
	for i := max(0, e.First); i <= min(n-1, e.Last); i++ {
		out = append(out, i)
	}
	return out
}

// end is the last sample an edit removes.
func (e Edit) end() int {
	if e.Kind != EditLasso {
		return e.Last
	}
	last := -1
	for _, i := range e.Points {
		last = max(last, i)
	}
	return last
}

type SpikesParams struct {
	Enabled bool `json:"enabled"`
	spikes.Params
}

type DriftParams struct {
	Enabled bool `json:"enabled"`
	hampel.Params
}

type SmoothParams struct {
	Enabled bool `json:"enabled"`
	kalman.Params
}

// StopsParams collapse the scribble inside each stop, found with these
// stay-point thresholds.
type StopsParams struct {
	Enabled  bool    `json:"enabled"`
	Distance float64 `json:"distance"` // metres
	Duration float64 `json:"duration"` // seconds
}

// Defaults are every filter's documented defaults, all switched off.
func Defaults() Params {
	stopDefaults := stops.Defaults()
	return Params{
		Spikes: SpikesParams{Params: spikes.Defaults()},
		Drift:  DriftParams{Params: hampel.Defaults()},
		Smooth: SmoothParams{Params: kalman.Defaults()},
		Stops:  StopsParams{Distance: stopDefaults.Distance, Duration: stopDefaults.Duration.Seconds()},
	}
}

// Active reports whether the params change anything.
func (p Params) Active() bool {
	return p.Spikes.Enabled || p.Drift.Enabled || p.Smooth.Enabled || p.Stops.Enabled || len(p.Edits) > 0 || len(p.Removed) > 0 || len(p.Ranges) > 0
}

// Result is the cleaning laid over a line: one entry per source sample.
type Result struct {
	Removed   []Rule       // Kept, or the rule that removed the sample
	Positions []geo.LatLon // where each kept sample now is; removed ones stay put
	Moved     []bool       // whether a kept sample's position changed
	Breaks    []bool       // whether a kept sample starts a new segment after a broken range
}

// Counts is how many samples each rule removed, and how many were moved.
type Counts struct {
	Manual int `json:"manual"`
	Spike  int `json:"spike"`
	Drift  int `json:"drift"`
	Stop   int `json:"stop"`
	Moved  int `json:"moved"`
}

// Counts tallies a result.
func (r Result) Counts() Counts {
	var c Counts
	for i, rule := range r.Removed {
		switch rule {
		case Manual:
			c.Manual++
		case Spike:
			c.Spike++
		case Drift:
			c.Drift++
		case Stop:
			c.Stop++
		}
		if r.Moved[i] {
			c.Moved++
		}
	}
	return c
}

// Kept is the cleaned line — kept samples at their new positions — and the
// source index of each of its samples. Its segments are numbered afresh from
// 0: a new one starts where the recording's segment changes and after a
// broken range, so distance and speed do not bridge either.
func (r Result) Kept(line track.Line) (track.Line, []int) {
	var kept track.Line
	var index []int
	segment, previous := -1, -1
	for i, sample := range line {
		if r.Removed[i] != Kept {
			continue
		}
		if previous < 0 || sample.Segment != previous || (r.Breaks != nil && r.Breaks[i]) {
			segment++
		}
		previous = sample.Segment
		sample.LatLon = r.Positions[i]
		sample.Segment = segment
		kept = append(kept, sample)
		index = append(index, i)
	}
	return kept, index
}

// Run cleans a line. Samples removed by hand go first, so they cannot sway a
// filter; then spikes, drift, smoothing and stop collapse, each seeing only
// what the ones before kept, and the smoothed positions.
func Run(line track.Line, p Params) Result {
	n := len(line)
	result := Result{Removed: make([]Rule, n), Positions: make([]geo.LatLon, n), Moved: make([]bool, n), Breaks: make([]bool, n)}
	for i, sample := range line {
		result.Positions[i] = sample.LatLon
	}
	keep := func() []bool {
		k := make([]bool, n)
		for i := range k {
			k[i] = result.Removed[i] == Kept
		}
		return k
	}
	remove := func(indices []int, rule Rule) {
		for _, i := range indices {
			if i >= 0 && i < n && result.Removed[i] == Kept {
				result.Removed[i] = rule
			}
		}
	}

	p = p.Normalize()
	for _, edit := range p.Edits {
		remove(edit.indices(n), Manual)
	}
	if p.Spikes.Enabled {
		remove(spikes.Detect(line, keep(), p.Spikes.Params), Spike)
	}
	if p.Drift.Enabled {
		remove(hampel.Detect(line, keep(), p.Drift.Params), Drift)
	}
	if p.Smooth.Enabled {
		smoothed := kalman.Smooth(line, keep(), p.Smooth.Params)
		for i := range line {
			if result.Removed[i] == Kept && smoothed[i] != line[i].LatLon {
				result.Positions[i], result.Moved[i] = smoothed[i], true
			}
		}
	}
	if p.Stops.Enabled {
		collapseStops(line, &result, p.Stops)
	}
	// A broken edit breaks before the first sample kept after it, whichever
	// rule removed the ones in between.
	for _, edit := range p.Edits {
		if edit.Join || edit.Kind == EditLasso {
			continue
		}
		for i := max(0, edit.end()+1); i < n; i++ {
			if result.Removed[i] == Kept {
				result.Breaks[i] = true
				break
			}
		}
	}
	return result
}

// collapseStops keeps each stop's entry and exit samples and one sample at its
// centre — the one recorded midway — and removes the rest of it.
func collapseStops(line track.Line, result *Result, p StopsParams) {
	kept, index := result.Kept(line)
	found := stops.Detect(kept, stops.Params{Distance: p.Distance, Duration: secondsToDuration(p.Duration)})
	for _, stop := range found {
		if stop.Last-stop.First < 2 {
			continue
		}
		middle := (stop.First + stop.Last) / 2
		for k := stop.First + 1; k < stop.Last; k++ {
			i := index[k]
			if k == middle {
				result.Positions[i], result.Moved[i] = stop.Center, true
				continue
			}
			result.Removed[i] = Stop
			result.Moved[i] = false
			result.Positions[i] = line[i].LatLon
		}
	}
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
