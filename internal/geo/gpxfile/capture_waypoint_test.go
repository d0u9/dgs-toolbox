package gpxfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/geo"
)

func TestCaptureIDWaypointExtensionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.gpx")
	first := Waypoint{Point: Point{LatLon: geo.LatLon{Lat: 1, Lon: 2}}, Name: "First note <&>", CaptureID: "id-1"}
	if err := CreateAll(path, "Captures", []Waypoint{first}, nil, nil); err != nil {
		t.Fatal(err)
	}
	second := Waypoint{Point: Point{LatLon: geo.LatLon{Lat: 3, Lon: 4}}, Name: "Second note", CaptureID: "id-2"}
	if err := AddWaypoint(path, second); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Waypoints) != 2 || file.Waypoints[0].Name != first.Name || file.Waypoints[0].CaptureID != first.CaptureID || file.Waypoints[1].Name != second.Name || file.Waypoints[1].CaptureID != second.CaptureID {
		t.Fatalf("waypoints = %+v", file.Waypoints)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `xmlns:dgs="urn:dgs-toolbox:capture"`) {
		t.Fatalf("extension missing: %v", err)
	}
}
