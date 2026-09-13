// Package compose lays what was brought into a GPX file over its recording:
// tracks added from other files, appended after the file's own, and stretches
// filled in along the road, inserted between two points of a track. Point
// indices everywhere else — cleaning edits, cuts, names — count the composed
// line, so they are shifted whenever something is inserted or taken out. It
// reads no files and draws nothing.
package compose

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/track"
)

// FilledSource is the <src> of a point filled in along the road.
const FilledSource = "dgs-toolbox: filled along the road (OSRM)"

// Fill replaces the points strictly between First and Last with a route. The
// route's points are inserted right after First, at First+1 to
// First+len(Route), so Last already counts them; the recorded points left
// between the route and Last are replaced.
type Fill struct {
	First   int          `json:"first"`
	Last    int          `json:"last"`
	Profile string       `json:"profile"` // the router's profile: "car", "bike" or "foot"
	Route   [][2]float64 `json:"route"`   // [lon, lat], WGS-84
	At      int64        `json:"at,omitempty"`
}

// Inserted is the range of composed indices the route's points take.
func (f Fill) Inserted() (first, last int) { return f.First + 1, f.First + len(f.Route) }

// Added is a track brought from another GPX file, as that file's cleaning
// left it. Added tracks follow the file's own tracks, in the order added.
type Added struct {
	Name     string         `json:"name"`
	From     string         `json:"from"` // the file it was taken from
	At       int64          `json:"at,omitempty"`
	Segments [][]AddedPoint `json:"segments"`
}

// AddedPoint is one point of an added track, with only what GPX gave it.
type AddedPoint struct {
	Lon        float64  `json:"lon"`
	Lat        float64  `json:"lat"`
	Elevation  *float64 `json:"ele,omitempty"`
	Time       *int64   `json:"time,omitempty"` // Unix milliseconds
	HDOP       *float64 `json:"hdop,omitempty"`
	Satellites *int     `json:"sat,omitempty"`
	Source     string   `json:"src,omitempty"`
}

// Track is an added track as a GPX track.
func (a Added) Track() gpxfile.Track {
	trk := gpxfile.Track{Name: a.Name}
	for _, seg := range a.Segments {
		var out gpxfile.Segment
		for _, pt := range seg {
			point := gpxfile.Point{LatLon: geo.LatLon{Lat: pt.Lat, Lon: pt.Lon}, Source: pt.Source}
			if pt.Elevation != nil {
				point.Elevation, point.HasElevation = *pt.Elevation, true
			}
			if pt.Time != nil {
				point.Time = time.UnixMilli(*pt.Time).UTC()
			}
			if pt.HDOP != nil {
				point.HDOP, point.HasHDOP = *pt.HDOP, true
			}
			if pt.Satellites != nil {
				point.Satellites, point.HasSatellites = *pt.Satellites, true
			}
			out.Points = append(out.Points, point)
		}
		trk.Segments = append(trk.Segments, out)
	}
	return trk
}

// AddedFrom turns a GPX track into an added track.
func AddedFrom(trk gpxfile.Track, from string, at int64) Added {
	added := Added{Name: trk.Name, From: from, At: at}
	for _, seg := range trk.Segments {
		var out []AddedPoint
		for _, pt := range seg.Points {
			point := AddedPoint{Lon: pt.Lon, Lat: pt.Lat, Source: pt.Source}
			if pt.HasElevation {
				point.Elevation = ptr(pt.Elevation)
			}
			if !pt.Time.IsZero() {
				point.Time = ptr(pt.Time.UnixMilli())
			}
			if pt.HasHDOP {
				point.HDOP = ptr(pt.HDOP)
			}
			if pt.HasSatellites {
				point.Satellites = ptr(pt.Satellites)
			}
			out = append(out, point)
		}
		if len(out) > 0 {
			added.Segments = append(added.Segments, out)
		}
	}
	return added
}

func ptr[T any](value T) *T { return &value }

// Result is a composed line and, per sample, what it is.
type Result struct {
	Line track.Line
	// Filled marks the samples a fill inserted; Replaced the recorded samples
	// a fill stands in for.
	Filled   []bool
	Replaced []bool
}

