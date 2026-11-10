package organizer

import "testing"

// A service is asked for by its English name too, and carries it for the TUI,
// while a note keeps writing its short name.
func TestMapLinksKnowTheEnglishName(t *testing.T) {
	links := MapLinks("-33.8", "151.2", "", "Amap")
	if len(links) != 1 || links[0].Short != "高德" || links[0].English != "Amap" {
		t.Fatalf("links = %+v", links)
	}
}
