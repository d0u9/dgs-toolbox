package view

import (
	"strings"
	"testing"

	"dgs-toolbox/internal/doc/tree"
)

// childrenRule is TestChildren's rule, behind a condition of its own.
func childrenRule() (View, []tree.Item, Types) {
	rec := func(id, typ string, fields map[string]string) tree.Item {
		return tree.Item{ID: id, Type: typ, Kind: tree.KindRecord, Fields: fields, Revisions: []tree.Revision{{Digest: id}}}
	}
	items := []tree.Item{
		rec("A", "contract", map[string]string{"home": "h1", "name": "合同"}),
		rec("B", "bill", map[string]string{"home": "h2", "name": "水费", "category": "水"}),
		rec("E", "photo", map[string]string{"home": "h1", "name": "照片"}),
		rec("F", "photo", map[string]string{"home": "h3", "name": "a/b"}),
		rec("G", "contract", map[string]string{"name": "无家"}),
	}
	types := TypesOf([]tree.Template{
		{Type: "bill", Lineage: []string{"bill", "money"}, Fields: []tree.Field{{Key: "category", Type: tree.FieldSelect,
			Values: tree.ValueGroups{{Name: "utility", Values: []string{"水", "电"}}}}}},
	})
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"home": {"h1", "h2"}},
		If:   "has(name) && home != h3",
		Path: "{#}-{home}", File: "{name}.{ext}",
		Children: []Node{
			{If: "category == utility", Path: "utility", File: "{category}-{name}.{ext}"},
			{If: "type == photo", Exclude: true},
		}}}
	return v, items, types
}

func explainOne(t *testing.T, v View, items []tree.Item, types Types, id string) Explanation {
	t.Helper()
	xs, err := Explain(v, items, types, id)
	if err != nil || len(xs) != 1 {
		t.Fatalf("%s: %v %+v", id, err, xs)
	}
	return xs[0]
}

// An explanation tells what the plan did: every placed file is where its
// explanation says, and every other revision says why it is not placed.
func TestExplainAgreesWithThePlan(t *testing.T) {
	v, items, types := childrenRule()
	plan, err := Build(v, items, types)
	if err != nil {
		t.Fatal(err)
	}
	at := map[string]string{}
	for _, f := range plan.Files {
		at[f.Item] = f.Path
	}
	for _, it := range items {
		x := explainOne(t, v, items, types, it.ID)
		if (x.Result == Placed) != (at[it.ID] != "") || x.Path != at[it.ID] {
			t.Errorf("%s: explained %s at %q, planned at %q", it.ID, x.Result, x.Path, at[it.ID])
		}
	}
}

func TestExplainSteps(t *testing.T) {
	v, items, types := childrenRule()

	b := explainOne(t, v, items, types, "B")
	if b.Result != Placed || b.Path != "02-h2/utility/水-水费.pdf" || b.Layout != "{#}-{home}/utility/{category}-{name}.{ext}" {
		t.Fatalf("%+v", b)
	}
	if len(b.Branches) != 1 || !b.Branches[0].Took || b.Branches[0].Where != "1" || !b.Branches[0].If.Met {
		t.Fatalf("%+v", b.Branches)
	}
	if n := b.Steps[0]; n.Part != "{#}" || n.Order != "home" || n.Name != "h2" || n.Place != 2 || n.Value != "02" {
		t.Fatalf("%+v", n)
	}
	if c := b.Steps[2]; c.Part != "{category}" || c.Value != "水" {
		t.Fatalf("%+v", c)
	}

	e := explainOne(t, v, items, types, "E")
	if e.Result != LeftOut || len(e.Branches) != 2 || e.Branches[0].Took || !e.Branches[1].Took {
		t.Fatalf("%+v", e)
	}

	f := explainOne(t, v, items, types, "F")
	if f.Result != NotSelected || f.If.Met || f.If.Parts[1].Held[0] != "h3" || f.Steps != nil {
		t.Fatalf("%+v", f)
	}

	g := explainOne(t, v, items, types, "G")
	if g.Result != Lacking || g.Lacking[0] != "home" || !g.Steps[0].Skipped || !g.Steps[1].Lacking {
		t.Fatalf("%+v", g)
	}
	text := g.Text()
	for _, want := range []string{"v, revision 1: lacking: home", "yes  has(name)   (holds 无家)", "{#}  not numbered", "{home}  home lacking"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
}

func TestExplainInheritedAndCleaned(t *testing.T) {
	const (
		bachelor = "01K00000000000000000000001"
		english  = "01K00000000000000000000003"
	)
	items := []tree.Item{
		item(bachelor, "diploma", map[string]string{"level": "本科", "name": "学位/证书"}, "d1"),
		item(english, "translation", map[string]string{"language": "en", "name": "degree", "original": bachelor}, "d2"),
	}
	v := View{Name: "x", Selection: Head, Inherit: []string{"original"}, Node: Node{Order: map[string][]string{"level": {"本科"}}, File: "{#}-{level}[/{language}]/{name}.{ext}"}}
	en := explainOne(t, v, items, Types{}, english)
	if level := en.Steps[1]; level.Key != "level" || level.From != "original" {
		t.Fatalf("%+v", level)
	}
	if group := en.Steps[2]; group.Part != "[/{language}]" || group.Value != "/en" || group.Inner[0].Value != "en" {
		t.Fatalf("%+v", group)
	}
	zh := explainOne(t, v, items, Types{}, bachelor)
	if group := zh.Steps[2]; len(group.Left) != 1 || group.Left[0] != "language" {
		t.Fatalf("%+v", group)
	}
	if name := zh.Steps[3]; name.Value != "学位_证书" || name.Raw != "学位/证书" {
		t.Fatalf("%+v", name)
	}
}

func TestExplainClashAcrossRules(t *testing.T) {
	items := []tree.Item{item("A", "id_card", map[string]string{"name": "x"}, "d1")}
	one := View{Name: "one", Selection: Head, Node: Node{File: "{name}.{ext}"}}
	two := View{Name: "two", Selection: Head, Node: Node{File: "{name}.{ext}"}}
	xs, err := ExplainWith([]View{one, two}, []File{{Path: "s/x.pdf", Item: "A", Digest: "d1", Revision: 1, View: "snap"}}, items, Types{}, "A")
	if err != nil || len(xs) != 3 {
		t.Fatalf("%v %+v", err, xs)
	}
	if xs[0].Result != Fixed || xs[0].Rule != "snap" {
		t.Fatalf("%+v", xs[0])
	}
	if xs[1].Result != Clashing || len(xs[1].Clash) != 1 || xs[1].Clash[0].View != "two" {
		t.Fatalf("%+v", xs[1])
	}
}
