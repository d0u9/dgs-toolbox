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
