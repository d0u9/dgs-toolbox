package stops

import (
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/track"
)

const metresPerDegree = 111195.08

var start = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// builder appends one sample a second, walking north or staying put.
type builder struct {
	line  track.Line
	north float64 // metres from the origin
}

func (b *builder) add(offsetEast float64) {
	b.line = append(b.line, track.Sample{
		LatLon: geo.LatLon{Lat: 30 + b.north/metresPerDegree, Lon: 120 + offsetEast/(metresPerDegree*0.866)},
		Time:   start.Add(time.Duration(len(b.line)) * time.Second),
	})
}

func (b *builder) walk(seconds int) {
	for range seconds {
		b.north += 1.5
		b.add(0)
	}
}

// stay wanders ±wander metres east and west around one place.
func (b *builder) stay(seconds int, wander float64) {
	for i := range seconds {
		offset := wander
		if i%2 == 0 {
			offset = -wander
		}
		b.add(offset)
	}
}

func TestDetectFindsAStay(t *testing.T) {
	var b builder
	b.walk(120)
	b.stay(600, 15)
	b.walk(120)
	got := Detect(b.line, Defaults())
	if len(got) != 1 {
		t.Fatalf("found %d stops, want 1: %+v", len(got), got)
	}
	stop := got[0]
	// Walking in, the stay starts within 50 m of the stop, about 33 s early.
	if stop.First < 80 || stop.First > 120 || stop.Last < 720 || stop.Last > 760 {
		t.Fatalf("stop spans %d–%d", stop.First, stop.Last)
	}
	if stop.Duration() < 10*time.Minute || stop.Duration() > 12*time.Minute {
		t.Fatalf("duration %v", stop.Duration())
	}
	if geo.Distance(stop.Center, geo.LatLon{Lat: 30 + 180/metresPerDegree, Lon: 120}) > 15 {
		t.Fatalf("center %+v is off the stay", stop.Center)
	}
}

func TestDetectIgnoresShortPausesAndMovement(t *testing.T) {
	var b builder
	b.walk(300)
	b.stay(120, 5)
	b.walk(300)
	if got := Detect(b.line, Defaults()); len(got) != 0 {
		t.Fatalf("found %+v", got)
	}
}

func TestDetectMergesNeighbouringStops(t *testing.T) {
	var b builder
	b.stay(400, 5)
	// Step out past the radius and straight back.
	b.north += 60
	b.stay(5, 0)
	b.north -= 60
	b.stay(400, 5)
	got := Detect(b.line, Defaults())
	if len(got) != 1 || got[0].First != 0 || got[0].Last != len(b.line)-1 {
		t.Fatalf("stops = %+v", got)
	}
}

func TestDetectSeparatesDistantStops(t *testing.T) {
	var b builder
	b.stay(400, 5)
	b.walk(200)
	b.stay(400, 5)
	if got := Detect(b.line, Defaults()); len(got) != 2 || !got[0].Departure.Before(got[1].Arrival) {
		t.Fatalf("stops = %+v", got)
	}
}

func TestDetectSpansARecordingPause(t *testing.T) {
	var b builder
	b.stay(10, 0)
	b.line[len(b.line)-1].Segment = 0
	pausedAt := len(b.line)
	b.stay(10, 0)
	for i := pausedAt; i < len(b.line); i++ {
		b.line[i].Segment = 1
		b.line[i].Time = b.line[i].Time.Add(time.Hour)
	}
	got := Detect(b.line, Defaults())
	if len(got) != 1 || got[0].Duration() < time.Hour {
		t.Fatalf("stops = %+v", got)
	}
}

func TestDetectSkipsUntimedSamples(t *testing.T) {
	var b builder
	b.stay(600, 5)
	for i := range b.line {
		if i%3 == 0 {
			b.line[i].Time = time.Time{}
		}
	}
	got := Detect(b.line, Defaults())
	if len(got) != 1 || got[0].Arrival.IsZero() || got[0].Departure.IsZero() {
		t.Fatalf("stops = %+v", got)
	}
	if got := Detect(track.Line{{}, {}, {}}, Defaults()); len(got) != 0 {
		t.Fatalf("untimed line has stops %+v", got)
	}
}
