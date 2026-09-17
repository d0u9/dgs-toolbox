package conf

import (
	"fmt"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/tui/scrolllist"
)

// instanceNode is one instance under a role.
type instanceNode struct {
	name   string
	broken string
}

// roleNode is one role under a service, holding the instances found for it.
type roleNode struct {
	name      string
	expanded  bool
	instances []*instanceNode
}

// serviceNode is one service, holding its roles. broken is the manifest's
// parse error, if any; a broken service has no roles to hold.
type serviceNode struct {
	name     string
	broken   string
	expanded bool
	roles    []*roleNode
}

// buildTree turns a discovered root into the tree the page walks, sorted the
// way confgen.Load already sorted it.
func buildTree(root *confgen.Root) []*serviceNode {
	services := make([]*serviceNode, 0, len(root.Services))
	for _, svc := range root.Services {
		s := &serviceNode{name: svc.Name, broken: svc.Broken, expanded: true}
		for _, role := range svc.Roles {
			r := &roleNode{name: role.Name, expanded: true}
			for _, inst := range role.Instances {
				r.instances = append(r.instances, &instanceNode{name: inst.Name, broken: inst.Broken})
			}
			s.roles = append(s.roles, r)
		}
		services = append(services, s)
	}
	return services
}

// target is service/role/instance, as docs/apps/conf/export.md#targets-and-selectors
// writes it.
func target(service, role, instance string) string {
	return fmt.Sprintf("%s/%s/%s", service, role, instance)
}

// row is one visible line of the tree: a service, a role or an instance,
// with what Space would check under it.
type row struct {
	id       string
	label    string
	detail   string
	checkbox string // empty for a row that cannot be checked
	keys     []string

	service *serviceNode
	role    *roleNode
}

// checkableKeys is every checkable instance target under a service or a
// role. A broken instance is excluded: it cannot be checked, so it never
// contributes to a parent's tri-state box or to the checked count.
func checkableKeys(s *serviceNode) []string {
	var keys []string
	for _, r := range s.roles {
		keys = append(keys, roleKeys(s.name, r)...)
	}
	return keys
}

func roleKeys(serviceName string, r *roleNode) []string {
	var keys []string
	for _, inst := range r.instances {
		if inst.broken == "" {
			keys = append(keys, target(serviceName, r.name, inst.name))
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
	for _, s := range m.services {
		m.rows = append(m.rows, m.serviceRow(s))
		if s.broken != "" || !s.expanded {
			continue
		}
		for _, r := range s.roles {
			m.rows = append(m.rows, m.roleRow(s, r))
			if !r.expanded {
				continue
			}
			for _, inst := range r.instances {
				m.rows = append(m.rows, m.instanceRow(s, r, inst))
			}
		}
	}
	items := make([]scrolllist.Item, len(m.rows))
	for i, r := range m.rows {
		items[i] = scrolllist.Item{ID: r.id, Label: r.label, Detail: r.detail}
	}
	m.list.SetItems(items)
}

func (m Model) serviceRow(s *serviceNode) row {
	arrow := foldArrow(s.expanded, len(s.roles) > 0 && s.broken == "")
	if s.broken != "" {
		return row{
			id:      "svc:" + s.name,
			label:   fmt.Sprintf("%s %s", arrow, s.name),
			detail:  "    broken: " + s.broken,
			service: s,
		}
	}
	keys := checkableKeys(s)
	box := checkboxFor(keys, m.checked)
	return row{
		id:       "svc:" + s.name,
		label:    fmt.Sprintf("%s %s %s", arrow, box, s.name),
		detail:   "    " + plural(len(keys), "target"),
		checkbox: box,
		keys:     keys,
		service:  s,
	}
}

func (m Model) roleRow(s *serviceNode, r *roleNode) row {
	arrow := foldArrow(r.expanded, len(r.instances) > 0)
	keys := roleKeys(s.name, r)
	box := checkboxFor(keys, m.checked)
	return row{
		id:       "role:" + s.name + "/" + r.name,
		label:    fmt.Sprintf("    %s %s %s", arrow, box, r.name),
		detail:   "        " + plural(len(keys), "target"),
		checkbox: box,
		keys:     keys,
		role:     r,
	}
}

func (m Model) instanceRow(s *serviceNode, r *roleNode, inst *instanceNode) row {
	id := "inst:" + target(s.name, r.name, inst.name)
	if inst.broken != "" {
		return row{
			id:     id,
			label:  "        [!] " + inst.name,
			detail: "            broken: " + inst.broken,
		}
	}
	key := target(s.name, r.name, inst.name)
	box := "[ ]"
	if m.checked[key] {
		box = "[x]"
	}
	return row{
		id:       id,
		label:    "        " + box + " " + inst.name,
		checkbox: box,
		keys:     []string{key},
	}
}

// foldArrow is the disclosure marker for a service or role row: "▾" open,
// "▸" closed, or a space when it holds nothing to fold.
func foldArrow(expanded, foldable bool) string {
	if !foldable {
		return " "
	}
	if expanded {
		return "▾"
	}
	return "▸"
}

func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
