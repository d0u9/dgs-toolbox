package tag

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"japan":         "japan",
		"  Japan 2019 ": "japan-2019",
		"Tax\tReturn":   "tax-return",
		"a   b":         "a-b",
		"   ":           "",
		"already-fine":  "already-fine",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListDropsEmptyAndRepeated(t *testing.T) {
	got := List([]string{"Tax", "", "japan 2019", "tax", "japan-2019", "keep"})
	want := []string{"tax", "japan-2019", "keep"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
}

func TestCountOrdersByUseThenName(t *testing.T) {
	got := Count([][]string{
		{"japan-2019", "keep"},
		{"Japan 2019", "japan-2019"},
		{"bank"},
		{"keep"},
		nil,
	})
	want := []Use{{"japan-2019", 2}, {"keep", 2}, {"bank", 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Count = %v, want %v", got, want)
	}
}
