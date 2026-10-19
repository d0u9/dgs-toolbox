package compose

import (
	"errors"
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
)

func joinTrack(name string, minutes ...int) gpxfile.Track {
	seg := gpxfile.Segment{}
	for _, m := range minutes {
		seg.Points = append(seg.Points, gpxfile.Point{
			LatLon: geo.LatLon{Lat: float64(m), Lon: 1},
			Time:   time.Date(2026, 1, 1, 9, m, 0, 0, time.UTC),
		})
	}
	return gpxfile.Track{Name: name, Segments: []gpxfile.Segment{seg}}
}

func TestJoinOrdersByTimeAndSharesTheJoinPoint(t *testing.T) {
	joined, err := Join([]gpxfile.Track{joinTrack("b", 10, 20), joinTrack("a", 0, 10)}, "day")
	if err != nil {
		t.Fatal(err)
	}
	if joined.Name != "day" || len(joined.Segments) != 2 {
		t.Fatalf("got %+v", joined)
	}
	if first := joined.Segments[0].Points[0].Lat; first != 0 {
		t.Fatalf("first track is not the earlier one: %v", first)
	}
	if got := len(joined.Segments[1].Points); got != 1 {
		t.Fatalf("shared point kept twice: %d points", got)
	}
}

func TestJoinRefusesOverlap(t *testing.T) {
	if _, err := Join([]gpxfile.Track{joinTrack("a", 0, 15), joinTrack("b", 10, 20)}, "x"); !errors.Is(err, ErrOverlap) {
		t.Fatalf("overlap: %v", err)
	}
}

func TestJoinKeepsGivenOrderWithoutTime(t *testing.T) {
	b, a := joinTrack("b", 10), joinTrack("a", 0)
	b.Segments[0].Points[0].Time = time.Time{}
	joined, err := Join([]gpxfile.Track{b, a}, "x")
	if err != nil || joined.Segments[0].Points[0].Lat != 10 {
		t.Fatalf("given order not kept: %+v %v", joined, err)
	}
}
