package gpxfile

import (
	"path/filepath"
	"testing"
)

func TestSetName(t *testing.T) {
	dir := t.TempDir()
	for _, initial := range []string{"", "Old & name"} {
		path := filepath.Join(dir, "a"+initial+".gpx")
		if err := CreateAll(path, initial, []Waypoint{{Name: "w"}}, nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := SetName(path, "New <name>"); err != nil {
			t.Fatal(err)
		}
		file, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if file.Name != "New <name>" || len(file.Waypoints) != 1 || file.Waypoints[0].Name != "w" {
			t.Fatalf("initial %q: got name %q, waypoints %+v", initial, file.Name, file.Waypoints)
		}
	}
}
