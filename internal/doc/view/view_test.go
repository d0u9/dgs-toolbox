package view

import (
	"fmt"
	"reflect"
	"sort"
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
		"emma":   "emma",
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
		item("B", "id_card", map[string]string{"owner": "emma", "country": "AU"}, "d1", "d2"),
		item("A", "passport", map[string]string{"owner": "tom", "country": "CN"}, "d3"),
		item("C", "id_card", map[string]string{"owner": "tom", "country": "CN"}, "d4"),
	}
	v := View{Name: "x", Selection: Head, Node: Node{If: "type == id_card", File: "{country}/{owner}/{type}.{ext}"}}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path+"@"+f.Digest)
	}
	want := []string{"AU/emma/id_card.pdf@d2", "CN/tom/id_card.pdf@d4"}
	if !reflect.DeepEqual(paths, want) || !plan.Complete() {
		t.Fatalf("got %v %+v", paths, plan)
	}
}

func TestBuildAllRevisions(t *testing.T) {
	items := []tree.Item{item("A", "id_card", map[string]string{"owner": "emma"}, "d1", "d2")}
	plan, _ := Build(View{Name: "x", Selection: All, Node: Node{File: "{owner}/{type}-{revision}.{ext}"}}, items, Types{})
	if len(plan.Files) != 2 || plan.Files[0].Path != "emma/id_card-1.pdf" || plan.Files[1].Path != "emma/id_card-2.pdf" {
		t.Fatalf("%+v", plan)
	}
}

