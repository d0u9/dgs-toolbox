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
	if want := (Mapping{Address: "127.0.0.1", Number: 8080}); got["web"] != want {
		t.Fatalf("bin-sfo01 web = %+v, want %+v", got["web"], want)
	}
}

// TestMappings_PortEnteredFromAnotherNodePublishesOnThisNodesAddress pins
// the second case, with the address the edge resolved on.
func TestMappings_PortEnteredFromAnotherNodePublishesOnThisNodesAddress(t *testing.T) {
	got := mappingsOf(t, mappingInventory("203.0.113.10"), "ss-sfo01")
	if want := (Mapping{Address: "203.0.113.10", Number: 38250}); got["main"] != want {
		t.Fatalf("ss-sfo01 main = %+v, want %+v", got["main"], want)
	}
}

// TestMappings_ANameIsNotAnAddressToBind is the same case with a node
// written as a DNS name: a container runtime binds addresses, and a node
// naming itself sfo1.example.net has not given it one.
func TestMappings_ANameIsNotAnAddressToBind(t *testing.T) {
	got := mappingsOf(t, mappingInventory("sfo1.example.net"), "ss-sfo01")
	if want := (Mapping{Address: "0.0.0.0", Number: 38250}); got["main"] != want {
		t.Fatalf("ss-sfo01 main = %+v, want %+v", got["main"], want)
	}
}

// TestMappings_PortNoEdgeEntersPublishesLikeOneEnteredFromElsewhere is the
// third case: a port reached from a browser. Nothing in the inventory says
// where it is reached from, so it is published the way anything reached from
// outside is.
func TestMappings_PortNoEdgeEntersPublishesLikeOneEnteredFromElsewhere(t *testing.T) {
	got := mappingsOf(t, mappingInventory("203.0.113.10"), "caddy-sfo01")
	if want := (Mapping{Address: "203.0.113.10", Number: 443}); got["https"] != want {
		t.Fatalf("caddy-sfo01 https = %+v, want %+v", got["https"], want)
	}
	got = mappingsOf(t, mappingInventory("sfo1.example.net"), "caddy-sfo01")
	if want := (Mapping{Address: "0.0.0.0", Number: 443}); got["https"] != want {
		t.Fatalf("caddy-sfo01 https = %+v, want %+v", got["https"], want)
	}
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
	if want := (Mapping{Address: "0.0.0.0", Number: 443}); got["https"] != want {
		t.Fatalf("caddy-sfo01 https = %+v, want %+v", got["https"], want)
	}
}

// TestMappings_UnknownInstanceHasNone keeps a caller from having to check
// first: an instance this inventory does not hold maps nothing.
func TestMappings_UnknownInstanceHasNone(t *testing.T) {
	if got := mappingsOf(t, mappingInventory("203.0.113.10"), "nonesuch"); len(got) != 0 {
		t.Fatalf("Mappings(nonesuch) = %v, want none", got)
	}
}
