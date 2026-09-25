package suggest

import (
	"testing"
	"time"

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
	got := Fields(tpl, card)
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
	all := All(templates, card)
	if all["id_card"]["number"] != "11010519491231002X" || all["id_card"]["expires"] != "2036.01.01" {
		t.Fatalf("got %v", all)
	}
	long := All(templates, "有效期限 2020.05.05-长期")
	if long["id_card"]["expires"] != "长期" {
		t.Fatalf("got %v", long)
	}
}
