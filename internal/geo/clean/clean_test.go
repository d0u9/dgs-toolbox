package clean

import (
	"encoding/json"
	"testing"
	"time"

	"dgs-toolbox/internal/geo"
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
	p.Removed = []int{60, 61, 9999} // the spike is removed by hand first; 9999 is ignored
	result := Run(line, p)
	if result.Removed[60] != Manual || result.Removed[61] != Manual {
		t.Fatalf("manual removals: %q %q", result.Removed[60], result.Removed[61])
	}
	counts := result.Counts()
	if counts.Spike != 0 || counts.Stop < 500 || counts.Moved != 1 {
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
