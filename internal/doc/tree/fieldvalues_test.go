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

// One authority issues the same type to everyone in a region, so another
// owner's issuer is a candidate too.
func TestFieldValuesIgnoreOwner(t *testing.T) {
	items := []Item{
		{Type: "driver_licence", Fields: map[string]string{"owner": "alex", "country": "CN", "issuer": "杭州交警支队"}},
		{Type: "driver_licence", Fields: map[string]string{"owner": "emma", "country": "中国", "issuer": "上海交警总队"}},
	}
	if got, want := FieldValues(items, "driver_licence", "CN", "issuer"), []string{"上海交警总队", "杭州交警支队"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTypeValuesAnyCountry(t *testing.T) {
	items := []Item{
		{Type: "payment", Fields: map[string]string{"country": "AU", "via": "CBA"}},
		{Type: "payment", Fields: map[string]string{"country": "CN", "via": " 支付宝 "}},
		{Type: "payment", Fields: map[string]string{"via": "CBA"}},
		{Type: "invoice", Fields: map[string]string{"via": "Other type"}},
	}
	if got, want := TypeValues(items, "payment", "via"), []string{"CBA", "支付宝"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
