package spikes

import (
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/track"
)

const metresPerDegree = 111195.08

var start = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// walk heads north at 1.5 m/s, one sample a second.
func walk(n int) track.Line {
	line := make(track.Line, n)
	for i := range line {
		line[i] = track.Sample{
			LatLon: geo.LatLon{Lat: 30 + float64(i)*1.5/metresPerDegree, Lon: 120},
			Time:   start.Add(time.Duration(i) * time.Second),
		}
	}
	return line
}

func eastBy(p geo.LatLon, metres float64) geo.LatLon {
	return geo.LatLon{Lat: p.Lat, Lon: p.Lon + metres/(metresPerDegree*0.866)}
}

func TestDetectFindsAFarFix(t *testing.T) {
	line := walk(20)
	line[10].LatLon = eastBy(line[10].LatLon, 2000)
	if got := Detect(line, nil, Defaults()); len(got) != 1 || got[0] != 10 {
		t.Fatalf("flagged %v", got)
	}
}

func TestDetectFindsANearOutAndBackWithoutTime(t *testing.T) {
	line := walk(20)
	for i := range line {
		line[i].Time = time.Time{}
	}
	line[10].LatLon = eastBy(line[10].LatLon, 40)
	if got := Detect(line, nil, Defaults()); len(got) != 1 || got[0] != 10 {
		t.Fatalf("flagged %v", got)
	}
}

func TestDetectRemovesABurst(t *testing.T) {
	line := walk(30)
	line[10].LatLon = eastBy(line[10].LatLon, 3000)
	line[11].LatLon = eastBy(line[11].LatLon, 3000)
	got := Detect(line, nil, Defaults())
	if len(got) != 2 || got[0] != 10 || got[1] != 11 {
		t.Fatalf("flagged %v", got)
	}
}

func TestDetectKeepsRealTurnsAndSteadyFlight(t *testing.T) {
	line := walk(20)
	// Walk back the way we came: a real U-turn, neighbours as far apart as usual.
	for i := 10; i < 20; i++ {
		line[i].LatLon = line[20-i].LatLon
	}
	if got := Detect(line, nil, Defaults()); len(got) != 0 {
		t.Fatalf("U-turn flagged %v", got)
	}
	flight := walk(20)
	for i := range flight {
		flight[i].LatLon = geo.LatLon{Lat: 30 + float64(i)*200/metresPerDegree, Lon: 120}
	}
	if got := Detect(flight, nil, Params{MaxSpeed: 300, MaxAcceleration: 15, TurnAngle: 150, MinJump: 30}); len(got) != 0 {
		t.Fatalf("flight flagged %v", got)
	}
}

func TestDetectRespectsKeepAndSegments(t *testing.T) {
	line := walk(20)
	line[10].LatLon = eastBy(line[10].LatLon, 2000)
	keep := make([]bool, len(line))
	for i := range keep {
		keep[i] = i != 10
	}
	if got := Detect(line, keep, Defaults()); len(got) != 0 {
		t.Fatalf("removed sample flagged again: %v", got)
	}
	for i := 10; i < 20; i++ {
		line[i].Segment = 1
	}
	if got := Detect(line, nil, Defaults()); len(got) != 0 {
		t.Fatalf("first sample of a segment has no previous neighbour: %v", got)
	}
}
