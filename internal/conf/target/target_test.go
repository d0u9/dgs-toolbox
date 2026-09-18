package target

import (
	"sort"
	"testing"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

// fixture is a small inventory: one server node, one managed client node,
// one unmanaged user, and a two-hop route so route: has something to pin
// beyond an entry hop.
func fixture(t *testing.T) (*inventory.Root, *derive.Model) {
	t.Helper()
	inv := &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "srv",
				Networks: inventory.Networks{"internet": "203.0.113.10"},
				Instances: []inventory.Instance{
					{ID: "ss-srv", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 38250}},
				},
			},
			{
				ID:       "relay",
				Networks: inventory.Networks{"internet": "203.0.113.20"},
				Instances: []inventory.Instance{
					{ID: "ss-relay", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 40000}},
				},
			},
			{ID: "laptop", Owner: "doug"},
			{ID: "broken.yaml", Broken: "yaml: line 3: did not find expected key"},
		},
		Users: map[string]inventory.User{
			"doug": {Access: []string{"sfo"}},
			"yak":  {Devices: inventory.DevicesUnmanaged, Access: []string{"sfo"}},
		},
		Routes: map[string]inventory.Route{
			"sfo":   {Hops: []string{"ss-srv:main"}},
			"chain": {Hops: []string{"ss-relay:main", "ss-srv:main"}},
		},
		Networks:  []string{"internet"},
		Universal: "internet",
	}
	manifests := map[string]confgen.Manifest{
		"shadowsocks-rust": {Roles: map[string]confgen.Role{
			"server":  {Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust"},
			"ss-rust": {Auth: confgen.AuthNone},
		}},
	}
	model, err := derive.Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return inv, model
}

func instanceNames(ts []Target) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Instance
	}
	sort.Strings(out)
	return out
}

func TestList_RealAndDerivedInstances(t *testing.T) {
	inv, model := fixture(t)
	targets := List(inv, model)

	var clean []Target
	for _, tg := range targets {
		if tg.Broken == "" {
			clean = append(clean, tg)
		}
	}
	got := instanceNames(clean)
	want := []string{"laptop-sfo", "ss-relay", "ss-srv", "yak-sfo"}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("List instances = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List instances = %v, want %v", got, want)
		}
	}
}

func TestList_BrokenNodeContributesOneTarget(t *testing.T) {
	inv, model := fixture(t)
	targets := List(inv, model)

	var found *Target
	for i := range targets {
		if targets[i].Node == "broken.yaml" {
			found = &targets[i]
		}
	}
	if found == nil {
		t.Fatal("no target for the broken node")
	}
	if found.Broken == "" {
		t.Fatal("Broken = empty, want the parse error")
	}
	if found.Instance != "" {
		t.Fatalf("Instance = %q, want empty for a broken node", found.Instance)
	}
}

func TestList_FieldsOnRealAndDerivedTargets(t *testing.T) {
	inv, model := fixture(t)
	targets := List(inv, model)

	byInstance := map[string]Target{}
	for _, tg := range targets {
		byInstance[tg.Instance] = tg
	}

	srv := byInstance["ss-srv"]
	if srv.Node != "srv" || srv.Service != "shadowsocks-rust" || srv.Role != "server" {
		t.Fatalf("ss-srv = %+v", srv)
	}
	if len(srv.Routes) != 2 { // both sfo (entry) and chain (a later hop).
		t.Fatalf("ss-srv.Routes = %v, want 2 routes", srv.Routes)
	}

	client := byInstance["laptop-sfo"]
	if client.Node != "laptop" || client.User != "doug" || client.Role != "ss-rust" {
		t.Fatalf("laptop-sfo = %+v", client)
	}
	if len(client.Routes) != 1 || client.Routes[0] != "sfo" {
		t.Fatalf("laptop-sfo.Routes = %v, want [sfo]", client.Routes)
	}

	unmanaged := byInstance["yak-sfo"]
	if unmanaged.Node != "" || unmanaged.User != "yak" {
		t.Fatalf("yak-sfo = %+v, want no node and User yak", unmanaged)
	}
}

func TestGroupByNode(t *testing.T) {
	inv, model := fixture(t)
	targets := List(inv, model)
	groups := GroupByNode(targets)

	byNode := map[string]int{}
	for _, g := range groups {
		byNode[g.Node] = len(g.Targets)
	}
	if byNode["srv"] != 1 || byNode["relay"] != 1 || byNode["laptop"] != 1 {
		t.Fatalf("byNode = %v", byNode)
	}
	// yak has no node, so its target groups under its user key.
	if byNode["yak"] != 1 {
		t.Fatalf("byNode[yak] = %d, want 1", byNode["yak"])
	}
}

