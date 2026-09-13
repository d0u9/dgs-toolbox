package gcj02

import (
	"math"
	"testing"

	"dgs-toolbox/internal/geo"
)

func TestFromWGS84InChina(t *testing.T) {
	// Tiananmen: the published conversion moves it about 450–500 m.
	wgs := geo.LatLon{Lat: 39.908692, Lon: 116.397477}
	got := FromWGS84(wgs)
	want := geo.LatLon{Lat: 39.910092, Lon: 116.403722}
	if math.Abs(got.Lat-want.Lat) > 5e-5 || math.Abs(got.Lon-want.Lon) > 5e-5 {
		t.Fatalf("FromWGS84 = %+v, want about %+v", got, want)
	}
	if d := geo.Distance(wgs, got); d < 400 || d > 700 {
		t.Fatalf("offset %.0f m", d)
	}
}

func TestFromWGS84OutsideChinaIsUnchanged(t *testing.T) {
	melbourne := geo.LatLon{Lat: -37.8136, Lon: 144.9631}
	if InChina(melbourne) || FromWGS84(melbourne) != melbourne {
		t.Fatal("Melbourne was converted")
	}
}
