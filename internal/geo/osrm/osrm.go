// Package osrm asks a public OSRM router for the route along the road between
// two points. OSRM routes on OpenStreetMap in WGS-84 and needs no key; the two
// points are sent to the service, and nothing else.
package osrm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"dgs-toolbox/internal/geo"
)

// Profiles are the ways of travel offered, each with the public service that
// routes it: the OSRM project's demo server has only cars; FOSSGIS's servers,
// behind openstreetmap.de, add bicycles and walking.
var Profiles = map[string]string{
	"car":  "https://router.project-osrm.org/route/v1/driving/",
	"bike": "https://routing.openstreetmap.de/routed-bike/route/v1/driving/",
	"foot": "https://routing.openstreetmap.de/routed-foot/route/v1/driving/",
}

// Route is a route along the road.
type Route struct {
	Points   []geo.LatLon
	Distance float64 // metres
	Duration float64 // seconds, as the router expects to travel it
}

// Client routes with the services in Bases, keyed by profile.
type Client struct {
	Bases map[string]string
	HTTP  *http.Client
}

// Default is a client of the public services.
func Default() Client {
	return Client{Bases: Profiles, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Route asks for the route from one point to another.
func (c Client) Route(ctx context.Context, profile string, from, to geo.LatLon) (Route, error) {
	base, ok := c.Bases[profile]
	if !ok {
		return Route{}, fmt.Errorf("unknown profile %q", profile)
	}
	url := base + coordinate(from) + ";" + coordinate(to) + "?overview=full&geometries=geojson"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Route{}, err
	}
	request.Header.Set("User-Agent", "dgs-toolbox (GPX gap filling)")
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return Route{}, fmt.Errorf("router: %w", err)
	}
	defer response.Body.Close()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Routes  []struct {
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Geometry struct {
				Coordinates [][2]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return Route{}, fmt.Errorf("router answered %s, not a route", response.Status)
	}
	if body.Code != "Ok" || len(body.Routes) == 0 {
		message := body.Message
		if message == "" {
			message = body.Code
		}
		if message == "" {
			message = response.Status
		}
		return Route{}, errors.New("router: " + message)
	}
	found := body.Routes[0]
	route := Route{Distance: found.Distance, Duration: found.Duration}
	for _, lonLat := range found.Geometry.Coordinates {
		route.Points = append(route.Points, geo.LatLon{Lat: lonLat[1], Lon: lonLat[0]})
	}
	if len(route.Points) == 0 {
		return Route{}, errors.New("router: the route has no points")
	}
	return route, nil
}

func coordinate(p geo.LatLon) string {
	return strconv.FormatFloat(p.Lon, 'f', 7, 64) + "," + strconv.FormatFloat(p.Lat, 'f', 7, 64)
}
