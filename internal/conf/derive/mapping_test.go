package derive

import (
	"testing"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/inventory"
)

// mappingInventory holds the three cases docs/apps/conf/export.md#a-second-file-what-deploys-it
// names, on one node written at the given address:
//
//   - bin-sfo01:web is entered only by the proxy on its own machine, and so
//     publishes on loopback.
//   - ss-sfo01:main is entered by a relay on another machine, and so
//     publishes on this node's own address — or on 0.0.0.0 when what this
//     node is written at is a name rather than an address.
//   - caddy-sfo01:https is entered by nothing at all — it is reached from a
//     browser — and publishes the way the second case does.
func mappingInventory(address string) *inventory.Root {
	return &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "sfo1",
				Networks: inventory.Networks{"internet": address},
				Instances: []inventory.Instance{
					{ID: "caddy-sfo01", Service: "caddy", Runtime: inventory.RuntimeDocker,
						Ports: inventory.PortsOf(map[string]int{"https": 443})},
					{ID: "bin-sfo01", Service: "microbin", Runtime: inventory.RuntimeDocker,
						Ports: inventory.PortsOf(map[string]int{"web": 8080})},
					{ID: "ss-sfo01", Service: "ssserver",
						Ports: inventory.PortsOf(map[string]int{"main": 38250})},
				},
			},
			{
				ID:       "hkg1",
				Networks: inventory.Networks{"internet": "203.0.113.20"},
				Instances: []inventory.Instance{
					{ID: "fwd-hkg01", Service: "realm", Ports: inventory.PortsOf(map[string]int{"ss": 38250})},
				},
			},
			{ID: "laptop", Owner: "doug"},
		},
		Users: map[string]inventory.User{
			"doug": {Username: "doug", Access: []string{"paste", "hkg-sfo"}},
		},
		Routes: map[string]inventory.Route{
			"paste":   {Hops: []string{"caddy-sfo01:https", "bin-sfo01:web"}},
			"hkg-sfo": {Hops: []string{"fwd-hkg01:ss", "ss-sfo01:main"}},
		},
		Networks:  []string{"internet"},
		Universal: "internet",
	}
}

// mappingManifests is workedManifests plus the two services this fixture
// runs that it does not: a reverse proxy that fans out, and a relay that
// terminates nothing.
func mappingManifests() map[string]confgen.Manifest {
	manifests := workedManifests()
	manifests["caddy"] = confgen.Manifest{Auth: confgen.AuthNone, Downstreams: confgen.DownstreamsMany, Template: "t"}
	manifests["realm"] = confgen.Manifest{Forwards: true, Template: "t"}
	return manifests
}

func mappingsOf(t *testing.T, inv *inventory.Root, instance string) map[string]Mapping {
	t.Helper()
	m, err := Derive(inv, mappingManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return m.Mappings(inv, instance)
}

// TestMappings_PortEnteredOnlyFromItsOwnNodePublishesOnLoopback is the case
// the derivation exists for: a backend behind a proxy on the same machine,
// published on every interface because someone typed it, is what writing the
// mapping by hand gets wrong.
func TestMappings_PortEnteredOnlyFromItsOwnNodePublishesOnLoopback(t *testing.T) {
	got := mappingsOf(t, mappingInventory("203.0.113.10"), "bin-sfo01")
	wantMapping(t, got, "web", 8080, "127.0.0.1")
}

// wantMapping is one port's whole mapping: the addresses it publishes on, in
// order, and the number, which is the port's own on both sides.
func wantMapping(t *testing.T, got map[string]Mapping, port string, number int, addresses ...string) {
	t.Helper()
	m, ok := got[port]
	if !ok {
		t.Fatalf("port %q has no mapping; got %+v", port, got)
	}
	if m.Number != number {
		t.Fatalf("port %q maps number %d, want %d", port, m.Number, number)
	}
	if len(m.Addresses) != len(addresses) {
		t.Fatalf("port %q publishes on %v, want %v", port, m.Addresses, addresses)
	}
	for i, want := range addresses {
		if m.Addresses[i] != want {
			t.Fatalf("port %q publishes on %v, want %v", port, m.Addresses, addresses)
		}
	}
}

// TestMappings_PortEnteredFromAnotherNodePublishesOnThisNodesAddress pins
// the second case, with the address the edge resolved on.
func TestMappings_PortEnteredFromAnotherNodePublishesOnThisNodesAddress(t *testing.T) {
	got := mappingsOf(t, mappingInventory("203.0.113.10"), "ss-sfo01")
	wantMapping(t, got, "main", 38250, "203.0.113.10")
}

// TestMappings_ANameIsNotAnAddressToBind is the same case with a node
// written as a DNS name: a container runtime binds addresses, and a node
// naming itself sfo1.example.net has not given it one.
func TestMappings_ANameIsNotAnAddressToBind(t *testing.T) {
	got := mappingsOf(t, mappingInventory("sfo1.example.net"), "ss-sfo01")
	wantMapping(t, got, "main", 38250, "0.0.0.0")
}

// TestMappings_PortNoEdgeEntersPublishesLikeOneEnteredFromElsewhere is the
// third case: a port reached from a browser. Nothing in the inventory says
// where it is reached from, so it is published the way anything reached from
// outside is.
func TestMappings_PortNoEdgeEntersPublishesLikeOneEnteredFromElsewhere(t *testing.T) {
	got := mappingsOf(t, mappingInventory("203.0.113.10"), "caddy-sfo01")
	wantMapping(t, got, "https", 443, "203.0.113.10")
	got = mappingsOf(t, mappingInventory("sfo1.example.net"), "caddy-sfo01")
	wantMapping(t, got, "https", 443, "0.0.0.0")
}

// TestMappings_ANodeWithNoAddressAtAllPublishesOnEveryInterface: there is
// nothing to bind but everything, and the alternative — publishing nothing —
// would render a container no one can reach.
func TestMappings_ANodeWithNoAddressAtAllPublishesOnEveryInterface(t *testing.T) {
	inv := mappingInventory("203.0.113.10")
	inv.Nodes[0].Networks = nil
	inv.Routes = map[string]inventory.Route{"paste": {Hops: []string{"caddy-sfo01:https", "bin-sfo01:web"}}}
	inv.Users = map[string]inventory.User{"doug": {Username: "doug", Access: []string{"paste"}}}
	got := mappingsOf(t, inv, "caddy-sfo01")
	wantMapping(t, got, "https", 443, "0.0.0.0")
}

// TestMappings_UnknownInstanceHasNone keeps a caller from having to check
// first: an instance this inventory does not hold maps nothing.
func TestMappings_UnknownInstanceHasNone(t *testing.T) {
	if got := mappingsOf(t, mappingInventory("203.0.113.10"), "nonesuch"); len(got) != 0 {
		t.Fatalf("Mappings(nonesuch) = %v, want none", got)
	}
}

// twoSegmentInventory is one machine on two networks, which is a second
// physical port and a second subnet. Nothing enters its DNS port — the
// clients that dial a resolver are not in this inventory — so it is the
// case that decides what a port reached from outside publishes on when the
// node is on more than one network.
func twoSegmentInventory() *inventory.Root {
	return &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "home01",
				Networks: inventory.Networks{"home": "10.10.10.10", "lab": "192.168.50.10"},
				Instances: []inventory.Instance{
					{ID: "adguard-home01", Service: "microbin", Runtime: inventory.RuntimeDocker,
						Ports: inventory.PortsOf(map[string]int{"dns": 53})},
				},
			},
		},
		Networks: []string{"home", "lab", "internet"},
	}
}

