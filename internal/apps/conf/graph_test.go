package conf

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"dgs-toolbox/internal/webgraph"
)

// TestBuildGraph_ThePortIsTheUnit covers the choice the picture is built
// around. A port is what holds principals, what a grant is written against
// and what a secret belongs to, so a server listening on one is drawn as
// that port and not as the instance around it. An instance that only dials
// out has no port of its own and is drawn whole.
func TestBuildGraph_ThePortIsTheUnit(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	if m.loadErr != nil {
		t.Fatalf("load: %v", m.loadErr)
	}
	g := buildGraph(m.l, "test")

	kinds := map[string]string{}
	for _, n := range g.Nodes {
		kinds[n.ID] = n.Kind
	}
	if kinds["srv/ss-srv:main"] != kindPort {
		t.Fatalf("srv/ss-srv:main = %q, want %q — a listening port is the unit", kinds["srv/ss-srv:main"], kindPort)
	}
	if _, drawn := kinds["srv/ss-srv"]; drawn {
		t.Fatal("ss-srv is drawn as a shape of its own; its ports are the shapes")
	}
	if kinds["yak-default-sfo-ssserver-ss-json"] != kindClient {
		t.Fatalf("yak-default-sfo-ssserver-ss-json = %q, want %q — it listens on nothing", kinds["yak-default-sfo-ssserver-ss-json"], kindClient)
	}
}

// TestBuildGraph_PortsSitInAProcessInsideItsNode covers the levels of
// containment: a process box holds the ports one running program serves, a
// node box holds the processes on one machine, and a group box holds whose
// machines those are. An unmanaged user has no node file, so the device box
// is one named for the slot they hold instead — the picture keeps one shape
// for everyone rather than two.
func TestBuildGraph_PortsSitInAProcessInsideItsNode(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	g := buildGraph(m.l, "test")

	parent := map[string]string{}
	for _, grp := range g.Groups {
		parent[grp.ID] = grp.Parent
	}
	if _, ok := parent["srv"]; !ok {
		t.Fatalf("groups = %+v, want one per node", g.Groups)
	}
	if parent["srv/ss-srv"] != "srv" {
		t.Fatalf("process ss-srv sits in %q, want the node it runs on", parent["srv/ss-srv"])
	}
	if parent["yak/default"] != groupBox("yak") {
		t.Fatalf("yak's default device sits in %q, want yak's own box", parent["yak/default"])
	}

	for _, n := range g.Nodes {
		switch n.ID {
		case "srv/ss-srv:main":
			if n.Group != "srv/ss-srv" {
				t.Fatalf("%s is in %q, want the process serving it", n.ID, n.Group)
			}
		case "yak-default-sfo-ssserver-ss-json":
			if n.Group != "yak/default" {
				t.Fatalf("%s is in %q, want the device box standing in for a node file", n.ID, n.Group)
			}
			if n.Label != "sfo" {
				t.Fatalf("%s is labelled %q, want the route it was derived for — its boxes already name whose it is", n.ID, n.Label)
			}
		}
	}
}

// TestBuildGraph_NoSecretValueReachesThePage covers the rule the graph
// shares with every other view: a tooltip names what a credential is for,
// never what it is.
func TestBuildGraph_NoSecretValueReachesThePage(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), buildSecretsDir(t))
	g := buildGraph(m.l, "test")

	for _, n := range g.Nodes {
		if n.ID != "srv/ss-srv:main" {
			continue
		}
		if !strings.Contains(n.Tooltip, "38250") {
			t.Fatalf("tooltip = %q, want the port it listens on", n.Tooltip)
		}
		if !strings.Contains(n.Tooltip, "yak") {
			t.Fatalf("tooltip = %q, want who holds a grant on it", n.Tooltip)
		}
		// buildSecretsDir writes these values; none may appear.
		for _, secret := range []string{"in-step", "rotating", "orphan"} {
			if strings.Contains(n.Tooltip, secret) {
				t.Fatalf("tooltip = %q leaked a credential", n.Tooltip)
			}
		}
	}
}

// TestBuildGraph_EdgesLandOnAPortAndNameWhatCrosses covers what an edge is
// for. The port it lands on says which of a server's ports was reached, so
// the line itself carries the one thing left: the principal crossing it.
func TestBuildGraph_EdgesLandOnAPortAndNameWhatCrosses(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	g := buildGraph(m.l, "test")

	if len(g.Edges) == 0 {
		t.Fatal("no edges")
	}
	for _, e := range g.Edges {
		if !strings.Contains(e.To, ":") {
			t.Fatalf("edge %s -> %s does not land on a port", e.From, e.To)
		}
		if e.Label == "" {
			t.Fatalf("edge %s -> %s does not say what crosses it", e.From, e.To)
		}
		if e.Kind == "" {
			t.Fatalf("edge %s -> %s has no kind, so it cannot be styled or keyed", e.From, e.To)
		}
	}
}

// A reverse proxy's lines all leave one entrance, so the thing worth reading
// on one is the name that route arrived at — the proxy holds no credential,
// and its own instance name on every line says nothing the shapes at either
// end do not.
func TestBuildGraph_FanOutEdgesCarryThePublishedName(t *testing.T) {
	root, _ := buildFanOutRoot(t)
	m := newInspectModel(root, "")
	if m.loadErr != nil {
		t.Fatalf("load: %v", m.loadErr)
	}
	g := buildGraph(m.l, "test")

	got := map[string]string{}
	for _, e := range g.Edges {
		got[e.To] = e.Label
	}
	for to, want := range map[string]string{
		"srv/vault:web": "vault.example.com",
		"srv/bin:web":   "clip.example.com",
	} {
		if got[to] != want {
			t.Fatalf("edge to %s = %q, want %q", to, got[to], want)
		}
	}
}

