// Package derive computes everything docs/apps/conf/inventory.md says is
// derived, never written by hand: client instances, edges between hops with
// their resolved addresses, grants and per-port principal tables. It is a
// pure function of an already-parsed inventory and the service manifests
// its routes' entry hops name — it touches no filesystem itself.
//
// The rules are in docs/apps/conf/inventory.md#what-is-derived and
// docs/apps/conf/inventory.md#networks-and-how-an-address-is-chosen.
package derive

import (
	"fmt"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/inventory"
)

// Hop is one parsed element of a route's hops list: "<instance>:<port>".
type Hop struct {
	Instance string
	Port     string
}

// ParseHop splits "<instance>:<port>" into its two parts.
func ParseHop(s string) (Hop, error) {
	instance, port, ok := strings.Cut(s, ":")
	if !ok || instance == "" || port == "" {
		return Hop{}, fmt.Errorf("hop %q: want <instance>:<port>", s)
	}
	return Hop{Instance: instance, Port: port}, nil
}

// PrincipalKind is what kind of thing a Principal is, per
// docs/apps/conf/inventory.md#the-model.
type PrincipalKind string

const (
	PrincipalNode     PrincipalKind = "node"
	PrincipalUser     PrincipalKind = "user"
	PrincipalInstance PrincipalKind = "instance"
)

// Principal is whatever holds a grant: a managed device (its node), an
// unmanaged user, or an upstream instance relaying for a non-terminal hop.
type Principal struct {
	Kind PrincipalKind
	// ID identifies the principal for deduplication: a node ID, a user's map
	// key, or an instance ID.
	ID string
	// Name is the account name a server-side render sees, per
	// docs/apps/conf/inventory.md#managed-and-unmanaged-devices.
	Name string
}

// Grant is one credential a principal needs for one port of one instance.
type Grant struct {
	Principal Principal
	Instance  string
	Port      string
}

// ClientInstance is one derived client instance: a managed user's device, or
// an unmanaged user, reaching a route's entry hop.
type ClientInstance struct {
	// ID is "<node>-<route>" for a managed device, "<username>-<route>" for
	// an unmanaged user.
	ID string
	// Node is the owning node's ID, empty for an unmanaged user (who has no
	// node file). User is the owning unmanaged user's map key, empty for a
	// managed device. Exactly one of the two is set.
	Node    string
	User    string
	Service string
	// Role is the entry hop role's ReachedBy, or the node's client_role when
	// it names one — never the entry hop's own role.
	Role  string
	Route string
	// Ports, Bind and Values come from an authored override with the same
	// ID, on the same node, if one exists — a derived instance has none of
	// them by default. See docs/apps/conf/inventory.md#what-is-derived.
	Ports  map[string]int
	Bind   string
	Values map[string]any
}

// Edge is one resolved hop-to-hop connection: either between two adjacent
// hops of a route, or from a derived client instance to the route's entry
// hop.
type Edge struct {
	Route string
	// From is the empty Hop for the edge out of a derived client instance;
	// FromInstance names it instead.
	From         Hop
	FromInstance string
	To           Hop
	// Address is the address the From side dials, per
	// docs/apps/conf/inventory.md#networks-and-how-an-address-is-chosen.
	Address string
	// Port is the numeric port the To hop's instance names for To.Port.
	Port int
}

// Model is everything Derive computes.
type Model struct {
	ClientInstances []ClientInstance
	Edges           []Edge
	Grants          []Grant
}

