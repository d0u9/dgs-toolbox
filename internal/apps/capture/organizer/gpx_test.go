package organizer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
)

func TestDailyGPXAppend(t *testing.T) {
	dir := t.TempDir()
	settings := DefaultSettings()
	settings.GPXDirectory = dir
	capture := beenHere()
	ctx := NewContext(capture, map[FieldID]any{FieldContent: "A note from Capture"}).WithSettings(settings)
	path := filepath.Join(dir, "20260909.capture.gpx")
	if got := dailyGPXPath(ctx); got != path {
		t.Fatalf("target = %q, want %q", got, path)
	}
	if skipped, err := appendToDailyGPX(ctx, ActionPlan{}); err != nil || skipped != "" {
		t.Fatalf("first append: skipped %q, err %v", skipped, err)
	}
	if skipped, err := appendToDailyGPX(ctx, ActionPlan{}); err != nil || skipped == "" {
		t.Fatalf("second append: skipped %q, err %v", skipped, err)
	}
	capture.Index.ID = "second"
	capture.Index.Source.Workflow = "another_workflow"
	capture.Index.CreatedAt = "2026-09-09T23:45:00+10:00"
	if _, err := appendToDailyGPX(NewContext(capture, map[FieldID]any{FieldContent: "Another note"}).WithSettings(settings), ActionPlan{}); err != nil {
		t.Fatal(err)
	}
	// A different id for the same position and second is still one waypoint.
	capture.Index.ID = "same-position-and-time"
	capture.Index.CreatedAt = "2026-09-09T23:45:00.900+10:00"
	if skipped, err := appendToDailyGPX(NewContext(capture, nil).WithSettings(settings), ActionPlan{}); err != nil || skipped == "" {
		t.Fatalf("same point append: skipped %q, err %v", skipped, err)
	}
	file, err := gpxfile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Waypoints) != 2 || file.Waypoints[0].CaptureID != beenHere().Index.ID || file.Waypoints[1].CaptureID != "second" || file.Waypoints[0].Name != "A note from Capture" || file.Waypoints[1].Name != "Another note" {
		t.Fatalf("waypoints = %+v", file.Waypoints)
	}
	if file.Waypoints[0].Time.IsZero() || file.Waypoints[1].Time.IsZero() {
		t.Fatal("waypoints lost capture time")
	}
}

func TestDailyGPXRefusesForeignFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20260909.capture.gpx")
	original := []byte(`<?xml version="1.0"?><gpx version="1.1" creator="other" xmlns="http://www.topografix.com/GPX/1/1"></gpx>`)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.GPXDirectory = dir
	_, err := appendToDailyGPX(NewContext(beenHere(), nil).WithSettings(settings), ActionPlan{})
	if !errors.Is(err, gpxfile.ErrNotOurs) {
		t.Fatalf("err = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatalf("foreign file changed: %v", err)
	}
}
