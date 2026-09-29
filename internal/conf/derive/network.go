package derive

import (
	"fmt"
	"net/netip"
	"sort"

	"dgs-toolbox/internal/conf/inventory"
)

// ResolveAddress is the address rule an edge follows, for a caller that is
// not an edge: a dial. See
// docs/apps/conf/inventory.md#dialling-a-service-that-is-not-on-a-route.
func ResolveAddress(inv *inventory.Root, from, to inventory.Node) (address, network string, err error) {
	return resolveAddress(from, to, inv.Networks, inv.Universal)
}

// Name is one entry of a network's name table: a name and the address that
// answers it there. Source says where it was written, for an error.
type Name struct {
	Name    string
	Address string
	Source  string
}

// Names is every network's name table, in name order, and the conflicts
// rule 30 reports: one name reaching two addresses on one network. See
// docs/apps/conf/inventory.md#names-on-a-network.
//
// fansOut says whether an instance is a proxy dispatching by name; only a
// route entered through one moves a name to the proxy's node, since a relay
// forwards bytes and never answers to a name. An address written as a
// hostname is left out: a resolver's record wants an IP address.
func Names(inv *inventory.Root, fansOut func(instance string) bool) (map[string][]Name, []string) {
	nodeOf := map[string]inventory.Node{}
	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.Service != "" {
				nodeOf[inst.ID] = n
			}
		}
	}
	// A port reached through a route answers at the node of the route's
	// first hop: the proxy in front of it.
	front := map[string]string{}
	routeNames := make([]string, 0, len(inv.Routes))
	for name := range inv.Routes {
		routeNames = append(routeNames, name)
	}
	sort.Strings(routeNames)
	for _, name := range routeNames {
		hops := inv.Routes[name].Hops
		for i := 1; i < len(hops); i++ {
			first, err := ParseHop(hops[0])
			if err != nil || !fansOut(first.Instance) {
				continue
			}
			if _, seen := front[hops[i]]; !seen {
				front[hops[i]] = first.Instance
			}
		}
	}

	byNetwork := map[string]map[string]Name{}
	var conflicts []string
	add := func(network string, n Name) {
		if _, err := netip.ParseAddr(n.Address); err != nil {
			return
		}
		table := byNetwork[network]
		if table == nil {
			table = map[string]Name{}
			byNetwork[network] = table
		}
		if prev, ok := table[n.Name]; ok {
			if prev.Address != n.Address {
				conflicts = append(conflicts, fmt.Sprintf("on network %q, name %q resolves to %s (%s) and %s (%s)",
					network, n.Name, prev.Address, prev.Source, n.Address, n.Source))
			}
			return
		}
		table[n.Name] = n
	}

	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.Service == "" {
				continue
			}
			for port, p := range inst.Ports {
				at := n
				if proxy, ok := front[inst.ID+":"+port]; ok {
					if pn, ok := nodeOf[proxy]; ok {
						at = pn
					}
				}
				for _, name := range p.Names {
					for network, addr := range at.Networks {
						add(network, Name{Name: name, Address: addr, Source: inst.ID + ":" + port})
					}
				}
			}
		}
	}
	for id, h := range inv.Hosts {
		for _, name := range h.Names {
			add(h.Network, Name{Name: name, Address: h.Address, Source: "host " + id})
		}
	}

	out := map[string][]Name{}
	for network, table := range byNetwork {
		list := make([]Name, 0, len(table))
		for _, n := range table {
			list = append(list, n)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
		out[network] = list
	}
	sort.Strings(conflicts)
	return out, conflicts
}

// Reservation is one line of what a network's router is given: a member
// with a hardware address, and the address it is to receive.
type Reservation struct {
	ID      string
	Address string
	MAC     string
}

// Reservations is every node and host on network with a mac, in address
// order. See docs/apps/conf/inventory.md#what-the-router-is-given.
func Reservations(inv *inventory.Root, network string) []Reservation {
	var out []Reservation
	for _, n := range inv.Nodes {
		if mac := n.MACs[network]; n.Broken == "" && mac != "" {
			out = append(out, Reservation{ID: n.ID, Address: n.Networks[network], MAC: mac})
		}
	}
	for id, h := range inv.Hosts {
		if h.Network == network && h.MAC != "" {
			out = append(out, Reservation{ID: id, Address: h.Address, MAC: h.MAC})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, errA := netip.ParseAddr(out[i].Address)
		b, errB := netip.ParseAddr(out[j].Address)
		if errA == nil && errB == nil && a != b {
			return a.Less(b)
		}
		return out[i].Address+out[i].ID < out[j].Address+out[j].ID
	})
	return out
}