// TestMappings_ANodeOnTwoNetworksPublishesOnBoth: a machine with a port on
// each of two segments serves both, and one address would leave the second
// one with nothing listening. The order is the inventory's preference
// order, so a rendered file does not change because a network was added
// above another.
func TestMappings_ANodeOnTwoNetworksPublishesOnBoth(t *testing.T) {
	got := mappingsOf(t, twoSegmentInventory(), "adguard-home01")
	wantMapping(t, got, "dns", 53, "10.10.10.10", "192.168.50.10")
}

// TestMappings_ANameAmongAddressesTakesTheWholePort: 0.0.0.0 already covers
// every interface, so listing it beside a literal address would publish the
// same port twice and the second bind would fail.
func TestMappings_ANameAmongAddressesTakesTheWholePort(t *testing.T) {
	inv := twoSegmentInventory()
	inv.Nodes[0].Networks["lab"] = "home01.lab.example"
	got := mappingsOf(t, inv, "adguard-home01")
	wantMapping(t, got, "dns", 53, "0.0.0.0")
}

// TestMappings_EnteredFromBothItsOwnNodeAndAnotherPublishesOnBoth is the
// case one address gets wrong in the other direction. A same-node hop
// resolves to 127.0.0.1, so a port reached by a local proxy and by another
// machine needs loopback as well as the address that machine dials — with
// only the second, the proxy beside it dials a number nothing published.
func TestMappings_EnteredFromBothItsOwnNodeAndAnotherPublishesOnBoth(t *testing.T) {
	inv := mappingInventory("203.0.113.10")
	// A second route reaching the same port from the proxy on its own node,
	// beside the relay on hkg1 that already enters it.
	inv.Routes["local-ss"] = inventory.Route{Hops: []string{"caddy-sfo01:https", "ss-sfo01:main"}}
	inv.Users["doug"] = inventory.User{Username: "doug", Access: []string{"paste", "hkg-sfo", "local-ss"}}
	got := mappingsOf(t, inv, "ss-sfo01")
	wantMapping(t, got, "main", 38250, "127.0.0.1", "203.0.113.10")
}

// TestMappings_ARouteStartingAtAProxiedPortStillPublishesOnThisNodesAddress
// is a backend behind the proxy on its own node that a second route also
// enters directly — a web interface kept reachable without the proxy, as a
// way in when the proxy is down. The proxy's edge alone would publish it on
// loopback only, and the direct route would dial a number nothing published.
func TestMappings_ARouteStartingAtAProxiedPortStillPublishesOnThisNodesAddress(t *testing.T) {
	inv := mappingInventory("203.0.113.10")
	inv.Routes["paste-direct"] = inventory.Route{Hops: []string{"bin-sfo01:web"}}
	got := mappingsOf(t, inv, "bin-sfo01")
	wantMapping(t, got, "web", 8080, "127.0.0.1", "203.0.113.10")
}
