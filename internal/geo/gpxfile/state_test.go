package gpxfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo"
)

func TestStandaloneStateAndWaypoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ours.gpx")
	if err := CreateAll(path, "Trip", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	state := []byte(`{"version":1,"segments":{"cuts":[2]}}`)
	if err := WriteState(path, state); err != nil {
		t.Fatal(err)
	}
	if err := AddWaypoint(path, Waypoint{Point: Point{LatLon: geo.LatLon{Lat: 30.2, Lon: 120.1}}, Name: "Hotel & home", Description: "Night one"}); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !file.IsOurs() || len(file.Waypoints) != 1 || file.Waypoints[0].Name != "Hotel & home" {
		t.Fatalf("file = %+v", file)
	}
	got, found, err := ReadState(path)
	if err != nil || !found || !bytes.Equal(got, state) {
		t.Fatalf("state = %s, %v, %v", got, found, err)
	}
	if err := WriteState(path, nil); err != nil {
		t.Fatal(err)
	}
	_, found, err = ReadState(path)
	if err != nil || found {
		t.Fatalf("state after discard = %v, %v", found, err)
	}
}

func TestForeignGPXCannotBeEditedInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recorded.gpx")
	if err := os.WriteFile(path, []byte(`<gpx version="1.1" creator="watch" xmlns="http://www.topografix.com/GPX/1/1"></gpx>`), 0o644); err != nil {
		t.Fatal(err)
	}
	point := Waypoint{Point: Point{LatLon: geo.LatLon{Lat: 30, Lon: 120}}, Name: "No"}
	if err := AddWaypoint(path, point); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("AddWaypoint = %v", err)
	}
	if err := WriteState(path, []byte(`{}`)); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("WriteState = %v", err)
	}
}