// Principals returns the Grants' principals for one instance's port, sorted
// by Name and deduplicated by (Kind, ID) — the account table one port
// renders.
func (m *Model) Principals(instance, port string) []Principal {
	seen := map[[2]string]bool{}
	var out []Principal
	for _, g := range m.Grants {
		if g.Instance != instance || g.Port != port {
			continue
		}
		key := [2]string{string(g.Principal.Kind), g.Principal.ID}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, g.Principal)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type instanceRef struct {
	node inventory.Node
	inst inventory.Instance
}

// Derive computes client instances, edges and grants from an inventory
// already loaded by inventory.Load, and the service manifests its routes'
// entry hops name, keyed by service name. Only non-broken nodes are
// considered; a broken node's instances are invisible to Derive, the same
// way a broken service is invisible to confgen's own callers.
func Derive(inv *inventory.Root, manifests map[string]confgen.Manifest) (*Model, error) {
	instances := map[string]instanceRef{}
	nodes := map[string]inventory.Node{}
	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		nodes[n.ID] = n
		for _, inst := range n.Instances {
			if _, dup := instances[inst.ID]; dup {
				return nil, fmt.Errorf("derive: instance %q is defined more than once", inst.ID)
			}
			instances[inst.ID] = instanceRef{node: n, inst: inst}
		}
	}

	m := &Model{}

	userKeys := make([]string, 0, len(inv.Users))
	for key := range inv.Users {
		userKeys = append(userKeys, key)
	}
	sort.Strings(userKeys)

	for _, key := range userKeys {
		user := inv.Users[key]
		routeNames := append([]string(nil), user.Access...)
		sort.Strings(routeNames)
		for _, routeName := range routeNames {
			route, ok := inv.Routes[routeName]
			if !ok || len(route.Hops) == 0 {
				continue
			}
			entryHop, err := ParseHop(route.Hops[0])
			if err != nil {
				return nil, fmt.Errorf("derive: route %q: %w", routeName, err)
			}
			entry, ok := instances[entryHop.Instance]
			if !ok {
				continue
			}
			manifest, ok := manifests[entry.inst.Service]
			if !ok {
				continue
			}
			entryRole, ok := manifest.Roles[entry.inst.Role]
			if !ok {
				continue
			}
			// A role with no ReachedBy derives no client instance — MicroBin
			// is reached from a browser — but the grant and its secret still
			// exist; only the rendered file does not.
			reachedBy := entryRole.ReachedBy

			username := user.UsernameOr(key)

			if user.Devices == inventory.DevicesUnmanaged {
				principal := Principal{Kind: PrincipalUser, ID: key, Name: username}
				m.Grants = append(m.Grants, Grant{Principal: principal, Instance: entryHop.Instance, Port: entryHop.Port})
				// An unmanaged user has no node, so its own client_role
				// substitutes for the node's, in the same precedence over
				// reached_by.
				role := reachedBy
				if user.ClientRole != "" {
					if user.ClientRole == inventory.ClientRoleNone {
						role = ""
					} else {
						role = user.ClientRole
					}
				}
				if role == "" {
					continue
				}
				m.ClientInstances = append(m.ClientInstances, ClientInstance{
					ID:      username + "-" + routeName,
					User:    key,
					Service: entry.inst.Service,
					Role:    role,
					Route:   routeName,
				})
				// An unmanaged user has no node, but is documented as
				// reachable only on the universal network — resolve as if
				// dialing from a node that reaches nothing else.
				address, err := resolveAddress(inventory.Node{}, entry.node, inv.Networks, inv.Universal)
				if err != nil {
					return nil, fmt.Errorf("derive: route %q for %s: %w", routeName, key, err)
				}
				m.Edges = append(m.Edges, Edge{
					Route:        routeName,
					FromInstance: username + "-" + routeName,
					To:           entryHop,
					Address:      address,
					Port:         entry.inst.Ports[entryHop.Port],
				})
				continue
			}

			var ownedNodeIDs []string
			for id, n := range nodes {
				if n.Owner == key {
					ownedNodeIDs = append(ownedNodeIDs, id)
				}
			}
			sort.Strings(ownedNodeIDs)
			for _, nodeID := range ownedNodeIDs {
				n := nodes[nodeID]
				principal := Principal{Kind: PrincipalNode, ID: nodeID, Name: username + "-" + nodeID}
				m.Grants = append(m.Grants, Grant{Principal: principal, Instance: entryHop.Instance, Port: entryHop.Port})

				// docs/apps/conf/inventory.md#which-role-a-client-derives-as:
				// the node's client_role wins over the role's own
				// reached_by; client_role: none derives nothing regardless
				// of reached_by.
				role := reachedBy
				if n.ClientRole != "" {
					if n.ClientRole == inventory.ClientRoleNone {
						role = ""
					} else {
						role = n.ClientRole
					}
				}
				if role == "" {
					continue
				}
				derivedID := nodeID + "-" + routeName
				ci := ClientInstance{ID: derivedID, Node: nodeID, Service: entry.inst.Service, Role: role, Route: routeName}
				for _, override := range n.Instances {
					if override.ID == derivedID {
						ci.Ports, ci.Bind, ci.Values = override.Ports, override.Bind, override.Values
						break
					}
				}
				m.ClientInstances = append(m.ClientInstances, ci)

				address, err := resolveAddress(n, entry.node, inv.Networks, inv.Universal)
				if err != nil {
					return nil, fmt.Errorf("derive: route %q for %s: %w", routeName, nodeID, err)
				}
				m.Edges = append(m.Edges, Edge{
					Route:        routeName,
					FromInstance: nodeID + "-" + routeName,
					To:           entryHop,
					Address:      address,
					Port:         entry.inst.Ports[entryHop.Port],
				})
			}
		}
	}

	routeNames := make([]string, 0, len(inv.Routes))
	for name := range inv.Routes {
		routeNames = append(routeNames, name)
	}
	sort.Strings(routeNames)

	for _, routeName := range routeNames {
		route := inv.Routes[routeName]
		hops := make([]Hop, 0, len(route.Hops))
		for _, raw := range route.Hops {
			hop, err := ParseHop(raw)
			if err != nil {
				return nil, fmt.Errorf("derive: route %q: %w", routeName, err)
			}
			hops = append(hops, hop)
		}

		for i := 0; i+1 < len(hops); i++ {
			from, ok := instances[hops[i].Instance]
			if !ok {
				return nil, fmt.Errorf("derive: route %q: hop %q: no such instance", routeName, hops[i].Instance)
			}
			to, ok := instances[hops[i+1].Instance]
			if !ok {
				return nil, fmt.Errorf("derive: route %q: hop %q: no such instance", routeName, hops[i+1].Instance)
			}
			port, ok := to.inst.Ports[hops[i+1].Port]
			if !ok {
				return nil, fmt.Errorf("derive: route %q: %s has no port %q", routeName, hops[i+1].Instance, hops[i+1].Port)
			}
			address, err := resolveAddress(from.node, to.node, inv.Networks, inv.Universal)
			if err != nil {
				return nil, fmt.Errorf("derive: route %q: %w", routeName, err)
			}

			m.Edges = append(m.Edges, Edge{Route: routeName, From: hops[i], To: hops[i+1], Address: address, Port: port})
			m.Grants = append(m.Grants, Grant{
				Principal: Principal{Kind: PrincipalInstance, ID: hops[i].Instance, Name: hops[i].Instance},
				Instance:  hops[i+1].Instance,
				Port:      hops[i+1].Port,
			})
		}
	}

	return m, nil
}

