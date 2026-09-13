// Package amap asks Amap (高德) for the route along the road between two
// points, with its Web Service route planning (v5). Amap needs a key, routes on
// its own map data — the most complete for China — and works in GCJ-02:
// positions are converted from WGS-84 on the way out and back on the way in.
// The two points and the key are sent to the service, and nothing else.
package amap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gcj02"
)

// Base is Amap's Web Service address.
const Base = "https://restapi.amap.com/v5/direction/"

// Modes are the ways of travel offered, each with Amap's route planning
// service for it.
var Modes = map[string]string{
	"car":   "driving",
	"foot":  "walking",
	"bike":  "bicycling",
	"ebike": "electrobike",
}

// Route is a route along the road, in WGS-84.
type Route struct {
	Points   []geo.LatLon
	Distance float64 // metres
	Duration float64 // seconds, as Amap expects to travel it
}

// Client routes with Amap under a key.
type Client struct {
	Key  string
	Base string
	HTTP *http.Client
}

// New is a client of Amap's service.
func New(key string) Client {
	return Client{Key: key, Base: Base, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Route asks for the route from one point to another, both in WGS-84.
func (c Client) Route(ctx context.Context, mode string, from, to geo.LatLon) (Route, error) {
	service, ok := Modes[mode]
	if !ok {
		return Route{}, fmt.Errorf("unknown mode %q", mode)
	}
	if c.Key == "" {
		return Route{}, errors.New("amap: no key; set geo.gpx.amap_key")
	}
	query := url.Values{
		"key":         {c.Key},
		"origin":      {coordinate(gcj02.FromWGS84(from))},
		"destination": {coordinate(gcj02.FromWGS84(to))},
		"show_fields": {"polyline,cost"},
	}
	base := c.Base
	if base == "" {
		base = Base
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+service+"?"+query.Encode(), nil)
	if err != nil {
		return Route{}, errors.New("amap: could not build the request")
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		// The request's URL holds the key: say what failed without it.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return Route{}, fmt.Errorf("amap: %w", err)
	}
	defer response.Body.Close()
	var body struct {
		Status   string `json:"status"`
		Info     string `json:"info"`
		Infocode string `json:"infocode"`
		Route    struct {
			Paths []struct {
				Distance string `json:"distance"`
				Cost     struct {
					Duration string `json:"duration"`
				} `json:"cost"`
				Steps []struct {
					Polyline string `json:"polyline"`
				} `json:"steps"`
			} `json:"paths"`
		} `json:"route"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return Route{}, fmt.Errorf("amap answered %s, not a route", response.Status)
	}
	if body.Status != "1" {
		return Route{}, fmt.Errorf("amap: %s (%s)", body.Info, body.Infocode)
	}
	if len(body.Route.Paths) == 0 {
		return Route{}, errors.New("amap: no route found")
	}
	path := body.Route.Paths[0]
	route := Route{}
	route.Distance, _ = strconv.ParseFloat(path.Distance, 64)
	route.Duration, _ = strconv.ParseFloat(path.Cost.Duration, 64)
	for _, step := range path.Steps {
		for _, pair := range strings.Split(step.Polyline, ";") {
			lon, lat, ok := strings.Cut(pair, ",")
			if !ok {
				continue
			}
			x, errX := strconv.ParseFloat(lon, 64)
			y, errY := strconv.ParseFloat(lat, 64)
			if errX != nil || errY != nil {
				continue
			}
			p := gcj02.ToWGS84(geo.LatLon{Lat: y, Lon: x})
			if n := len(route.Points); n > 0 && route.Points[n-1] == p {
				continue // steps share their ends
			}
			route.Points = append(route.Points, p)
		}
	}
	if len(route.Points) == 0 {
		return Route{}, errors.New("amap: the route has no points")
	}
	return route, nil
}

// coordinate keeps six decimals, as Amap takes.
func coordinate(p geo.LatLon) string {
	return strconv.FormatFloat(p.Lon, 'f', 6, 64) + "," + strconv.FormatFloat(p.Lat, 'f', 6, 64)
}
