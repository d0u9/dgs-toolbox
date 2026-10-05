package gpx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
)

func TestFailedSaveLeavesOriginalAndRetryAddsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.gpx")
	original := `<gpx creator="dgs-toolbox"><trk><name>One</name></trk><!-- preserve --><trk><name>Two</name></trk></gpx>`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := addTracks(path, "synthetic.gpx", []gpxfile.Track{{Name: "Added"}}); err != nil {
		t.Fatal(err)
	}
	defer pending.drop(path)
	held, _ := pending.get(path)
	held.Order = map[string][]string{"t": {"t1", "t0", "t2"}}
	pending.set(path, held)
	if err := flush(path); err == nil {
		t.Fatal("reordering across a comment should fail")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, []byte(original)) {
		t.Fatal("failed Save changed the original GPX")
	}
	if !pending.has(path) {
		t.Fatal("failed Save lost pending changes")
	}
	held.Order = nil
	pending.set(path, held)
	if err := flush(path); err != nil {
		t.Fatal(err)
	}
	parsed, err := gpxfile.Open(path)
	if err != nil || len(parsed.Tracks) != 3 || parsed.Tracks[2].Name != "Added" {
		t.Fatalf("retry duplicated or lost addition: %+v %v", parsed, err)
	}
	after, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(after), "<!-- preserve -->") {
		t.Fatal("Save lost unknown XML")
	}
	if pending.has(path) {
		t.Fatal("successful Save remains pending")
	}
}
