package conf

import (
	"fmt"

	"dgs-toolbox/internal/conf/target"
	"dgs-toolbox/internal/tui/scrolllist"
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
	name      string
	broken    string
	expanded  bool
	instances []*instanceNode
}

// buildTree turns target.List's result into the tree the page walks: nodes
// (and unmanaged users, who have none) at the top level, their instances
// under them.
func buildTree(targets []target.Target) []*nodeGroup {
	var nodes []*nodeGroup
	for _, g := range target.GroupByNode(targets) {
		n := &nodeGroup{name: g.Node, expanded: true}
		for _, t := range g.Targets {
			if t.Instance == "" {
				// The one synthetic target a broken node file contributes —
				// see target.List.
				n.broken = t.Broken
				continue
			}
			n.instances = append(n.instances, &instanceNode{
				name:   t.Instance,
				detail: t.Service + " / " + t.Role,
				broken: t.Broken,
			})
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// row is one visible line of the tree: a node/user or an instance, with what
// Space would check under it.
type row struct {
	id       string
	label    string
	detail   string
	checkbox string // empty for a row that cannot be checked
	keys     []string

	node *nodeGroup
}

// checkableKeys is every checkable instance under a node. A broken instance
// is excluded: it cannot be checked, so it never contributes to the node's
// tri-state box or to the checked count.
func checkableKeys(n *nodeGroup) []string {
	var keys []string
	for _, inst := range n.instances {
		if inst.broken == "" {
			keys = append(keys, inst.name)
		}
	}
	return keys
}

// checkboxFor is "[x]", "[-]" or "[ ]" for all, some or none of keys checked.
// No keys at all is drawn as "[ ]": there is nothing to check, not everything.
func checkboxFor(keys []string, checked map[string]bool) string {
	if len(keys) == 0 {
		return "[ ]"
	}
	count := 0
	for _, k := range keys {
		if checked[k] {
			count++
		}
	}
	switch {
	case count == len(keys):
		return "[x]"
	case count > 0:
		return "[-]"
	}
	return "[ ]"
}

// refresh rebuilds rows from the tree's current expand and check state, and
// feeds them to the list.
func (m *Model) refresh() {
	m.rows = nil
	for _, n := range m.nodes {
		m.rows = append(m.rows, m.nodeRow(n))
		if n.broken != "" || !n.expanded {
			continue
		}
		for _, inst := range n.instances {
			m.rows = append(m.rows, m.instanceRow(n, inst))
		}
	}
	items := make([]scrolllist.Item, len(m.rows))
	for i, r := range m.rows {
		items[i] = scrolllist.Item{ID: r.id, Label: r.label, Detail: r.detail}
	}
	m.list.SetItems(items)
}

func (m Model) nodeRow(n *nodeGroup) row {
	arrow := foldArrow(n.expanded, len(n.instances) > 0 && n.broken == "")
	if n.broken != "" {
		return row{
			id:     "node:" + n.name,
			label:  fmt.Sprintf("%s %s", arrow, n.name),
			detail: "    broken: " + n.broken,
			node:   n,
		}
	}
	keys := checkableKeys(n)
	box := checkboxFor(keys, m.checked)
	return row{
		id:       "node:" + n.name,
		label:    fmt.Sprintf("%s %s %s", arrow, box, n.name),
		detail:   "    " + plural(len(keys), "target"),
		checkbox: box,
		keys:     keys,
		node:     n,
	}
}

func (m Model) instanceRow(n *nodeGroup, inst *instanceNode) row {
	id := "inst:" + n.name + "/" + inst.name
	if inst.broken != "" {
		return row{
			id:     id,
			label:  "    [!] " + inst.name,
			detail: "        broken: " + inst.broken,
		}
	}
	box := "[ ]"
	if m.checked[inst.name] {
		box = "[x]"
	}
	return row{
		id:       id,
		label:    "    " + box + " " + inst.name,
		detail:   "        " + inst.detail,
		checkbox: box,
		keys:     []string{inst.name},
	}
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
