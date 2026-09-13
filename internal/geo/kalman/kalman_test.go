package kalman

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/track"
)

const metresPerDegree = 111195.08

var start = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// noisyWalk heads north at 1.5 m/s with Gaussian east–west noise of sigma
// metres; truth is the line without it.
func noisyWalk(n int, sigma float64) (line track.Line, truth []geo.LatLon) {
	random := rand.New(rand.NewSource(7))
	line = make(track.Line, n)
	truth = make([]geo.LatLon, n)
	for i := range line {
		truth[i] = geo.LatLon{Lat: 30 + float64(i)*1.5/metresPerDegree, Lon: 120}
		line[i] = track.Sample{
			LatLon: geo.LatLon{Lat: truth[i].Lat, Lon: 120 + random.NormFloat64()*sigma/(metresPerDegree*0.866)},
			Time:   start.Add(time.Duration(i) * time.Second),
		}
	}
	return line, truth
}

func rmsError(positions, truth []geo.LatLon) float64 {
	sum := 0.0
	for i := range positions {
		d := geo.Distance(positions[i], truth[i])
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(positions)))
}

func TestSmoothReducesJitter(t *testing.T) {
	line, truth := noisyWalk(300, 5)
	raw := make([]geo.LatLon, len(line))
	for i := range line {
		raw[i] = line[i].LatLon
	}
	smoothed := Smooth(line, nil, Defaults())
	before, after := rmsError(raw, truth), rmsError(smoothed, truth)
	if after > before/2 {
		t.Fatalf("rms error %.2f m → %.2f m, want at least halved", before, after)
	}
}

func TestSmoothFollowsATurn(t *testing.T) {
	line := make(track.Line, 120)
	for i := range line {
		north, east := float64(min(i, 60))*5, float64(max(0, i-60))*5
		line[i] = track.Sample{
			LatLon: geo.LatLon{Lat: 30 + north/metresPerDegree, Lon: 120 + east/(metresPerDegree*0.866)},
			Time:   start.Add(time.Duration(i) * time.Second),
		}
	}
	smoothed := Smooth(line, nil, Defaults())
	worst := 0.0
	for i := range line {
		worst = math.Max(worst, geo.Distance(line[i].LatLon, smoothed[i]))
	}
	if worst > 10 {
		t.Fatalf("a clean corner moved %.1f m", worst)
	}
}

func TestSmoothLeavesWhatDoesNotTakePart(t *testing.T) {
	line, _ := noisyWalk(40, 5)
	line[5].Time = time.Time{}
	keep := make([]bool, len(line))
	for i := range keep {
		keep[i] = i != 10
	}
	for i := 20; i < 40; i++ {
		line[i].Time = line[i].Time.Add(time.Hour) // a long pause starts a new run
	}
	line[39].Segment = 1 // alone in its segment
	smoothed := Smooth(line, keep, Defaults())
	for _, i := range []int{5, 10, 39} {
		if smoothed[i] != line[i].LatLon {
			t.Fatalf("sample %d moved", i)
		}
	}
	if smoothed[15] == line[15].LatLon || smoothed[25] == line[25].LatLon {
		t.Fatal("samples in runs were not smoothed")
	}
}
