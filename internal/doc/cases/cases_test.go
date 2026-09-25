package cases

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

var now = time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

func items() []tree.Item {
	return []tree.Item{
		{ID: "A", Type: "passport", Kind: tree.Kind("document"), Head: "a1", Revisions: []tree.Revision{{Digest: "a1"}, {Digest: "a2"}}},
		{ID: "B", Type: "bill", Kind: tree.Kind("record"), Revisions: []tree.Revision{{Digest: "b1"}}},
		{ID: "C", Type: "bill", Kind: tree.Kind("record"), Revisions: []tree.Revision{{Digest: "c1"}}},
	}
}

func TestNeedsAreMetAsTheyComeIn(t *testing.T) {
	c, err := New("lease", "Lease in Ashfield", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Add("A", "for the application", now); err != nil {
		t.Fatal(err)
	}
	if err := c.Add("A", "", now); err == nil {
		t.Fatal("an Item went in twice")
	}
	if err := c.AddNeed("  the last electricity bill ", now); err != nil {
		t.Fatal(err)
	}
	if err := c.Meet(0, "B", now); err != nil {
		t.Fatal(err)
	}
	if !c.Has("B") || c.Entries[1].Note != "the last electricity bill" || c.Needs[0].Item != "B" {
		t.Fatalf("meeting a need: %+v", c)
	}
	if err := c.Remove("B"); err != nil {
		t.Fatal(err)
	}
	if c.Needs[0].Item != "" || c.Needs[0].Met != "" {
		t.Fatalf("a need met by an Item taken out stays met: %+v", c.Needs[0])
	}
}

func TestArchiveRecordsTheRevisionHandedOver(t *testing.T) {
	c, _ := New("passport", "", now)
	_ = c.Add("A", "", now)
	_ = c.Add("B", "", now)
	if err := c.Archive(items(), now); err != nil {
		t.Fatal(err)
	}
	if c.Entries[0].Digest != "a1" || c.Entries[1].Digest != "b1" {
		t.Fatalf("recorded: %+v", c.Entries)
	}
	if err := c.Add("C", "", now); err == nil {
		t.Fatal("an archived Case changed")
	}
	// HEAD moves on: the archived Case still takes what was handed over.
	later := items()
	later[0].Head = "a2"
	got, missing := c.Items(later)
	if len(missing) != 0 || got[0].Current() != "a1" {
		t.Fatalf("archived export takes %s, missing %v", got[0].Current(), missing)
	}
	if err := c.Reopen(); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Items(later); got[0].Current() != "a2" {
		t.Fatal("an open Case takes the current revision")
	}
}

func TestItemsReportsWhatIsGone(t *testing.T) {
	c, _ := New("x", "", now)
	_ = c.Add("Z", "", now)
	_ = c.Add("B", "", now)
	got, missing := c.Items(items())
	if len(got) != 1 || got[0].ID != "B" || len(missing) != 1 || missing[0] != "Z" {
		t.Fatalf("got %v, missing %v", got, missing)
	}
}

func TestTheDefaultViewNumbersOneType(t *testing.T) {
	c, _ := New("x", "", now)
	_ = c.Add("B", "", now)
	_ = c.Add("C", "", now)
	_ = c.Add("A", "", now)
	found, _ := c.Items(items())
	plan, err := view.Build(c.View(), found)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	if !plan.Complete() || strings.Join(paths, " ") != "bill.pdf bill_01.pdf passport.pdf" {
		t.Fatalf("paths %v, plan %+v", paths, plan)
	}
}

func TestSaveLoadTrash(t *testing.T) {
	root := t.TempDir()
	if err := tree.Init(root, now); err != nil {
		t.Fatal(err)
	}
	b, _ := New("b", "", now)
	a, _ := New("a", "", now)
	_ = b.Archive(nil, now)
	for _, c := range []Case{b, a} {
		if err := Save(root, c, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(root, a, true); err == nil {
		t.Fatal("a second Case took a name")
	}
	all, err := Load(root)
	if err != nil || len(all) != 2 || all[0].Name != "a" || all[1].Name != "b" {
		t.Fatalf("open first: %v %v", all, err)
	}
	to, err := Trash(root, "a", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(to); err != nil || filepath.Dir(to) != filepath.Join(root, tree.TrashDir, Dir) {
		t.Fatalf("trashed to %s: %v", to, err)
	}
	if _, err := New("Bad Name", "", now); err == nil {
		t.Fatal("a bad name passed")
	}
}
