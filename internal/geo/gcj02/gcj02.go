// Package gcj02 converts WGS-84 positions to GCJ-02, the obfuscated system
// base maps published in China use (Gaode, Tencent). A WGS-84 track drawn on
// such a map is off by up to several hundred metres until converted.
//
// The conversion is the widely published reverse-engineered one, accurate to
// about 1–2 m. It is for display only: tracks keep their WGS-84 positions.
package gcj02

import (
	"math"

	"dgs-toolbox/internal/geo"
)

// Krasovsky 1940 ellipsoid, which the conversion is defined on.
const (
	semiMajor    = 6378245.0
	eccentricity = 0.00669342162296594323
)

// InChina reports whether p lies in the rough box the conversion applies to.
// Outside it GCJ-02 equals WGS-84. The box is the conventional one and also
// covers parts of neighbouring countries.
func InChina(p geo.LatLon) bool {
	return p.Lon >= 72.004 && p.Lon <= 137.8347 && p.Lat >= 0.8293 && p.Lat <= 55.8271
}

// FromWGS84 is the GCJ-02 position of a WGS-84 one. Positions outside China
// are returned unchanged.
func FromWGS84(p geo.LatLon) geo.LatLon {
	if !InChina(p) {
		return p
	}
	x, y := p.Lon-105.0, p.Lat-35.0
	dLat := transformLat(x, y)
	dLon := transformLon(x, y)
	radLat := p.Lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - eccentricity*magic*magic
	sqrtMagic := math.Sqrt(magic)
	dLat = (dLat * 180.0) / ((semiMajor * (1 - eccentricity)) / (magic * sqrtMagic) * math.Pi)
	dLon = (dLon * 180.0) / (semiMajor / sqrtMagic * math.Cos(radLat) * math.Pi)
	return geo.LatLon{Lat: p.Lat + dLat, Lon: p.Lon + dLon}
}

// ToWGS84 is the WGS-84 position of a GCJ-02 one, found by inverting
// FromWGS84 to well under a millimetre.
func ToWGS84(p geo.LatLon) geo.LatLon {
	if !InChina(p) {
		return p
	}
	w := p
	for range 10 {
		g := FromWGS84(w)
		w.Lat -= g.Lat - p.Lat
		w.Lon -= g.Lon - p.Lon
	}
	return w
}

func transformLat(x, y float64) float64 {
	r := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	r += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	r += (20.0*math.Sin(y*math.Pi) + 40.0*math.Sin(y/3.0*math.Pi)) * 2.0 / 3.0
	r += (160.0*math.Sin(y/12.0*math.Pi) + 320*math.Sin(y*math.Pi/30.0)) * 2.0 / 3.0
	return r
}

func transformLon(x, y float64) float64 {
	r := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	r += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	r += (20.0*math.Sin(x*math.Pi) + 40.0*math.Sin(x/3.0*math.Pi)) * 2.0 / 3.0
	r += (150.0*math.Sin(x/12.0*math.Pi) + 300.0*math.Sin(x/30.0*math.Pi)) * 2.0 / 3.0
	return r
}
