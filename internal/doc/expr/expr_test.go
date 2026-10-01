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
		"type == bill":                                 true,
		"type == money":                                true,
		"type == payment":                              false,
		"category == utility && type == money":         true,
		"tags in [network-6, network-5]":                     true,
		"tags == paid && tags != late":                 true,
		"!(type == bill || type == invoice)":           false,
		"name ~ water":                                 true,
		"has(number)":                                  false,
		"!has(number) && has(name)":                    true,
		"number == 1 || type == bill":                  true,
		`name == "Water Bill"`:                         true,
		"type==bill&&(category == 电 || category == 水)": true,
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
	e := MustParse("type == bill && (about.type == tenancy || type == bill) && has(number)")
	if got := strings.Join(e.Keys(), ","); got != "type,about.type,number" {
		t.Fatal(got)
	}
}

func TestParseErrors(t *testing.T) {
	for s, want := range map[string]string{
		"":                "empty",
		"type bill":       "column 6",
		"type is a":       "column 6",
		"a & b":           "write &&",
		"has number":      "has takes a key",
		"type ==":         "column 8",
		"type in bill":    "in takes a list",
		"type in [a b]":   "expected , or ]",
		"(type == a":      "not closed",
		"type == a b":     "column 11",
		`name == "open`:   "quote",
		"== a":            "expected a key",
		"type == a || ||": "expected a key",
	} {
		_, err := Parse(s)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", s, err, want)
		}
	}
}

func TestTree(t *testing.T) {
	got := Tree(MustParse("a == 1 && b != 2 && !(c in [x, y] || has(d)) && e ~ f"))
	if got.Op != "&&" || len(got.Items) != 4 {
		t.Fatalf("%+v", got)
	}
	if b := got.Items[1]; b.Key != "b" || b.Cmp != "==" || !b.Not {
		t.Fatalf("%+v", b)
	}
	if g := got.Items[2]; g.Op != "||" || !g.Not || g.Items[0].Cmp != "in" || len(g.Items[0].Values) != 2 || g.Items[1].Cmp != "has" {
		t.Fatalf("%+v", g)
	}
	if one := Tree(MustParse("a == 1")); one.Op != "" || one.Key != "a" {
		t.Fatalf("%+v", one)
	}
}

func TestExplain(t *testing.T) {
	bill := env{"type": {"bill"}, "tags": {"network-5"}}
	got := Explain(MustParse("type == money || tags != network-5 && !(has(number))"), bill)
	if !got.Met || got.Op != "||" || len(got.Parts) != 2 {
		t.Fatalf("%+v", got)
	}
	// Asked although the first part already held.
	and := got.Parts[1]
	if and.Met || and.Op != "&&" || len(and.Parts) != 2 {
		t.Fatalf("%+v", and)
	}
	if tags := and.Parts[0]; tags.Met || !tags.Not || tags.Held[0] != "network-5" {
		t.Fatalf("%+v", tags)
	}
	if number := and.Parts[1]; !number.Met || !number.Not || number.Cmp != "has" || number.Held != nil {
		t.Fatalf("%+v", number)
	}
}
