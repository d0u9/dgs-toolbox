package topology

import (
	"sort"
	"testing"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

// fixture is a small inventory: one server node with two instances, one
// managed client node whose device derives a client, and one unmanaged
// user's own derived instance — enough to exercise every shape a Container
// or a missing one can hold.
func fixture(t *testing.T) (*inventory.Root, *derive.Model) {
	t.Helper()
	inv := &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "srv",
				Networks: inventory.Networks{"internet": "203.0.113.10"},
				Instances: []inventory.Instance{
					{ID: "ss-srv", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 38250}},
					{ID: "bin-srv", Service: "microbin", Role: "server", Ports: map[string]int{"web": 8080}},
				},
			},
			{ID: "laptop", Owner: "doug"},
		},
		Users: map[string]inventory.User{
			"doug": {Access: []string{"sfo"}},
			"yak":  {Devices: inventory.DevicesUnmanaged, Access: []string{"sfo"}},
		},
		Routes:    map[string]inventory.Route{"sfo": {Hops: []string{"ss-srv:main"}}},
		Networks:  []string{"internet"},
		Universal: "internet",
	}
	manifests := map[string]confgen.Manifest{
		"shadowsocks-rust": {Roles: map[string]confgen.Role{
			"server":  {Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust"},
			"ss-rust": {Auth: confgen.AuthNone},
		}},
		"microbin": {Roles: map[string]confgen.Role{"server": {Auth: confgen.AuthShared}}},
	}
	model, err := derive.Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return inv, model
}

func TestBuild_ContainersAreNonBrokenNodes(t *testing.T) {
	inv, model := fixture(t)
	inv.Nodes = append(inv.Nodes, inventory.Node{ID: "broken.yaml", Broken: "bad yaml"})
	g := Build(inv, model)

	var ids []string
	for _, c := range g.Containers {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	want := []string{"laptop", "srv"}
	if len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("Containers = %v, want %v (the broken node contributes none)", ids, want)
	}
	for _, c := range g.Containers {
		if c.ID == "laptop" && c.Owner != "doug" {
			t.Fatalf("laptop container owner = %q, want doug", c.Owner)
		}
	}
}

func TestBuild_ShapesCoverRealAndDerivedInstances(t *testing.T) {
	inv, model := fixture(t)
	g := Build(inv, model)

	byID := map[string]Shape{}
	for _, s := range g.Shapes {
		byID[s.Instance] = s
	}

	srv, ok := byID["ss-srv"]
	if !ok || srv.Container != "srv" || srv.Owner != "" {
		t.Fatalf("ss-srv shape = %+v, want container srv and no owner", srv)
	}
	bin, ok := byID["bin-srv"]
	if !ok || bin.Container != "srv" {
		t.Fatalf("bin-srv shape = %+v", bin)
	}

	client, ok := byID["laptop-sfo"]
	if !ok || client.Container != "laptop" || client.Owner != "doug" || client.Role != "ss-rust" {
		t.Fatalf("laptop-sfo shape = %+v, want container laptop, owner doug, role ss-rust", client)
	}

	// yak is unmanaged: its derived instance belongs to no container, and is
	// owned by yak itself.
	unmanaged, ok := byID["yak-sfo"]
	if !ok {
		t.Fatal("yak-sfo shape not found")
	}
	if unmanaged.Container != "" {
		t.Fatalf("yak-sfo.Container = %q, want empty (an unmanaged user's shape has no container)", unmanaged.Container)
	}
	if unmanaged.Owner != "yak" {
		t.Fatalf("yak-sfo.Owner = %q, want yak", unmanaged.Owner)
	}
}

func TestBuild_EdgesFollowDeriveModel(t *testing.T) {
	inv, model := fixture(t)
	g := Build(inv, model)

	var toSrv []Edge
	for _, e := range g.Edges {
		if e.To == "ss-srv" {
			toSrv = append(toSrv, e)
		}
	}
	if len(toSrv) != 2 {
		t.Fatalf("edges into ss-srv = %+v, want 2 (laptop-sfo and yak-sfo)", toSrv)
	}
	froms := map[string]bool{}
	for _, e := range toSrv {
		froms[e.From] = true
		if e.Route != "sfo" {
			t.Fatalf("edge %+v has the wrong route", e)
		}
	}
	if !froms["laptop-sfo"] || !froms["yak-sfo"] {
		t.Fatalf("edges into ss-srv = %+v, want from laptop-sfo and yak-sfo", toSrv)
	}
}

func TestBuild_BrokenNodeContributesNoShapes(t *testing.T) {
	inv, model := fixture(t)
	inv.Nodes = append(inv.Nodes, inventory.Node{ID: "broken.yaml", Broken: "bad yaml", Instances: []inventory.Instance{
		{ID: "ghost", Service: "shadowsocks-rust", Role: "server"},
	}})
	g := Build(inv, model)
	for _, s := range g.Shapes {
		if s.Instance == "ghost" {
			t.Fatal("a broken node's instance produced a shape")
		}
	}
}