// resolveAddress is docs/apps/conf/inventory.md's address rule: the same
// node dials loopback; otherwise the downstream's address is used on the
// first network, in networkPref's preference order, that the downstream has
// an address on and the upstream can reach. The rule is one-directional —
// only the downstream needs to be reachable — so a node behind NAT with no
// address anywhere can still dial out. universal is networks.yaml's
// `universal` key: the one network every node reaches without saying so, or
// empty if none is.
func resolveAddress(from, to inventory.Node, networkPref []string, universal string) (string, error) {
	if from.ID == to.ID {
		return "127.0.0.1", nil
	}

	pref := networkPref
	if universal != "" && !containsString(pref, universal) {
		pref = append(append([]string{}, pref...), universal)
	}

	for _, network := range pref {
		addr, hasAddr := to.Networks[network]
		if !hasAddr {
			continue
		}
		if !reaches(from, network, universal) {
			continue
		}
		return addr, nil
	}
	return "", fmt.Errorf("%s and %s share no reachable network (%s reaches %s, %s reaches %s)",
		from.ID, to.ID, from.ID, strings.Join(reachSet(from, pref, universal), ", "), to.ID, strings.Join(reachSet(to, pref, universal), ", "))
}

// reaches reports whether node can open a connection on network: it has an
// address there, it is named in Reaches, or network is universal, which
// every node reaches implicitly.
func reaches(node inventory.Node, network, universal string) bool {
	if universal != "" && network == universal {
		return true
	}
	if _, ok := node.Networks[network]; ok {
		return true
	}
	return containsString(node.Reaches, network)
}

func reachSet(node inventory.Node, networks []string, universal string) []string {
	var out []string
	for _, n := range networks {
		if reaches(node, n, universal) {
			out = append(out, n)
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
