package tree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

func newTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := Init(root, now); err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func idCard(t *testing.T, root string) Template {
	t.Helper()
	templates, err := LoadTemplates(root)
	if err != nil || len(templates) != 1 || templates[0].Type != "id_card" {
		t.Fatalf("templates = %+v, %v", templates, err)
	}
	return templates[0]
}

func TestInitWritesMarkerAndExampleOnce(t *testing.T) {
	root := newTree(t)
	if err := Require(root); err != nil {
		t.Fatal(err)
	}
	idCard(t, root)
	if err := Init(root, now); err == nil {
		t.Fatal("second Init succeeded")
	}
	if err := Require(t.TempDir()); !errors.Is(err, ErrNotATree) {
		t.Fatalf("Require on a plain folder = %v", err)
	}
}

func TestLoadTemplatesRefusesBadOnes(t *testing.T) {
	for name, content := range map[string]string{
		"wrong_name.yaml": "type: other\nkind: record\n",
		"bad_kind.yaml":   "type: bad_kind\nkind: thing\n",
		"loose.yaml":      "type: loose\nkind: document\nfields:\n  - key: a\n    distinguishing: true\n",
		"unknown.yaml":    "type: unknown\nkind: record\ncolour: red\n",
	} {
		root := t.TempDir()
		write(t, filepath.Join(root, TemplatesDir, name), content)
		if _, err := LoadTemplates(root); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestImportMakesAnItemAndLeavesTheSource(t *testing.T) {
	root := newTree(t)
	source := write(t, filepath.Join(root, "scan.pdf"), "%PDF jane")
	item, err := Import(context.Background(), ImportRequest{
		Root: root, Source: source, Template: idCard(t, root), Now: now,
		Fields: map[string]string{"owner": " jane ", "country": "AU", "number": ""},
		Notes:  "  old card for records  ",
		Tags:   []string{"Travel Plans", "travel-plans", "  visa  "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Head == "" || item.Head != item.Revisions[0].Ref() || item.Fields["owner"] != "jane" {
		t.Fatalf("item = %+v", item)
	}
	if item.Notes != "old card for records" {
		t.Fatalf("notes = %q", item.Notes)
	}
	if strings.Join(item.Tags, ",") != "travel-plans,visa" {
		t.Fatalf("tags = %v", item.Tags)
	}
	if _, ok := item.Fields["number"]; ok {
		t.Error("empty field kept")
	}
	if data, err := os.ReadFile(PDFPath(root, item.ID, item.CurrentDigest())); err != nil || string(data) != "%PDF jane" {
		t.Fatalf("stored PDF: %q %v", data, err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("source was touched")
	}
	items, err := LoadItems(root)
	if err != nil || len(items) != 1 || items[0].ID != item.ID || items[0].Notes != item.Notes || strings.Join(items[0].Tags, ",") != strings.Join(item.Tags, ",") {
		t.Fatalf("LoadItems = %+v, %v", items, err)
	}
	updated, err := SetTags(root, item.ID, []string{"Visa", "visa", "  archive "}, time.Now())
	if err != nil || strings.Join(updated.Tags, ",") != "visa,archive" {
		t.Fatalf("SetTags = %+v, %v", updated.Tags, err)
	}
}

func TestImportRefuses(t *testing.T) {
	root := newTree(t)
	tpl := idCard(t, root)
	first := write(t, filepath.Join(root, "a.pdf"), "%PDF a")
	if _, err := Import(context.Background(), ImportRequest{Root: root, Source: first, Template: tpl, Now: now,
		Fields: map[string]string{"owner": "jane", "country": "AU"}}); err != nil {
		t.Fatal(err)
	}
	other := write(t, filepath.Join(root, "b.pdf"), "%PDF b")
	cases := []struct {
		name   string
		source string
		fields map[string]string
		want   string
	}{
		{"same PDF", first, map[string]string{"owner": "doug", "country": "AU"}, "already in the tree"},
		{"same document", other, map[string]string{"owner": "Jane", "country": "au"}, "exists"},
		{"missing", other, map[string]string{"owner": "doug"}, "missing country"},
		{"unknown key", other, map[string]string{"owner": "doug", "country": "CN", "colour": "red"}, "no field colour"},
	}
	for _, c := range cases {
		_, err := Import(context.Background(), ImportRequest{Root: root, Source: c.source, Template: tpl, Now: now, Fields: c.fields})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
	// A different country is a different document.
	if _, err := Import(context.Background(), ImportRequest{Root: root, Source: other, Template: tpl, Now: now,
		Fields: map[string]string{"owner": "jane", "country": "CN"}}); err != nil {
		t.Fatal(err)
	}
}

func TestImportNeedsATree(t *testing.T) {
	root := t.TempDir()
	source := write(t, filepath.Join(root, "a.pdf"), "x")
	_, err := Import(context.Background(), ImportRequest{Root: root, Source: source, Template: Template{Type: "x", Kind: KindRecord}, Now: now})
	if !errors.Is(err, ErrNotATree) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewIDIsAULID(t *testing.T) {
	a, _ := NewID(now)
	b, _ := NewID(now.Add(time.Millisecond))
	if len(a) != 26 || a == b || a[:10] >= b[:10] {
		t.Fatalf("ids %s %s", a, b)
	}
	if a[:10] != "01M3C03V80" {
		t.Errorf("time part = %s", a[:10])
	}
}

func importOne(t *testing.T, root, name, content string, fields map[string]string) Item {
	t.Helper()
	source := write(t, filepath.Join(root, name), content)
	item, err := Import(context.Background(), ImportRequest{Root: root, Source: source, Template: idCard(t, root), Fields: fields, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestRevisionsMoveHeadAndBack(t *testing.T) {
	root := newTree(t)
	item := importOne(t, root, "old.pdf", "%PDF old", map[string]string{"owner": "jane", "country": "AU"})
	first := item.Head
	renewed := write(t, filepath.Join(root, "new.pdf"), "%PDF new")
	item, err := AddRevision(context.Background(), root, item.ID, renewed, idCard(t, root), nil, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Revisions) != 2 || item.Head == first || item.Head != item.Revisions[1].Ref() {
		t.Fatalf("after adding: %+v", item)
	}
	if again, err := AddRevision(context.Background(), root, item.ID, renewed, idCard(t, root), nil, now); err != nil || len(again.Revisions) != 2 {
		t.Fatalf("same PDF again: %v", err)
	}
	if item, err = SetHead(root, item.ID, first, time.Now()); err != nil || item.Head != first {
		t.Fatalf("SetHead: %+v %v", item, err)
	}
	loaded, _, _ := FindItem(root, item.ID)
	if loaded.Head != first || len(loaded.Revisions) != 2 {
		t.Fatalf("on disk: %+v", loaded)
	}
	if _, err := SetHead(root, item.ID, "nope", time.Now()); err == nil {
		t.Fatal("HEAD moved to a digest that is not a revision")
	}
	for _, r := range loaded.Revisions {
		if _, err := os.Stat(PDFPath(root, item.ID, r.Digest)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecordsDoNotAcceptRenewals(t *testing.T) {
	root := newTree(t)
	write(t, filepath.Join(root, TemplatesDir, "payslip.yaml"), "type: payslip\nkind: record\nfields:\n  - key: owner\n")
	templates, _ := LoadTemplates(root)
	source := write(t, filepath.Join(root, "p.pdf"), "%PDF p")
	item, err := Import(context.Background(), ImportRequest{Root: root, Source: source, Template: templates[1], Now: now})
	if err != nil || item.Head == "" {
		t.Fatalf("%+v %v", item, err)
	}
	other := write(t, filepath.Join(root, "q.pdf"), "%PDF q")
	if _, err := AddRevision(context.Background(), root, item.ID, other, templates[1], nil, now); err == nil {
		t.Fatal("record took a revision")
	}
	if _, err := SetHead(root, item.ID, item.Revisions[0].Ref(), time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestSetFieldsKeepsDocumentsDistinct(t *testing.T) {
	root := newTree(t)
	au := importOne(t, root, "au.pdf", "%PDF au", map[string]string{"owner": "jane", "country": "AU"})
	importOne(t, root, "cn.pdf", "%PDF cn", map[string]string{"owner": "jane", "country": "CN"})
	tpl := idCard(t, root)
	if _, err := SetFields(root, au.ID, "", tpl, map[string]string{"owner": "jane", "country": "CN"}, time.Now()); !errors.Is(err, ErrTaken) {
		t.Fatalf("clash: %v", err)
	}
	item, err := SetFields(root, au.ID, "", tpl, map[string]string{"owner": "jane", "country": "AU", "number": "123"}, time.Now())
	if err != nil || item.CurrentFields()["number"] != "123" {
		t.Fatalf("%+v %v", item, err)
	}
	if _, err := SetFields(root, au.ID, "", tpl, map[string]string{"owner": "jane"}, time.Now()); err == nil {
		t.Fatal("required field dropped")
	}
}

func TestFieldTypes(t *testing.T) {
	bad := []Template{
		{Type: "x", Kind: KindRecord, Fields: []Field{{Key: "a", Type: "number"}}},
		{Type: "x", Kind: KindRecord, Fields: []Field{{Key: "a", Type: FieldSelect}}},
		{Type: "x", Kind: KindRecord, Fields: []Field{{Key: "a", Options: []string{"y"}}}},
		{Type: "x", Kind: KindRecord, Fields: []Field{{Key: "a", Type: FieldDate}}, Defaults: map[string]string{"a": "soon"}},
		{Type: "x", Kind: KindRecord, IgnoreDates: []string{"1990-02-30"}},
	}
	for i, tpl := range bad {
		if tpl.Validate() == nil {
			t.Errorf("bad template %d accepted", i)
		}
	}
	tpl := Template{Type: "bill", Kind: KindRecord, Fields: []Field{
		{Key: "issued_at", Type: FieldDate},
		{Key: "currency", Type: FieldSelect, Options: []string{"AUD", "CNY"}},
		{Key: "replaces", Type: FieldItem},
	}}
	if err := tpl.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, given := range []map[string]string{
		{"issued_at": "2026-13-01"}, {"currency": "USD"}, {"replaces": "not-an-id"},
	} {
		if _, err := CleanFields(tpl, given); err == nil {
			t.Errorf("%v accepted", given)
		}
	}
	if _, err := CleanFields(tpl, map[string]string{"issued_at": "2026-09-25", "currency": "AUD"}); err != nil {
		t.Fatal(err)
	}
}

func TestItemLinksAndNotes(t *testing.T) {
	root := newTree(t)
	tpl := Template{Type: "passport", Kind: KindDocument, Fields: []Field{
		{Key: "owner", Required: true, Distinguishing: true},
		{Key: "previous", Type: FieldItem},
	}}
	add := func(owner, previous string) (Item, error) {
		source := write(t, filepath.Join(t.TempDir(), "p.pdf"), "%PDF "+owner)
		return Import(context.Background(), ImportRequest{Root: root, Source: source, Template: tpl, Now: now,
			Fields: map[string]string{"owner": owner, "previous": previous}})
	}
	old, err := add("jane", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := add("tom", "01ARZ3NDEKTSV4RRFFQ69G5FAV"); err == nil {
		t.Fatal("link to a missing Item accepted")
	}
	current, err := add("ann", old.ID)
	if err != nil || current.Fields["previous"] != old.ID {
		t.Fatal(err, current)
	}
	if _, err := SetFields(root, current.ID, "", tpl, map[string]string{"owner": "ann", "previous": current.ID}, time.Now()); err == nil {
		t.Fatal("self link accepted")
	}
	noted, err := SetNotes(root, old.ID, "  expired; kept for the visa file \n", time.Now())
	if err != nil || noted.Notes != "expired; kept for the visa file" {
		t.Fatal(err, noted.Notes)
	}
	again, _, _ := FindItem(root, old.ID)
	if again.Notes != noted.Notes {
		t.Fatal("notes not written")
	}
}

func TestTrashMovesTheItem(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	if err := Init(root, now); err != nil {
		t.Fatal(err)
	}
	item := Item{ID: "A", Type: "id_card", Kind: KindRecord, Revisions: []Revision{{Digest: "d"}}}
	if err := os.MkdirAll(Dir(root, "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PDFPath(root, "A", "d"), []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteItem(root, item); err != nil {
		t.Fatal(err)
	}
	to, err := Trash(root, "A", now)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(to, "d.pdf")); err != nil || string(data) != "pdf" {
		t.Fatalf("trashed PDF: %v %q", err, data)
	}
	if items, err := LoadItems(root); err != nil || len(items) != 0 {
		t.Fatalf("items after trash: %v %+v", err, items)
	}
	if _, err := Trash(root, "A", now); err == nil {
		t.Fatal("trashed an Item that is gone")
	}
}

func TestTrashRevisionMovesPDFAndUpdatesHead(t *testing.T) {
	root := newTree(t)
	item := Item{ID: "A", Type: "card", Kind: KindDocument, Head: "new", Revisions: []Revision{{Digest: "old"}, {Digest: "new"}}}
	if err := WriteItem(root, item); err != nil {
		t.Fatal(err)
	}
	for _, digest := range []string{"old", "new"} {
		if err := os.WriteFile(PDFPath(root, "A", digest), []byte(digest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := TrashRevision(root, "A", "missing", now); err == nil {
		t.Fatal("missing revision accepted")
	}
	updated, to, err := TrashRevision(root, "A", "new", now)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Head != "old" || len(updated.Revisions) != 1 {
		t.Fatalf("updated: %+v", updated)
	}
	if _, err := os.Stat(strings.TrimSuffix(to, ".pdf") + ".yaml"); err != nil {
		t.Fatal("snapshot metadata not retained in trash")
	}
	if data, err := os.ReadFile(to); err != nil || string(data) != "new" {
		t.Fatalf("trash: %q %v", data, err)
	}
	if _, err := os.Stat(PDFPath(root, "A", "new")); !os.IsNotExist(err) {
		t.Fatalf("source PDF: %v", err)
	}
	if saved, _, err := FindItem(root, "A"); err != nil || saved.Head != "old" || len(saved.Revisions) != 1 {
		t.Fatalf("saved: %+v %v", saved, err)
	}
	if _, _, err := TrashRevision(root, "A", "old", now); err == nil {
		t.Fatal("last revision deleted")
	}
}

func TestPerRevisionFields(t *testing.T) {
	root := newTree(t)
	tpl := Template{Type: "card", Kind: KindDocument, Fields: []Field{
		{Key: "owner", Required: true, Distinguishing: true},
		{Key: "number", Required: true, PerRevision: true},
		{Key: "expires", PerRevision: true},
		{Key: "note"},
	}}
	if err := tpl.Validate(); err != nil {
		t.Fatal(err)
	}
	old := write(t, filepath.Join(root, "old.pdf"), "%PDF old")
	item, err := Import(context.Background(), ImportRequest{Root: root, Source: old, Template: tpl, Now: now,
		Fields: map[string]string{"owner": "jane", "number": "111", "expires": "2020", "note": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if item.CurrentFields()["number"] != "111" || item.Revisions[0].Fields["number"] != "111" || item.Fields["note"] != "x" {
		t.Fatalf("import split: %+v", item)
	}
	renewed := write(t, filepath.Join(root, "new.pdf"), "%PDF new")
	if _, err := AddRevision(context.Background(), root, item.ID, renewed, tpl, map[string]string{"expires": "2030"}, now); err == nil {
		t.Fatal("a revision without its required number was added")
	}
	if _, err := AddRevision(context.Background(), root, item.ID, renewed, tpl, map[string]string{"number": "222", "owner": "tom"}, now); err == nil {
		t.Fatal("a revision changed the Item's own field")
	}
	item, err = AddRevision(context.Background(), root, item.ID, renewed, tpl, map[string]string{"number": "222", "expires": "2030"}, now)
	if err != nil {
		t.Fatal(err)
	}
	first, second := item.Revisions[0].Ref(), item.Revisions[1].Ref()
	if got := item.CurrentFields(); got["number"] != "222" || got["expires"] != "2030" || got["note"] != "x" {
		t.Fatalf("current: %v", got)
	}
	if got := item.FieldsAt(first); got["number"] != "111" || got["expires"] != "2020" {
		t.Fatalf("old card: %v", got)
	}
	// Editing an old snapshot creates a new HEAD and preserves both old cards.
	item, err = SetFields(root, item.ID, first, tpl, map[string]string{"owner": "jane", "number": "110", "note": "y"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Revisions) != 3 || item.FieldsAt(first)["number"] != "111" || item.FieldsAt(first)["expires"] != "2020" || item.CurrentFields()["number"] != "110" || item.CurrentFields()["expires"] != "" || item.FieldsAt(second)["number"] != "222" || item.Fields["note"] != "y" {
		t.Fatalf("after editing the old card: %+v", item)
	}
	for _, bad := range []Field{{Key: "k", PerRevision: true, Distinguishing: true, Required: true}} {
		if (Template{Type: "c", Kind: KindDocument, Fields: []Field{bad}}).Validate() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if (Template{Type: "r", Kind: KindRecord, Fields: []Field{{Key: "k", PerRevision: true}}}).Validate() == nil {
		t.Error("per_revision on a record accepted")
	}
}

func TestRecordExportsWritesEachItemOnce(t *testing.T) {
	root := newTree(t)
	for _, id := range []string{"A", "B"} {
		if err := WriteItem(root, Item{ID: id, Type: "card", Kind: KindDocument}); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	err := RecordExports(root, []Exported{
		{Item: "A", Digest: "a1", Target: "/t", View: "v"},
		{Item: "A", Digest: "a2", Target: "/t", View: "w"},
		{Item: "B", Digest: "b1", Target: "/t", View: "v"},
		{Item: "gone", Digest: "g", Target: "/t", View: "v"},
	}, at)
	if err != nil {
		t.Fatal(err)
	}
	a, _, _ := FindItem(root, "A")
	b, _, _ := FindItem(root, "B")
	if len(a.History) != 2 || a.History[1].Digest != "a2" || a.History[1].View != "w" || len(b.History) != 1 || b.History[0].Action != "export" {
		t.Fatalf("A %+v, B %+v", a.History, b.History)
	}
}

func TestInsertByTimeKeepsHistoryOrdered(t *testing.T) {
	history := []HistoryEvent{{At: "2026-01-01T00:00:00Z", Action: "a"}, {At: "2026-03-01T00:00:00Z", Action: "c"}}
	got := insertByTime(history, HistoryEvent{At: "2026-02-01T10:00:00+10:00", Action: "b"})
	if len(got) != 3 || got[1].Action != "b" {
		t.Fatalf("got %+v", got)
	}
	if got = insertByTime(got, HistoryEvent{At: "bad", Action: "z"}); got[3].Action != "z" {
		t.Fatalf("unreadable time not last: %+v", got)
	}
}

func TestSetFrequentRecordsOnlyChanges(t *testing.T) {
	root := newTree(t)
	if err := WriteItem(root, Item{ID: "A", Type: "card", Kind: KindDocument}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for _, on := range []bool{true, true, false} {
		if _, err := SetFrequent(root, "A", on, at); err != nil {
			t.Fatal(err)
		}
	}
	item, _, _ := FindItem(root, "A")
	if item.Frequent || len(item.History) != 2 || item.History[0].Action != "mark_frequent" || item.History[1].Action != "unmark_frequent" {
		t.Fatalf("item: %+v", item)
	}
}

func TestTimelineAddsImportsOlderSidecarsLack(t *testing.T) {
	item := Item{ID: "A", Revisions: []Revision{{Digest: "x", Added: "2026-01-01T00:00:00Z"}, {Digest: "y", Added: "2026-03-01T00:00:00Z"}},
		History: []HistoryEvent{{At: "2026-02-01T00:00:00Z", Action: "edit_notes"}, {At: "2026-03-01T00:00:00Z", Action: "import_revision", Digest: "y"}}}
	got := Timeline(item)
	if len(got) != 3 || got[0].Action != "import" || got[0].Digest != "x" || got[1].Action != "edit_notes" {
		t.Fatalf("timeline: %+v", got)
	}
}

func TestLogIsNewestFirstAcrossItems(t *testing.T) {
	items := []Item{
		{ID: "A", Type: "a", History: []HistoryEvent{{At: "2026-01-01T00:00:00Z", Action: "one"}, {At: "2026-03-01T00:00:00Z", Action: "three"}, {At: "2026-03-01T00:00:00Z", Action: "four"}}},
		{ID: "B", Type: "b", History: []HistoryEvent{{At: "2026-02-01T10:00:00+10:00", Action: "two"}, {At: "bad", Action: "last"}}},
	}
	var got []string
	for _, e := range Log(items) {
		got = append(got, e.Item+":"+e.Event.Action)
	}
	if strings.Join(got, " ") != "A:four A:three B:two A:one B:last" {
		t.Fatalf("log: %v", got)
	}
}

func TestSetRetiredKeepsReasonAndHistory(t *testing.T) {
	root := newTree(t)
	if err := WriteItem(root, Item{ID: "A", Type: "card", Kind: KindDocument}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	steps := []struct {
		retired bool
		reason  string
	}{{true, " left the company "}, {true, "left the company"}, {true, "moved away"}, {false, "ignored"}}
	for _, step := range steps {
		if _, err := SetRetired(root, "A", step.retired, step.reason, at); err != nil {
			t.Fatal(err)
		}
	}
	item, _, _ := FindItem(root, "A")
	var actions []string
	for _, e := range item.History {
		actions = append(actions, e.Action)
	}
	if item.Retired || item.RetiredReason != "" || strings.Join(actions, " ") != "retire edit_retired_reason unretire" {
		t.Fatalf("item %+v, actions %v", item, actions)
	}
	if got := item.History[0].Changes["reason"]; got[1] != "left the company" {
		t.Fatalf("retire reason: %v", got)
	}
}

func TestRevisionTagsAreTheRevisionsOwn(t *testing.T) {
	root := newTree(t)
	item := Item{ID: "A", Type: "card", Kind: KindDocument, Tags: []string{"home"}, Head: "y",
		Revisions: []Revision{{Digest: "x"}, {Digest: "y"}}}
	if err := WriteItem(root, item); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	got, err := SetRevisionTags(root, "A", "x", []string{"Reissued", "reissued", "home"}, at)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Tags, ",") != "home" || strings.Join(got.Revisions[0].Tags, ",") != "reissued,home" || len(got.Revisions[1].Tags) != 0 {
		t.Fatalf("tags: %+v", got)
	}
	if strings.Join(got.TagsAt("x"), ",") != "home,reissued" || strings.Join(got.TagsAt("y"), ",") != "home" {
		t.Fatalf("TagsAt: %v %v", got.TagsAt("x"), got.TagsAt("y"))
	}
	if len(got.History) != 1 || got.History[0].Action != "edit_revision_tags" || got.History[0].Digest != "x" {
		t.Fatalf("history: %+v", got.History)
	}
	if _, err := SetRevisionTags(root, "A", "nope", nil, at); err == nil {
		t.Fatal("unknown revision accepted")
	}
}
