package gpx

import (
	"strings"
	"testing"
	"time"
)

func TestSummaryViewShowsDurationDistanceAndStops(t *testing.T) {
	view := summaryView(&Summary{Name: "walk", Distance: 12345, Duration: 2*time.Hour + 5*time.Minute, Stops: 1})
	for _, want := range []string{"walk", "2h 05m", "12.3 km", "1 stop"} {
		if !strings.Contains(view, want) {
			t.Fatalf("summary %q lacks %q", view, want)
		}
	}
	if !strings.Contains(summaryView(nil), "Focus a track") {
		t.Fatal("no hint without a focused track")
	}
}
