package track

import (
	"math"
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
)

var start = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// straight builds a line heading north at metresPerSecond, one sample a second.
func straight(n int, metresPerSecond float64, segment int) Line {
	const metresPerDegree = 111195.08
	line := make(Line, n)
	for i := range line {
		line[i] = Sample{
			LatLon:  geo.LatLon{Lat: 30 + float64(i)*metresPerSecond/metresPerDegree, Lon: 120},
			Time:    start.Add(time.Duration(i) * time.Second),
			Segment: segment,
		}
	}
	return line
}

func TestSpeedsOfSteadyMovement(t *testing.T) {
	line := straight(120, 5, 0)
	speeds := Speeds(line, Distances(line), DefaultSpeedWindow)
	for i, speed := range speeds {
		if math.Abs(speed-5) > 0.05 {
			t.Fatalf("speed[%d] = %v, want 5", i, speed)
		}
	}
}

func TestSpeedsDoNotBridgeSegments(t *testing.T) {
	line := append(straight(10, 5, 0), straight(10, 5, 1)...)
	for i := 10; i < 20; i++ {
		line[i].Time = line[i].Time.Add(time.Hour)
		line[i].Lat += 1 // far away: a bridged distance would show up
	}
	distances := Distances(line)
	if distances[10] != distances[9] {
		t.Fatalf("distance bridged segments: %v -> %v", distances[9], distances[10])
	}
	for i, speed := range Speeds(line, distances, DefaultSpeedWindow) {
		if math.Abs(speed-5) > 0.05 {
			t.Fatalf("speed[%d] = %v, want 5", i, speed)
		}
	}
}

func TestSpeedsWithoutTimeAreNaN(t *testing.T) {
	line := straight(5, 5, 0)
	for i := range line {
		line[i].Time = time.Time{}
	}
	for i, speed := range Speeds(line, Distances(line), DefaultSpeedWindow) {
		if !math.IsNaN(speed) {
			t.Fatalf("speed[%d] = %v, want NaN", i, speed)
		}
	}
}

func TestSummarise(t *testing.T) {
	line := straight(101, 10, 0)
	elevations := []float64{100, 101, 99, 110, 105, 90}
	for i := range line {
		line[i].Elevation, line[i].HasElevation = elevations[i*len(elevations)/len(line)], true
	}
	stats := Summarise(line, Distances(line))
	if math.Abs(stats.Distance-1000) > 5 || stats.Duration != 100*time.Second {
		t.Fatalf("stats = %+v", stats)
	}
	// Noise under the threshold is ignored: +10 to 110, then -20 to 90.
	if stats.Ascent != 10 || stats.Descent != 20 {
		t.Fatalf("ascent %v descent %v", stats.Ascent, stats.Descent)
	}
}

func TestFromGPXNumbersSegments(t *testing.T) {
	point := gpxfile.Point{LatLon: geo.LatLon{Lat: 30, Lon: 120}}
	file := &gpxfile.File{Tracks: []gpxfile.Track{
		{Segments: []gpxfile.Segment{{Points: []gpxfile.Point{point, point}}, {}}},
		{Segments: []gpxfile.Segment{{Points: []gpxfile.Point{point}}}},
	}}
	line := FromGPX(file)
	if len(line) != 3 || line[1].Segment != 0 || line[2].Segment != 1 {
		t.Fatalf("line = %+v", line)
	}
}

func TestCoreBoundsIgnoresStrayFixes(t *testing.T) {
	line := straight(1000, 1, 0)                       // about 9 m north of 30°N, 120°E
	line[0].LatLon = geo.LatLon{Lat: 31.9, Lon: 118.8} // a stale fix far away
	core := CoreBounds(line, DefaultCoreTrim)
	if core.Min.Lat < 30 || core.Max.Lat > 30.01 || core.Min.Lon != 120 || core.Max.Lon != 120 {
		t.Fatalf("core bounds = %+v", core)
	}
	full := Summarise(line, Distances(line)).Bounds
	if full.Max.Lat != 31.9 {
		t.Fatalf("full bounds lost the stray fix: %+v", full)
	}
}

func TestCoreBoundsKeepsShortLinesWhole(t *testing.T) {
	line := straight(100, 1, 0)
	line[0].LatLon = geo.LatLon{Lat: 31.9, Lon: 118.8}
	if core := CoreBounds(line, DefaultCoreTrim); core.Max.Lat != 31.9 {
		t.Fatalf("short line was trimmed: %+v", core)
	}
}
