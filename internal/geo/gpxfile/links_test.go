package gpxfile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSetWaypointLinksReplacesDescAndLinksOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	if err := CreateAll(path, "Trip", []Waypoint{{Point: Point{}, Name: "Summit", Description: "old", Links: []Link{{Href: "a.jpg"}}}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	links := []Link{{Href: "https://example.com/"}, {Href: "trip.photos/b.jpg", Text: "North & up", Type: "image/jpeg"}}
	if err := SetWaypointLinks(path, 0, "Windy", links); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := file.Waypoints[0]
	if got.Name != "Summit" || got.Description != "Windy" || !reflect.DeepEqual(got.Links, links) {
		t.Fatalf("waypoint = %+v", got)
	}
	if err := SetWaypointLinks(path, 0, "", nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if file, _ := Open(path); file.Waypoints[0].Description != "" || len(file.Waypoints[0].Links) != 0 {
		t.Fatalf("not cleared:\n%s", data)
	}
}
