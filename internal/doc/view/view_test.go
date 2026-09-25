package view

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dgs-toolbox/internal/doc/tree"
)

func item(id, typ string, fields map[string]string, digests ...string) tree.Item {
	it := tree.Item{ID: id, Type: typ, Kind: tree.KindDocument, Fields: fields}
	for _, d := range digests {
		it.Revisions = append(it.Revisions, tree.Revision{Digest: d})
	}
	it.Head = digests[len(digests)-1]
	return it
}

func TestParse(t *testing.T) {
	l, err := Parse("{country}/{owner}/important/{type}.{ext}")
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Keys(); !reflect.DeepEqual(got, []string{"country", "owner", "type", "ext"}) {
		t.Fatalf("keys %v", got)
	}
	for _, bad := range []string{"", "/a", "a//b", "a/", "../a", "a/./b", "{a", "a}", "{A}", `a\b`, "{}"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		"jane":   "jane",
		"a/b":    "a_b",
		`c:\d`:   "c__d",
		"..":     "_",
		" .x. ":  "x",
		"a\tb":   "a_b",
		"what?*": "what__",
		"中国":     "中国",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildHeadAndQuery(t *testing.T) {
	items := []tree.Item{
		item("B", "id_card", map[string]string{"owner": "jane", "country": "AU"}, "d1", "d2"),
		item("A", "passport", map[string]string{"owner": "tom", "country": "CN"}, "d3"),
		item("C", "id_card", map[string]string{"owner": "tom", "country": "CN"}, "d4"),
	}
	v := View{Name: "x", Selection: Head, Layout: "{country}/{owner}/{type}.{ext}",
		Query: map[string]Values{"type": {"id_card"}}}
	plan, err := Build(v, items)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path+"@"+f.Digest)
	}
	want := []string{"AU/jane/id_card.pdf@d2", "CN/tom/id_card.pdf@d4"}
	if !reflect.DeepEqual(paths, want) || !plan.Complete() {
		t.Fatalf("got %v %+v", paths, plan)
	}
}

func TestBuildAllRevisions(t *testing.T) {
	items := []tree.Item{item("A", "id_card", map[string]string{"owner": "jane"}, "d1", "d2")}
	plan, _ := Build(View{Name: "x", Selection: All, Layout: "{owner}/{type}-{revision}.{ext}"}, items)
	if len(plan.Files) != 2 || plan.Files[0].Path != "jane/id_card-1.pdf" || plan.Files[1].Path != "jane/id_card-2.pdf" {
		t.Fatalf("%+v", plan)
	}
}

func TestBuildMissingAndDefault(t *testing.T) {
	items := []tree.Item{item("A", "id_card", map[string]string{"owner": "jane"}, "d1")}
	v := View{Name: "x", Selection: Head, Layout: "{country}/{owner}/{year}.{ext}"}
	plan, _ := Build(v, items)
	if len(plan.Missing) != 1 || !reflect.DeepEqual(plan.Missing[0].Keys, []string{"country", "year"}) || len(plan.Files) != 0 {
		t.Fatalf("%+v", plan)
	}
	none := "none"
	v.Default = &none
	plan, _ = Build(v, items)
	if !plan.Complete() || plan.Files[0].Path != "none/jane/none.pdf" {
		t.Fatalf("%+v", plan)
	}
}

func TestBuildDates(t *testing.T) {
	items := []tree.Item{item("A", "bill", map[string]string{"issued_at": "2026-09-25"}, "d1")}
	plan, _ := Build(View{Name: "x", Selection: Head, Layout: "{year}/{month}/{date}.{ext}"}, items)
	if plan.Files[0].Path != "2026/09/2026-09-25.pdf" {
		t.Fatalf("%+v", plan)
	}
}

func TestBuildClashAndNumbering(t *testing.T) {
	items := []tree.Item{
		item("B", "bill", map[string]string{"owner": "Jane"}, "d2"),
		item("A", "bill", map[string]string{"owner": "jane"}, "d1"),
		item("C", "bill", map[string]string{"owner": "jane"}, "d3"),
	}
	v := View{Name: "x", Selection: Head, Layout: "{owner}/{type}.{ext}"}
	plan, _ := Build(v, items)
	if len(plan.Clashes) != 1 || len(plan.Clashes[0].Files) != 3 || plan.Complete() {
		t.Fatalf("case should clash: %+v", plan)
	}
	v.Dedupe = DedupeNumber
	plan, _ = Build(v, items)
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Item+":"+f.Path)
	}
	want := []string{"B:Jane/bill_01.pdf", "A:jane/bill.pdf", "C:jane/bill_02.pdf"}
	if !plan.Complete() || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestSaveLoadDelete(t *testing.T) {
	root := t.TempDir()
	none := "none"
	v := View{Name: "important", Selection: Head, Layout: "{owner}/{type}.{ext}",
		Query: map[string]Values{"type": {"id_card", "passport"}, "owner": {"jane"}}, Default: &none}
	if err := Save(root, v); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, Dir, "important.yaml"))
	if !strings.Contains(string(data), "owner: jane") {
		t.Fatalf("single value should be a scalar:\n%s", data)
	}
	got, err := Load(root)
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], v) {
		t.Fatalf("%v %+v", err, got)
	}
	if err := Save(root, View{Name: "Bad", Selection: Head, Layout: "x"}); err == nil {
		t.Fatal("bad name saved")
	}
	if err := Delete(root, "important"); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(root); len(got) != 0 {
		t.Fatal("not deleted")
	}
}

func TestFieldsFor(t *testing.T) {
	got := FieldsFor([]string{"year", "owner", "month", "date"})
	if !reflect.DeepEqual(got, []string{DateField, "owner"}) {
		t.Fatalf("FieldsFor = %v", got)
	}
}

func TestLayoutWritesACountryInAFormat(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "jane", "country": "中国"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "tom", "country": "AU"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "sam", "country": "Atlantis"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	plan, err := Build(View{Name: "v", Selection: Head, Layout: "{country:alpha3}/{owner}-{country:en}.{ext}"}, items)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Path)
	}
	want := []string{"AUS/tom-Australia.pdf", "Atlantis/sam-Atlantis.pdf", "CHN/jane-China.pdf"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v", got)
	}
	for _, bad := range []string{"{country:iso}", "{country:}"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if l, _ := Parse("{country:zh}"); len(l.Keys()) != 1 || l.Keys()[0] != "country" {
		t.Errorf("keys: %v", l.Keys())
	}
}
