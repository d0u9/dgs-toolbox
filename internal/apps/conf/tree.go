package conf

import (
	"sort"

	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/target"
	"dgs-toolbox/internal/tui/text"
)

// instanceNode is one instance under a node or an unmanaged user.
type instanceNode struct {
	name   string
	detail string // "service / role"
	broken string
}

// nodeGroup is one top-level tree entry: a node, or an unmanaged user
// holding the instances derived for them. broken is a node file's own parse
// error, if any; a broken node has no instances to hold.
type nodeGroup struct {
	name string
	// key is what this entry is called in the inventory — a node ID, or an
	// unmanaged user's key. It is not always the name shown: an unmanaged
	// user's row is drawn as a device called inventory.DefaultCredential under
	// a group of their own, so that a person's devices sit at one level
	// whether or not this inventory has a file for them.
	key string
	// user is set when this entry is an unmanaged user rather than a node.
	// They hold instances the same way and group the same way, but nothing
	// else about them is a node's: there is no node file, no networks, and
	// no node detail to show. See
	// docs/apps/conf/inventory.md#managed-and-unmanaged-devices.
	user bool
	// group is the directory this node's file sits in under nodes/: whose
	// machines these are. owner is the person a device belongs to, which is
	// the group when the group names a user. Both are empty for an
	// unmanaged user's entry, which has no node file.
	group     string
	owner     string
	broken    string
	expanded  bool
	instances []*instanceNode
}

// groupRow is one level above a node: the directory its file sits in. It is
// not a thing anyone writes twice — nodes/doug/phone.yaml says both that the
// phone is doug's and that it is grouped with the rest of doug's devices.
type groupRow struct {
	name     string
	isUser   bool
	expanded bool
	nodes    []*nodeGroup
}

// userTree arranges what is rendered for people: one row per person, holding
// the devices and credentials their files hang off. It is the other half of
// groupTree — that one groups machines by whose they are, this one groups
// what a person receives — and the two are separate groupings rather than one
// tree filtered twice, because a provider is not a person and has nothing to
// receive.
func userTree(nodes []*nodeGroup, inv *inventory.Root, previous []*groupRow) []*groupRow {
	was := map[string]bool{}
	for _, g := range previous {
		was[g.name] = g.expanded
	}
	var out []*groupRow
	index := map[string]*groupRow{}
	for _, n := range nodes {
		owner := n.owner
		if n.user {
			owner = n.key
		}
		if owner == "" {
			continue // a machine nobody owns receives nothing.
		}
		g, ok := index[owner]
		if !ok {
			expanded := true
			if e, seen := was[owner]; seen {
				expanded = e
			}
			g = &groupRow{name: owner, isUser: true, expanded: expanded}
			index[owner] = g
			out = append(out, g)
		}
		g.nodes = append(g.nodes, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	for _, g := range out {
		sort.Slice(g.nodes, func(i, j int) bool { return g.nodes[i].name < g.nodes[j].name })
	}
	return out
}

// groupTree arranges nodes under their group, in the order the groups first
// appear. Nodes with no group — a file directly in nodes/, and an unmanaged
// user's entry — are collected under an empty group drawn without a header.
func groupTree(nodes []*nodeGroup, isUser func(string) bool, previous []*groupRow) []*groupRow {
	was := map[string]bool{}
	for _, g := range previous {
		was[g.name] = g.expanded
	}
	var out []*groupRow
	index := map[string]*groupRow{}
	for _, n := range nodes {
		g, ok := index[n.group]
		if !ok {
			expanded := true
			if e, seen := was[n.group]; seen {
				expanded = e
			}
			g = &groupRow{name: n.group, isUser: isUser(n.group), expanded: expanded}
			index[n.group] = g
			out = append(out, g)
		}
		g.nodes = append(g.nodes, n)
	}
	return out
}

// buildTree turns target.List's result into the tree the page walks: nodes
// (and unmanaged users, who have none) at the top level, their instances
// under them.
func buildTree(targets []target.Target) []*nodeGroup {
	var nodes []*nodeGroup
	for _, g := range target.GroupByNode(targets) {
		n := &nodeGroup{name: g.Node, key: g.Node, expanded: true}
		for _, t := range g.Targets {
			// GroupByNode keys an unmanaged user's targets by the user,
			// since they have no node; a target with no node is how the
			// group says which of the two it is.
			if t.Node == "" && t.User != "" {
				// An unmanaged user has no node file, so the level a
				// device would occupy is filled by one named `default` —
				// the same way a directory with no page of its own still
				// answers at its index.
				n.user = true
				n.key = t.User
				n.group = t.User
				n.name = inventory.DefaultCredential
			}
			if t.Instance == "" {
				// The one synthetic target a broken node file contributes —
				// see target.List.
				n.broken = t.Broken
				continue
			}
			// A deployment says the program it runs; a file written for a
			// person says the way it was written, which is what tells two
			// of them for one route apart.
			detail := t.Service
			if t.Export != "" {
				detail = t.Export
			}
			n.instances = append(n.instances, &instanceNode{
				name:   t.Instance,
				detail: detail,
				broken: t.Broken,
			})
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// foldArrow is the disclosure marker for a node row: "▾" open, "▸" closed, or
// a space when it holds nothing to fold.
func foldArrow(expanded, foldable bool) string {
	if !foldable {
		return " "
	}
	if expanded {
		return "▾"
	}
	return "▸"
}

func plural(count int, noun string) string { return text.Plural(count, noun) }