func TestMatch_ByEachField(t *testing.T) {
	inv, model := fixture(t)
	targets := List(inv, model)

	cases := []struct {
		selector string
		want     []string
	}{
		{"node:srv", []string{"ss-srv"}},
		{"user:doug", []string{"laptop-sfo"}},
		{"service:shadowsocks-rust role:server", []string{"ss-relay", "ss-srv"}},
		{"route:chain", []string{"ss-relay", "ss-srv"}},
		{"instance:ss-srv", []string{"ss-srv"}},
		{"ss-srv", []string{"ss-srv"}}, // bare word is an instance term.
		{"node:*rv", []string{"ss-srv"}},
	}
	for _, c := range cases {
		matched, err := Match(c.selector, targets)
		if err != nil {
			t.Fatalf("Match(%q): %v", c.selector, err)
		}
		got := instanceNames(matched)
		want := append([]string(nil), c.want...)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("Match(%q) = %v, want %v", c.selector, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Match(%q) = %v, want %v", c.selector, got, want)
			}
		}
	}
}

func TestMatch_AllTermsMustMatch(t *testing.T) {
	inv, model := fixture(t)
	targets := List(inv, model)
	// node:srv holds only a shadowsocks-rust instance, so combined with an
	// unrelated service the AND of both terms matches nothing — were this an
	// OR, node:srv alone would still produce ss-srv.
	if _, err := Match("node:srv service:hysteria2", targets); err == nil {
		t.Fatal("Match: want an error, terms are ANDed and none of node:srv holds a hysteria2 instance")
	}
}

func TestMatch_NothingIsAnError(t *testing.T) {
	inv, model := fixture(t)
	targets := List(inv, model)
	if _, err := Match("instance:nonesuch", targets); err == nil {
		t.Fatal("Match: want an error for a selector matching nothing")
	}
}

// TestMatch_DocumentedExamples pins the selector examples export.md prints,
// against nodes and a route named as that page's worked example does — so a
// change to selector syntax that leaves the doc's prose but breaks its
// examples is caught here rather than by a reader trying them by hand.
func TestMatch_DocumentedExamples(t *testing.T) {
	inv := &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "us-sfo-dgo-linux-01",
				Networks: inventory.Networks{"internet": "203.0.113.10"},
				Instances: []inventory.Instance{
					{ID: "ss-sfo01", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 38250}},
					{ID: "hy2-sfo01", Service: "hysteria2", Role: "server", Ports: map[string]int{"main": 443}},
				},
			},
			{ID: "macbook", Owner: "doug"},
		},
		Users: map[string]inventory.User{
			"doug": {Access: []string{"jp", "sfo"}},
		},
		Routes: map[string]inventory.Route{
			"jp":  {Hops: []string{"hy2-sfo01:main"}},
			"sfo": {Hops: []string{"ss-sfo01:main"}},
		},
		Networks:  []string{"internet"},
		Universal: "internet",
	}
	manifests := map[string]confgen.Manifest{
		"shadowsocks-rust": {Roles: map[string]confgen.Role{
			"server":  {Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust"},
			"ss-rust": {Auth: confgen.AuthNone},
		}},
		"hysteria2": {Roles: map[string]confgen.Role{
			"server":   {Auth: confgen.AuthPerPrincipal, ReachedBy: "sing-box"},
			"sing-box": {Auth: confgen.AuthNone},
		}},
	}
	model, err := derive.Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	targets := List(inv, model)

	cases := []struct {
		selector string
		want     []string
	}{
		{"node:us-sfo-dgo-linux-01", []string{"ss-sfo01", "hy2-sfo01"}},
		{"node:macbook", []string{"macbook-jp", "macbook-sfo"}},
		{"user:doug", []string{"macbook-jp", "macbook-sfo"}},
		{"service:hysteria2 role:server", []string{"hy2-sfo01"}},
		{"route:jp", []string{"hy2-sfo01", "macbook-jp"}},
		{"instance:ss-sfo01", []string{"ss-sfo01"}},
		{"node:us-sfo-*", []string{"ss-sfo01", "hy2-sfo01"}},
	}
	for _, c := range cases {
		matched, err := Match(c.selector, targets)
		if err != nil {
			t.Fatalf("Match(%q): %v", c.selector, err)
		}
		got := instanceNames(matched)
		want := append([]string(nil), c.want...)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("Match(%q) = %v, want %v", c.selector, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Match(%q) = %v, want %v", c.selector, got, want)
			}
		}
	}
}

func TestMatch_UnknownFieldIsAnError(t *testing.T) {
	inv, model := fixture(t)
	targets := List(inv, model)
	if _, err := Match("bogus:x", targets); err == nil {
		t.Fatal("Match: want an error for an unknown field")
	}
}
