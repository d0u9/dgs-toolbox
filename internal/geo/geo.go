// Package geo holds the geographic primitives every geo algorithm builds on.
// It knows nothing about files, the TUI or the web page, so any command can
// reuse it.
package geo

import "math"

// EarthRadius is the mean Earth radius in metres used for distances.
const EarthRadius = 6371008.8

// LatLon is a WGS-84 position in degrees.
type LatLon struct {
	Lat float64
	Lon float64
}

// Distance is the great-circle distance between a and b in metres (haversine).
// It is accurate to about 0.5% — far below GPS error — and cheap enough for
// every pair of neighbouring track points.
func Distance(a, b LatLon) float64 {
	lat1, lat2 := radians(a.Lat), radians(b.Lat)
	dLat := lat2 - lat1
	dLon := radians(b.Lon - a.Lon)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * EarthRadius * math.Asin(math.Min(1, math.Sqrt(h)))
}

// PathLength is the distance in metres along a path through the positions in
// order.
func PathLength(path []LatLon) float64 {
	total := 0.0
	for i := 1; i < len(path); i++ {
		total += Distance(path[i-1], path[i])
	}
	return total
}

// Bounds is the smallest box holding a set of positions.
type Bounds struct {
	Min LatLon
	Max LatLon
}

// Extend grows b to hold p. The zero Bounds holds nothing; use Empty to start.
func (b Bounds) Extend(p LatLon) Bounds {
	b.Min.Lat, b.Min.Lon = math.Min(b.Min.Lat, p.Lat), math.Min(b.Min.Lon, p.Lon)
	b.Max.Lat, b.Max.Lon = math.Max(b.Max.Lat, p.Lat), math.Max(b.Max.Lon, p.Lon)
	return b
}

// Empty is a Bounds that any position extends.
func Empty() Bounds {
	return Bounds{Min: LatLon{math.Inf(1), math.Inf(1)}, Max: LatLon{math.Inf(-1), math.Inf(-1)}}
}

// IsEmpty reports whether no position has extended b.
func (b Bounds) IsEmpty() bool { return b.Min.Lat > b.Max.Lat }

func radians(degrees float64) float64 { return degrees * math.Pi / 180 }
