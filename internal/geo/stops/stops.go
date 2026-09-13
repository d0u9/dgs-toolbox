// Package stops finds where a track stayed put, by stay-point detection (Li et
// al., 2008, "Mining user similarity based on location history"). It reads no
// files and draws nothing: the browser shows its stops, and cleaning and day
// splitting build on them.
package stops

import (
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/track"
)

// DefaultDistance is the radius a stop stays within. GPS indoors or under
// trees wanders tens of metres while the recorder sits still; 50 m holds that
// wander without swallowing a slow walk.
const DefaultDistance = 50.0

// DefaultDuration is how long a stay must last to be a stop: long enough that
// traffic lights and a pause to look at the map are not stops.
const DefaultDuration = 5 * time.Minute

// Params are the two thresholds of stay-point detection.
type Params struct {
	Distance float64       // metres
	Duration time.Duration // at least this long
}

// Defaults are the documented default parameters.
func Defaults() Params {
	return Params{Distance: DefaultDistance, Duration: DefaultDuration}
}

// Stop is one place the track stayed. First and Last are the indices of its
// first and last sample in the line; Center is the mean of the samples between.
type Stop struct {
	First     int
	Last      int
	Center    geo.LatLon
	Arrival   time.Time
	Departure time.Time
}

// Duration is the time from arrival to departure.
func (s Stop) Duration() time.Duration { return s.Departure.Sub(s.Arrival) }

// Detect finds the stops of a line in time order.
//
// Starting from an anchor sample, the samples that follow within p.Distance of
// it form a stay; a stay lasting at least p.Duration is a stop, and the search
// resumes after it. Otherwise the anchor moves to the next sample. Samples
// without a time are skipped. A stay may span a pause between segments: a
// recorder switched off at a hotel and on again at its door stayed there.
//
// Neighbouring stops whose centres are closer than p.Distance are then merged,
// so one visit briefly broken by wander past the radius stays one stop.
func Detect(line track.Line, p Params) []Stop {
	timed := make([]int, 0, len(line))
	for i, sample := range line {
		if !sample.Time.IsZero() {
			timed = append(timed, i)
		}
	}
	var found []Stop
	for i := 0; i < len(timed); {
		anchor := line[timed[i]]
		j := i + 1
		for j < len(timed) && geo.Distance(anchor.LatLon, line[timed[j]].LatLon) <= p.Distance {
			j++
		}
		last := line[timed[j-1]]
		if j-1 > i && last.Time.Sub(anchor.Time) >= p.Duration {
			found = append(found, newStop(line, timed[i], timed[j-1]))
			i = j
			continue
		}
		i++
	}
	return merge(line, found, p.Distance)
}

func merge(line track.Line, found []Stop, distance float64) []Stop {
	var merged []Stop
	for _, stop := range found {
		if n := len(merged); n > 0 && geo.Distance(merged[n-1].Center, stop.Center) < distance {
			merged[n-1] = newStop(line, merged[n-1].First, stop.Last)
			continue
		}
		merged = append(merged, stop)
	}
	return merged
}

// newStop measures the stop holding samples first to last. Both are timed.
func newStop(line track.Line, first, last int) Stop {
	var lat, lon float64
	n := 0
	for _, sample := range line[first : last+1] {
		if sample.Time.IsZero() {
			continue
		}
		lat += sample.Lat
		lon += sample.Lon
		n++
	}
	return Stop{
		First:     first,
		Last:      last,
		Center:    geo.LatLon{Lat: lat / float64(n), Lon: lon / float64(n)},
		Arrival:   line[first].Time,
		Departure: line[last].Time,
	}
}
