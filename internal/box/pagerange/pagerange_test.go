package pagerange_test

import (
	"reflect"
	"testing"

	"dgs-toolbox/internal/box/pagerange"
)

func TestParseKeepsOrderAndOverlap(t *testing.T) {
	got, err := pagerange.Parse(" 2-5, 1-3 ,6,")
	if err != nil {
		t.Fatal(err)
	}
	want := []pagerange.Range{{2, 5}, {1, 3}, {6, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if text := pagerange.Format(got); text != "2-5,1-3,6" {
		t.Errorf("format: %q", text)
	}
}

func TestParseRefusesNonsense(t *testing.T) {
	for _, text := range []string{"0", "3-1", "a", "1-", "-2", "1-2-3"} {
		if _, err := pagerange.Parse(text); err == nil {
			t.Errorf("%q: accepted", text)
		}
	}
}

func TestParseEmptyIsNothing(t *testing.T) {
	got, err := pagerange.Parse("  ")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestWithin(t *testing.T) {
	ranges := []pagerange.Range{{1, 3}, {4, 6}}
	if err := pagerange.Within(ranges, 6); err != nil {
		t.Errorf("6 pages: %v", err)
	}
	if err := pagerange.Within(ranges, 5); err == nil {
		t.Error("5 pages: accepted page 6")
	}
}

func TestCompact(t *testing.T) {
	got := pagerange.Compact([]int{7, 2, 3, 1, 9, 8, 3})
	want := []pagerange.Range{{1, 3}, {7, 9}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMissing(t *testing.T) {
	got := pagerange.Missing(10, []pagerange.Range{{1, 3}, {2, 5}}, []pagerange.Range{{6, 6}, {9, 9}})
	want := []int{7, 8, 10}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
