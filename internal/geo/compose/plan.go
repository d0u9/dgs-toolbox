package compose

import (
	"errors"
	"fmt"
	"math"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
)

// PlannedSource is the <src> of a point of a route planned by hand.
const PlannedSource = "dgs-toolbox: planned route"

// Ways a leg of a planned route can go: along the road for a router profile,
// or in a straight line.
const WayLine = "line"

// Plan is a route drawn by hand rather than recorded: waypoints, and between
// each two a leg that follows the road or goes straight. Positions are
// [lon, lat] in WGS-84.
type Plan struct {
	Waypoints [][2]float64 `json:"waypoints"`
	Legs      []Leg        `json:"legs"` // one fewer than the waypoints
}

// Leg is the way between two waypoints and the points it takes, from the
// first waypoint to the second.
type Leg struct {
	Way   string       `json:"way"` // "car", "bike", "foot" or "line"
	Route [][2]float64 `json:"route"`
}

// Check says whether a plan can be written: two waypoints or more, a leg
// between each two with known ways, and every position on the globe.
func (p Plan) Check(ways map[string]string) error {
	if len(p.Waypoints) < 2 {
		return errors.New("a route needs two waypoints or more")
	}
	if len(p.Legs) != len(p.Waypoints)-1 {
		return errors.New("a route needs one leg between each two waypoints")
	}
	for _, w := range p.Waypoints {
		if !onGlobe(w) {
			return errors.New("a waypoint is off the globe")
		}
	}
	for i, leg := range p.Legs {
		if _, ok := ways[leg.Way]; !ok && leg.Way != WayLine {
			return fmt.Errorf("leg %d: unknown way %q", i+1, leg.Way)
		}
		if len(leg.Route) < 2 || len(leg.Route) > 200000 {
			return fmt.Errorf("leg %d has no route; route it again", i+1)
		}
		for _, pt := range leg.Route {
			if !onGlobe(pt) {
				return fmt.Errorf("leg %d holds a position off the globe", i+1)
			}
		}
	}
	return nil
}

func onGlobe(p [2]float64) bool {
	return p[1] >= -90 && p[1] <= 90 && p[0] >= -180 && p[0] <= 180 && !math.IsNaN(p[0]) && !math.IsNaN(p[1])
}

// Track is the plan as one GPX track: its legs joined, each joint written
// once.
func (p Plan) Track(name string) gpxfile.Track {
	var seg gpxfile.Segment
	for i, leg := range p.Legs {
		route := leg.Route
		if i > 0 && len(route) > 0 {
			route = route[1:]
		}
		for _, pt := range route {
			seg.Points = append(seg.Points, gpxfile.Point{LatLon: geo.LatLon{Lat: pt[1], Lon: pt[0]}, Source: PlannedSource})
		}
	}
	return gpxfile.Track{Name: name, Segments: []gpxfile.Segment{seg}}
}

// Route is the plan's waypoints as a GPX route.
func (p Plan) Route(name string) gpxfile.Route {
	rte := gpxfile.Route{Name: name}
	for _, w := range p.Waypoints {
		rte.Points = append(rte.Points, gpxfile.Point{LatLon: geo.LatLon{Lat: w[1], Lon: w[0]}})
	}
	return rte
}
