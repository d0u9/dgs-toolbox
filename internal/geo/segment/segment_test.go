package segment

import (
	"reflect"
	"testing"

	"dgs-toolbox/internal/geo/stops"
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
	got := Candidates([]stops.Stop{{First: 10, Last: 20}, {First: 30, Last: 31}})
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
