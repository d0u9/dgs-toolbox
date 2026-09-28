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
	v := View{Name: "x", Selection: Head, Layout: "{country}/{owner}/{type}.{ext}",
		Query: map[string]Values{"type": {"id_card"}}}
	plan, err := Build(v, items, nil)
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
	plan, _ := Build(View{Name: "x", Selection: All, Layout: "{owner}/{type}-{revision}.{ext}"}, items, nil)
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
	v := View{Name: "x", Selection: All, Layout: "{owner}/{revision}.{ext}", Query: map[string]Values{"tags": {"important", "original"}}}
	plan, err := Build(v, []tree.Item{a, b, c}, nil)
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
	v := View{Name: "x", Selection: Head, Layout: "{owner}/{id}.{ext}", Query: map[string]Values{"owner": {"Emma"}}}
	paths := func() []string {
		plan, err := Build(v, []tree.Item{a, b}, nil)
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
	v.Query = nil
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
	v := View{Name: "x", Selection: All, Layout: "{id}-{revision}.{ext}",
		Exclude: map[string]Values{"tags": {"translation", "copy"}, "name": {"成绩单"}}, Skip: []string{"A"}}
	plan, err := Build(v, []tree.Item{a, b, c, d}, nil)
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
		plan, _ := Build(View{Name: "v", Selection: All, Layout: "{id}-{revision}.{ext}", Exclude: map[string]Values{"status": {status}}}, []tree.Item{e, f, g}, nil)
		var got []string
		for _, file := range plan.Files {
			got = append(got, file.Path)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("exclude %s: got %v, want %v", status, got, want)
		}
	}
	v.Skip = []string{"A", "A"}
	if err := v.Validate(); err == nil {
		t.Fatal("an Item skipped twice was accepted")
	}
}