func TestBuildQueryTags(t *testing.T) {
	a := item("A", "id_card", map[string]string{"owner": "emma"}, "d1", "d2")
	a.Revisions[0].Tags = []string{"original"}
	b := item("B", "id_card", map[string]string{"owner": "tom"}, "d3")
	b.Tags = []string{"Important"}
	c := item("C", "id_card", map[string]string{"owner": "sam"}, "d4")
	v := View{Name: "x", Selection: All, Node: Node{If: "tags in [important, original]", File: "{owner}/{revision}.{ext}"}}
	plan, err := Build(v, []tree.Item{a, b, c}, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	// An Item's tag holds for its every revision, a revision's for itself.
	if want := []string{"emma/1.pdf", "tom/1.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
}

func TestBuildShared(t *testing.T) {
	a := item("A", "letter", map[string]string{"owner": "alex"}, "d1")
	a.SharedWith = []string{"emma"}
	b := item("B", "letter", map[string]string{"owner": "emma"}, "d2")
	v := View{Name: "x", Selection: Head, Node: Node{If: "owner == Emma", File: "{owner}/{id}.{ext}"}}
	paths := func() []string {
		plan, err := Build(v, []tree.Item{a, b}, Types{})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range plan.Files {
			out = append(out, f.Path)
		}
		return out
	}
	if got := paths(); !reflect.DeepEqual(got, []string{"emma/B.pdf"}) {
		t.Fatalf("without shared: %v", got)
	}
	// A shared Item is taken for Emma and keeps its own owner in the path.
	v.Shared = true
	if got := paths(); !reflect.DeepEqual(got, []string{"alex/A.pdf", "emma/B.pdf"}) {
		t.Fatalf("with shared: %v", got)
	}
	if a.Fields["owner"] != "alex" {
		t.Fatalf("matching changed the Item: %v", a.Fields)
	}
	v.If = ""
	if err := v.Validate(); err == nil {
		t.Fatal("shared without an owner in the query was accepted")
	}
}

func TestBuildExcludeAndSkip(t *testing.T) {
	a := item("A", "diploma", map[string]string{"owner": "emma", "name": "毕业证书"}, "d1")
	b := item("B", "diploma", map[string]string{"owner": "emma", "name": "DIPLOMA"}, "d2")
	b.Tags = []string{"Translation"}
	c := item("C", "diploma", map[string]string{"owner": "emma", "name": "学位证书"}, "d3", "d4")
	c.Revisions[1].Tags = []string{"copy"}
	d := item("D", "diploma", map[string]string{"owner": "emma", "name": "成绩单"}, "d5")
	v := View{Name: "x", Selection: All, Node: Node{File: "{id}-{revision}.{ext}",
		If: "id != A && !tags in [translation, copy] && name != 成绩单"}}
	plan, err := Build(v, []tree.Item{a, b, c, d}, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	// A skipped Item, an Item's tag, a field and a revision's own tag each
	// leave out what they meet, and nothing else.
	if want := []string{"C-1.pdf"}; !reflect.DeepEqual(paths, want) || len(plan.Missing) != 0 {
		t.Fatalf("got %v, want %v; missing %v", paths, want, plan.Missing)
	}
	e := item("E", "visa", map[string]string{"owner": "emma"}, "d6")
	e.SupersededBy = "F"
	f := item("F", "visa", map[string]string{"owner": "emma"}, "d7")
	g := item("G", "visa", map[string]string{"owner": "emma"}, "d8")
	g.Retired = true
	for status, want := range map[string][]string{"superseded": {"F-1.pdf", "G-1.pdf"}, "retired": {"F-1.pdf"}} {
		plan, _ := Build(View{Name: "v", Selection: All, Node: Node{If: "!status ==" + status, File: "{id}-{revision}.{ext}"}}, []tree.Item{e, f, g}, Types{})
		var got []string
		for _, file := range plan.Files {
			got = append(got, file.Path)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("exclude %s: got %v, want %v", status, got, want)
		}
	}
}

func TestBuildContains(t *testing.T) {
	a := item("A", "diploma", map[string]string{"owner": "emma", "name": "Bachelor Diploma"}, "d1")
	b := item("B", "diploma", map[string]string{"owner": "emma", "name": "毕业证书英文版"}, "d2")
	b.Tags = []string{"translation-en"}
	c := item("C", "diploma", map[string]string{"owner": "emma", "name": "学位证书"}, "d3")
	build := func(v View) []string {
		v.Name, v.Selection, v.File = "x", Head, "{id}.{ext}"
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		plan, err := Build(v, []tree.Item{a, b, c}, Types{})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range plan.Files {
			out = append(out, f.Path)
		}
		return out
	}
	// Any listed text found in the value, ignoring case, is a match.
	if got := build(View{Node: Node{If: "name ~ DIPLOMA || name ~ 证书"}}); !reflect.DeepEqual(got, []string{"A.pdf", "B.pdf", "C.pdf"}) {
		t.Fatalf("query: %v", got)
	}
	if got := build(View{Node: Node{If: "!(name ~ 英文 || tags ~ translation)"}}); !reflect.DeepEqual(got, []string{"A.pdf", "C.pdf"}) {
		t.Fatalf("exclude: %v", got)
	}
	// is and contains on one key must both hold.
	if got := build(View{Node: Node{If: "name == 学位证书 && name ~ 英文"}}); len(got) != 0 {
		t.Fatalf("both: %v", got)
	}
	for _, bad := range []string{"name has", "status is"} {
		v := View{Name: "x", Selection: Head, Node: Node{If: bad, File: "{id}.{ext}"}}
		if err := v.Validate(); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestBuildNumbersSkip(t *testing.T) {
	var items []tree.Item
	for i, name := range []string{"身份证", "护照", "结婚证", "户口"} {
		items = append(items, item(string(rune('A'+i)), "id_card", map[string]string{"name": name}, fmt.Sprint("d", i)))
	}
	v := View{Name: "x", Selection: Head, Node: Node{Order: map[string][]string{"name": {"身份证", "护照", "结婚证", "户口"}}, Numbers: map[string]map[string]int{"name": {"结婚证": 6}}, File: "{#}-{name}.{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, _ := Build(v, items, Types{})
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Path)
	}
	if want := []string{"01-身份证.pdf", "02-护照.pdf", "06-结婚证.pdf", "07-户口.pdf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// A number may repeat the one before it, sharing it; the next counts on.
	v.Numbers = map[string]map[string]int{"name": {"结婚证": 2}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, _ = Build(v, items, Types{})
	got = nil
	for _, f := range plan.Files {
		got = append(got, f.Path)
	}
	if want := []string{"01-身份证.pdf", "02-护照.pdf", "02-结婚证.pdf", "03-户口.pdf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared: got %v, want %v", got, want)
	}
	// A number below the one before it is refused.
	v.Numbers = map[string]map[string]int{"name": {"结婚证": 1}}
	if err := v.Validate(); err == nil {
		t.Fatal("a number below the one before was accepted")
	}
	v.Numbers = map[string]map[string]int{"name": {"驾照": 9}}
	if err := v.Validate(); err == nil {
		t.Fatal("a number for a name not in the order was accepted")
	}
}

func TestBuildMissingAndDefault(t *testing.T) {
	items := []tree.Item{item("A", "id_card", map[string]string{"owner": "emma"}, "d1")}
	v := View{Name: "x", Selection: Head, Node: Node{File: "{country}/{owner}/{year}.{ext}"}}
	plan, _ := Build(v, items, Types{})
	if len(plan.Missing) != 1 || !reflect.DeepEqual(plan.Missing[0].Keys, []string{"country", "year"}) || len(plan.Files) != 0 {
		t.Fatalf("%+v", plan)
	}
	none := "none"
	v.Default = &none
	plan, _ = Build(v, items, Types{})
	if !plan.Complete() || plan.Files[0].Path != "none/emma/none.pdf" {
		t.Fatalf("%+v", plan)
	}
}

func TestBuildDates(t *testing.T) {
	items := []tree.Item{item("A", "bill", map[string]string{"date": "2026-09-25"}, "d1")}
	plan, _ := Build(View{Name: "x", Selection: Head, Node: Node{File: "{year}/{month}/{date}.{ext}"}}, items, Types{})
	if plan.Files[0].Path != "2026/09/2026-09-25.pdf" {
		t.Fatalf("%+v", plan)
	}
	span := []tree.Item{item("A", "bill", map[string]string{"date": "2026-01-01/2026-03-31"}, "d1")}
	plan, _ = Build(View{Name: "x", Selection: Head, Node: Node{File: "{year}/{date.end}/{date}/{date:compact}.{ext}"}}, span, Types{})
	if plan.Files[0].Path != "2026/2026-03-31/2026-01-01-2026-03-31/20260101-20260331.pdf" {
		t.Fatalf("%+v", plan)
	}
}

func TestBuildClashAndNumbering(t *testing.T) {
	items := []tree.Item{
		item("B", "bill", map[string]string{"owner": "Emma"}, "d2"),
		item("A", "bill", map[string]string{"owner": "emma"}, "d1"),
		item("C", "bill", map[string]string{"owner": "emma"}, "d3"),
	}
	v := View{Name: "x", Selection: Head, Node: Node{File: "{owner}/{type}.{ext}"}}
	plan, _ := Build(v, items, Types{})
	if len(plan.Clashes) != 1 || len(plan.Clashes[0].Files) != 3 || plan.Complete() {
		t.Fatalf("case should clash: %+v", plan)
	}
	v.Dedupe = DedupeNumber
	plan, _ = Build(v, items, Types{})
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Item+":"+f.Path)
	}
	want := []string{"B:Emma/bill_01.pdf", "A:emma/bill.pdf", "C:emma/bill_02.pdf"}
	if !plan.Complete() || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestFieldsFor(t *testing.T) {
	got := FieldsFor([]string{"year", "owner", "month", "date"})
	if !reflect.DeepEqual(got, []string{DateField, "owner"}) {
		t.Fatalf("FieldsFor = %v", got)
	}
}

func TestFieldsForLeavesOutAnOrderKey(t *testing.T) {
	// {about.address} lacking a number is an order to extend, not a field.
	if got := FieldsFor([]string{"{about.address}", "about.address"}); !reflect.DeepEqual(got, []string{"about"}) {
		t.Fatalf("FieldsFor = %v", got)
	}
}

func TestLayoutWritesACountryInAFormat(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "emma", "country": "中国"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "tom", "country": "AU"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "sam", "country": "Atlantis"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	plan, err := Build(View{Name: "v", Selection: Head, Node: Node{File: "{country:alpha3}/{owner}-{country:en}.{ext}"}}, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Path)
	}
	want := []string{"AUS/tom-Australia.pdf", "Atlantis/sam-Atlantis.pdf", "CHN/emma-China.pdf"}
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

func TestKeysComeFromTheirRevision(t *testing.T) {
	item := tree.Item{ID: "A", Type: "card", Kind: tree.KindDocument, Head: "b",
		Fields: map[string]string{"owner": "emma"},
		Revisions: []tree.Revision{
			{Digest: "a", Fields: map[string]string{"expires": "2020"}},
			{Digest: "b", Fields: map[string]string{"expires": "2030"}},
		}}
	plan, err := Build(View{Name: "v", Selection: All, Node: Node{File: "{owner}-{expires}.{ext}"}}, []tree.Item{item}, Types{})
	if err != nil || len(plan.Files) != 2 || plan.Files[0].Path != "emma-2020.pdf" || plan.Files[1].Path != "emma-2030.pdf" {
		t.Fatalf("%v %+v", err, plan)
	}
	head, _ := Build(View{Name: "v", Selection: Head, Node: Node{If: "expires == 2030", File: "{id}.{ext}"}}, []tree.Item{item}, Types{})
	if len(head.Files) != 1 {
		t.Fatal("a condition does not read HEAD's fields")
	}
}

func TestCombineFindsClashesAcrossViews(t *testing.T) {
	items := []tree.Item{
		item("A", "id_card", map[string]string{"owner": "emma"}, "d1"),
		item("B", "bill", map[string]string{"owner": "emma"}, "d2"),
	}
	ids := View{Name: "ids", Selection: Head, Node: Node{If: "type == id_card", File: "{owner}/{type}.{ext}"}}
	bills := View{Name: "bills", Selection: Head, Node: Node{If: "type == bill", File: "{owner}/id_card.{ext}"}}
	c, err := Combine([]View{ids, bills}, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Complete() || len(c.Clashes) != 1 || len(c.Files) != 0 {
		t.Fatalf("combined %+v", c)
	}
	views := []string{c.Clashes[0].Files[0].View, c.Clashes[0].Files[1].View}
	if !reflect.DeepEqual(views, []string{"ids", "bills"}) {
		t.Fatalf("clash views %v", views)
	}
	bills.File = "{owner}/bills/{type}.{ext}"
	c, _ = Combine([]View{ids, bills}, items, Types{})
	if !c.Complete() || len(c.Files) != 2 || c.Files[0].View != "bills" {
		t.Fatalf("combined %+v", c)
	}
}

func TestAFileWhereAFolderIsWantedClashes(t *testing.T) {
	files, clashes := separate([]File{{Path: "emma/x", Item: "A"}, {Path: "Emma/x/y.pdf", Item: "B"}, {Path: "tom.pdf", Item: "C"}})
	if len(files) != 1 || files[0].Path != "tom.pdf" || len(clashes) != 1 || len(clashes[0].Files) != 2 {
		t.Fatalf("files %v clashes %v", files, clashes)
	}
}

func TestNumberedKeysFollowTheOrder(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "emma", "country": "中国"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "alex", "country": "AU"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "alex", "country": "NZ"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"owner": {"alex", "emma"}, "country": {"CHN", "Australia"}}, File: "{#}-{owner}/{#}-{country:alpha3}/{type}.{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Path)
	}
	if want := []string{"01-alex/02-AUS/id_card.pdf", "02-emma/01-CHN/id_card.pdf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	// A value the order does not list is not numbered by guess.
	if len(plan.Missing) != 1 || plan.Missing[0].Item != "C" || !reflect.DeepEqual(plan.Missing[0].Unordered, map[string]string{"country": "NZL"}) {
		t.Fatalf("missing %+v", plan.Missing)
	}
	for _, bad := range []View{
		{Name: "v", Selection: Head, Node: Node{File: "{#}-{owner}.{ext}"}},
		{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"country": {"CN", "中国"}}, File: "{#}-{country}.{ext}"}},
		{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"owner": {}}, File: "{owner}.{ext}"}},
	} {
		if bad.Validate() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestPadWidensWithTheOrder(t *testing.T) {
	if got := padTo(7, 120); got != "007" {
		t.Fatal(got)
	}
	if got := padTo(7, 3); got != "07" {
		t.Fatal(got)
	}
}

func TestTypeIsWrittenInALanguage(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "driver_licence", Kind: tree.KindRecord, Fields: map[string]string{"owner": "emma"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "visa", Kind: tree.KindRecord, Fields: map[string]string{"owner": "emma"}, Revisions: []tree.Revision{{Digest: "b"}}},
	}
	names := TypesOf([]tree.Template{{Type: "driver_licence", Names: map[string]string{"zh": "驾驶证", "en": "Driver licence"}}})
	plan, err := Build(View{Name: "v", Selection: Head, Node: Node{File: "{type:zh}/{type:en}-{type}.{ext}"}}, items, names)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != "驾驶证/Driver licence-driver_licence.pdf" {
		t.Fatalf("files %+v", plan.Files)
	}
	// A type whose Template names it in no language is missing that name.
	if len(plan.Missing) != 1 || !reflect.DeepEqual(plan.Missing[0].Keys, []string{"type:zh", "type:en"}) || len(plan.Missing[0].Fields) != 0 {
		t.Fatalf("missing %+v", plan.Missing)
	}
	for _, bad := range []string{"{type:alpha3}", "{type:fr}"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

// {a|b} writes the first key an Item has, so one layout names an ID card by
// its name and a driver licence by its type.
func TestAlternativeKeys(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "driver_licence", Kind: tree.KindRecord, Fields: map[string]string{"country": "AU"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"country": "CN", "name": "户口首页"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "visa", Kind: tree.KindRecord, Fields: map[string]string{"country": "CN"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	names := TypesOf([]tree.Template{{Type: "driver_licence", Names: map[string]string{"zh": "驾驶证"}}})
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"country": {"CN", "AU"}}, File: "{#}-{country:alpha3}/{name|type:zh}.{ext}"}}
	plan, err := Build(v, items, names)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	sort.Strings(paths)
	if want := []string{"01-CHN/户口首页.pdf", "02-AUS/驾驶证.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	// An Item with none of the keys lacks the first.
	if len(plan.Missing) != 1 || !reflect.DeepEqual(plan.Missing[0].Keys, []string{"name"}) {
		t.Fatalf("missing %+v", plan.Missing)
	}
	layout, err := Parse("{name|type:zh|type}")
	if err != nil {
		t.Fatal(err)
	}
	if got := layout.Keys(); !reflect.DeepEqual(got, []string{"name", "type"}) {
		t.Fatalf("keys %v", got)
	}
	for _, bad := range []string{"{name|}", "{|type}", "{name|type:fr}"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if _, err := Parse("{name#|owner}"); err == nil {
		t.Error("one numbered alternative accepted")
	}
}

// {#}-{a|b} numbers alternatives from one order, so an ID card and a driver
// licence in one folder never share a number.
func TestAlternativesAreNumberedTogether(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "driver_licence", Kind: tree.KindRecord, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"name": "户口首页"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"name": "护照"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	names := TypesOf([]tree.Template{{Type: "driver_licence", Names: map[string]string{"zh": "驾驶证"}}})
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"name": {"户口首页", "驾驶证"}}, File: "{#}-{name|type:zh}.{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, names)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	sort.Strings(paths)
	if want := []string{"01-户口首页.pdf", "02-驾驶证.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	if len(plan.Missing) != 1 || !reflect.DeepEqual(plan.Missing[0].Unordered, map[string]string{"name": "护照"}) {
		t.Fatalf("missing %+v", plan.Missing)
	}
	v.Order = nil
	if err := v.Validate(); err == nil || !strings.Contains(err.Error(), "name|type:zh") {
		t.Fatalf("no order: %v", err)
	}
	// # comes last inside the braces, and only there.
	for _, bad := range []string{"{owner}#", "{name|type:zh}#", "{country#:alpha3}", "{name#|type:zh}", "{owner#}", "{#}-{owner}{#}", "{#}-x.{ext}", "{#}"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

// A condition names a country in any form and ignores case.
func TestMatchesCountryInAnyForm(t *testing.T) {
	item := item("A", "id_card", map[string]string{"country": "中国", "owner": "Alex"}, "d1")
	for cond, want := range map[string]bool{
		"country == CN": true, "country == CHN": true, "country == China": true, "owner == alex": true, "country == AU": false,
	} {
		plan, err := Build(View{Name: "v", Selection: Head, Node: Node{If: cond, File: "{id}.{ext}"}}, []tree.Item{item}, Types{})
		if err != nil || (len(plan.Files) == 1) != want {
			t.Errorf("%s: %v %v", cond, err, plan.Files)
		}
	}
}

func TestOptionalKey(t *testing.T) {
	items := []tree.Item{
		item("A", "diploma", map[string]string{"owner": "emma", "degree": "本科"}, "d1"),
		item("B", "id_card", map[string]string{"owner": "emma"}, "d2"),
	}
	v := View{Name: "x", Selection: Head, Node: Node{File: "{owner}/{type}[-{degree}][ ({country:alpha2})].{ext}"}}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"emma/diploma-本科.pdf", "emma/id_card.pdf"}; !reflect.DeepEqual(paths, want) || !plan.Complete() {
		t.Fatalf("got %v %+v, want %v", paths, plan, want)
	}
	// A folder of only optional parts, all empty, would vanish: not placed.
	plan, _ = Build(View{Name: "x", Selection: Head, Node: Node{File: "{owner}/[{degree}]/{type}.{ext}"}}, items, Types{})
	if len(plan.Files) != 1 || len(plan.Missing) != 1 || plan.Missing[0].Item != "B" {
		t.Fatalf("empty folder: %+v", plan)
	}
	// [/{key}] adds a folder when the key is there, and none when it is not.
	plan, _ = Build(View{Name: "x", Selection: Head, Node: Node{File: "licence[/{degree}]/{type}[-{degree}].{ext}"}}, items, Types{})
	paths = nil
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"licence/id_card.pdf", "licence/本科/diploma-本科.pdf"}; !reflect.DeepEqual(paths, want) || !plan.Complete() {
		t.Fatalf("got %v %+v, want %v", paths, plan, want)
	}
	// [/{#}-{key}] numbers the folder from the order {key}, from where numbers set.
	v = View{Name: "x", Selection: Head, Node: Node{Order: map[string][]string{"degree": {"硕士", "本科"}}, Numbers: map[string]map[string]int{"degree": {"硕士": 10}}, File: "licence[/{#}-{degree}]/{type}.{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, _ = Build(v, items, Types{})
	paths = nil
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"licence/11-本科/diploma.pdf", "licence/id_card.pdf"}; !reflect.DeepEqual(paths, want) || !plan.Complete() {
		t.Fatalf("got %v %+v, want %v", paths, plan, want)
	}
	if err := (View{Name: "x", Selection: Head, Node: Node{File: v.File}}).Validate(); err == nil {
		t.Error("[/{#}-{degree}] without an order validated")
	}
	// A group of several keys is left out when any of them is lacking.
	plan, _ = Build(View{Name: "x", Selection: Head, Node: Node{File: "{owner}/{type}[ {degree}-{owner}][ {degree}-{level}].{ext}"}}, items, Types{})
	paths = nil
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"emma/diploma 本科-emma.pdf", "emma/id_card.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	for _, bad := range []string{"{?}", "{-?}", "{-degree#?}", "[-]", "a[-[{x}]]", "a[{x}", "a{x}]", "[{#}-{x}]", "a[-{#}{x}]",
		"[/{degree}]/x", "a[/{degree}]b", "a[/{degree}/]", "a[{degree}/]", "a[/{degree}][/{level}]"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

// {#} numbers everything after it in the name, so a name with a level and
// the same name with another level each have their own number.
func TestCounterNumbersTheWholeName(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "diploma", Kind: tree.KindRecord, Fields: map[string]string{"name": "毕业证书", "level": "本科"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "diploma", Kind: tree.KindRecord, Fields: map[string]string{"name": "毕业证书", "level": "硕士"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"name": "护照"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"name": {"护照", "毕业证书-本科", "毕业证书-硕士"}}, File: "x/{#}-{name}[-{level}].{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"x/01-护照.pdf", "x/02-毕业证书-本科.pdf", "x/03-毕业证书-硕士.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
}

// {/#} stops the number: two contracts told apart by date share one.
func TestCounterEndStopsTheNumber(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "contract", Kind: tree.KindRecord, Fields: map[string]string{"name": "合同", "signed": "2018-03-20"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "contract", Kind: tree.KindRecord, Fields: map[string]string{"name": "合同", "signed": "2019-03-22"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "invoice", Kind: tree.KindRecord, Fields: map[string]string{"name": "物业发票"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"name": {"合同", "物业发票"}}, File: "x/{#}-{name}{/#}[-{signed:compact}].{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	sort.Strings(paths)
	if want := []string{"x/01-合同-20180320.pdf", "x/01-合同-20190322.pdf", "x/02-物业发票.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	for _, bad := range []string{"x/{name}{/#}.{ext}", "x/{#}-{name}{/#}{/#}.{ext}", "x/{/#}{#}-{name}.{ext}"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) took it", bad)
		}
	}
}

// An unnumbered name has no number nor the text after {#}, and takes no
// number from the names after it.
func TestUnnumbered(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "contract", Kind: tree.KindRecord, Fields: map[string]string{"name": "合同"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "other", Kind: tree.KindRecord, Fields: map[string]string{"name": "押金"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "invoice", Kind: tree.KindRecord, Fields: map[string]string{"name": "物业发票"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"name": {"合同", "押金", "物业发票"}}, Numbers: map[string]map[string]int{"name": {"合同": 0}}, Unnumbered: map[string][]string{"name": {"押金"}}, File: "x/{#}-{name}.{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"x/00-合同.pdf", "x/01-物业发票.pdf", "x/押金.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	v.Numbers = map[string]map[string]int{"name": {"押金": 5}}
	if v.Validate() == nil {
		t.Error("a name both numbered and unnumbered was taken")
	}
	v.Numbers, v.Unnumbered = nil, map[string][]string{"name": {"租约"}}
	if v.Validate() == nil {
		t.Error("an unnumbered name not in the order was taken")
	}
}

// An order may leave the names it does not list unnumbered, or give them
// one number, so their PDFs are placed rather than missing.
func TestUnlistedUnnumbered(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "contract", Kind: tree.KindRecord, Fields: map[string]string{"name": "合同"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "other", Kind: tree.KindRecord, Fields: map[string]string{"name": "账单"}, Revisions: []tree.Revision{{Digest: "b"}}},
	}
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"name": {"合同"}}, File: "x/{#}-{name}.{ext}"}}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || len(plan.Missing) != 1 {
		t.Fatalf("without the option: %d files, %d missing", len(plan.Files), len(plan.Missing))
	}
	v.Unlisted = map[string]string{"name": UnlistedNone}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	if plan, err = Build(v, items, Types{}); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"x/01-合同.pdf", "x/账单.pdf"}; !reflect.DeepEqual(paths, want) || len(plan.Missing) != 0 {
		t.Fatalf("got %v, %d missing, want %v", paths, len(plan.Missing), want)
	}
	// A number gives every name not listed that one number.
	v.Unlisted = map[string]string{"name": "99"}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	if plan, err = Build(v, items, Types{}); err != nil {
		t.Fatal(err)
	}
	paths = nil
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"x/01-合同.pdf", "x/99-账单.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("numbered: got %v, want %v", paths, want)
	}
	for _, bad := range []map[string]string{{"other": UnlistedNone}, {"name": "-1"}, {"name": "last"}} {
		v.Unlisted = bad
		if v.Validate() == nil {
			t.Errorf("unlisted %v was taken", bad)
		}
	}
}

// Children are an if/elif/else chain: the first an Item meets places it
// under the parent's path; one that excludes leaves it out; an Item none
// takes is placed by the parent. A {#} takes the nearest order, and a
// type condition takes the types below it, a value one in its group.
func TestChildren(t *testing.T) {
	rec := func(id, typ string, fields map[string]string) tree.Item {
		return tree.Item{ID: id, Type: typ, Kind: tree.KindRecord, Fields: fields, Revisions: []tree.Revision{{Digest: id}}}
	}
	items := []tree.Item{
		rec("A", "contract", map[string]string{"home": "h1", "name": "合同"}),
		rec("B", "bill", map[string]string{"home": "h2", "name": "水费", "category": "水"}),
		rec("C", "invoice", map[string]string{"home": "h2", "name": "物业", "category": "物业"}),
		rec("D", "payment", map[string]string{"home": "h2", "name": "转账", "category": "水"}),
		rec("E", "photo", map[string]string{"home": "h1", "name": "照片"}),
	}
	types := TypesOf([]tree.Template{
		{Type: "bill", Lineage: []string{"bill", "money"}, Fields: []tree.Field{{Key: "category", Type: tree.FieldSelect,
			Values: tree.ValueGroups{{Name: "utility", Values: []string{"水", "电"}}}}}},
		{Type: "invoice", Lineage: []string{"invoice", "money"}},
		{Type: "payment", Lineage: []string{"payment", "money"}},
	})
	v := View{Name: "v", Selection: Head, Node: Node{Order: map[string][]string{"home": {"h1", "h2"}},
		Path: "{#}-{home}", File: "{name}.{ext}",
		Children: []Node{
			{If: "category == utility", Path: "utility", File: "{category}-{name}.{ext}"},
			{If: "type == money", Path: "rental"},
			{If: "type == photo", Exclude: true},
		}}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, types)
	if err != nil {
		t.Fatal(err)
	}
	var paths, nodes []string
	for _, f := range plan.Files {
		paths, nodes = append(paths, f.Path), append(nodes, f.Node)
	}
	// D is a payment, whose category has no groups: 水 is not utility.
	if want := []string{"01-h1/合同.pdf", "02-h2/rental/物业.pdf", "02-h2/rental/转账.pdf", "02-h2/utility/水-水费.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	// Each file names the node that placed it, "" for the rule itself.
	if want := []string{"", "2", "2", "1"}; !reflect.DeepEqual(nodes, want) {
		t.Fatalf("nodes %q, want %q", nodes, want)
	}
	for name, bad := range map[string][]Node{
		"else not last":    {{Path: "x"}, {If: "type == bill", Path: "y"}},
		"else leaving out": {{If: "type == bill", Path: "y"}, {}},
		"a bad condition":  {{If: "type ==", Path: "x"}},
		"an unordered {#}": {{If: "type == bill", Path: "{#}-{name}"}},
		"a broken path":    {{If: "type == bill", Path: "x/{name"}},
	} {
		v.Children = bad
		if v.Validate() == nil {
			t.Errorf("%s was taken", name)
		}
	}
	// A child with only an order numbers its Items apart, placed as its
	// parent places them; one beside it keeps the rule's order.
	v.Children = []Node{
		{If: "category == utility", Path: "utility"},
		{If: "type == money", Order: map[string][]string{"home": {"h2", "h1"}}},
	}
	plan, err = Build(v, items, types)
	if err != nil {
		t.Fatal(err)
	}
	paths = nil
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"01-h1/合同.pdf", "01-h1/照片.pdf", "01-h2/物业.pdf", "01-h2/转账.pdf", "02-h2/utility/水费.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	for name, bad := range map[string][]Node{
		"a child changing nothing": {{If: "type == bill"}},
		"exclude with a path":      {{If: "type == bill", Exclude: true, Path: "x"}},
		"two rests in one order":   {{If: "type == bill", File: "{#}-{home}-{name}.{ext}"}},
		"a bad order name":         {{If: "type == bill", File: "{#X}-{name}.{ext}"}},
	} {
		v.Children = bad
		if v.Validate() == nil {
			t.Errorf("%s was taken", name)
		}
	}
	// {#label} names an order of its own, for a second rest.
	v.Children = []Node{{If: "type == bill", File: "{#n}-{home}-{name}.{ext}", Order: map[string][]string{"n": {"h2-水费"}}}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
}

// Orders named the old way, by the rest as written, are renamed for their
// first key and moved to the node holding every {#} that numbers them.
func TestUpgradeRenamesOrders(t *testing.T) {
	v := View{Name: "v", Selection: Head, Node: Node{
		File:     "{#}-{owner}/{#}-{name}[-{level}].{ext}",
		Order:    map[string][]string{"{owner}": {"alex"}, "{name}[-{level}]": {"a"}, "{name}[-{level|issuer}]": {"b"}},
		Numbers:  map[string]map[string]int{"{name}[-{level|issuer}]": {"b": 5}},
		Children: []Node{{If: "type == translation", File: "{#}-{owner}/{#}-{name}[-{level|issuer}].{ext}"}},
	}}
	u := v.Upgrade()
	if err := u.Validate(); err != nil {
		t.Fatal(err)
	}
	if want := map[string][]string{"owner": {"alex"}, "name": {"a"}}; !reflect.DeepEqual(u.Order, want) {
		t.Fatalf("rule %v", u.Order)
	}
	if c := u.Children[0]; !reflect.DeepEqual(c.Order, map[string][]string{"name": {"b"}}) || c.Numbers["name"]["b"] != 5 {
		t.Fatalf("child %+v", c)
	}
	if len(v.Order) != 3 || v.Children[0].Order != nil {
		t.Fatalf("the rule upgraded was changed: %+v", v)
	}
}

func TestName(t *testing.T) {
	it := item("i1", "passport", map[string]string{"owner": "alex", "country": "AU"}, "d1")
	got, lacking, err := Name("{owner}-{type}[-{number}].{ext}", it, 1, Types{})
	if err != nil || got != "alex-passport.pdf" || len(lacking) != 0 {
		t.Fatalf("got %q %v %v", got, lacking, err)
	}
	if _, lacking, _ := Name("{owner}-{name}.{ext}", it, 1, Types{}); len(lacking) != 1 || lacking[0] != "name" {
		t.Fatalf("lacking %v", lacking)
	}
	if _, _, err := Name("{#}-{owner}.{ext}", it, 1, Types{}); err == nil {
		t.Fatal("{#} accepted")
	}
}

// A translation takes its original's keys through the link, and {#} numbers
// a level without the language folder after it.
func TestLinkedKeys(t *testing.T) {
	const (
		bachelor = "01K00000000000000000000001"
		master   = "01K00000000000000000000002"
		english  = "01K00000000000000000000003"
		lost     = "01K00000000000000000000004"
	)
	items := []tree.Item{
		item(bachelor, "diploma", map[string]string{"owner": "emma", "level": "本科", "name": "学位证书"}, "d1", "d2"),
		item(master, "diploma", map[string]string{"owner": "emma", "level": "硕士研究生", "name": "毕业证书"}, "d3"),
		item(english, "translation", map[string]string{"owner": "emma", "language": "en", "original": bachelor + "@d1"}, "d4"),
		item(lost, "translation", map[string]string{"owner": "emma", "language": "fr"}, "d5"),
	}
	items[0].Revisions[0].Fields = map[string]string{"level": "本科"}
	v := View{Name: "x", Selection: Head, Node: Node{Order: map[string][]string{"level": {"硕士研究生", "本科"}, "name": {"毕业证书", "学位证书"}}, File: "{#}-{level|original.level}[/{language}]/{#}-{name|original.name}.{ext}"}}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	want := []string{"01-硕士研究生/01-毕业证书.pdf", "02-本科/02-学位证书.pdf", "02-本科/en/02-学位证书.pdf"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	if len(plan.Missing) != 1 || plan.Missing[0].Item != lost || !reflect.DeepEqual(plan.Missing[0].Fields, []string{"level", "name"}) {
		t.Fatalf("missing %+v", plan.Missing)
	}
}

// A folder group may add several folders, all or none.
func TestFolderGroupAddsSeveralFolders(t *testing.T) {
	items := []tree.Item{
		item("01K00000000000000000000001", "bill", map[string]string{"owner": "emma", "service": "水"}, "d1"),
		item("01K00000000000000000000002", "contract", map[string]string{"owner": "emma"}, "d2"),
	}
	v := View{Name: "x", Selection: Head, Node: Node{File: "r[/bill/{service}]/{type}.{ext}"}}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"r/bill/水/bill.pdf", "r/contract.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	for _, bad := range []string{"r[/bill//{service}]/x", "r[/../{service}]/x", "r[-a/{service}]/x", "r[/{#}-a/{service}]/x"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%s parsed", bad)
		}
	}
}

// With inherit, a key a translation lacks is its original's, and its own
// key stays its own.
func TestInherit(t *testing.T) {
	const (
		bachelor = "01K00000000000000000000001"
		english  = "01K00000000000000000000003"
	)
	items := []tree.Item{
		item(bachelor, "diploma", map[string]string{"owner": "emma", "level": "本科", "name": "学位证书"}, "d1"),
		item(english, "translation", map[string]string{"owner": "emma", "language": "en", "name": "degree", "original": bachelor}, "d2"),
	}
	v := View{Name: "x", Selection: Head, Inherit: []string{"original"}, Node: Node{Order: map[string][]string{"level": {"本科"}}, File: "{#}-{level}[/{language}]/{name}.{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, Types{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"01-本科/en/degree.pdf", "01-本科/学位证书.pdf"}; !reflect.DeepEqual(paths, want) || !plan.Complete() {
		t.Fatalf("got %v %+v, want %v", paths, plan, want)
	}
	v.Inherit = []string{"original.level"}
	if err := v.Validate(); err == nil {
		t.Error("inherit original.level validated")
	}
}

func TestInheritExcludesAndLinkedType(t *testing.T) {
	const (
		transcript = "01K00000000000000000000001"
		licence    = "01K00000000000000000000002"
		ofScript   = "01K00000000000000000000003"
		ofLicence  = "01K00000000000000000000004"
	)
	items := []tree.Item{
		item(transcript, "diploma", map[string]string{"name": "成绩单"}, "d1"),
		item(licence, "driver_licence", map[string]string{}, "d2"),
		item(ofScript, "translation", map[string]string{"original": transcript}, "d3"),
		item(ofLicence, "translation", map[string]string{"original": licence}, "d4"),
	}
	names := TypesOf([]tree.Template{{Type: "driver_licence", Names: map[string]string{"zh": "驾驶证"}}, {Type: "translation", Names: map[string]string{"zh": "翻译件"}}, {Type: "diploma", Names: map[string]string{"zh": "文凭"}}})
	v := View{Name: "x", Selection: Head, Inherit: []string{"original"}, Node: Node{If: "name != 成绩单", File: "{type}/{name|original.type:zh|type:zh}.{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, names)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"driver_licence/驾驶证.pdf", "translation/驾驶证.pdf"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
}

func TestDateFormats(t *testing.T) {
	a := item("01K00000000000000000000001", "tenancy", map[string]string{"start": "2010-01-01", "end": "2011-01-01"}, "d1")
	b := item("01K00000000000000000000002", "bill", map[string]string{"tenancy": "01K00000000000000000000001", "date": "2010-08-03"}, "d2")
	v := View{Name: "x", Selection: Head, Node: Node{If: "type == bill", File: "{tenancy.start:compact}-{tenancy.end:compact}/{date:fy}.{ext}"}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, []tree.Item{a, b}, Types{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != "20100101-20110101/FY2011.pdf" {
		t.Fatalf("%+v %+v", plan.Files, plan.Missing)
	}
	if _, err := Parse("{date:weekday}.{ext}"); err == nil {
		t.Fatal("an unknown format was taken")
	}
}

// A select value is written in a language its field names it in; one
// without a name, as kept.
func TestValueIsWrittenInALanguage(t *testing.T) {
	bill := tree.Template{Type: "bill", Fields: []tree.Field{{Key: "category", Type: tree.FieldSelect, Options: []string{"房租", "电", "水"},
		Names: map[string]map[string]string{"en": {"房租": "rent", "电": "power"}}}}}
	items := []tree.Item{
		{ID: "A", Type: "bill", Kind: tree.KindRecord, Fields: map[string]string{"category": "房租"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "bill", Kind: tree.KindRecord, Fields: map[string]string{"category": "电, 水"}, Revisions: []tree.Revision{{Digest: "b"}}},
	}
	plan, err := Build(View{Name: "v", Selection: Head, Node: Node{File: "{category:en}/{id}.{ext}"}}, items, TypesOf([]tree.Template{bill}))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Path)
	}
	if !reflect.DeepEqual(got, []string{"power, 水/B.pdf", "rent/A.pdf"}) {
		t.Fatal(got)
	}
}

// A span open at one end is written as the one day it has.
func TestOpenSpanIsItsDay(t *testing.T) {
	items := []tree.Item{{ID: "A", Type: "bill", Kind: tree.KindRecord, Fields: map[string]string{"date": "/2019-01-20"}, Revisions: []tree.Revision{{Digest: "a"}}}}
	plan, err := Build(View{Name: "v", Selection: Head, Node: Node{File: "{date:compact}-{date}-{year}.{ext}"}}, items, Types{})
	if err != nil || len(plan.Files) != 1 || plan.Files[0].Path != "20190120-2019-01-20-2019.pdf" {
		t.Fatalf("%v %+v", err, plan)
	}
}
