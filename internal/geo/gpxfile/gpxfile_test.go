package gpxfile

import (
	"strings"
	"testing"
)

const sample = `<?xml version="1.0"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <metadata><name>Walk</name></metadata>
  <trk><name>Morning</name>
    <trkseg>
      <trkpt lat="30.25" lon="120.15"><ele>12.5</ele><time>2026-09-01T01:00:00Z</time><sat>9</sat><hdop>0.8</hdop></trkpt>
      <trkpt lat="30.26" lon="120.16"><time>garbage</time><hdop>n/a</hdop></trkpt>
    </trkseg>
    <trkseg>
      <trkpt lat="30.27" lon="120.17"><ele>20</ele></trkpt>
    </trkseg>
  </trk>
  <rte><name>Plan</name><rtept lat="30.1" lon="120.1"/><rtept lat="30.2" lon="120.2"/></rte>
  <wpt lat="30.3" lon="120.3"><ele>5</ele><name>Hotel</name><desc>Night one</desc></wpt>
</gpx>`

func TestParse(t *testing.T) {
	file, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != "Walk" || len(file.Tracks) != 1 || file.Tracks[0].Name != "Morning" {
		t.Fatalf("file = %+v", file)
	}
	segments := file.Tracks[0].Segments
	if len(segments) != 2 || len(segments[0].Points) != 2 || len(segments[1].Points) != 1 {
		t.Fatalf("segments = %+v", segments)
	}
	first, second := segments[0].Points[0], segments[0].Points[1]
	if !first.HasElevation || first.Elevation != 12.5 || first.Time.IsZero() || first.Lat != 30.25 {
		t.Fatalf("first = %+v", first)
	}
	if !first.HasHDOP || first.HDOP != 0.8 || !first.HasSatellites || first.Satellites != 9 {
		t.Fatalf("first quality = %+v", first)
	}
	if second.HasElevation || !second.Time.IsZero() || second.HasHDOP || second.HasSatellites {
		t.Fatalf("second kept a value it did not have: %+v", second)
	}
}

func TestParseRoutesAndWaypoints(t *testing.T) {
	file, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Routes) != 1 || file.Routes[0].Name != "Plan" || len(file.Routes[0].Points) != 2 {
		t.Fatalf("routes = %+v", file.Routes)
	}
	if len(file.Waypoints) != 1 || file.Waypoints[0].Name != "Hotel" || file.Waypoints[0].Description != "Night one" || !file.Waypoints[0].HasElevation {
		t.Fatalf("waypoints = %+v", file.Waypoints)
	}
}

func TestParseRejectsNonXML(t *testing.T) {
	if _, err := Parse(strings.NewReader("not xml")); err == nil {
		t.Fatal("parsed non-XML")
	}
}
