package gpx

import (
	"context"
	"errors"
	"strings"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/amap"
	"dgs-toolbox/internal/geo/osrm"
)

// amapPrefix names the ways Amap routes: "amap-car", "amap-foot" and so on.
const amapPrefix = "amap-"

// wayJSON is a way a stretch can be routed, as the page offers it.
type wayJSON struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Service string `json:"service"`
}

// ways are the ways of travel the router offers: OSRM's always, Amap's when a
// key is configured.
func (a api) ways() []wayJSON {
	list := []wayJSON{
		{ID: "car", Label: "Car", Service: "OSRM"},
		{ID: "bike", Label: "Bicycle", Service: "OSRM"},
		{ID: "foot", Label: "Foot", Service: "OSRM"},
	}
	if a.settings.AmapKey != "" || a.settings.Amap != nil {
		list = append(list,
			wayJSON{ID: amapPrefix + "car", Label: "Car", Service: "Amap"},
			wayJSON{ID: amapPrefix + "bike", Label: "Bicycle", Service: "Amap"},
			wayJSON{ID: amapPrefix + "ebike", Label: "E-bike", Service: "Amap"},
			wayJSON{ID: amapPrefix + "foot", Label: "Foot", Service: "Amap"},
		)
	}
	return list
}

// wayIDs are the ways offered, as a set.
func (a api) wayIDs() map[string]string {
	ids := map[string]string{}
	for _, way := range a.ways() {
		ids[way.ID] = way.Service
	}
	return ids
}

var errUnknownWay = errors.New("unknown way of travel; reload the page")

// route asks the service of a way for the road between two points.
func (a api) route(ctx context.Context, way string, from, to geo.LatLon) (osrm.Route, error) {
	if _, ok := a.wayIDs()[way]; !ok {
		return osrm.Route{}, errUnknownWay
	}
	if mode, ok := strings.CutPrefix(way, amapPrefix); ok {
		if a.settings.Amap != nil {
			return a.settings.Amap(ctx, mode, from, to)
		}
		found, err := amap.New(a.settings.AmapKey).Route(ctx, mode, from, to)
		return osrm.Route{Points: found.Points, Distance: found.Distance, Duration: found.Duration}, err
	}
	return a.router()(ctx, way, from, to)
}
