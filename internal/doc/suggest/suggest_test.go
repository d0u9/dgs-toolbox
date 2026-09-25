package suggest

import (
	"reflect"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/dates"
	"dgs-toolbox/internal/doc/tree"
)

const card = `中华人民共和国
居民身份证
签发机关 某市公安局
有效期限 2016.01.01-2036.01.01
公民身份号码 11010519491231002X`

func TestFields(t *testing.T) {
	tpl := tree.Template{Type: "x", Kind: tree.KindDocument, Fields: []tree.Field{
		{Key: "number", Pattern: `(\d{17}[\dXx])`},
		{Key: "whole", Pattern: `居民身份证`},
		{Key: "missing", Pattern: `passport (\w+)`},
		{Key: "plain"},
		{Key: "broken", Pattern: `(`},
	}}
	got := Fields(tpl, card, dates.DMY)
	want := map[string]string{"number": "11010519491231002X", "whole": "居民身份证"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// The example Template init writes reads the example card.
func TestExampleTemplate(t *testing.T) {
	root := t.TempDir()
	if err := tree.Init(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	templates, err := tree.LoadTemplates(root)
	if err != nil {
		t.Fatal(err)
	}
	all := All(templates, card, dates.DMY)
	number := all["id_card"]["number"]
	if number.Value != "11010519491231002X" || card[number.Start:number.End] != number.Value || all["id_card"]["expires"].Value != "2036.01.01" {
		t.Fatalf("got %v", all)
	}
	long := All(templates, "有效期限 2020.05.05-长期", dates.DMY)
	if long["id_card"]["expires"].Value != "长期" {
		t.Fatalf("got %v", long)
	}
	// Recognition often reads the dash as 一, — or －.
	for _, dash := range []string{"一", "—", "－", " - "} {
		got := All(templates, "有效期限 2016.01.01"+dash+"2036.01.01", dates.DMY)
		if got["id_card"]["expires"].Value != "2036.01.01" {
			t.Errorf("dash %q: got %v", dash, got)
		}
	}
}

func TestMatchPointsAtTheTrimmedValue(t *testing.T) {
	tpl := tree.Template{Fields: []tree.Field{{Key: "k", Pattern: `name:(\s*\w+)`}}}
	text := "x\nname:  jane"
	m := Matches(tpl, text, dates.DMY)["k"]
	if m.Value != "jane" || text[m.Start:m.End] != "jane" {
		t.Fatalf("%+v", m)
	}
}

func TestDateFields(t *testing.T) {
	tpl := tree.Template{Type: "visa", Kind: tree.KindRecord, IgnoreDates: []string{"1990-04-03"},
		Fields: []tree.Field{
			{Key: "birth", Type: tree.FieldDate, Pattern: `born (\S+)`},
			{Key: "issued_at", Type: tree.FieldDate},
			{Key: "expires", Type: tree.FieldDate},
			{Key: "odd", Type: tree.FieldDate, Pattern: `until (\S+)`},
		}}
	text := "born 03/04/1990 granted 2026年9月25日 valid to 25/09/2031 until 长期"
	got := Fields(tpl, text, dates.DMY)
	want := map[string]string{"birth": "1990-04-03", "issued_at": "2026-09-25", "expires": "2031-09-25"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	m := Matches(tpl, text, dates.DMY)["issued_at"]
	if text[m.Start:m.End] != "2026年9月25日" {
		t.Fatalf("offsets %q", text[m.Start:m.End])
	}
}

func TestCountryFieldIsNormalized(t *testing.T) {
	tpl := tree.Template{Fields: []tree.Field{{Key: "country", Type: tree.FieldCountry, Format: "alpha3", Pattern: `Nationality[:：]?\s*(\S+)`}}}
	if got := Fields(tpl, "Nationality: 中华人民共和国", dates.DMY); got["country"] != "CHN" {
		t.Fatalf("got %v", got)
	}
	if got := Fields(tpl, "Nationality: Atlantis", dates.DMY); len(got) != 0 {
		t.Fatalf("an unknown country was suggested: %v", got)
	}
}

func TestPatternsAreTriedInOrder(t *testing.T) {
	tmpl := tree.Template{Type: "card", Fields: []tree.Field{{Key: "expires", Type: tree.FieldDate, Patterns: []string{
		`有效期限.*[-－]\s*(\d{4}\.\d{2}\.\d{2})`,
		`(?i)expiry\W*(\d{2}/\d{2}/\d{4})`,
		`(?i)valid until\W*(\S+)`,
	}}}}
	for text, want := range map[string]string{
		card:                          "2036-01-01",
		"Date of expiry: 03/04/2031":  "2031-04-03",
		"Valid until: forever":        "", // no pattern finds a date
		"valid until 12/05/2030":      "2030-05-12",
		"Expiry: someday soon 2030\n": "",
	} {
		got := Matches(tmpl, text, dates.DMY)["expires"].Value
		if got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

func TestTheExampleReadsAnEnglishExpiry(t *testing.T) {
	root := t.TempDir()
	if err := tree.Init(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	templates, _ := tree.LoadTemplates(root)
	got := All(templates, "DRIVER LICENCE\nExpiry: 03/04/2031", dates.DMY)
	if got["id_card"]["expires"].Value != "03/04/2031" {
		t.Fatalf("got %v", got)
	}
}
