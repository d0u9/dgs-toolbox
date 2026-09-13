package amap

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gcj02"
)

func TestRouteConvertsBothWays(t *testing.T) {
	from, to := geo.LatLon{Lat: 30.25, Lon: 120.15}, geo.LatLon{Lat: 30.26, Lon: 120.17}
	var asked string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path + "?" + r.URL.RawQuery
		g1, g2 := gcj02.FromWGS84(from), gcj02.FromWGS84(to)
		w.Write([]byte(`{"status":"1","info":"OK","infocode":"10000","route":{"paths":[{"distance":"2100","cost":{"duration":"1500"},"steps":[` +
			`{"polyline":"` + coordinate(g1) + `;120.160000,30.255000"},{"polyline":"120.160000,30.255000;` + coordinate(g2) + `"}]}]}}`))
	}))
	defer server.Close()
	route, err := Client{Key: "secret", Base: server.URL + "/"}.Route(context.Background(), "foot", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(asked, "/walking?") || !strings.Contains(asked, "origin="+strings.ReplaceAll(coordinate(gcj02.FromWGS84(from)), ",", "%2C")) {
		t.Fatalf("asked %s", asked)
	}
	if len(route.Points) != 3 || route.Distance != 2100 || route.Duration != 1500 {
		t.Fatalf("route = %+v", route)
	}
	if math.Abs(route.Points[0].Lat-from.Lat) > 1e-6 || math.Abs(route.Points[2].Lon-to.Lon) > 1e-6 {
		t.Fatalf("not back in WGS-84: %v", route.Points)
	}
}

func TestErrorsKeepTheKeyOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"0","info":"INVALID_USER_KEY","infocode":"10001"}`))
	}))
	_, err := Client{Key: "secret", Base: server.URL + "/"}.Route(context.Background(), "car", geo.LatLon{Lat: 30, Lon: 120}, geo.LatLon{Lat: 30.1, Lon: 120.1})
	if err == nil || !strings.Contains(err.Error(), "INVALID_USER_KEY") {
		t.Fatalf("err = %v", err)
	}
	server.Close() // now unreachable
	_, err = Client{Key: "secret", Base: server.URL + "/"}.Route(context.Background(), "car", geo.LatLon{Lat: 30, Lon: 120}, geo.LatLon{Lat: 30.1, Lon: 120.1})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}
