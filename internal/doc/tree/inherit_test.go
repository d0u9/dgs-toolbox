package tree

import (
	"strings"
	"testing"
)

func parseAll(t *testing.T, files ...string) ([]Template, error) {
	t.Helper()
	var raw []Template
	for _, f := range files {
		tpl, err := ParseTemplate([]byte(f))
		if err != nil {
			return nil, err
		}
		raw = append(raw, tpl)
	}
	return Resolve(raw)
}

const recordBase = `type: record
abstract: true
fields:
  - {key: owner, required: true}
  - {key: country, type: country, required: true}
  - {key: name}
  - {key: date, type: date, shape: [day, span]}
defaults: {country: CN}
`

func TestATypeInheritsItsParentsFields(t *testing.T) {
	all, err := parseAll(t, recordBase, `type: money
extends: record
abstract: true
fields:
  - key: category
    type: select
    values: {utility: [水, 电], housing: [房租]}
`, `type: bill
extends: money
kind: record
fields:
  - {key: name, required: true, distinguishing: true, description: what the bill is for}
  - {key: date, shape: span}
  - {key: due, type: date}
`)
	if err != nil {
		t.Fatal(err)
	}
	bill := all[0]
	if bill.Type != "bill" || strings.Join(bill.Lineage, ">") != "bill>money>record" {
		t.Fatalf("got %s %v", bill.Type, bill.Lineage)
	}
	var keys []string
	for _, f := range bill.Fields {
		keys = append(keys, f.Key)
	}
	if got := strings.Join(keys, ","); got != "owner,country,name,date,category,due" {
		t.Fatalf("fields %s", got)
	}
	name, date, category := bill.field("name"), bill.field("date"), bill.field("category")
	if !name.Distinguishing || name.Description != "what the bill is for" {
		t.Errorf("name %+v", name)
	}
	if strings.Join(category.Options, ",") != "水,电,房租" {
		t.Errorf("options %v", category.Options)
	}
	if group, ok := category.Group("utility"); !ok || len(group) != 2 {
		t.Errorf("group %v", group)
	}
	if bill.Defaults["country"] != "CN" || !bill.Is("money") || bill.Is("payment") {
		t.Errorf("defaults %v or lineage", bill.Defaults)
	}
	if _, err := date.clean("2025-01-01"); err == nil {
		t.Error("a bill's date is a span, and took one day")
	}
	if v, err := date.clean("2025-01-01/2025-03-31"); err != nil || v != "2025-01-01/2025-03-31" {
		t.Errorf("span %q %v", v, err)
	}
}

