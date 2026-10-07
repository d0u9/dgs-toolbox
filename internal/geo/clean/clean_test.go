package clean

import (
	"encoding/json"
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
	"dgs-toolbox/internal/geo/stops"
	"dgs-toolbox/internal/geo/track"
)

const metresPerDegree = 111195.08

var start = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// walkStayWalk walks north for 120 s, scribbles in place for 600 s, walks on.
func walkStayWalk() track.Line {
	var line track.Line
	north := 0.0
	add := func(east float64) {
		line = append(line, track.Sample{
			LatLon: geo.LatLon{Lat: 30 + north/metresPerDegree, Lon: 120 + east/(metresPerDegree*0.866)},
			Time:   start.Add(time.Duration(len(line)) * time.Second),
		})
	}
	for range 120 {
		north += 1.5
		add(0)
	}
	for i := range 600 {
		add(float64(i%7) * 3)
	}
	for range 120 {
		north += 1.5
		add(0)
	}
	return line
}

func TestZeroParamsCleanNothing(t *testing.T) {
	line := walkStayWalk()
	result := Run(line, Params{})
	if (Params{}).Active() || result.Counts() != (Counts{}) {
		t.Fatalf("counts %+v", result.Counts())
	}
	if kept, index := result.Kept(line); len(kept) != len(line) || index[5] != 5 {
		t.Fatal("kept line differs")
	}
}

func TestRunAppliesRulesInOrder(t *testing.T) {
	line := walkStayWalk()
	line[60].Lon += 3000 / (metresPerDegree * 0.866) // a spike
	p := Defaults()
	p.Spikes.Enabled = true
	p.Stops.Enabled = true
	p.Edits = []Edit{{Kind: EditLasso, Points: []int{60, 61, 9999}, Join: true}} // the spike is removed by hand first; 9999 is ignored
	result := Run(line, p)
	if result.Removed[60] != Manual || result.Removed[61] != Manual {
		t.Fatalf("manual removals: %q %q", result.Removed[60], result.Removed[61])
	}
	counts := result.Counts()
	if counts.Spike != 0 || counts.Stop < 500 || counts.Moved != 0 {
		t.Fatalf("counts %+v", counts)
	}
	kept, _ := result.Kept(line)
	if len(kept) != len(line)-counts.Manual-counts.Stop {
		t.Fatalf("kept %d of %d", len(kept), len(line))
	}
}

func TestRunRemovesSpikesAndSmooths(t *testing.T) {
	line := walkStayWalk()
	line[60].Lon += 3000 / (metresPerDegree * 0.866)
	p := Defaults()
	p.Spikes.Enabled = true
	p.Smooth.Enabled = true
	result := Run(line, p)
	if result.Removed[60] != Spike || result.Moved[60] {
		t.Fatalf("spike: %q moved %v", result.Removed[60], result.Moved[60])
	}
	if result.Counts().Moved == 0 {
		t.Fatal("nothing smoothed")
	}
}

func TestParamsJSONIsFlat(t *testing.T) {
	data, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]map[string]any
	_ = json.Unmarshal(data, &back)
	if back["spikes"]["maxSpeed"] != 100.0 || back["spikes"]["enabled"] != false || back["drift"]["halfWindow"] != 7.0 {
		t.Fatalf("json %s", data)
	}
}

func TestRangesRemoveAndBreakOrJoin(t *testing.T) {
	line := walkStayWalk()
	p := Params{Edits: []Edit{{Kind: EditRange, First: 10, Last: 19}, {Kind: EditStop, First: 100, Last: 109, Join: true}}}
	result := Run(line, p)
	if result.Counts().Manual != 20 || result.Removed[10] != Manual || result.Removed[20] != Kept {
		t.Fatalf("counts %+v", result.Counts())
	}
	kept, index := result.Kept(line)
	segmentOf := func(source int) int {
		for k, i := range index {
			if i == source {
				return kept[k].Segment
			}
		}
		return -1
	}
	if segmentOf(9) != 0 || segmentOf(20) != 1 || segmentOf(99) != 1 || segmentOf(110) != 1 {
		t.Fatalf("segments: 9→%d 20→%d 99→%d 110→%d", segmentOf(9), segmentOf(20), segmentOf(99), segmentOf(110))
	}
	if !p.Active() || (Params{}).Active() {
		t.Fatal("Active")
	}
}

func TestNormalizeReadsOldRemovals(t *testing.T) {
	old := Params{Removed: []int{3, 4}, Ranges: []Range{{First: 10, Last: 12}}}
	p := old.Normalize()
	if len(p.Edits) != 2 || p.Edits[0].Kind != EditLasso || len(p.Edits[0].Points) != 2 || p.Edits[1].Kind != EditRange || p.Edits[1].Join || p.Removed != nil || p.Ranges != nil {
		t.Fatalf("normalized %+v", p)
	}
	line := walkStayWalk()
	if got := Run(line, old).Counts().Manual; got != 5 {
		t.Fatalf("old removals remove %d", got)
	}
}

