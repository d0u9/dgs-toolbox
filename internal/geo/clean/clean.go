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

// Params are every filter's switch and settings, and the samples removed by
// hand. The zero value cleans nothing.
type Params struct {
	Spikes  SpikesParams `json:"spikes"`
	Drift   DriftParams  `json:"drift"`
	Smooth  SmoothParams `json:"smooth"`
	Stops   StopsParams  `json:"stops"`
	Removed []int        `json:"removed"` // source sample indices removed by hand
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
	return p.Spikes.Enabled || p.Drift.Enabled || p.Smooth.Enabled || p.Stops.Enabled || len(p.Removed) > 0
}

// Result is the cleaning laid over a line: one entry per source sample.
type Result struct {
	Removed   []Rule       // Kept, or the rule that removed the sample
	Positions []geo.LatLon // where each kept sample now is; removed ones stay put
	Moved     []bool       // whether a kept sample's position changed
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
// source index of each of its samples.
func (r Result) Kept(line track.Line) (track.Line, []int) {
	var kept track.Line
	var index []int
	for i, sample := range line {
		if r.Removed[i] != Kept {
			continue
		}
		sample.LatLon = r.Positions[i]
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
	result := Result{Removed: make([]Rule, n), Positions: make([]geo.LatLon, n), Moved: make([]bool, n)}
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

	remove(p.Removed, Manual)
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
