package expr

import (
	"strings"
	"testing"
)

type env map[string][]string

func (e env) Values(key string) []string { return e[key] }

// Is knows money above bill and utility above 水.
func (e env) Is(key, value, want string) bool {
	above := map[string]string{"bill": "money", "水": "utility"}
	return value == want || above[value] == want
}

func TestEval(t *testing.T) {
	bill := env{"type": {"bill"}, "category": {"水"}, "tags": {"network-5", "paid"}, "name": {"Water Bill"}}
	for s, want := range map[string]bool{
		"type is bill":                                      true,
		"type is money":                                     true,
		"type is payment":                                   false,
		"category is utility and type is money":             true,
		"tags in [network-6, network-5]":                          true,
		"tags has paid and not tags has late":               true,
		"not (type is bill or type is invoice)":             false,
		"name contains water":                               true,
		"has number":                                        false,
		"not has number and has name":                       true,
		"number is 1 or type is bill":                       true,
		`name is "Water Bill"`:                              true,
		"type is bill and (category is 电 or category is 水)": true,
	} {
		e, err := Parse(s)
		if err != nil {
			t.Errorf("%s: %v", s, err)
			continue
		}
		if got := e.Eval(bill); got != want {
			t.Errorf("%s: got %v", s, got)
		}
	}
}

func TestKeys(t *testing.T) {
	e := MustParse("type is bill and (about.type is tenancy or type is bill) and has number")
	if got := strings.Join(e.Keys(), ","); got != "type,about.type,number" {
		t.Fatal(got)
	}
}

func TestParseErrors(t *testing.T) {
	for s, want := range map[string]string{
		"":                "empty",
		"type bill":       "column 6",
		"type is":         "column 8",
		"type in bill":    "in takes a list",
		"type in [a b]":   "expected , or ]",
		"(type is a":      "not closed",
		"type is a b":     "column 11",
		`name is "open`:   "quote",
		"and is a":        "expected a key",
		"type is a or or": "expected a key",
	} {
		_, err := Parse(s)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", s, err, want)
		}
	}
}