func TestInheritanceRefuses(t *testing.T) {
	for name, child := range map[string]string{
		"a changed type":  "type: x\nextends: record\nkind: record\nfields:\n  - {key: name, type: date}\n",
		"a widened shape": "type: x\nextends: day\nkind: record\nfields:\n  - {key: date, shape: span}\n",
		"new options":     "type: x\nextends: record\nkind: record\nfields:\n  - {key: name, type: select, options: [a]}\n",
		"no parent":       "type: x\nextends: nothing\nkind: record\n",
		"no kind":         "type: x\nextends: record\n",
		"a cycle":         "",
	} {
		files := []string{recordBase, "type: day\nextends: record\nabstract: true\nfields:\n  - {key: date, shape: day}\n", child}
		if name == "a cycle" {
			files = []string{"type: x\nextends: y\nkind: record\n", "type: y\nextends: x\nkind: record\n"}
		}
		if _, err := parseAll(t, files...); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSplitSpan(t *testing.T) {
	for value, want := range map[string]string{
		"2025-01-01":            "2025-01-01 2025-01-01 true",
		"2025-01-01/2025-12-31": "2025-01-01 2025-12-31 true",
		"2025-01-01/":           "2025-01-01  true",
		"2025-12-31/2025-01-01": "  false",
		"/2025-01-01":           " 2025-01-01 true",
		"/":                     "  false",
	} {
		a, b, ok := SplitSpan(value)
		if got := a + " " + b + " " + map[bool]string{true: "true", false: "false"}[ok]; got != want {
			t.Errorf("%s: %q", value, got)
		}
	}
}

func TestValueNamesAreChecked(t *testing.T) {
	for _, bad := range []string{
		"type: x\nkind: record\nfields:\n  - key: owner\n    required: true\n  - key: country\n    type: country\n    required: true\n  - key: a\n    names: {en: {b: c}}\n",
		"type: x\nkind: record\nfields:\n  - key: owner\n    required: true\n  - key: country\n    type: country\n    required: true\n  - key: a\n    type: select\n    options: [b]\n    names: {en: {z: c}}\n",
		"type: x\nkind: record\nfields:\n  - key: owner\n    required: true\n  - key: country\n    type: country\n    required: true\n  - key: a\n    type: select\n    options: [b]\n    names: {fr: {b: c}}\n",
	} {
		if _, err := ParseTemplate([]byte(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	tpl, err := ParseTemplate([]byte("type: x\nkind: record\nfields:\n  - key: owner\n    required: true\n  - key: country\n    type: country\n    required: true\n  - key: a\n    type: select\n    values: {g: [b]}\n    names: {en: {b: c}}\n"))
	if err != nil || tpl.Fields[2].ValueName("b", "en") != "c" {
		t.Fatalf("%v %+v", err, tpl)
	}
}

// A text field that suggests may list options, offered before the values
// saved; any other value is still taken. Without suggest, options belong
// to a select field.
func TestSuggestingTextFieldTakesOptions(t *testing.T) {
	child := "type: x\nextends: record\nkind: record\nfields:\n  - {key: driver, suggest: true, options: [alice, bob]}\n"
	templates, err := parseAll(t, recordBase, child)
	if err != nil {
		t.Fatal(err)
	}
	var x Template
	for _, tpl := range templates {
		if tpl.Type == "x" {
			x = tpl
		}
	}
	if fields, err := CleanFields(x, map[string]string{"owner": "alice", "driver": "carol"}); err != nil || fields["driver"] != "carol" {
		t.Fatalf("a value not in the options: %v %v", fields, err)
	}
	plain := "type: x\nextends: record\nkind: record\nfields:\n  - {key: driver, options: [alice, bob]}\n"
	if _, err := parseAll(t, recordBase, plain); err == nil {
		t.Fatal("options on a text field that does not suggest were taken")
	}
}

// An inherited text field may be given suggestions in a child: they offer
// values and narrow none.
func TestInheritedTextFieldTakesSuggestions(t *testing.T) {
	child := "type: x\nextends: record\nkind: record\nfields:\n  - {key: name, suggest: true, options: [罚单, 处罚决定书]}\n"
	templates, err := parseAll(t, recordBase, child)
	if err != nil {
		t.Fatal(err)
	}
	for _, tpl := range templates {
		if tpl.Type != "x" {
			continue
		}
		for _, f := range tpl.Fields {
			if f.Key == "name" && (!f.Suggest || len(f.Options) != 2) {
				t.Fatalf("suggestions not taken: %+v", f)
			}
		}
	}
}

func TestWhenAsksForAFieldOnlyForListedValues(t *testing.T) {
	parent := "type: paid\nextends: record\nabstract: true\nfields:\n  - {key: category, type: select, options: [医疗, 其他]}\n"
	child := "type: x\nextends: paid\nkind: record\nfields:\n  - {key: form, suggest: true, options: [发票, 回执]}\n  - {key: category, required: true, when: {form: [发票]}}\n"
	templates, err := parseAll(t, recordBase, parent, child)
	if err != nil {
		t.Fatal(err)
	}
	var x Template
	for _, tpl := range templates {
		if tpl.Type == "x" {
			x = tpl
		}
	}
	if _, err := CleanFields(x, map[string]string{"owner": "alice", "form": "发票"}); err == nil || !strings.Contains(err.Error(), "category") {
		t.Fatalf("a listed value should require category: %v", err)
	}
	fields, err := CleanFields(x, map[string]string{"owner": "alice", "form": "回执", "category": "医疗"})
	if err != nil {
		t.Fatal(err)
	}
	if _, kept := fields["category"]; kept {
		t.Fatalf("an unlisted value should drop category: %v", fields)
	}
	bad := "type: y\nextends: record\nkind: record\nfields:\n  - {key: note, when: {nothing: [a]}}\n"
	if _, err := parseAll(t, recordBase, bad); err == nil {
		t.Fatal("when naming no field should be refused")
	}
}
