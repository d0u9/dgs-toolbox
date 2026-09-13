// Package track turns recorded points into the series a track is studied by:
// distance along it, elevation, speed. It reads no files and draws nothing, so
// the browser, stop detection and cleaning all share one definition of these.
package track

import (
	"math"
	"sort"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
)

// Sample is one point of a Line. Segment numbers the recorded piece it came
// from; distance and speed never bridge two segments.
type Sample struct {
	geo.LatLon
	Elevation    float64
	HasElevation bool
	Time         time.Time
	Segment      int
	// Track numbers the <trk> of the file the sample came from.
	Track int
	// HDOP and Satellites are the recorder's own quality figures, when given.
	HDOP          float64
	HasHDOP       bool
	Satellites    int
	HasSatellites bool
	// Source is where a sample came from when it was not recorded, as GPX's
	// <src>, such as compose.FilledSource.
	Source string
}

// Line is a track's samples in recorded order.
type Line []Sample

// FromGPX joins every track and segment of a file into one Line, numbering the
// segments in file order.
func FromGPX(file *gpxfile.File) Line {
	var line Line
	segment := 0
	for trackIndex, trk := range file.Tracks {
		for _, seg := range trk.Segments {
			if len(seg.Points) == 0 {
				continue
			}
			for _, pt := range seg.Points {
				line = append(line, Sample{
					LatLon:        pt.LatLon,
					Elevation:     pt.Elevation,
					HasElevation:  pt.HasElevation,
					Time:          pt.Time,
					Segment:       segment,
					Track:         trackIndex,
					HDOP:          pt.HDOP,
					HasHDOP:       pt.HasHDOP,
					Satellites:    pt.Satellites,
					HasSatellites: pt.HasSatellites,
					Source:        pt.Source,
				})
			}
			segment++
		}
	}
	return line
}

// Distances is the distance in metres from the start of the line to each
// sample, measured along the line. The gap between segments adds nothing.
func Distances(line Line) []float64 {
	distances := make([]float64, len(line))
	for i := 1; i < len(line); i++ {
		distances[i] = distances[i-1]
		if line[i].Segment == line[i-1].Segment {
			distances[i] += geo.Distance(line[i-1].LatLon, line[i].LatLon)
		}
	}
	return distances
}

// DefaultSpeedWindow is the span Speeds averages over. Speed between two
// neighbouring points is mostly GPS noise at a 1 s recording interval; half a
// minute keeps real changes of pace while hiding that noise.
const DefaultSpeedWindow = 30 * time.Second

// Speeds is the speed in metres per second at each sample: the distance
// travelled over the window centred on it divided by the time taken. The window
// shrinks at segment edges and never crosses a segment. A sample is NaN when it
// has no time or its window holds no elapsed time.
func Speeds(line Line, distances []float64, window time.Duration) []float64 {
	speeds := make([]float64, len(line))
	half := window / 2
	start, end := 0, 0
	for i := range line {
		speeds[i] = math.NaN()
		if line[i].Time.IsZero() {
			continue
		}
		// Move the window edges along with i; both only ever advance.
		if start > i {
			start = i
		}
		for start < i && (line[start].Segment != line[i].Segment || line[start].Time.IsZero() || line[i].Time.Sub(line[start].Time) > half) {
			start++
		}
		if end < i {
			end = i
		}
		for end+1 < len(line) && line[end+1].Segment == line[i].Segment && !line[end+1].Time.IsZero() && line[end+1].Time.Sub(line[i].Time) <= half {
			end++
		}
		elapsed := line[end].Time.Sub(line[start].Time).Seconds()
		if elapsed > 0 {
			speeds[i] = (distances[end] - distances[start]) / elapsed
		}
	}
	return speeds
}

// Stats summarises a line.
type Stats struct {
	Distance float64       // metres along the line
	Duration time.Duration // earliest to latest timed sample, pauses included
	Ascent   float64       // metres climbed
	Descent  float64       // metres descended
	Start    time.Time
	End      time.Time
	Bounds   geo.Bounds
}

// ElevationThreshold is the climb Summarise ignores between counted changes,
// so recorder noise on flat ground does not add up to a hill.
const ElevationThreshold = 3.0

// Summarise measures a line whose distances were computed by Distances.
func Summarise(line Line, distances []float64) Stats {
	stats := Stats{Bounds: geo.Empty()}
	if len(line) == 0 {
		return stats
	}
	stats.Distance = distances[len(distances)-1]
	reference, haveReference := 0.0, false
	for _, sample := range line {
		stats.Bounds = stats.Bounds.Extend(sample.LatLon)
		// A file's tracks need not be in time order, as when one is added from
		// another file: the span runs from the earliest time to the latest.
		if !sample.Time.IsZero() {
			if stats.Start.IsZero() || sample.Time.Before(stats.Start) {
				stats.Start = sample.Time
			}
			if sample.Time.After(stats.End) {
				stats.End = sample.Time
			}
		}
		if !sample.HasElevation {
			continue
		}
		if !haveReference {
			reference, haveReference = sample.Elevation, true
			continue
		}
		switch change := sample.Elevation - reference; {
		case change >= ElevationThreshold:
			stats.Ascent += change
			reference = sample.Elevation
		case change <= -ElevationThreshold:
			stats.Descent -= change
			reference = sample.Elevation
		}
	}
	if !stats.Start.IsZero() {
		stats.Duration = stats.End.Sub(stats.Start)
	}
	return stats
}

// DefaultCoreTrim is the share of samples CoreBounds leaves out at each end of
// latitude and longitude: 0.2%, so a handful of stray fixes in thousands —
// a stale first fix a thousand kilometres away — cannot stretch the box, while
// a short track (under 500 samples) loses nothing.
const DefaultCoreTrim = 0.002

// CoreBounds is the box holding the bulk of a line: per axis, the samples
// between the trim and 1-trim quantiles. Use it to frame a view; use
// Summarise's Bounds when every sample must be inside.
func CoreBounds(line Line, trim float64) geo.Bounds {
	if len(line) == 0 {
		return geo.Empty()
	}
	lats := make([]float64, len(line))
	lons := make([]float64, len(line))
	for i, sample := range line {
		lats[i], lons[i] = sample.Lat, sample.Lon
	}
	sort.Float64s(lats)
	sort.Float64s(lons)
	skip := int(float64(len(line)) * trim)
	last := len(line) - 1 - skip
	return geo.Bounds{
		Min: geo.LatLon{Lat: lats[skip], Lon: lons[skip]},
		Max: geo.LatLon{Lat: lats[last], Lon: lons[last]},
	}
}
