package derive

import (
	"net"

	"dgs-toolbox/internal/conf/inventory"
)

// PublishEverywhere is the address a mapping binds when there is no single
// address to bind: the node is written at a name rather than an address, or
// it is written at nothing at all. A container runtime binds addresses, and
// a name is not one.
const PublishEverywhere = "0.0.0.0"

// PublishLoopback is the address a port entered only from its own node is
// published at. A reverse proxy's backend is that case, and it is the one
// the derivation exists for: published on every interface because someone
// typed it is what a hand-written mapping gets wrong.
const PublishLoopback = "127.0.0.1"

// Mapping is where an instance's port is reached on the machine the process
// runs on: the addresses a container runtime publishes it at, and the
// number, which is the port's own. There is no second number to choose
// from — the program's own configuration is rendered from the same field,
// so a mapping changing it would point at a listener that does not exist.
// See docs/apps/conf/export.md#a-second-file-what-deploys-it.
type Mapping struct {
	// Addresses is every address this port is published at, in the
	// inventory's network preference order, with loopback first when it is
	// among them. It is a list because one port may be reached over more
	// than one network: a machine with a port on each of two segments
	// serves both, and a port reached by a proxy on its own node and by
	// another machine needs loopback as well as the address that machine
	// dials. It is never empty.
	Addresses []string
	Number    int
}

// Mappings is the host mapping of each of one instance's ports, by port
// name, or nil when this inventory holds no such instance. It is derived
// from `ports` and from the edges already resolved, and never written: a
// port written twice is the second truth the deployment file exists to
// remove.
//
// The three cases are the ones docs/apps/conf/export.md pins. A port only
// hops from its own node enter publishes on PublishLoopback. A port entered
// from another node publishes on this node's own address, on the network
// that edge resolved. A port no edge enters — one reached from a browser —
// publishes the same way as the second: it is reached from outside, and
// nothing in the inventory says from where. An address that is a name, and
// a node written at no address at all, publish on PublishEverywhere.
//
// It answers for every instance, containerised or not. What runs the
// process decides whether a deployment file is rendered; where its ports
// are reached is the same question either way.
func (m *Model) Mappings(inv *inventory.Root, instance string) map[string]Mapping {
	var node inventory.Node
	var inst inventory.Instance
	found := false
	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, i := range n.Instances {
			if i.ID == instance {
				node, inst, found = n, i, true
			}
		}
	}
	if !found || len(inst.Ports) == 0 {
		return nil
	}

	// Which node each edge leaves from: the hop's own instance for an edge
	// between two hops, and the device for one out of a file written for a
	// person. A file a person carries has no node at all, and is another
	// machine by definition.
	nodeOf := map[string]string{}
	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, i := range n.Instances {
			nodeOf[i.ID] = n.ID
		}
	}
	for _, ci := range m.ExportInstances {
		nodeOf[ci.ID] = ci.Node
	}

	out := make(map[string]Mapping, len(inst.Ports))
	for name, port := range inst.Ports {
		// Which networks this port is entered over, and whether anything
		// on its own node enters it. The two are independent: a port a
		// local proxy dials and another machine dials is reached at
		// loopback and at this node's address, and publishing only one of
		// them leaves the other end dialling a number nothing published.
		local, entered := false, false
		networks := map[string]bool{}
		for _, e := range m.Edges {
			if e.To.Instance != instance || e.To.Port != name {
				continue
			}
			entered = true
			from := e.From.Instance
			if from == "" {
				from = e.FromInstance
			}
			if nodeOf[from] == node.ID {
				local = true
				continue
			}
			networks[e.Network] = true
		}

		var addresses []string
		if local {
			addresses = append(addresses, PublishLoopback)
		}
		switch {
		case len(networks) > 0:
			for _, network := range networkOrder(inv) {
				if networks[network] {
					addresses = append(addresses, bindable(node.Networks[network]))
				}
			}
		case !entered:
			// Nothing entered it, so no edge chose a network: it is
			// reached from outside and the inventory does not say from
			// where, which every network this node answers on satisfies.
			for _, network := range networkOrder(inv) {
				if address, ok := node.Networks[network]; ok {
					addresses = append(addresses, bindable(address))
				}
			}
			if len(addresses) == 0 {
				// A node written at no address at all, reached from
				// outside: there is no interface to name, and publishing
				// nothing would render a container nobody can reach.
				addresses = append(addresses, PublishEverywhere)
			}
		}
		out[name] = Mapping{Addresses: collapse(addresses), Number: port.Number}
	}
	return out
}

// collapse is the addresses a port actually binds: PublishEverywhere alone
// when it is among them, since it already covers every interface and a
// second bind on one of them would fail; the list deduplicated and in the
// order given otherwise; and PublishLoopback alone when there is nothing
// else, which is a port only its own node enters — and a port nothing
// enters on a node with no address anywhere, where there is no interface
// this derivation can name.
func collapse(addresses []string) []string {
	out := make([]string, 0, len(addresses))
	seen := map[string]bool{}
	for _, address := range addresses {
		if address == PublishEverywhere {
			return []string{PublishEverywhere}
		}
		if seen[address] {
			continue
		}
		seen[address] = true
		out = append(out, address)
	}
	if len(out) == 0 {
		return []string{PublishLoopback}
	}
	return out
}

// bindable is an address a container runtime can bind, or PublishEverywhere
// when what the node is written at is a name — or nothing.
func bindable(address string) string {
	if net.ParseIP(address) == nil {
		return PublishEverywhere
	}
	return address
}

// networkOrder is networks.yaml's order with the universal network after
// it, which is how resolveAddress reads it too.
func networkOrder(inv *inventory.Root) []string {
	order := inv.Networks
	if inv.Universal != "" && !containsString(order, inv.Universal) {
		order = append(append([]string{}, order...), inv.Universal)
	}
	return order
}
