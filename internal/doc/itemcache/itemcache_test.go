package itemcache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

func newTree(t *testing.T, ids ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := tree.Init(root, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		write(t, root, id, "one")
	}
	return root
}

func write(t *testing.T, root, id, value string) {
	t.Helper()
	item := tree.Item{ID: id, Type: "bill", Kind: "record", Fields: map[string]string{"issuer": value},
		Revisions: []tree.Revision{{ID: "r1", Added: "2026-01-01"}}}
	if err := tree.WriteItem(root, item); err != nil {
		t.Fatal(err)
	}
}

// same fails unless the cache answers as the sidecars say, compared as the
// page receives them.
func same(t *testing.T, c *Cache, root string) {
	t.Helper()
	got, err := c.Items()
	if err != nil {
		t.Fatal(err)
	}
	want, err := tree.LoadItems(root)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Fatalf("cache %s\nsidecars %s", a, b)
	}
}

func TestRebuiltFromSidecarsMatches(t *testing.T) {
	root, dir := newTree(t, "A", "B"), t.TempDir()
	same(t, New(root, dir), root)
	// A second run takes the kept file, and answers the same.
	same(t, New(root, dir), root)
	os.RemoveAll(dir)
	same(t, New(root, dir), root)
}

func TestAMissingMarkIsWritten(t *testing.T) {
	root := newTree(t, "A")
	if _, err := New(root, "").Items(); err != nil {
		t.Fatal(err)
	}
	if token, _ := tree.ReadChangeMark(root); token == "" {
		t.Fatal("no change mark after the first read")
	}
}

func TestWroteReadsOnlyTheItemsNamed(t *testing.T) {
	root := newTree(t, "B")
	c := New(root, t.TempDir())
	same(t, c, root)
	write(t, root, "A", "new")
	write(t, root, "C", "new")
	write(t, root, "B", "changed")
	if err := c.Wrote("A", "B", "C"); err != nil {
		t.Fatal(err)
	}
	same(t, c, root)
	os.RemoveAll(tree.Dir(root, "B"))
	if err := c.Wrote("B"); err != nil {
		t.Fatal(err)
	}
	same(t, c, root)
}

func TestAChangeElsewhereIsSeenByItsMark(t *testing.T) {
	root := newTree(t, "A")
	c := New(root, "")
	same(t, c, root)
	write(t, root, "A", "elsewhere")
	// Without a new mark the cache does not look.
	if items, _ := c.Items(); items[0].Fields["issuer"] != "one" {
		t.Fatal("read the tree with no change marked")
	}
	if _, err := tree.WriteChangeMark(root); err != nil {
		t.Fatal(err)
	}
	same(t, c, root)
}

func TestAnUnreadableOrOtherFileIsRebuilt(t *testing.T) {
	root, dir := newTree(t, "A"), t.TempDir()
	c := New(root, dir)
	same(t, c, root)
	for _, content := range []string{"not json", `{"version":999}`} {
		if err := os.WriteFile(c.file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		same(t, New(root, dir), root)
	}
	if _, err := os.Stat(filepath.Dir(c.file)); err != nil {
		t.Fatal(err)
	}
}

func TestACopyChangedLeavesTheCache(t *testing.T) {
	root := newTree(t, "A")
	c := New(root, "")
	items, _ := c.Items()
	items[0].Fields["issuer"] = "changed"
	same(t, c, root)
}
