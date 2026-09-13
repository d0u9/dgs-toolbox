package compose

import (
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
)

func walk() *gpxfile.File {
	at := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	point := func(k int, lat float64) gpxfile.Point {
		return gpxfile.Point{LatLon: geo.LatLon{Lat: lat, Lon: 120}, Time: at.Add(time.Duration(k) * time.Minute), Elevation: float64(k * 10), HasElevation: true}
	}
	// Two recorded segments, the second after a tunnel.
	return &gpxfile.File{Tracks: []gpxfile.Track{{Segments: []gpxfile.Segment{
		{Points: []gpxfile.Point{point(0, 30), point(1, 30.001), point(2, 30.002)}},
		{Points: []gpxfile.Point{point(10, 30.010), point(11, 30.011)}},
	}}}}
}

func TestBuildInsertsAFillAndJoinsAcrossIt(t *testing.T) {
	// From point 1 to the first point after the tunnel (recorded index 3), with
	// point 2 replaced: the route's two points go at 2 and 3, so the end is at 5.
	fill := Fill{First: 1, Last: 5, Route: [][2]float64{{120, 30.004}, {120, 30.007}}}
	result, err := Build(walk(), nil, []Fill{fill})
	if err != nil {
		t.Fatal(err)
	}
	line := result.Line
	if len(line) != 7 || !result.Filled[2] || !result.Filled[3] || !result.Replaced[4] || result.Replaced[5] {
		t.Fatalf("filled %v replaced %v", result.Filled, result.Replaced)
	}
	if line[2].Source != FilledSource || line[5].Lat != 30.010 {
		t.Fatalf("line %+v", line)
	}
	// Times and elevation spread by distance between 08:01 and 08:10.
	if !line[2].Time.After(line[1].Time) || !line[3].Time.After(line[2].Time) || !line[3].Time.Before(line[5].Time) {
		t.Fatalf("times %v %v %v %v", line[1].Time, line[2].Time, line[3].Time, line[5].Time)
	}
	if !line[3].HasElevation || line[3].Elevation <= line[2].Elevation || line[3].Elevation >= 100 {
		t.Fatalf("elevation %v", line[3].Elevation)
	}
	for i := 1; i < len(line); i++ {
		if line[i].Segment != line[0].Segment {
			t.Fatalf("sample %d is in segment %d: the fill should join the tunnel", i, line[i].Segment)
		}
	}
}

func TestBuildAppendsAddedTracks(t *testing.T) {
	file := walk()
	added := AddedFrom(file.Tracks[0], "/trips/phone.gpx", 1)
	result, err := Build(file, []Added{added}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Line) != 10 || result.Line[5].Track != 1 || !result.Line[9].Time.Equal(file.Tracks[0].Segments[1].Points[1].Time) {
		t.Fatalf("line %+v", result.Line)
	}
	if len(file.Tracks) != 1 {
		t.Fatal("Build changed the file")
	}
}

func TestBuildRefusesAFillThatDoesNotFit(t *testing.T) {
	if _, err := Build(walk(), nil, []Fill{{First: 3, Last: 9, Route: [][2]float64{{120, 30}}}}); err == nil {
		t.Fatal("fill past the end accepted")
	}
	overlapping := []Fill{
		{First: 0, Last: 3, Route: [][2]float64{{120, 30}}},
		{First: 2, Last: 5, Route: [][2]float64{{120, 30}}},
	}
	if _, err := Build(walk(), nil, overlapping); err == nil {
		t.Fatal("overlapping fills accepted")
	}
}
