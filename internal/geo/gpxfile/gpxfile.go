// Package gpxfile reads GPX 1.0 and 1.1 files into plain Go values. It only
// reads: what a command does with the points belongs to other packages.
package gpxfile

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"dgs-toolbox/internal/geo"
)

// File is a parsed GPX document: its recorded tracks, its planned routes and
// its standalone waypoints, each in file order.
type File struct {
	Name      string
	Tracks    []Track
	Routes    []Route
	Waypoints []Waypoint
}

// Route is one <rte>: a planned path, its points in order, usually untimed.
type Route struct {
	Name   string
	Points []Point
}

// Waypoint is one <wpt>: a named place on its own, not part of a path.
type Waypoint struct {
	Point
	Name        string
	Description string
}

// Track is one <trk>; its segments are the pieces the recorder kept apart,
// usually because the signal or the recording paused in between.
type Track struct {
	Name     string
	Segments []Segment
}

type Segment struct {
	Points []Point
}

// Point is one <trkpt>. Elevation, time, HDOP and satellite count are optional
// in GPX; the Has flags and a zero Time say they were absent.
type Point struct {
	geo.LatLon
	Elevation    float64
	HasElevation bool
	Time         time.Time
	// HDOP is the horizontal dilution of precision the recorder reported:
	// lower is better, and above about 5 the position is doubtful.
	HDOP          float64
	HasHDOP       bool
	Satellites    int
	HasSatellites bool
}

// Open reads the GPX file at path.
func Open(path string) (*File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	parsed, err := Parse(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return parsed, nil
}

type xmlGPX struct {
	Metadata struct {
		Name string `xml:"name"`
	} `xml:"metadata"`
	Name      string     `xml:"name"` // GPX 1.0 keeps the name at the root
	Tracks    []xmlTrack `xml:"trk"`
	Routes    []xmlRoute `xml:"rte"`
	Waypoints []xmlPoint `xml:"wpt"`
}

type xmlRoute struct {
	Name   string     `xml:"name"`
	Points []xmlPoint `xml:"rtept"`
}

type xmlTrack struct {
	Name     string       `xml:"name"`
	Segments []xmlSegment `xml:"trkseg"`
}

type xmlSegment struct {
	Points []xmlPoint `xml:"trkpt"`
}

type xmlPoint struct {
	Lat       float64 `xml:"lat,attr"`
	Lon       float64 `xml:"lon,attr"`
	Elevation *string `xml:"ele"`
	Time      *string `xml:"time"`
	HDOP      *string `xml:"hdop"`
	Sat       *string `xml:"sat"`
	Name      string  `xml:"name"`
	Desc      string  `xml:"desc"`
}

// Parse reads a GPX document. A point with an unreadable time or elevation is
// kept without it rather than failing the file, because recorders disagree on
// the details and the position is what matters.
func Parse(r io.Reader) (*File, error) {
	var doc xmlGPX
	decoder := xml.NewDecoder(r)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse GPX: %w", err)
	}
	file := &File{Name: strings.TrimSpace(doc.Metadata.Name)}
	if file.Name == "" {
		file.Name = strings.TrimSpace(doc.Name)
	}
	for _, trk := range doc.Tracks {
		track := Track{Name: strings.TrimSpace(trk.Name)}
		for _, seg := range trk.Segments {
			segment := Segment{Points: make([]Point, 0, len(seg.Points))}
			for _, pt := range seg.Points {
				segment.Points = append(segment.Points, pt.point())
			}
			track.Segments = append(track.Segments, segment)
		}
		file.Tracks = append(file.Tracks, track)
	}
	for _, rte := range doc.Routes {
		route := Route{Name: strings.TrimSpace(rte.Name), Points: make([]Point, 0, len(rte.Points))}
		for _, pt := range rte.Points {
			route.Points = append(route.Points, pt.point())
		}
		file.Routes = append(file.Routes, route)
	}
	for _, wpt := range doc.Waypoints {
		file.Waypoints = append(file.Waypoints, Waypoint{
			Point:       wpt.point(),
			Name:        strings.TrimSpace(wpt.Name),
			Description: strings.TrimSpace(wpt.Desc),
		})
	}
	return file, nil
}

func (pt xmlPoint) point() Point {
	point := Point{LatLon: geo.LatLon{Lat: pt.Lat, Lon: pt.Lon}}
	point.Elevation, point.HasElevation = parseFloat(pt.Elevation)
	point.HDOP, point.HasHDOP = parseFloat(pt.HDOP)
	if pt.Sat != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(*pt.Sat)); err == nil && n >= 0 {
			point.Satellites, point.HasSatellites = n, true
		}
	}
	if pt.Time != nil {
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*pt.Time)); err == nil {
			point.Time = parsed
		}
	}
	return point
}

func parseFloat(text *string) (float64, bool) {
	if text == nil {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(*text), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}
