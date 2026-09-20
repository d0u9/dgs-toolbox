package gpxfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
)

func sampleTracks() []Track {
	at := time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC)
	return []Track{{
		Name: "Day 1 <&>",
		Segments: []Segment{{Points: []Point{
			{LatLon: geo.LatLon{Lat: 30.1234567, Lon: 120.7654321}, Elevation: 12.5, HasElevation: true, Time: at, HDOP: 0.9, HasHDOP: true, Satellites: 8, HasSatellites: true},
			{LatLon: geo.LatLon{Lat: 30.2, Lon: 120.8}},
		}}},
	}}
}

func TestCreateWritesWhatParseReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.gpx")
	if err := Create(path, "Trip", sampleTracks()); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	pt := file.Tracks[0].Segments[0].Points[0]
	if file.Name != "Trip" || file.Tracks[0].Name != "Day 1 <&>" || pt.Lat != 30.1234567 || pt.Elevation != 12.5 || pt.Satellites != 8 || pt.HDOP != 0.9 || pt.Time.Second() != 3 {
		t.Fatalf("read back %+v / %+v", file, pt)
	}
	if second := file.Tracks[0].Segments[0].Points[1]; second.HasElevation || !second.Time.IsZero() {
		t.Fatalf("second point gained values: %+v", second)
	}
	if err := Create(path, "Trip", sampleTracks()); !errors.Is(err, ErrExists) {
		t.Fatalf("second Create = %v", err)
	}
}

func TestAppendKeepsTheRestOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	original := strings.Replace(sample, "</gpx>", "  <extensions><mine>kept</mine></extensions>\n</gpx>", 1)
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, sampleTracks()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, "<mine>kept</mine>") || strings.Replace(text, text[strings.Index(text, "  <trk>\n    <name>Day 1"):strings.Index(text, "  <extensions>")], "", 1) != original {
		t.Fatalf("original content changed:\n%s", data)
	}
	if strings.Index(text, "Day 1") > strings.Index(text, "<extensions>") {
		t.Fatal("track appended after the root extensions")
	}
	file, err := Open(path)
	if err != nil || len(file.Tracks) != 2 || file.Tracks[1].Name != "Day 1 <&>" || len(file.Waypoints) != 1 {
		t.Fatalf("appended file = %+v, %v", file, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode changed to %v", info.Mode())
	}
	if err := os.WriteFile(path, []byte("not gpx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, sampleTracks()); err == nil {
		t.Fatal("appended to a file that is not GPX")
	}
}

func TestRenameTrackOnlyTouchesTheName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	if err := Create(path, "Trip", append(sampleTracks(), Track{Segments: []Segment{{Points: []Point{
		{LatLon: geo.LatLon{Lat: 31, Lon: 121}},
	}}}})); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := RenameTrack(path, 0, "Day one <two>"); err != nil {
		t.Fatal(err)
	}
	// The second track has no <name>: renaming it opens one.
	if err := RenameTrack(path, 1, "Day two"); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Tracks[0].Name != "Day one <two>" || file.Tracks[1].Name != "Day two" {
		t.Fatalf("names = %q, %q", file.Tracks[0].Name, file.Tracks[1].Name)
	}
	if len(file.Tracks[0].Segments[0].Points) != 2 || file.Name != "Trip" {
		t.Fatalf("the rest of the file changed: %+v", file)
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "Day 1 <&>") {
		t.Fatal("the old name is still in the file")
	}
	if strings.Count(string(after), "<trkpt") != strings.Count(string(before), "<trkpt") {
		t.Fatal("points changed")
	}
	if err := RenameTrack(path, 5, "no such track"); err == nil {
		t.Fatal("renamed a track that is not there")
	}
}

func TestRenameTrackRefusesAFileWeDidNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recorded.gpx")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RenameTrack(path, 0, "Mine"); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("RenameTrack = %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != sample {
		t.Fatal("the recording was written")
	}
}
