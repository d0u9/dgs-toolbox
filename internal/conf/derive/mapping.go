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
// runs on: the address a container runtime publishes it at, and the number,
// which is the port's own. There is no second number to choose from — the
// program's own configuration is rendered from the same field, so a mapping
// changing it would point at a listener that does not exist. See
// docs/apps/conf/export.md#a-second-file-what-deploys-it.
type Mapping struct {
	Address string
	Number  int
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
		entered, fromElsewhere := false, false
		network := ""
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
				continue
			}
			// The first network in preference order any edge from another
			// node resolved on, which is the one this node is reached at.
			if !fromElsewhere || preferredBefore(inv, e.Network, network) {
				network, fromElsewhere = e.Network, true
			}
		}

		address := PublishLoopback
		switch {
		case entered && !fromElsewhere:
		case fromElsewhere:
			address = bindable(node.Networks[network])
		default:
			// Nothing entered it, so no edge chose a network: the node's
			// own preference order picks one, as an edge into it would
			// have.
			address = bindable(firstAddress(inv, node))
		}
		out[name] = Mapping{Address: address, Number: port.Number}
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

// firstAddress is the node's address on the first network it has one on, in
// the inventory's preference order, or empty when it has none anywhere.
func firstAddress(inv *inventory.Root, node inventory.Node) string {
	for _, network := range networkOrder(inv) {
		if address, ok := node.Networks[network]; ok {
			return address
		}
	}
	return ""
}

// preferredBefore reports whether network a comes before b in the
// inventory's preference order. An unnamed network comes last, as the
// universal one appended to the order does.
func preferredBefore(inv *inventory.Root, a, b string) bool {
	order := networkOrder(inv)
	return indexOf(order, a) < indexOf(order, b)
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

// indexOf is a network's place in the preference order, or one past its end
// for a name that is not in it.
func indexOf(order []string, network string) int {
	for i, n := range order {
		if n == network {
			return i
		}
	}
	return len(order)
}
