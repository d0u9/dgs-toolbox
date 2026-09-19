package conf

import (
	"strings"
	"testing"
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
	if kinds["ss-srv:main"] != kindPort {
		t.Fatalf("ss-srv:main = %q, want %q — a listening port is the unit", kinds["ss-srv:main"], kindPort)
	}
	if _, drawn := kinds["ss-srv"]; drawn {
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
	if parent["ss-srv"] != "srv" {
		t.Fatalf("process ss-srv sits in %q, want the node it runs on", parent["ss-srv"])
	}
	if parent["yak/default"] != groupBox("yak") {
		t.Fatalf("yak's default device sits in %q, want yak's own box", parent["yak/default"])
	}

	for _, n := range g.Nodes {
		switch n.ID {
		case "ss-srv:main":
			if n.Group != "ss-srv" {
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
		if n.ID != "ss-srv:main" {
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
		"vault:web": "vault.example.com",
		"bin:web":   "clip.example.com",
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
