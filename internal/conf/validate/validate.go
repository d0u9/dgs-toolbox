// Package validate checks an inventory and its derivation against the rules
// in docs/apps/conf/inventory.md#validation. Each rule is one function,
// contributing zero or more Issues; Validate runs all of them and returns
// everything found, rather than stopping at the first problem.
//
// It is a pure function of already-loaded, already-derived values — the
// same inventory.Root, confgen.Manifest set and derive.Model the rest of
// dgs conf works with — and touches no filesystem itself.
//
// Rule 15 (the secrets tree matching what the inventory implies, both
// directions) needs secretstore.Sync and is not here; doc's own words are
// "neither is fixed here". Rule 16 needs only a `.previous` file's
// modification time, which Validate takes as a plain value — the one
// concession to touching a filesystem, still made by the caller, not here.
package validate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

// Issue is one validation failure, naming what it read and where.
type Issue struct {
	Message string
}

func (i Issue) Error() string { return i.Message }

// instRef is one authored instance, with the node it was found on.
type instRef struct {
	nodeID string
	inst   inventory.Instance
}

// isOverride reports whether inst is a client override rather than a full
// instance definition: docs/apps/conf/inventory.md says "service and role
// may not be written in an override", so an instance naming neither is one.
func isOverride(inst inventory.Instance) bool {
	return inst.Service == "" && inst.Role == ""
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Validate runs every rule in docs/apps/conf/inventory.md#validation but
// rule 15, against an inventory already loaded by inventory.Load, the
// service manifests it names, the derive.Model derive.Derive computed from
// both, and every .previous file's modification time — keyed by the path
// of the secret it is the previous value of, exactly what
// secretstore.PreviousModTimes returns — for rule 16. previous may be nil.
func Validate(inv *inventory.Root, manifests map[string]confgen.Manifest, model *derive.Model, previous map[string]time.Time) []Issue {
	var issues []Issue
	add := func(format string, args ...any) {
		issues = append(issues, Issue{fmt.Sprintf(format, args...)})
	}

	nodeByID := map[string]inventory.Node{}
	var nodeIDsInOrder []string
	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		if _, dup := nodeByID[n.ID]; dup {
			add("node %q is defined more than once", n.ID)
		} else {
			nodeIDsInOrder = append(nodeIDsInOrder, n.ID)
		}
		nodeByID[n.ID] = n
	}
	sort.Strings(nodeIDsInOrder)

	var userKeys []string
	for k := range inv.Users {
		userKeys = append(userKeys, k)
	}
	sort.Strings(userKeys)

	// realInstances is every authored, non-override instance: the ones a hop
	// can name and a service manifest describes.
	realInstances := map[string]instRef{}
	var realDupes []string
	overridesByNode := map[string][]inventory.Instance{}
	portByInstance := map[string]map[string]int{}
	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if isOverride(inst) {
				overridesByNode[n.ID] = append(overridesByNode[n.ID], inst)
				continue
			}
			if _, dup := realInstances[inst.ID]; dup {
				realDupes = append(realDupes, inst.ID)
			}
			realInstances[inst.ID] = instRef{nodeID: n.ID, inst: inst}
			portByInstance[inst.ID] = inst.Ports
		}
	}
	sort.Strings(realDupes)
	for _, id := range realDupes {
		add("instance %q is defined more than once", id)
	}

	// Rule 1: instance identifiers unique, derived ones included, and
	// universal, if written, names a network in the list.
	if inv.Universal != "" && !containsString(inv.Networks, inv.Universal) {
		add("networks.yaml: universal %q does not name a network in the list", inv.Universal)
	}

	// A derived ID matching an authored override on the same node is the
	// intended pin, not a collision; matching a real instance, or another
	// derived instance, is.
	derivedByID := map[string][]derive.ClientInstance{}
	for _, ci := range model.ClientInstances {
		derivedByID[ci.ID] = append(derivedByID[ci.ID], ci)
	}
	var derivedIDs []string
	for id := range derivedByID {
		derivedIDs = append(derivedIDs, id)
	}
	sort.Strings(derivedIDs)
	for _, id := range derivedIDs {
		cis := derivedByID[id]
		if len(cis) > 1 {
			routes := make([]string, len(cis))
			for i, ci := range cis {
				routes[i] = ci.Route
			}
			add("instance %q is derived more than once, by routes %s", id, strings.Join(routes, ", "))
		}
		if r, ok := realInstances[id]; ok {
			add("instance %q is both a real instance (on node %q) and a derived client instance", id, r.nodeID)
		}
	}

	// Rule 2: instance.service names a service; instance.role names one of
	// its roles; a reached_by names another role of the same service; a
	// combine_own names one of that role's own secrets.
	var realIDs []string
	for id := range realInstances {
		realIDs = append(realIDs, id)
	}
	sort.Strings(realIDs)
	for _, id := range realIDs {
		r := realInstances[id]
		manifest, ok := manifests[r.inst.Service]
		if !ok {
			add("instance %q: service %q is not defined", id, r.inst.Service)
			continue
		}
		role, ok := manifest.Roles[r.inst.Role]
		if !ok {
			add("instance %q: service %q has no role %q", id, r.inst.Service, r.inst.Role)
			continue
		}
		if role.ReachedBy != "" {
			if _, ok := manifest.Roles[role.ReachedBy]; !ok {
				add("service %q role %q: reached_by %q is not a role of this service", r.inst.Service, r.inst.Role, role.ReachedBy)
			}
		}
		if role.CombineOwn != "" {
			listed := false
			for _, name := range role.Own {
				if name == role.CombineOwn {
					listed = true
					break
				}
			}
			if !listed {
				// The own list is the one place a role's own secrets are
				// named, so a combine_own outside it names a file sync
				// neither generates nor reports.
				add("service %q role %q: combine_own %q is not in this role's own list %v", r.inst.Service, r.inst.Role, role.CombineOwn, role.Own)
			}
		}
	}

	// Rule 3: a client_role, on a node or on an unmanaged user, names a role
	// held by every service its granted routes derive a client for, or is
	// "none".
	servicesByNode := map[string]map[string]bool{}
	servicesByUser := map[string]map[string]bool{}
	for _, ci := range model.ClientInstances {
		switch {
		case ci.Node != "":
			if servicesByNode[ci.Node] == nil {
				servicesByNode[ci.Node] = map[string]bool{}
			}
			servicesByNode[ci.Node][ci.Service] = true
		case ci.User != "":
			if servicesByUser[ci.User] == nil {
				servicesByUser[ci.User] = map[string]bool{}
			}
			servicesByUser[ci.User][ci.Service] = true
		}
	}
	checkClientRole := func(subject, clientRole string, services map[string]bool) {
		if clientRole == "" || clientRole == inventory.ClientRoleNone {
			return
		}
		var names []string
		for s := range services {
			names = append(names, s)
		}
		sort.Strings(names)
		for _, service := range names {
			manifest := manifests[service] // rule 2 already reports a missing service.
			if _, ok := manifest.Roles[clientRole]; !ok {
				var roles []string
				for r := range manifest.Roles {
					roles = append(roles, r)
				}
				sort.Strings(roles)
				add("%s: client_role %q is not a role of %q, which has %s", subject, clientRole, service, strings.Join(roles, ", "))
			}
		}
	}
	for _, nodeID := range nodeIDsInOrder {
		n := nodeByID[nodeID]
		checkClientRole(fmt.Sprintf("node %q", nodeID), n.ClientRole, servicesByNode[nodeID])
	}
	for _, key := range userKeys {
		u := inv.Users[key]
		if u.Devices != inventory.DevicesUnmanaged {
			continue
		}
		checkClientRole(fmt.Sprintf("user %q", key), u.ClientRole, servicesByUser[key])
	}

	// Rule 4 and 9: every hop names an existing instance and existing port
	// on it; no route names one instance twice.
	var routeNames []string
	for name := range inv.Routes {
		routeNames = append(routeNames, name)
	}
	sort.Strings(routeNames)
	for _, routeName := range routeNames {
		route := inv.Routes[routeName]
		seen := map[string]bool{}
		for _, raw := range route.Hops {
			hop, err := derive.ParseHop(raw)
			if err != nil {
				add("route %q: %s", routeName, err)
				continue
			}
			if seen[hop.Instance] {
				add("route %q names instance %q twice", routeName, hop.Instance)
			}
			seen[hop.Instance] = true

			ports, ok := portByInstance[hop.Instance]
			if !ok {
				add("route %q: hop %q names an instance that does not exist", routeName, raw)
				continue
			}
			if _, ok := ports[hop.Port]; !ok {
				add("route %q: hop %q: instance %q has no port %q", routeName, raw, hop.Instance, hop.Port)
			}
		}
	}

	// Rule 5: no port named "own".
	for _, id := range realIDs {
		if _, ok := realInstances[id].inst.Ports["own"]; ok {
			add("instance %q: port \"own\" is reserved", id)
		}
	}

	// Rule 6: every route in an access list exists; every owner exists.
	for _, key := range userKeys {
		for _, routeName := range inv.Users[key].Access {
			if _, ok := inv.Routes[routeName]; !ok {
				add("user %q: access names route %q, which does not exist", key, routeName)
			}
		}
	}
	for _, nodeID := range nodeIDsInOrder {
		owner := nodeByID[nodeID].Owner
		if owner == "" {
			continue
		}
		if _, ok := inv.Users[owner]; !ok {
			add("node %q: owner %q is not a user", nodeID, owner)
		}
	}

	// Rule 7: an override's identifier matches an instance the owner's
	// access derives for that node, and it sets neither service nor role.
	for _, nodeID := range nodeIDsInOrder {
		for _, inst := range overridesByNode[nodeID] {
			matched := false
			for _, ci := range derivedByID[inst.ID] {
				if ci.Node == nodeID {
					matched = true
					break
				}
			}
			if !matched {
				add("node %q: instance %q overrides nothing this node derives (check the route name)", nodeID, inst.ID)
			}
		}
	}
	// An instance on an owned node that sets service or role is not a valid
	// override either — it is neither a real instance (those only exist on
	// unowned, service-hosting nodes by convention) nor a bare override.
	for _, nodeID := range nodeIDsInOrder {
		n := nodeByID[nodeID]
		if n.Owner == "" {
			continue
		}
		for _, inst := range n.Instances {
			if !isOverride(inst) {
				add("node %q: instance %q sets service or role; an override may only set ports and bind", nodeID, inst.ID)
			}
		}
	}

	// Rule 8: a non-terminal hop has the same successor in every route
	// through it.
	type successor struct {
		next, route string
	}
	successors := map[string]successor{}
	for _, routeName := range routeNames {
		hops := inv.Routes[routeName].Hops
		for i := 0; i+1 < len(hops); i++ {
			cur, err := derive.ParseHop(hops[i])
			if err != nil {
				continue // rule 4 already reported this hop.
			}
			if prev, ok := successors[cur.Instance]; ok {
				if prev.next != hops[i+1] {
					add("instance %q has different successors in routes %q and %q — rule-based routing is not supported",
						cur.Instance, prev.route, routeName)
				}
				continue
			}
			successors[cur.Instance] = successor{next: hops[i+1], route: routeName}
		}
	}

	// Rule 12: an unmanaged user's granted routes enter on the universal
	// network. With no universal network declared, there is nothing an
	// unmanaged user could ever reach, which is rule 3 or the author's
	// problem, not this check's to invent an answer for.
	if inv.Universal != "" {
		for _, key := range userKeys {
			user := inv.Users[key]
			if user.Devices != inventory.DevicesUnmanaged {
				continue
			}
			for _, routeName := range user.Access {
				route, ok := inv.Routes[routeName]
				if !ok || len(route.Hops) == 0 {
					continue // rule 6 already reported the missing route.
				}
				hop, err := derive.ParseHop(route.Hops[0])
				if err != nil {
					continue
				}
				r, ok := realInstances[hop.Instance]
				if !ok {
					continue // rule 4 already reported the missing instance.
				}
				node := nodeByID[r.nodeID]
				if _, ok := node.Networks[inv.Universal]; !ok {
					add("user %q (unmanaged): route %q enters %q, which has no address on the universal network %q", key, routeName, hop.Instance, inv.Universal)
				}
			}
		}
	}

	// Rule 13: account names rendered for one port are distinct.
	type portKey struct{ instance, port string }
	seenPorts := map[portKey]bool{}
	for _, g := range model.Grants {
		seenPorts[portKey{g.Instance, g.Port}] = true
	}
	var portKeys []portKey
	for k := range seenPorts {
		portKeys = append(portKeys, k)
	}
	sort.Slice(portKeys, func(i, j int) bool {
		if portKeys[i].instance != portKeys[j].instance {
			return portKeys[i].instance < portKeys[j].instance
		}
		return portKeys[i].port < portKeys[j].port
	})
	for _, k := range portKeys {
		byName := map[string]int{}
		for _, p := range model.Principals(k.instance, k.port) {
			byName[p.Name]++
		}
		var names []string
		for n := range byName {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			if byName[n] > 1 {
				add("%s, port %s: account %q is rendered by more than one principal", k.instance, k.port, n)
			}
		}
	}

	// Rule 14: two instances on one node do not bind the same address and
	// port. Protocol is not modelled yet, so this checks address and port
	// only — a stricter check than the rule asks for, never a looser one.
	type bound struct {
		bind string
		port int
	}
	for _, nodeID := range nodeIDsInOrder {
		seen := map[bound][]string{}
		for _, inst := range nodeByID[nodeID].Instances {
			if isOverride(inst) {
				continue // a client's own local listeners, not reachable from outside.
			}
			bind := inst.Bind
			for portName, portNum := range inst.Ports {
				b := bound{bind: bind, port: portNum}
				seen[b] = append(seen[b], fmt.Sprintf("%s:%s", inst.ID, portName))
			}
		}
		var bounds []bound
		for b := range seen {
			bounds = append(bounds, b)
		}
		sort.Slice(bounds, func(i, j int) bool {
			if bounds[i].bind != bounds[j].bind {
				return bounds[i].bind < bounds[j].bind
			}
			return bounds[i].port < bounds[j].port
		})
		for _, b := range bounds {
			names := seen[b]
			if len(names) > 1 {
				sort.Strings(names)
				add("node %q: %s bind the same address and port (%s:%d)", nodeID, strings.Join(names, ", "), b.bind, b.port)
			}
		}
	}

	// Rule 16: no .previous file older than seven days. previous is keyed
	// by the secret path it is the previous value of, per
	// secretstore.PreviousModTimes.
	var previousPaths []string
	for path := range previous {
		previousPaths = append(previousPaths, path)
	}
	sort.Strings(previousPaths)
	for _, path := range previousPaths {
		age := time.Since(previous[path])
		if age > sevenDays {
			add("%s.previous: %s old, over the seven-day limit — finish the rotation or delete it", path, age.Round(time.Hour))
		}
	}

	return issues
}

// sevenDays is rule 16's staleness limit for a .previous file.
const sevenDays = 7 * 24 * time.Hour
