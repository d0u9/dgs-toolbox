package outline

import (
	"os"
	"path/filepath"
	"testing"

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

func TestGroupNumbersFoldersAndCounts(t *testing.T) {
	items := []tree.Item{
		item("A", "car", map[string]string{"country": "中国", "make": "Honda", "plate": "浙AT73C7"}, "d1"),
		item("B", "car", map[string]string{"country": "CN", "make": "Honda", "plate": "浙AF3897"}, "d2"),
		item("C", "car", map[string]string{"country": "CN", "make": "Honda", "plate": "浙AF3897"}, "d3"),
		item("D", "car", map[string]string{"country": "CN", "make": "Honda"}, "d4"),
		item("E", "passport", map[string]string{"country": "CN"}, "d5"),
	}
	o := Outline{Name: "cars", Query: map[string]view.Values{"type": {"car"}}, Layout: "{country:alpha2} {make}/{plate#}",
		Order: map[string][]string{"plate": {"浙AF3897", "浙AT73C7"}}}
	g, err := Group(o, items, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Root.Count != 3 || len(g.Root.Children) != 1 {
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
	if second.Name != "02-浙AT73C7" || second.Files[0].Item != "A" {
		t.Fatalf("%+v", second)
	}
	if len(g.Missing) != 1 || g.Missing[0].Item != "D" {
		t.Fatalf("%+v", g.Missing)
	}
}

func TestSaveRenamesAndRefuses(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, "", Outline{Name: "a", Layout: "{owner}"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, "a", Outline{Name: "b", Layout: "{owner}"}); err != nil {
		t.Fatal(err)
	}
	list, err := Load(root)
	if err != nil || len(list) != 1 || list[0].Name != "b" || list[0].Selection != view.Head {
		t.Fatalf("%v %+v", err, list)
	}
	if _, err := os.Stat(filepath.Join(root, Dir, "a.yaml")); !os.IsNotExist(err) {
		t.Fatal("a.yaml is still there")
	}
	for _, bad := range []Outline{{Name: "c", Layout: "{owner#}"}, {Name: "C", Layout: "x"}, {Name: "c", Layout: "../x"}} {
		if err := Save(root, "", bad); err == nil {
			t.Fatalf("%+v saved", bad)
		}
	}
	if _, err := Parse([]byte("name: c\nlayout: x\nextra: 1\n")); err == nil {
		t.Fatal("an unknown key parsed")
	}
}