func TestBuildContains(t *testing.T) {
	a := item("A", "diploma", map[string]string{"owner": "emma", "name": "Bachelor Diploma"}, "d1")
	b := item("B", "diploma", map[string]string{"owner": "emma", "name": "毕业证书英文版"}, "d2")
	b.Tags = []string{"translation-en"}
	c := item("C", "diploma", map[string]string{"owner": "emma", "name": "学位证书"}, "d3")
	build := func(v View) []string {
		v.Name, v.Selection, v.Layout = "x", Head, "{id}.{ext}"
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		plan, err := Build(v, []tree.Item{a, b, c}, nil)
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
	if got := build(View{Query: map[string]Values{"name contains": {"DIPLOMA", "证书"}}}); !reflect.DeepEqual(got, []string{"A.pdf", "B.pdf", "C.pdf"}) {
		t.Fatalf("query: %v", got)
	}
	if got := build(View{Exclude: map[string]Values{"name contains": {"英文"}, "tags contains": {"translation"}}}); !reflect.DeepEqual(got, []string{"A.pdf", "C.pdf"}) {
		t.Fatalf("exclude: %v", got)
	}
	// is and contains on one key must both hold.
	if got := build(View{Query: map[string]Values{"name": {"学位证书"}, "name contains": {"英文"}}}); len(got) != 0 {
		t.Fatalf("both: %v", got)
	}
	for _, bad := range []string{"name has", "status contains"} {
		v := View{Name: "x", Selection: Head, Layout: "{id}.{ext}", Exclude: map[string]Values{bad: {"a"}}}
		if err := v.Validate(); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestBuildConditionTypes(t *testing.T) {
	letter := item("L1", "official_letter", map[string]string{"name": "Tax notice"}, "d1")
	other := item("L2", "official_letter", map[string]string{"name": "Parking fine"}, "d2")
	visa := item("V", "visa", map[string]string{"name": "Subclass 500"}, "d3")
	card := item("C", "social_card", map[string]string{}, "d4")
	v := View{Name: "x", Selection: Head, Layout: "{id}.{ext}",
		Query:        map[string]Values{"type": {"official_letter", "visa", "social_card"}, "name contains": {"tax"}},
		QueryTypes:   map[string]Values{"name contains": {"official_letter"}},
		Exclude:      map[string]Values{"name contains": {"500"}},
		ExcludeTypes: map[string]Values{"name contains": {"official_letter"}}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, _ := Build(v, []tree.Item{letter, other, visa, card}, nil)
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Path)
	}
	// Only letters are asked the name; the visa's name is not excluded, as
	// that exclusion is for letters.
	if want := []string{"C.pdf", "L1.pdf", "V.pdf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	v.QueryTypes = map[string]Values{"owner": {"visa"}}
	if err := v.Validate(); err == nil {
		t.Fatal("types for a condition the query lacks were accepted")
	}
}

func TestBuildNumbersSkip(t *testing.T) {
	var items []tree.Item
	for i, name := range []string{"身份证", "护照", "结婚证", "户口"} {
		items = append(items, item(string(rune('A'+i)), "id_card", map[string]string{"name": name}, fmt.Sprint("d", i)))
	}
	v := View{Name: "x", Selection: Head, Layout: "{#}-{name}.{ext}",
		Order:   map[string][]string{"{name}": {"身份证", "护照", "结婚证", "户口"}},
		Numbers: map[string]map[string]int{"{name}": {"结婚证": 6}}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, _ := Build(v, items, nil)
	var got []string
	for _, f := range plan.Files {
		got = append(got, f.Path)
	}
	if want := []string{"01-身份证.pdf", "02-护照.pdf", "06-结婚证.pdf", "07-户口.pdf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// A number at or below the one before it is refused.
	v.Numbers = map[string]map[string]int{"{name}": {"结婚证": 2}}
	if err := v.Validate(); err == nil {
		t.Fatal("a number not after the one before was accepted")
	}
	v.Numbers = map[string]map[string]int{"{name}": {"驾照": 9}}
	if err := v.Validate(); err == nil {
		t.Fatal("a number for a name not in the order was accepted")
	}
}

func TestBuildMissingAndDefault(t *testing.T) {
	items := []tree.Item{item("A", "id_card", map[string]string{"owner": "emma"}, "d1")}
	v := View{Name: "x", Selection: Head, Layout: "{country}/{owner}/{year}.{ext}"}
	plan, _ := Build(v, items, nil)
	if len(plan.Missing) != 1 || !reflect.DeepEqual(plan.Missing[0].Keys, []string{"country", "year"}) || len(plan.Files) != 0 {
		t.Fatalf("%+v", plan)
	}
	none := "none"
	v.Default = &none
	plan, _ = Build(v, items, nil)
	if !plan.Complete() || plan.Files[0].Path != "none/emma/none.pdf" {
		t.Fatalf("%+v", plan)
	}
}

func TestBuildDates(t *testing.T) {
	items := []tree.Item{item("A", "bill", map[string]string{"issued_at": "2026-09-25"}, "d1")}
	plan, _ := Build(View{Name: "x", Selection: Head, Layout: "{year}/{month}/{date}.{ext}"}, items, nil)
	if plan.Files[0].Path != "2026/09/2026-09-25.pdf" {
		t.Fatalf("%+v", plan)
	}
}

func TestBuildClashAndNumbering(t *testing.T) {
	items := []tree.Item{
		item("B", "bill", map[string]string{"owner": "Emma"}, "d2"),
		item("A", "bill", map[string]string{"owner": "emma"}, "d1"),
		item("C", "bill", map[string]string{"owner": "emma"}, "d3"),
	}
	v := View{Name: "x", Selection: Head, Layout: "{owner}/{type}.{ext}"}
	plan, _ := Build(v, items, nil)
	if len(plan.Clashes) != 1 || len(plan.Clashes[0].Files) != 3 || plan.Complete() {
		t.Fatalf("case should clash: %+v", plan)
	}
	v.Dedupe = DedupeNumber
	plan, _ = Build(v, items, nil)
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

func TestLayoutWritesACountryInAFormat(t *testing.T) {
	items := []tree.Item{
		{ID: "A", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "emma", "country": "中国"}, Revisions: []tree.Revision{{Digest: "a"}}},
		{ID: "B", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "tom", "country": "AU"}, Revisions: []tree.Revision{{Digest: "b"}}},
		{ID: "C", Type: "id_card", Kind: tree.KindRecord, Fields: map[string]string{"owner": "sam", "country": "Atlantis"}, Revisions: []tree.Revision{{Digest: "c"}}},
	}
	plan, err := Build(View{Name: "v", Selection: Head, Layout: "{country:alpha3}/{owner}-{country:en}.{ext}"}, items, nil)
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
	plan, err := Build(View{Name: "v", Selection: All, Layout: "{owner}-{expires}.{ext}"}, []tree.Item{item}, nil)
	if err != nil || len(plan.Files) != 2 || plan.Files[0].Path != "emma-2020.pdf" || plan.Files[1].Path != "emma-2030.pdf" {
		t.Fatalf("%v %+v", err, plan)
	}
	if !Matches(map[string]Values{"expires": {"2030"}}, item) || Matches(map[string]Values{"expires": {"2020"}}, item) {
		t.Fatal("a query matches HEAD's fields")
	}
}

func TestCombineFindsClashesAcrossViews(t *testing.T) {
	items := []tree.Item{
		item("A", "id_card", map[string]string{"owner": "emma"}, "d1"),
		item("B", "bill", map[string]string{"owner": "emma"}, "d2"),
	}
	ids := View{Name: "ids", Selection: Head, Layout: "{owner}/{type}.{ext}", Query: map[string]Values{"type": {"id_card"}}}
	bills := View{Name: "bills", Selection: Head, Layout: "{owner}/id_card.{ext}", Query: map[string]Values{"type": {"bill"}}}
	c, err := Combine([]View{ids, bills}, items, nil)
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
	bills.Layout = "{owner}/bills/{type}.{ext}"
	c, _ = Combine([]View{ids, bills}, items, nil)
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
	v := View{Name: "v", Selection: Head, Layout: "{#}-{owner}/{#}-{country:alpha3}/{type}.{ext}",
		Order: map[string][]string{"{owner}": {"alex", "emma"}, "{country:alpha3}": {"CHN", "Australia"}}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, nil)
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
	if len(plan.Missing) != 1 || plan.Missing[0].Item != "C" || !reflect.DeepEqual(plan.Missing[0].Keys, []string{"{country:alpha3}"}) {
		t.Fatalf("missing %+v", plan.Missing)
	}
	for _, bad := range []View{
		{Name: "v", Selection: Head, Layout: "{#}-{owner}.{ext}"},
		{Name: "v", Selection: Head, Layout: "{#}-{country}.{ext}", Order: map[string][]string{"{country}": {"CN", "中国"}}},
		{Name: "v", Selection: Head, Layout: "{owner}.{ext}", Order: map[string][]string{"owner": {}}},
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
	names := NamesOf([]tree.Template{{Type: "driver_licence", Names: map[string]string{"zh": "驾驶证", "en": "Driver licence"}}})
	plan, err := Build(View{Name: "v", Selection: Head, Layout: "{type:zh}/{type:en}-{type}.{ext}"}, items, names)
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
	names := NamesOf([]tree.Template{{Type: "driver_licence", Names: map[string]string{"zh": "驾驶证"}}})
	v := View{Name: "v", Selection: Head, Layout: "{#}-{country:alpha3}/{name|type:zh}.{ext}", Order: map[string][]string{"{country:alpha3}": {"CN", "AU"}}}
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
	names := NamesOf([]tree.Template{{Type: "driver_licence", Names: map[string]string{"zh": "驾驶证"}}})
	v := View{Name: "v", Selection: Head, Layout: "{#}-{name|type:zh}.{ext}", Order: map[string][]string{"{name|type:zh}": {"户口首页", "驾驶证"}}}
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
	if len(plan.Missing) != 1 || !reflect.DeepEqual(plan.Missing[0].Keys, []string{"{name|type:zh}"}) {
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

// Layouts written before # came last inside the braces are rewritten.
func TestRewrite(t *testing.T) {
	for old, want := range map[string]string{
		"{owner#}/{country#:alpha3}/{name|type:zh}#-{x}.{ext}": "{#}-{owner}/{#}-{country:alpha3}/{#}-{name|type:zh}-{x}.{ext}",
		"{owner}#/{type}.{ext}":                                "{#}-{owner}/{type}.{ext}",
	} {
		got, changed := Rewrite(old)
		if got != want || !changed {
			t.Errorf("Rewrite(%q) = %q %v, want %q", old, got, changed, want)
		}
		if _, err := Parse(got); err != nil {
			t.Error(err)
		}
	}
	if _, changed := Rewrite("{#}-{owner}/{-degree?}.{ext}"); changed {
		t.Error("a layout written now was rewritten")
	}
}

// A condition names a country in any form and ignores case.
func TestMatchesCountryInAnyForm(t *testing.T) {
	item := tree.Item{Type: "id_card", Fields: map[string]string{"country": "中国", "owner": "Alex"}}
	for _, query := range []map[string]Values{
		{"country": {"CN"}}, {"country": {"CHN"}}, {"country": {"China"}}, {"owner": {"alex"}},
	} {
		if !Matches(query, item) {
			t.Fatalf("%v does not match", query)
		}
	}
	if Matches(map[string]Values{"country": {"AU"}}, item) {
		t.Fatal("AU matches 中国")
	}
}

func TestOptionalKey(t *testing.T) {
	items := []tree.Item{
		item("A", "diploma", map[string]string{"owner": "emma", "degree": "本科"}, "d1"),
		item("B", "id_card", map[string]string{"owner": "emma"}, "d2"),
	}
	v := View{Name: "x", Selection: Head, Layout: "{owner}/{type}{-degree?}{ (country:alpha2)?}.{ext}"}
	plan, err := Build(v, items, nil)
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
	plan, _ = Build(View{Name: "x", Selection: Head, Layout: "{owner}/{degree?}/{type}.{ext}"}, items, nil)
	if len(plan.Files) != 1 || len(plan.Missing) != 1 || plan.Missing[0].Item != "B" {
		t.Fatalf("empty folder: %+v", plan)
	}
	// {/key?} adds a folder when the key is there, and none when it is not.
	plan, _ = Build(View{Name: "x", Selection: Head, Layout: "licence{/degree?}/{type}{-degree?}.{ext}"}, items, nil)
	paths = nil
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"licence/id_card.pdf", "licence/本科/diploma-本科.pdf"}; !reflect.DeepEqual(paths, want) || !plan.Complete() {
		t.Fatalf("got %v %+v, want %v", paths, plan, want)
	}
	// {/#-key?} numbers the folder from the order {key}, from where numbers set.
	v = View{Name: "x", Selection: Head, Layout: "licence{/#-degree?}/{type}.{ext}",
		Order: map[string][]string{"{degree}": {"硕士", "本科"}}, Numbers: map[string]map[string]int{"{degree}": {"硕士": 10}}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, _ = Build(v, items, nil)
	paths = nil
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"licence/11-本科/diploma.pdf", "licence/id_card.pdf"}; !reflect.DeepEqual(paths, want) || !plan.Complete() {
		t.Fatalf("got %v %+v, want %v", paths, plan, want)
	}
	if err := (View{Name: "x", Selection: Head, Layout: v.Layout}).Validate(); err == nil {
		t.Error("{/#-degree?} without an order validated")
	}
	for _, bad := range []string{"{?}", "{-?}", "{-degree#?}", "a{#-degree?}", "a{/-#degree?}", "{degree?}#", "{:degree?}", "{degree|?}",
		"{/degree?}/x", "a{/degree?}b", "a{/-/degree?}", "a{degree/?}"} {
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
	v := View{Name: "v", Selection: Head, Layout: "x/{#}-{name}{-level?}.{ext}",
		Order: map[string][]string{"{name}{-level?}": {"护照", "毕业证书-本科", "毕业证书-硕士"}}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(v, items, nil)
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

// An old rule's orders move to the names its layout numbers now.
func TestUpgradeRenamesOrders(t *testing.T) {
	v, changed := Upgrade(View{Layout: "{owner#}/{name|type:zh#}{-level?}.{ext}",
		Order: map[string][]string{"owner": {"alex"}, "name|type:zh": {"护照"}}})
	want := map[string][]string{"{owner}": {"alex"}, "{name|type:zh}{-level?}": {"护照"}}
	if !changed || v.Layout != "{#}-{owner}/{#}-{name|type:zh}{-level?}.{ext}" || !reflect.DeepEqual(v.Order, want) {
		t.Fatalf("%v %+v", changed, v)
	}
}

func TestName(t *testing.T) {
	it := item("i1", "passport", map[string]string{"owner": "alex", "country": "AU"}, "d1")
	got, lacking, err := Name("{owner}-{type}{-number?}.{ext}", it, 1, nil)
	if err != nil || got != "alex-passport.pdf" || len(lacking) != 0 {
		t.Fatalf("got %q %v %v", got, lacking, err)
	}
	if _, lacking, _ := Name("{owner}-{name}.{ext}", it, 1, nil); len(lacking) != 1 || lacking[0] != "name" {
		t.Fatalf("lacking %v", lacking)
	}
	if _, _, err := Name("{#}-{owner}.{ext}", it, 1, nil); err == nil {
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
	v := View{Name: "x", Selection: Head, Layout: "{#}-{level|original.level}{/language?}/{#}-{name|original.name}.{ext}",
		Order: map[string][]string{"{level|original.level}": {"硕士研究生", "本科"}, "{name|original.name}": {"毕业证书", "学位证书"}}}
	plan, err := Build(v, items, nil)
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
