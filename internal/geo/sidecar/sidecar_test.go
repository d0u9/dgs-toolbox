package sidecar

import (
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/geo/clean"
	"dgs-toolbox/internal/geo/compose"
	"dgs-toolbox/internal/geo/segment"
)

func TestLoadWithoutSidecarGivesDefaults(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	file, found, err := Load(source)
	if err != nil || found || file.Clean.Active() || file.Clean.Spikes.MaxSpeed != 100 {
		t.Fatalf("file %+v found %v err %v", file, found, err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	file, _, _ := Load(source)
	file.Clean.Spikes.Enabled = true
	file.Clean.Spikes.MaxSpeed = 250
	file.Clean.Edits = []clean.Edit{{Kind: clean.EditLasso, Points: []int{3, 4}, Join: true}}
	if err := Save(source, file); err != nil {
		t.Fatal(err)
	}
	back, found, err := Load(source)
	if err != nil || !found || !back.Clean.Spikes.Enabled || back.Clean.Spikes.MaxSpeed != 250 || len(back.Clean.Edits) != 1 || len(back.Clean.Edits[0].Points) != 2 {
		t.Fatalf("back %+v found %v err %v", back, found, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(source))
	if len(entries) != 1 || entries[0].Name() != "walk.gpx.dgs.json" {
		t.Fatalf("directory holds %v", entries)
	}
}

func TestPartialSidecarKeepsDefaults(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	if err := os.WriteFile(PathFor(source), []byte(`{"version":1,"clean":{"spikes":{"enabled":true}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	file, found, err := Load(source)
	if err != nil || !found || !file.Clean.Spikes.Enabled || file.Clean.Spikes.MaxSpeed != 100 || file.Clean.Drift.HalfWindow != 7 {
		t.Fatalf("file %+v", file.Clean)
	}
}

func TestSavingNothingRemovesTheSidecar(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	file, _, _ := Load(source)
	file.Clean.Edits = []clean.Edit{{Kind: clean.EditRange, First: 1, Last: 2}}
	_ = Save(source, file)
	file.Clean.Edits = nil
	if err := Save(source, file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PathFor(source)); !os.IsNotExist(err) {
		t.Fatalf("sidecar still there: %v", err)
	}
}

func TestLoadRefusesBrokenOrNewer(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	for _, body := range []string{"not json", `{"version":99}`} {
		_ = os.WriteFile(PathFor(source), []byte(body), 0o644)
		if _, _, err := Load(source); err == nil {
			t.Fatalf("%q loaded", body)
		}
	}
}

func TestSegmentsAloneKeepTheSidecar(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	file, _, _ := Load(source)
	file.Segments.Cuts = []int{120}
	file.Segments.Names = []segment.Name{{Start: 0, Name: "Morning"}}
	if err := Save(source, file); err != nil {
		t.Fatal(err)
	}
	back, found, err := Load(source)
	if err != nil || !found || len(back.Segments.Cuts) != 1 || back.Segments.Names[0].Name != "Morning" {
		t.Fatalf("back %+v found %v err %v", back.Segments, found, err)
	}
}

func TestOldRemovalsLoadAsEdits(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	_ = os.WriteFile(PathFor(source), []byte(`{"version":1,"clean":{"removed":[5,6],"ranges":[{"first":10,"last":20,"join":false}]}}`), 0o644)
	file, _, err := Load(source)
	if err != nil || len(file.Clean.Edits) != 2 || file.Clean.Removed != nil || file.Clean.Edits[1].Last != 20 {
		t.Fatalf("loaded %+v, %v", file.Clean.Edits, err)
	}
}

func TestInsertAndDeleteShiftIndices(t *testing.T) {
	file := File{Clean: clean.Defaults()}
	file.Clean.Edits = []clean.Edit{
		{Kind: clean.EditLasso, Points: []int{2, 8, 20}, Join: true},
		{Kind: clean.EditRange, First: 9, Last: 12},
		{Kind: clean.EditStop, First: 30, Last: 40, Join: true},
	}
	file.Segments = segment.Params{Cuts: []int{5, 25}, Names: []segment.Name{{Start: 25, Name: "Lake"}}}
	file.Fills = []compose.Fill{{First: 50, Last: 53, Route: [][2]float64{{1, 1}, {1, 2}}}}

	file.Insert(4, 3) // three points after index 4
	edits := file.Clean.Edits
	if edits[0].Points[0] != 2 || edits[0].Points[1] != 11 || edits[1].First != 12 || edits[2].Last != 43 {
		t.Fatalf("edits after insert %+v", edits)
	}
	if file.Segments.Cuts[0] != 8 || file.Segments.Names[0].Start != 28 || file.Fills[0].First != 53 {
		t.Fatalf("after insert %+v", file)
	}

	file.Delete(10, 13) // takes the range's first two points and a lasso point
	edits = file.Clean.Edits
	if len(edits[0].Points) != 2 || edits[0].Points[1] != 19 || edits[1].First != 10 || edits[1].Last != 11 {
		t.Fatalf("edits after delete %+v", edits)
	}

	file.Delete(50, 50) // a point of the fill's route: the fill goes
	if len(file.Fills) != 0 {
		t.Fatalf("fills %+v", file.Fills)
	}
	file.Delete(20, 30) // the named cut
	if len(file.Segments.Cuts) != 1 || len(file.Segments.Names) != 0 {
		t.Fatalf("segments %+v", file.Segments)
	}
}

// Two tracks of ten points, then one of five, put in the order 2, 0, 1: the
// edits on each track's points go with it, and a range spanning the first
// two tracks parts where they are put apart.
func TestReorderMovesEditsWithTheirPoints(t *testing.T) {
	file := File{Clean: clean.Defaults()}
	file.Clean.Edits = []clean.Edit{
		{Kind: clean.EditLasso, Points: []int{1, 21}},
		{Kind: clean.EditRange, First: 8, Last: 11},
	}
	file.Segments = segment.Params{Cuts: []int{15}, Names: []segment.Name{{Start: 15, Name: "Lake"}}}
	file.Fills = []compose.Fill{{First: 3, Last: 6, Route: [][2]float64{{1, 1}, {1, 2}}}}

	file.Reorder([][2]int{{20, 24}, {0, 9}, {10, 19}})

	edits := file.Clean.Edits
	if len(edits[0].Points) != 2 || edits[0].Points[0] != 6 || edits[0].Points[1] != 1 {
		t.Fatalf("lasso %+v", edits[0])
	}
	// 8–9 of the first track now at 13–14, 10–11 of the second at 15–16:
	// they follow one another, so the range stays whole.
	if len(edits) != 2 || edits[1].First != 13 || edits[1].Last != 16 {
		t.Fatalf("range %+v", edits)
	}
	if file.Segments.Cuts[0] != 20 || file.Segments.Names[0].Start != 20 || file.Fills[0].First != 8 || file.Fills[0].Last != 11 {
		t.Fatalf("after reorder %+v %+v", file.Segments, file.Fills)
	}

	file.Reorder([][2]int{{15, 24}, {5, 14}, {0, 4}}) // the first two tracks swap
	edits = file.Clean.Edits
	if len(edits) != 3 || edits[1].First != 18 || edits[1].Last != 19 || edits[2].First != 0 || edits[2].Last != 1 {
		t.Fatalf("split range %+v", edits)
	}
}