func TestCollapsedStopMarkerUsesTheRetainedTimeMidpoint(t *testing.T) {
	point := func(east, north float64, seconds int) track.Sample {
		return track.Sample{LatLon: geo.LatLon{Lat: north / metresPerDegree, Lon: east / metresPerDegree}, Time: start.Add(time.Duration(seconds) * time.Second)}
	}
	line := track.Line{point(0, 0, 0)}
	for i := 1; i < 100; i++ {
		line = append(line, point(10, 15, i*6))
	}
	line = append(line, point(30, 0, 600))
	params := Defaults()
	params.Stops.Enabled = true
	result := Run(line, params)
	kept, index := result.Kept(line)
	if len(kept) != 3 {
		t.Fatalf("kept %d points, want entry, centre and exit", len(kept))
	}
	found := stops.Detect(kept, stops.Defaults())
	if len(found) != 1 {
		t.Fatalf("stops = %+v", found)
	}
	stop := found[0]
	if geo.Distance(stop.Center, kept[1].LatLon) < 5 {
		t.Fatal("case did not reproduce the centre moving after collapse")
	}
	anchor := stop.Anchor(kept)
	if anchor != 1 || result.Moved[index[anchor]] {
		t.Fatalf("anchor %d did not preserve the retained stay point", anchor)
	}
}

func TestCollapsedTurnaroundPreservesSparseStay(t *testing.T) {
	var line track.Line
	add := func(north float64, seconds int) {
		line = append(line, track.Sample{LatLon: geo.LatLon{Lat: north / metresPerDegree}, Time: start.Add(time.Duration(seconds) * time.Second)})
	}
	for seconds := 0; seconds <= 40; seconds++ {
		add(float64(80-seconds*2), seconds)
	}
	add(0, 640)
	for seconds := 641; seconds <= 680; seconds++ {
		add(float64((seconds-640)*2), seconds)
	}
	params := Defaults()
	params.Stops.Enabled = true
	result := Run(line, params)
	kept, index := result.Kept(line)
	found := stops.Detect(kept, stops.Defaults())
	if len(found) != 1 {
		t.Fatalf("stops = %+v", found)
	}
	anchor := found[0].Anchor(kept)
	if distance := geo.Distance(kept[anchor].LatLon, geo.LatLon{}); distance > 0.01 {
		t.Fatalf("cleaned marker is %.1f metres from the turnaround", distance)
	}
	if result.Moved[index[anchor]] {
		t.Fatal("cleaning moved the representative stay point")
	}
}

// TestRunTracksCleansEachTrackWithItsOwnFilters joins two copies of a walk as
// two tracks: only the second turns stop collapse on, and an edit across both
// removes its samples in each.
func TestRunTracksCleansEachTrackWithItsOwnFilters(t *testing.T) {
	first, second := walkStayWalk(), walkStayWalk()
	for i := range second {
		second[i].Track = 1
	}
	line := append(append(track.Line{}, first...), second...)
	n := len(first)
	p := Defaults()
	p.Edits = []Edit{{Kind: EditRange, First: n - 2, Last: n + 1, Join: true}, {Kind: EditLasso, Points: []int{5, n + 5}, Join: true}}
	collapse := Defaults().Filters
	collapse.Stops.Enabled = true
	result := RunTracks(line, p, []TrackFilters{{First: n, Filters: collapse}}, Composed{})
	stopsIn := func(from, to int) int {
		count := 0
		for _, rule := range result.Removed[from:to] {
			if rule == Stop {
				count++
			}
		}
		return count
	}
	if stopsIn(0, n) != 0 || stopsIn(n, 2*n) == 0 {
		t.Fatalf("stops removed: first track %d, second %d", stopsIn(0, n), stopsIn(n, 2*n))
	}
	for _, i := range []int{5, n - 2, n - 1, n, n + 1, n + 5} {
		if result.Removed[i] != Manual {
			t.Fatalf("sample %d: %q, want manual", i, result.Removed[i])
		}
	}
	// A track without its own filters takes p's: the same as one run.
	p.Stops.Enabled = true
	if got, want := RunTracks(line, p, nil, Composed{}).Counts(), Run(line, p).Counts(); got.Manual != want.Manual || got.Stop == 0 {
		t.Fatalf("defaults: %+v, one run %+v", got, want)
	}
}

// A fill on the second track keeps its samples and their indices when the
// line is cleaned track by track.
func TestRunTracksKeepsFillsInPlace(t *testing.T) {
	line := append(walkStayWalk(), walkStayWalk()...)
	n := len(line) / 2
	for i := n; i < len(line); i++ {
		line[i].Track = 1
	}
	c := Composed{Filled: make([]bool, len(line)), Replaced: make([]bool, len(line)), Spans: [][2]int{{n + 10, n + 20}}}
	c.Filled[n+11], c.Replaced[n+15] = true, true
	p := Defaults()
	p.Stops.Enabled = true
	result := RunTracks(line, p, nil, c)
	if result.Removed[n+15] != Fill || result.Removed[n+11] != Kept || !result.Filled[n+11] {
		t.Fatalf("fill samples: replaced %q, filled %q", result.Removed[n+15], result.Removed[n+11])
	}
}