// Build composes a file: its own tracks, then the added ones, joined into one
// line, with every fill's route inserted. Fills are refused when they
// overlap or fall outside the line, as an edited source file can make them.
func Build(file *gpxfile.File, added []Added, fills []Fill) (Result, error) {
	composed := *file
	composed.Tracks = append(append([]gpxfile.Track{}, file.Tracks...), addedTracks(added)...)
	recorded := track.FromGPX(&composed)

	ordered := append([]Fill{}, fills...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].First < ordered[j].First })
	total := len(recorded)
	for _, fill := range ordered {
		total += len(fill.Route)
	}
	result := Result{Line: make(track.Line, 0, total), Filled: make([]bool, total), Replaced: make([]bool, total)}
	next := 0 // the next recorded sample
	for i, fill := range ordered {
		if len(fill.Route) == 0 || fill.First < len(result.Line) || fill.Last <= fill.First+len(fill.Route) || fill.Last >= total {
			return Result{}, fmt.Errorf("fill %d–%d does not fit the track; remove it", fill.First, fill.Last)
		}
		if i > 0 && fill.First < ordered[i-1].Last {
			return Result{}, fmt.Errorf("fill %d–%d overlaps another; remove it", fill.First, fill.Last)
		}
		for len(result.Line) <= fill.First {
			if next >= len(recorded) {
				return Result{}, fmt.Errorf("fill %d–%d does not fit the track; remove it", fill.First, fill.Last)
			}
			result.Line = append(result.Line, recorded[next])
			next++
		}
		start := result.Line[fill.First]
		for _, lonLat := range fill.Route {
			result.Filled[len(result.Line)] = true
			result.Line = append(result.Line, track.Sample{
				LatLon:  geo.LatLon{Lat: lonLat[1], Lon: lonLat[0]},
				Segment: start.Segment,
				Track:   start.Track,
				Source:  FilledSource,
			})
		}
	}
	result.Line = append(result.Line, recorded[next:]...)
	if len(result.Line) != total {
		return Result{}, errors.New("fills do not fit the track")
	}
	for _, fill := range ordered {
		spread(&result, fill)
	}
	return result, nil
}

// spread gives a fill's route times by distance between its ends, and
// elevations interpolated the same way; marks the recorded samples it replaces;
// and joins the recording across it, so a fill over a pause is one segment.
func spread(result *Result, fill Fill) {
	line := result.Line
	first, last := fill.Inserted()
	for i := last + 1; i < fill.Last; i++ {
		result.Replaced[i] = true
	}
	start, end := line[fill.First], line[fill.Last]
	along := make([]float64, fill.Last-fill.First+1) // distance from start, over the route then straight to end
	for i := fill.First + 1; i <= fill.Last; i++ {
		k := i - fill.First
		switch {
		case i <= last:
			along[k] = along[k-1] + geo.Distance(line[i-1].LatLon, line[i].LatLon)
		case i == fill.Last:
			along[k] = along[last-fill.First] + geo.Distance(line[last].LatLon, end.LatLon)
		default:
			along[k] = along[k-1]
		}
	}
	total := along[len(along)-1]
	for i := first; i <= last; i++ {
		share := 0.5
		if total > 0 {
			share = along[i-fill.First] / total
		}
		sample := &line[i]
		if !start.Time.IsZero() && !end.Time.IsZero() {
			sample.Time = start.Time.Add(time.Duration(math.Round(share * float64(end.Time.Sub(start.Time)))))
		}
		if start.HasElevation && end.HasElevation {
			sample.Elevation, sample.HasElevation = start.Elevation+share*(end.Elevation-start.Elevation), true
		}
	}
	if end.Segment != start.Segment {
		old := end.Segment
		for i := fill.Last; i < len(line) && line[i].Segment == old; i++ {
			line[i].Segment = start.Segment
		}
	}
}

func addedTracks(added []Added) []gpxfile.Track {
	tracks := make([]gpxfile.Track, len(added))
	for i, a := range added {
		tracks[i] = a.Track()
	}
	return tracks
}
