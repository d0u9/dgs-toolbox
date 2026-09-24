package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/gpxfile"
)

var ErrNoGPXDirectory = errors.New("capture.gpx.directory is not configured")

func dailyGPXPath(ctx Context) string {
	day, err := captureDay(ctx)
	if err != nil || ctx.Settings.GPXDirectory == "" {
		return ""
	}
	return filepath.Join(ctx.Settings.GPXDirectory, day.Format("20060102")+".capture.gpx")
}

func appendToDailyGPX(ctx Context, _ ActionPlan) (string, error) {
	if strings.TrimSpace(ctx.Settings.GPXDirectory) == "" {
		return "", ErrNoGPXDirectory
	}
	day, err := captureDay(ctx)
	if err != nil {
		return "", err
	}
	path := dailyGPXPath(ctx)
	if path == "" {
		return "", errors.New("daily GPX target is unavailable")
	}
	position, ok := ctx.Position()
	if !ok {
		return "", errors.New("Capture has no valid coordinates")
	}
	id := ctx.Capture.Index.ID
	if id == "" {
		id = ctx.Capture.Name
	}
	if id == "" {
		return "", errors.New("Capture has no id")
	}
	waypoint := gpxfile.Waypoint{Point: gpxfile.Point{
		LatLon: geo.LatLon{Lat: position.Latitude, Lon: position.Longitude},
		Time:   day,
	}, Name: strings.TrimSpace(ctx.String(FieldContent)), CaptureID: id}
	if !position.Sourced && ctx.Capture.Index.Coordinates != nil {
		waypoint.Elevation = ctx.Capture.Index.Coordinates.Altitude
		waypoint.HasElevation = ctx.Capture.Index.Coordinates.Altitude != 0
	}
	if err := os.MkdirAll(ctx.Settings.GPXDirectory, 0o755); err != nil {
		return "", err
	}
	file, err := gpxfile.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := gpxfile.CreateAll(path, "Captures "+day.Format("2006-01-02"), []gpxfile.Waypoint{waypoint}, nil, nil); err != nil {
			return "", err
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !file.IsOurs() {
		return "", fmt.Errorf("%s: %w", path, gpxfile.ErrNotOurs)
	}
	for _, existing := range file.Waypoints {
		if existing.CaptureID == id {
			return fmt.Sprintf("this capture is already in %s", path), nil
		}
		if sameWaypoint(existing, waypoint) {
			return fmt.Sprintf("a waypoint at this position and time is already in %s", path), nil
		}
	}
	if err := gpxfile.AddWaypoint(path, waypoint); err != nil {
		return "", err
	}
	return "", nil
}

// GPX writes coordinates to seven decimal places and waypoint times to whole
// seconds. Compare those stored values so a retry still matches after a
// round-trip through the file.
func sameWaypoint(a, b gpxfile.Waypoint) bool {
	coordinate := func(v float64) string { return strconv.FormatFloat(v, 'f', 7, 64) }
	second := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }
	return coordinate(a.Lat) == coordinate(b.Lat) &&
		coordinate(a.Lon) == coordinate(b.Lon) && second(a.Time) == second(b.Time)
}
