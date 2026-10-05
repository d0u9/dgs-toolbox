package segment

import (
	"reflect"
	"testing"
	"time"

	"dgs-toolbox/internal/geo/stops"
	"dgs-toolbox/internal/geo/track"
)

func TestSplitSharesTheCutPoint(t *testing.T) {
	got := Split(10, []int{6, 3, 3, 0, 9, 42})
	want := []Range{{0, 3}, {3, 6}, {6, 9}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Split = %v, want %v", got, want)
	}
	if got := Split(5, nil); !reflect.DeepEqual(got, []Range{{0, 4}}) {
		t.Fatalf("no cuts = %v", got)
	}
	if Split(0, []int{1}) != nil {
		t.Fatal("empty line has segments")
	}
}

func TestCandidatesAreMidStop(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	line := make(track.Line, 32)
	for i := range line {
		line[i].Time = at.Add(time.Duration(i) * time.Second)
	}
	got := Candidates(line, []stops.Stop{{First: 10, Last: 20, Arrival: line[10].Time, Departure: line[20].Time}, {First: 30, Last: 31, Arrival: line[30].Time, Departure: line[31].Time}})
	if !reflect.DeepEqual(got, []int{15, 30}) {
		t.Fatalf("Candidates = %v", got)
	}
}

func TestNameOf(t *testing.T) {
	names := []Name{{Start: 0, Name: "Morning"}, {Start: 15, Name: "Afternoon"}}
	if NameOf(names, 15) != "Afternoon" || NameOf(names, 3) != "" {
		t.Fatal("NameOf")
	}
	if (Params{}).Active() || !(Params{Cuts: []int{1}}).Active() {
		t.Fatal("Active")
	}
}

func TestMergeRangesPreservesGapsAndInput(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input, want []Range
	}{
		{"shared boundary", []Range{{2, 4}, {0, 2}}, []Range{{0, 4}}},
		{"gap", []Range{{4, 6}, {0, 2}}, []Range{{0, 2}, {4, 6}}},
		{"duplicates and contained", []Range{{0, 4}, {0, 4}, {1, 3}}, []Range{{0, 4}}},
		{"recorded break", []Range{{0, 2}, {3, 5}}, []Range{{0, 2}, {3, 5}}},
		{"empty", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]Range(nil), tc.input...)
			if got := Merge(tc.input); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Merge = %v, want %v", got, tc.want)
			}
			if !reflect.DeepEqual(tc.input, before) {
				t.Fatal("Merge changed caller's ranges")
			}
		})
	}
}

func TestCandidateUsesDwellTimeRatherThanSampleCount(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	line := make(track.Line, 12)
	for i := 0; i < 10; i++ {
		line[i].Time = at.Add(time.Duration(i) * time.Second)
	}
	line[10].Time = at.Add(5 * time.Minute)
	line[11].Time = at.Add(10 * time.Minute)
	stop := stops.Stop{First: 0, Last: 11, Arrival: at, Departure: line[11].Time}
	if got := Candidates(line, []stops.Stop{stop}); !reflect.DeepEqual(got, []int{10}) {
		t.Fatalf("cut drifted onto the approach: %v", got)
	}
}
