package tree

import (
	"reflect"
	"testing"
)

func TestFieldValues(t *testing.T) {
	items := []Item{
		{Type: "visa", Fields: map[string]string{"country": "Australia", "issuer": " Agency "}, Revisions: []Revision{
			{ID: "old", Snapshot: true, Type: "visa", Fields: map[string]string{"country": "澳大利亚", "issuer": "Former Agency"}},
			{ID: "foreign", Snapshot: true, Type: "visa", Fields: map[string]string{"country": "CN", "issuer": "Foreign Agency"}},
			{ID: "other", Snapshot: true, Type: "official_letter", Fields: map[string]string{"country": "AU", "issuer": "Other type"}},
		}},
		{Type: "bank_card", Fields: map[string]string{"country": "AU", "issuer": "Bank"}},
		{Type: "visa", Retired: true, Fields: map[string]string{"country": "AUS", "issuer": "Agency"}},
		{Type: "visa", Fields: map[string]string{"issuer": "Unknown country"}},
	}
	if got, want := FieldValues(items, "visa", "AU", "issuer"), []string{"Agency", "Former Agency"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, nation := range []string{"", "invalid", "NZ"} {
		if got := FieldValues(items, "visa", nation, "issuer"); got == nil || len(got) != 0 {
			t.Fatalf("%q: %v", nation, got)
		}
	}
}
