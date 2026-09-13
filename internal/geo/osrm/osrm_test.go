package osrm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dgs-toolbox/internal/geo"
)

func TestRouteReadsTheGeometry(t *testing.T) {
	var asked string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path + "?" + r.URL.RawQuery
		w.Write([]byte(`{"code":"Ok","routes":[{"distance":1234.5,"duration":90,"geometry":{"coordinates":[[120.1,30.1],[120.15,30.12],[120.2,30.2]]}}]}`))
	}))
	defer server.Close()
	client := Client{Bases: map[string]string{"car": server.URL + "/route/v1/driving/"}}
	route, err := client.Route(context.Background(), "car", geo.LatLon{Lat: 30.1, Lon: 120.1}, geo.LatLon{Lat: 30.2, Lon: 120.2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(asked, "/route/v1/driving/120.1000000,30.1000000;120.2000000,30.2000000?") {
		t.Fatalf("asked %s", asked)
	}
	if len(route.Points) != 3 || route.Points[1].Lat != 30.12 || route.Distance != 1234.5 {
		t.Fatalf("route %+v", route)
	}
}

func TestRouteReportsTheRoutersError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":"NoRoute","message":"Impossible route between points"}`))
	}))
	defer server.Close()
	client := Client{Bases: map[string]string{"foot": server.URL + "/"}}
	if _, err := client.Route(context.Background(), "foot", geo.LatLon{}, geo.LatLon{}); err == nil || !strings.Contains(err.Error(), "Impossible") {
		t.Fatalf("err = %v", err)
	}
	if _, err := client.Route(context.Background(), "boat", geo.LatLon{}, geo.LatLon{}); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
