package hampel

import (
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/track"
)

const metresPerDegree = 111195.08

var start = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// cycle rides north at speed metres per second, one sample a second, with
// alternating ±wobble m/s so the speeds are not all equal.
func cycle(n int, speed, wobble float64) track.Line {
	line := make(track.Line, n)
	north := 0.0
	for i := range line {
		if i > 0 {
			if i%2 == 0 {
				north += speed + wobble
			} else {
				north += speed - wobble
			}
		}
		line[i] = track.Sample{
			LatLon: geo.LatLon{Lat: 30 + north/metresPerDegree, Lon: 120},
			Time:   start.Add(time.Duration(i) * time.Second),
		}
	}
	return line
}

func TestDetectFindsDriftAheadOfThePath(t *testing.T) {
	line := cycle(60, 5, 0.3)
	// Drift 25 m ahead, then the recorder catches up with the true path.
	line[30].Lat += 25 / metresPerDegree
	got := Detect(line, nil, Defaults())
	if len(got) != 1 || got[0] != 30 {
		t.Fatalf("flagged %v", got)
	}
}

func TestDetectLeavesSteadyAndSlowingMovement(t *testing.T) {
	line := cycle(60, 5, 0.3)
	for i := 30; i < 60; i++ {
		line[i].LatLon = line[30].LatLon // stopped
	}
	if got := Detect(line, nil, Defaults()); len(got) != 0 {
		t.Fatalf("flagged %v", got)
	}
}

func TestDetectLowersTheThresholdForPoorFixes(t *testing.T) {
	line := cycle(60, 5, 0.3)
	line[30].Lat += 1.5 / metresPerDegree // a small jump, under the normal threshold
	if got := Detect(line, nil, Defaults()); len(got) != 0 {
		t.Fatalf("good fix flagged %v", got)
	}
	line[30].HDOP, line[30].HasHDOP = 8, true
	if got := Detect(line, nil, Defaults()); len(got) != 1 || got[0] != 30 {
		t.Fatalf("poor fix flagged %v", got)
	}
	noQuality := Defaults()
	noQuality.Quality = false
	if got := Detect(line, nil, noQuality); len(got) != 0 {
		t.Fatalf("quality ignored, flagged %v", got)
	}
}

func TestDetectSkipsRemovedAndUntimed(t *testing.T) {
	line := cycle(60, 5, 0.3)
	line[30].Lat += 25 / metresPerDegree
	keep := make([]bool, len(line))
	for i := range keep {
		keep[i] = i != 30
	}
	if got := Detect(line, keep, Defaults()); len(got) != 0 {
		t.Fatalf("flagged %v", got)
	}
	for i := range line {
		line[i].Time = time.Time{}
	}
	if got := Detect(line, nil, Defaults()); len(got) != 0 {
		t.Fatalf("untimed flagged %v", got)
	}
}