// An ordinary edge still carries what crosses it. A published name on the
// port it lands on does not change that: the principal is the thing on the
// line, and only a fan-out has siblings to be told apart from.
func TestBuildGraph_OrdinaryEdgesStillCarryThePrincipal(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	if m.loadErr != nil {
		t.Fatalf("load: %v", m.loadErr)
	}
	for _, e := range buildGraph(m.l, "test").Edges {
		if strings.Contains(e.Label, ".") && strings.Contains(e.Label, "example") {
			t.Fatalf("edge %s -> %s carries %q, want a principal", e.From, e.To, e.Label)
		}
	}
}

// TestBuildGraph_AContainerisedProcessIsBadged: the badge sits on the
// process box, not on the ports inside it. A container boundary and one
// running program are the same line, and repeating it per port would state
// one fact several times. What it tells a reader is how to read the bind
// under it — see docs/apps/conf/inspect.md#the-connectivity-graph.
func TestBuildGraph_AContainerisedProcessIsBadged(t *testing.T) {
	root := buildInspectRoot(t)
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: ssserver
    runtime: docker
    ports:
      main: {port: 38250, self: [psk.main]}
      alt: {port: 49217, self: [psk.alt]}
`)
	m := newInspectModel(root, "")
	g := buildGraph(m.l, "test")

	found := false
	for _, grp := range g.Groups {
		if grp.ID != "srv/ss-srv" {
			continue
		}
		found = true
		if grp.Detail != "docker" {
			t.Fatalf("process ss-srv is badged %q, want %q", grp.Detail, "docker")
		}
	}
	if !found {
		t.Fatalf("groups = %+v, want the process box", g.Groups)
	}
	for _, n := range g.Nodes {
		if strings.Contains(n.Detail, "docker") {
			t.Errorf("%s carries the badge, which belongs to the process box", n.ID)
		}
	}
}

// TestBuildGraph_AHostProcessIsNotBadged is the default, and the reason the
// badge means anything: most processes carry none.
func TestBuildGraph_AHostProcessIsNotBadged(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	g := buildGraph(m.l, "test")
	for _, grp := range g.Groups {
		if grp.ID == "srv/ss-srv" && grp.Detail != "" {
			t.Fatalf("process ss-srv is badged %q, want nothing for a host process", grp.Detail)
		}
	}
}

// TestBuildGraph_ContainerNetworksAndHosts: a process sits in the box of the
// first container network it joins and names the others with their fixed
// addresses; every line and host is tagged with the network it runs over,
// and each network is one filter.
func TestBuildGraph_ContainerNetworksAndHosts(t *testing.T) {
	root := buildInspectRoot(t)
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
runtime: docker
containers:
  - {name: apps, subnet: 172.30.0.0/24}
  - {name: tailnet, subnet: 172.30.250.0/24}
instances:
  - id: ss-srv
    service: ssserver
    containers:
      apps:
      tailnet: 172.30.250.10
    ports:
      main: {port: 38250, self: [psk.main]}
      alt: {port: 49217, self: [psk.alt]}
`)
	writeFile(t, filepath.Join(root, "hosts.yaml"), `
hosts:
  nas:
    network: internet
    address: 203.0.113.50
    names: [nas.example.test]
`)
	m := newInspectModel(root, "")
	g := buildGraph(m.l, "test")

	groups := map[string]webgraph.Group{}
	for _, grp := range g.Groups {
		groups[grp.ID] = grp
	}
	proc, ok := groups["srv/ss-srv"]
	if !ok || proc.Parent != "srv/container:apps" || proc.Detail != "docker · tailnet 172.30.250.10" {
		t.Errorf("process box = %+v, want it inside apps naming tailnet 172.30.250.10", proc)
	}
	if box, ok := groups["srv/container:apps"]; !ok || box.Parent != "srv" || box.Detail != "172.30.0.0/24" {
		t.Errorf("apps box = %+v, want it inside srv with its subnet", box)
	}
	if _, ok := groups["srv/container:tailnet"]; !ok {
		t.Error("tailnet has no box, but ss-srv joins it")
	}
	joins := false
	for _, e := range g.Edges {
		if e.Kind == kindJoins && e.From == "srv/ss-srv" && e.To == "srv/container:tailnet" && e.Label == "172.30.250.10" {
			joins = true
		}
	}
	if !joins {
		t.Errorf("edges = %+v, want ss-srv joining tailnet at 172.30.250.10", g.Edges)
	}

	var filters []string
	for _, f := range g.Filters {
		filters = append(filters, f.ID)
	}
	for _, want := range []string{"net:internet", "container:apps", "container:tailnet", "loopback"} {
		if !slices.Contains(filters, want) {
			t.Errorf("filters = %v, want %s", filters, want)
		}
	}
	for _, e := range g.Edges {
		if len(e.Tags) != 1 || !slices.Contains(filters, e.Tags[0]) {
			t.Errorf("edge %s -> %s tags %v, want one of the filters", e.From, e.To, e.Tags)
		}
	}
	var host *webgraph.Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == "hosts:nas" {
			host = &g.Nodes[i]
		}
	}
	if host == nil || host.Detail != "internet 203.0.113.50" || !slices.Equal(host.Tags, []string{"net:internet"}) {
		t.Errorf("host = %+v, want nas on internet", host)
	}
}
