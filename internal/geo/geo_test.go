package geo

import (
	"math"
	"testing"
)

func TestDistance(t *testing.T) {
	// One degree of latitude is about 111.2 km.
	if got := Distance(LatLon{30, 120}, LatLon{31, 120}); math.Abs(got-111195) > 50 {
		t.Fatalf("1° latitude = %.0f m", got)
	}
	if got := Distance(LatLon{30, 120}, LatLon{30, 120}); got != 0 {
		t.Fatalf("same point = %v", got)
	}
}

func TestBounds(t *testing.T) {
	b := Empty()
	if !b.IsEmpty() {
		t.Fatal("Empty is not empty")
	}
	b = b.Extend(LatLon{30, 121}).Extend(LatLon{29, 120})
	if b.Min != (LatLon{29, 120}) || b.Max != (LatLon{30, 121}) {
		t.Fatalf("bounds = %+v", b)
	}
}

func TestPathLength(t *testing.T) {
	a, b := LatLon{Lat: 30, Lon: 120}, LatLon{Lat: 30.01, Lon: 120}
	if got := PathLength([]LatLon{a, b, a}); math.Abs(got-2*Distance(a, b)) > 1e-9 {
		t.Fatalf("PathLength = %v", got)
	}
	if PathLength(nil) != 0 || PathLength([]LatLon{a}) != 0 {
		t.Fatal("a path of fewer than two positions has length")
	}
}
