package outline

import (
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/doc/snapshot"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

func item(id, typ string, fields map[string]string, digests ...string) tree.Item {
	it := tree.Item{ID: id, Type: typ, Kind: tree.KindDocument, Fields: fields}
	for _, d := range digests {
		it.Revisions = append(it.Revisions, tree.Revision{Digest: d})
	}
	it.Head = digests[len(digests)-1]
	return it
}

func TestGroupNestsEveryRuleAndCounts(t *testing.T) {
	items := []tree.Item{
		item("A", "car", map[string]string{"country": "中国", "make": "Honda", "plate": "浙AT73C7"}, "d1"),
		item("B", "car", map[string]string{"country": "CN", "make": "Honda", "plate": "浙AF3897"}, "d2"),
		item("C", "car", map[string]string{"country": "CN", "make": "Honda", "plate": "浙AF3897"}, "d3"),
		item("D", "car", map[string]string{"country": "CN", "make": "Honda"}, "d4"),
		item("E", "passport", map[string]string{"country": "CN", "owner": "alex"}, "d5"),
	}
	o := Outline{Name: "mine", Rules: []view.View{
		{Name: "cars", Selection: view.Head, Query: map[string]view.Values{"type": {"car"}},
			Layout: "{country:alpha2} {make}/{#}-{plate}/{id}.{ext}", Order: map[string][]string{"{plate}": {"浙AF3897", "浙AT73C7"}}},
		{Name: "ids", Selection: view.Head, Query: map[string]view.Values{"type": {"passport"}}, Layout: "ids/{owner}.{ext}"},
	}}
	g, err := Group(o, nil, items, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Root.Count != 4 || len(g.Root.Children) != 2 {
		t.Fatalf("%+v", g.Root)
	}
	top := g.Root.Children[0]
	if top.Name != "CN Honda" || top.Count != 3 || len(top.Children) != 2 {
		t.Fatalf("%+v", top)
	}
	first, second := top.Children[0], top.Children[1]
	if first.Name != "01-浙AF3897" || first.Count != 2 || first.Path != "CN Honda/01-浙AF3897" || len(first.Files) != 2 {
		t.Fatalf("%+v", first)
	}
	if first.Files[0].Path != "CN Honda/01-浙AF3897/B.pdf" || first.Files[0].View != "cars" {
		t.Fatalf("%+v", first.Files[0])
	}
	if second.Name != "02-浙AT73C7" || second.Files[0].Item != "A" {
		t.Fatalf("%+v", second)
	}
	if ids := g.Root.Children[1]; ids.Name != "ids" || ids.Files[0].View != "ids" {
		t.Fatalf("%+v", ids)
	}
	if m := g.Plans["cars"].Missing; len(m) != 1 || m[0].Item != "D" {
		t.Fatalf("%+v", m)
	}
}

func TestSaveRenamesAndRefuses(t *testing.T) {
	root := t.TempDir()
	rule := view.View{Name: "all", Layout: "{owner}.{ext}"}
	if err := Save(root, "", Outline{Name: "a", Folder: "~/x", Rules: []view.View{rule}}); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, "a", Outline{Name: "b", Rules: []view.View{rule}}); err != nil {
		t.Fatal(err)
	}
	list, err := Load(root)
	if err != nil || len(list) != 1 || list[0].Name != "b" || list[0].Rules[0].Selection != view.Head {
		t.Fatalf("%v %+v", err, list)
	}
	if _, err := os.Stat(filepath.Join(root, Dir, "a.yaml")); !os.IsNotExist(err) {
		t.Fatal("a.yaml is still there")
	}
	for _, bad := range []Outline{
		{Name: "C"},
		{Name: "c", Folder: "relative"},
		{Name: "c", Rules: []view.View{rule, rule}},
		{Name: "c", Rules: []view.View{{Name: "r", Layout: "{owner#}"}}},
		{Name: "c", Rules: []view.View{{Name: "r", Layout: "../x"}}},
	} {
		if err := Save(root, "", bad); err == nil {
			t.Fatalf("%+v saved", bad)
		}
	}
	if _, err := Parse([]byte("name: c\nrules: []\nextra: 1\n")); err == nil {
		t.Fatal("an unknown key parsed")
	}
}

func TestMigrateTurnsViewsAndTargetsIntoOutlines(t *testing.T) {
	root := t.TempDir()
	put := func(rel, text string) {
		path := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("targets.yaml", "icloud:\n  about: phone\n  folder: ~/iCloud\nspare: {}\n")
	put("views/ids.yaml", "name: ids\nselection: head\nlayout: '{owner}.{ext}'\ntarget: icloud\n")
	put("views/bills.yaml", "name: bills\nselection: all\nlayout: 'b/{id}.{ext}'\ntarget: icloud\n")
	put("views/spare.yaml", "name: spare\nselection: head\nlayout: '{id}.{ext}'\n")
	names, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := Load(root)
	if len(names) != 3 || len(list) != 3 {
		t.Fatalf("%v %+v", names, list)
	}
	icloud, spare, loose := list[0], list[1], list[2]
	if icloud.Name != "icloud" || icloud.Folder != "~/iCloud" || icloud.About != "phone" || len(icloud.Rules) != 2 || icloud.Rules[0].Name != "bills" {
		t.Fatalf("%+v", icloud)
	}
	if spare.Name != "spare" || len(spare.Rules) != 0 || loose.Name != "spare-view" || loose.Rules[0].Name != "spare" {
		t.Fatalf("%+v %+v", spare, loose)
	}
	for _, gone := range []string{"views", "targets.yaml"} {
		if _, err := os.Stat(filepath.Join(root, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s is still there", gone)
		}
		if _, err := os.Stat(filepath.Join(root, MigratedDir, gone)); err != nil {
			t.Fatal(err)
		}
	}
	if names, err := Migrate(root); err != nil || len(names) != 0 {
		t.Fatalf("a second run: %v %v", names, err)
	}
}

func TestRulesAreSharedRenamedAndSplit(t *testing.T) {
	root := t.TempDir()
	ids := view.View{Name: "ids", Layout: "{owner}.{ext}"}
	cars := view.View{Name: "cars", Layout: "cars/{id}.{ext}"}
	if err := Save(root, "", Outline{Name: "phone", Rules: []view.View{ids}}); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, "", Outline{Name: "kindle", Rules: []view.View{ids, cars}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, Dir, "phone.yaml"))
	if string(data) != "name: phone\nrules:\n    - ids\n" {
		t.Fatalf("%q", data)
	}
	// Editing the rule in one Outline changes it in both.
	ids.Layout = "ids/{owner}.{ext}"
	if err := Save(root, "", Outline{Name: "phone", Rules: []view.View{ids}}); err != nil {
		t.Fatal(err)
	}
	list, err := Load(root)
	if err != nil || list[0].Name != "kindle" || list[0].Rules[0].Layout != "ids/{owner}.{ext}" {
		t.Fatalf("%v %+v", err, list)
	}
	if err := Fresh(root, []string{"ids"}); err == nil {
		t.Fatal("a new rule could replace ids")
	}
	if err := RenameRule(root, "ids", "people"); err != nil {
		t.Fatal(err)
	}
	rules, used, err := Rules(root)
	if err != nil || len(rules) != 2 || rules[1].Name != "people" || len(used["people"]) != 2 {
		t.Fatalf("%v %+v %v", err, rules, used)
	}
	if err := DeleteRule(root, "people"); err == nil {
		t.Fatal("a rule in use was deleted")
	}
	if err := Delete(root, "kindle"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteRule(root, "cars"); err != nil {
		t.Fatal(err)
	}

	// An Outline written with its rules whole is split; a rule of the same
	// name but different is renamed.
	whole := "name: old\nrules:\n  - name: people\n    layout: 'x/{id}.{ext}'\n  - name: bills\n    layout: 'b/{id}.{ext}'\n"
	if err := os.WriteFile(filepath.Join(root, Dir, "old.yaml"), []byte(whole), 0o644); err != nil {
		t.Fatal(err)
	}
	if names, err := Migrate(root); err != nil || len(names) != 1 {
		t.Fatalf("%v %v", names, err)
	}
	list, _ = Load(root)
	if old := list[0]; old.Name != "old" || old.Rules[0].Name != "old-people" || old.Rules[1].Name != "bills" {
		t.Fatalf("%+v", old)
	}
	if names, err := Migrate(root); err != nil || len(names) != 0 {
		t.Fatalf("a second run: %v %v", names, err)
	}
}

// A Snapshot is a folder of its name at the path the Outline gives it,
// checked against the rules' PDFs like another rule's.
func TestGroupMountsSnapshots(t *testing.T) {
	items := []tree.Item{
		item("A", "passport", map[string]string{"owner": "alex"}, "d1", "d2"),
		item("B", "passport", map[string]string{"owner": "emma"}, "d3"),
	}
	items[0].Revisions[0].ID, items[0].Revisions[1].ID, items[1].Revisions[0].ID = "r1", "r2", "r3"
	items[0].Head, items[1].Head = "r2", "r3"
	visa := snapshot.Snapshot{Name: "visa", Taken: "2026-09-26T10:00:00Z", Files: []snapshot.File{
		{Path: "alex.pdf", Item: "A", Revision: "r1"},
		{Path: "gone.pdf", Item: "Z", Revision: "r9"},
	}}
	o := Outline{Name: "mine",
		Rules:     []view.View{{Name: "ids", Selection: view.Head, Layout: "ids/{owner}.{ext}"}},
		Snapshots: []Mount{{Name: "visa", At: "ids/2026"}}}
	g, err := Group(o, []snapshot.Snapshot{visa}, items, nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := g.Root.Children[0]
	if ids.Count != 3 || len(ids.Children) != 1 || ids.Children[0].Name != "2026" {
		t.Fatalf("%+v", ids)
	}
	folder := ids.Children[0].Children[0]
	if folder.Snapshot != "visa" || folder.Path != "ids/2026/visa" || folder.Files[0].Path != "ids/2026/visa/alex.pdf" || folder.Files[0].Digest != "d1" || folder.Files[0].View != "visa" {
		t.Fatalf("%+v", folder)
	}
	if len(g.Lost) != 1 || g.Lost[0].Path != "ids/2026/visa/gone.pdf" || g.Complete() {
		t.Fatalf("%+v", g.Lost)
	}

	// Put where a rule's PDF is, it clashes and the export fails.
	o.Snapshots = []Mount{{Name: "visa", At: ""}}
	o.Rules[0].Layout = "visa/{owner}.{ext}"
	visa.Files = visa.Files[:1]
	if g, err = Group(o, []snapshot.Snapshot{visa}, items, nil); err != nil || len(g.Clashes) != 1 || g.Clashes[0].Path != "visa/alex.pdf" {
		t.Fatalf("%v %+v", err, g.Clashes)
	}
	if _, err := Group(o, nil, items, nil); err == nil {
		t.Fatal("a Snapshot the tree lacks was put in")
	}
	o.Snapshots = []Mount{{Name: "ids"}}
	if err := o.Validate(); err != nil {
		t.Fatalf("a Snapshot could not share a rule's name: %v", err)
	}
	o.Snapshots = []Mount{{Name: "ids"}, {Name: "ids", At: "old"}}
	if err := o.Validate(); err == nil {
		t.Fatal("two Snapshots of one name were put in")
	}
	o.Snapshots = []Mount{{Name: "visa", At: "../out"}}
	if err := o.Validate(); err == nil {
		t.Fatal("a Snapshot was put outside the tree")
	}
	o.Snapshots = []Mount{{Name: "visa", At: "ids", As: "a/b"}}
	if err := o.Validate(); err == nil {
		t.Fatal("a Snapshot's folder name held a /")
	}

	// As names its folder in place of the Snapshot's name.
	o.Rules[0].Layout = "ids/{owner}.{ext}"
	o.Snapshots = []Mount{{Name: "visa", At: "ids", As: "签证"}}
	if g, err = Group(o, []snapshot.Snapshot{visa}, items, nil); err != nil {
		t.Fatal(err)
	}
	if f := g.Root.Children[0].Children[0]; f.Snapshot != "visa" || f.Path != "ids/签证" || f.Files[0].Path != "ids/签证/alex.pdf" {
		t.Fatalf("%+v", f)
	}
}

// A rule numbering a key the way it once was is written the way it is now.
func TestMigrateRewritesOldNumbering(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, RulesDir), 0o755)
	old := "name: ids\nselection: head\nlayout: '{owner}#/{country#:alpha3}/{name|type:zh}#.{ext}'\norder:\n  owner: [alex]\n  country: [CN]\n  name|type:zh: [身份证]\n"
	if err := os.WriteFile(filepath.Join(root, RulesDir, "ids.yaml"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := Migrate(root)
	if err != nil || len(names) != 1 || names[0] != "rules/ids" {
		t.Fatalf("%v %v", names, err)
	}
	rules, err := LoadRules(root)
	if err != nil || rules["ids"].Layout != "{#}-{owner}/{#}-{country:alpha3}/{#}-{name|type:zh}.{ext}" || rules["ids"].Order["{name|type:zh}"][0] != "身份证" {
		t.Fatalf("%+v %v", rules["ids"], err)
	}
	if names, err := Migrate(root); err != nil || len(names) != 0 {
		t.Fatalf("again: %v %v", names, err)
	}
}
