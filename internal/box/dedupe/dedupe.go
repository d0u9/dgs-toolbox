// Package dedupe groups the scans that are the same piece of paper.
//
// Two scans match on either digest. The whole-file digest catches a rescan and
// a restored backup; the image digest catches the same scan rewrapped by other
// software, where the container differs byte for byte and the scan inside it
// does not. For scanned material the second case is the common one, which is
// why matching on the first alone would miss most of the pile.
//
// Matching is transitive: if A and B hold the same bytes, and B and C hold the
// same picture inside different containers, all three are one document. Leaving
// that out would present a person with the same decision two or three times and
// let them answer it differently each time.
package dedupe

import "sort"

// Item is one scan as far as grouping is concerned. The caller keys it by
// whatever it needs back — a path, a candidate id — and this package returns
// the same values untouched.
type Item struct {
	// ID is the caller's key. It must be unique across the items given.
	ID string
	// Digest is the whole-file digest. An item with none is only ever grouped
	// by its image digest.
	Digest string
	// ImageDigest is the digest of the embedded image streams alone, empty for
	// a scan with no embedded image. Empty never matches empty: a vector page
	// and a composite page are not the same document just because neither holds
	// a picture, and treating them as one would collapse a whole class of
	// scans into a single bogus duplicate group.
	ImageDigest string
	// InTrash marks an item that lives in the Box's trash. The trash takes part
	// in deduplication so the same judgement is not made twice: a file thrown
	// away in March must not be silently taken back in in September.
	InTrash bool
	// TrashedAt is when it was discarded, for showing beside a trash match.
	TrashedAt string
}

// Group is one document and every copy of it that turned up.
type Group struct {
	Items []Item
}

// Duplicated reports whether this group holds more than one copy.
func (g Group) Duplicated() bool { return len(g.Items) > 1 }

// InTrash reports whether any copy in this group is in the trash, which is what
// says a decision about it has already been made once.
func (g Group) InTrash() bool {
	for _, item := range g.Items {
		if item.InTrash {
			return true
		}
	}
	return false
}

// Live returns the copies that are not in the trash.
func (g Group) Live() []Item {
	live := make([]Item, 0, len(g.Items))
	for _, item := range g.Items {
		if !item.InTrash {
			live = append(live, item)
		}
	}
	return live
}

// Groups collects items into one group per document.
//
// Every item appears in exactly one group, including an item with no match:
// a caller that had to handle "grouped" and "ungrouped" separately would write
// the same code twice. The order is stable — groups by their first item's ID,
// items by ID within a group — because these are shown in a list and a list
// that reshuffles between runs cannot be worked through.
func Groups(items []Item) []Group {
	parent := make(map[string]string, len(items))
	for _, item := range items {
		parent[item.ID] = item.ID
	}
	join := func(a, b string) {
		rootA, rootB := find(parent, a), find(parent, b)
		if rootA != rootB {
			parent[rootA] = rootB
		}
	}
	// One pass per digest kind: the first item seen with a given digest becomes
	// what every later one joins to.
	byWhole := map[string]string{}
	byImage := map[string]string{}
	for _, item := range items {
		if item.Digest != "" {
			if first, seen := byWhole[item.Digest]; seen {
				join(item.ID, first)
			} else {
				byWhole[item.Digest] = item.ID
			}
		}
		if item.ImageDigest != "" {
			if first, seen := byImage[item.ImageDigest]; seen {
				join(item.ID, first)
			} else {
				byImage[item.ImageDigest] = item.ID
			}
		}
	}

	collected := map[string][]Item{}
	for _, item := range items {
		root := find(parent, item.ID)
		collected[root] = append(collected[root], item)
	}
	groups := make([]Group, 0, len(collected))
	for _, members := range collected {
		sort.Slice(members, func(a, b int) bool { return members[a].ID < members[b].ID })
		groups = append(groups, Group{Items: members})
	}
	sort.Slice(groups, func(a, b int) bool { return groups[a].Items[0].ID < groups[b].Items[0].ID })
	return groups
}

func find(parent map[string]string, id string) string {
	root := id
	for parent[root] != root {
		root = parent[root]
	}
	// Flatten what was walked, so a long chain is walked once rather than once
	// per member.
	for parent[id] != root {
		parent[id], id = root, parent[id]
	}
	return root
}
