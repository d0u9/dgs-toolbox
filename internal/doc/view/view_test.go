package view

import (
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
	for _, bad := range []string{"{?}", "{-?}", "{-degree#?}", "{degree?}#", "{:degree?}", "{degree|?}"} {
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
