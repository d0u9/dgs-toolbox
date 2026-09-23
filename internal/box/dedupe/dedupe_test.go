package dedupe_test

import (
	"testing"

	"dgs-toolbox/internal/box/dedupe"
)

func ids(group dedupe.Group) []string {
	out := make([]string, 0, len(group.Items))
	for _, item := range group.Items {
		out = append(out, item.ID)
	}
	return out
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestEveryItemLandsInExactlyOneGroup(t *testing.T) {
	groups := dedupe.Groups([]dedupe.Item{
		{ID: "a", Digest: "sha256:1"},
		{ID: "b", Digest: "sha256:2"},
		{ID: "c", Digest: "sha256:3"},
	})
	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}
	for _, group := range groups {
		if group.Duplicated() {
			t.Errorf("a lone scan was called a duplicate: %v", ids(group))
		}
	}
}

func TestSameBytesGroup(t *testing.T) {
	groups := dedupe.Groups([]dedupe.Item{
		{ID: "a", Digest: "sha256:1"},
		{ID: "b", Digest: "sha256:1"},
	})
	if len(groups) != 1 || !equal(ids(groups[0]), []string{"a", "b"}) {
		t.Fatalf("got %v", groups)
	}
}

// The case whole-file hashing misses and this tool exists for: the same scan
// rewrapped by other software.
func TestSamePictureInADifferentContainerGroups(t *testing.T) {
	groups := dedupe.Groups([]dedupe.Item{
		{ID: "a", Digest: "sha256:1", ImageDigest: "sha256:img"},
		{ID: "b", Digest: "sha256:2", ImageDigest: "sha256:img"},
	})
	if len(groups) != 1 || !equal(ids(groups[0]), []string{"a", "b"}) {
		t.Fatalf("got %v", groups)
	}
}

// A and B hold the same bytes; B and C hold the same picture in different
// containers. All three are one document, and a person answers once.
func TestMatchingIsTransitive(t *testing.T) {
	groups := dedupe.Groups([]dedupe.Item{
		{ID: "a", Digest: "sha256:1"},
		{ID: "b", Digest: "sha256:1", ImageDigest: "sha256:img"},
		{ID: "c", Digest: "sha256:2", ImageDigest: "sha256:img"},
	})
	if len(groups) != 1 || !equal(ids(groups[0]), []string{"a", "b", "c"}) {
		t.Fatalf("got %v", groups)
	}
}

// A vector page and a composite page both have no picture. They are not the
// same document, and grouping them would collapse a whole class of scans into
// one bogus pile.
func TestNoImageDigestNeverMatches(t *testing.T) {
	groups := dedupe.Groups([]dedupe.Item{
		{ID: "a", Digest: "sha256:1"},
		{ID: "b", Digest: "sha256:2"},
	})
	if len(groups) != 2 {
		t.Fatalf("two scans with no picture were grouped: %v", groups)
	}
}

// A file thrown away in March must not be quietly taken back in in September.
func TestTrashTakesPart(t *testing.T) {
	groups := dedupe.Groups([]dedupe.Item{
		{ID: "incoming", Digest: "sha256:1"},
		{ID: "discarded", Digest: "sha256:1", InTrash: true, TrashedAt: "2026-03-04"},
	})
	if len(groups) != 1 {
		t.Fatalf("got %d groups", len(groups))
	}
	group := groups[0]
	if !group.InTrash() {
		t.Error("the group does not say a decision was already made")
	}
	if live := group.Live(); len(live) != 1 || live[0].ID != "incoming" {
		t.Errorf("live copies: got %v", live)
	}
}

// These are worked through as a list, and a list that reshuffles between runs
// cannot be worked through.
func TestOrderIsStable(t *testing.T) {
	items := []dedupe.Item{
		{ID: "c", Digest: "sha256:3"},
		{ID: "a", Digest: "sha256:1"},
		{ID: "b", Digest: "sha256:1"},
	}
	first := dedupe.Groups(items)
	for range 20 {
		again := dedupe.Groups(items)
		if len(again) != len(first) {
			t.Fatalf("group count moved: %d then %d", len(first), len(again))
		}
		for i := range first {
			if !equal(ids(first[i]), ids(again[i])) {
				t.Fatalf("group %d moved: %v then %v", i, ids(first[i]), ids(again[i]))
			}
		}
	}
	if !equal(ids(first[0]), []string{"a", "b"}) {
		t.Errorf("first group: got %v", ids(first[0]))
	}
}
