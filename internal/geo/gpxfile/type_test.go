package gpxfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A track's <type> is written after its name and before its extensions, and
// read back.
func TestTrackTypeRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gpx")
	err := CreateAll(path, "", nil, nil, []Track{
		{Name: "t", Type: "drive & park", Translations: map[string]string{"en": "T"}, Segments: []Segment{{Points: []Point{{}}}}},
		{Name: "u", Segments: []Segment{{Points: []Point{{}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "<name>t</name>\n    <type>drive &amp; park</type>\n    <extensions>") {
		t.Fatalf("written:\n%s", data)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Tracks[0].Type != "drive & park" || file.Tracks[1].Type != "" {
		t.Fatalf("types %q %q", file.Tracks[0].Type, file.Tracks[1].Type)
	}
}

// SetTrackType adds, replaces and removes the <type> in place, keeping the
// rest of the track, and leaves a <link>'s own <type> alone.
func TestSetTrackType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gpx")
	original := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="dgs-toolbox" xmlns="http://www.topografix.com/GPX/1/1">
  <trk>
    <name>a</name>
    <link href="https://example.com/a"><type>text/html</type></link>
    <extensions><dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" xml:lang="en">A</dgs:name></extensions>
    <trkseg>
      <trkpt lat="1.0000000" lon="2.0000000"></trkpt>
    </trkseg>
  </trk>
  <trk>
    <trkseg>
      <trkpt lat="1.0000000" lon="2.0000000"></trkpt>
    </trkseg>
  </trk>
</gpx>
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		index int
		kind  string
	}{{0, "hike"}, {0, "drive"}, {1, "bike"}} {
		if err := SetTrackType(path, step.index, step.kind); err != nil {
			t.Fatal(err)
		}
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Tracks[0].Type != "drive" || file.Tracks[1].Type != "bike" || file.Tracks[0].Translations["en"] != "A" {
		t.Fatalf("tracks %+v", file.Tracks)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "</link>\n    <type>drive</type>\n    <extensions>") || strings.Count(string(data), "<type>") != 3 {
		t.Fatalf("written:\n%s", data)
	}
	for _, index := range []int{0, 1} {
		if err := SetTrackType(path, index, ""); err != nil {
			t.Fatal(err)
		}
	}
	if data, _ := os.ReadFile(path); string(data) != original {
		t.Fatalf("taking the types out left:\n%s", data)
	}
}

func TestSetTrackTypeRefusesOtherFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gpx")
	data := `<gpx version="1.1" creator="device"><trk><trkseg><trkpt lat="1" lon="2"/></trkseg></trk></gpx>`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetTrackType(path, 0, "drive"); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("err = %v, want ErrNotOurs", err)
	}
}

// A track written on one line gets its type after its last own child, and
// gives it back byte for byte; a track that is not there is refused.
func TestSetTrackTypeOnOneLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gpx")
	original := `<gpx version="1.1" creator="dgs-toolbox"><trk><name>a</name><trkseg><trkpt lat="1" lon="2"/></trkseg></trk></gpx>`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetTrackType(path, 0, "ship"); err != nil {
		t.Fatal(err)
	}
	if file, err := Open(path); err != nil || file.Tracks[0].Type != "ship" || file.Tracks[0].Name != "a" {
		t.Fatalf("%+v %v", file, err)
	}
	if err := SetTrackType(path, 0, ""); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != original {
		t.Fatalf("left:\n%s", data)
	}
	if err := SetTrackType(path, 1, "ship"); err == nil {
		t.Fatal("a second track was typed")
	}
}
