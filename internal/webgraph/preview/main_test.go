package preview

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"dgs-toolbox/internal/webgraph"
)

// TestPreview serves a small graph for looking at the page by hand. It runs
// only when DGS_PREVIEW is set, so the suite stays quiet.
func TestPreview(t *testing.T) {
	if os.Getenv("DGS_PREVIEW") == "" {
		t.Skip("set DGS_PREVIEW to serve the page")
	}
	load := func() (webgraph.Graph, error) { return previewGraph(), nil }
	if err := http.ListenAndServe("127.0.0.1:8791", webgraph.Handler(load)); err != nil {
		t.Fatal(err)
	}
}

func previewGraph() webgraph.Graph {
	g := webgraph.Graph{
		Title: "Synthetic connectivity",
		Groups: []webgraph.Group{
			{ID: "owner-a", Label: "alice"}, {ID: "owner-b", Label: "bob"},
			{ID: "host-a", Label: "host-a", Parent: "owner-a", Collapse: true},
			{ID: "host-b", Label: "host-b", Parent: "owner-a", Collapse: true},
			{ID: "host-c", Label: "host-c", Parent: "owner-b", Collapse: true},
			{ID: "declarations", Label: "Declarations"},
			{ID: "apps", Label: "apps", Parent: "host-a", Region: true, ColorFilter: "apps"},
			{ID: "private", Label: "private", Parent: "host-a", Region: true, ColorFilter: "private"},
			{ID: "edge", Label: "edge", Parent: "host-a", Region: true, ColorFilter: "edge"},
		},
		Filters: []webgraph.Filter{{ID: "lan", Label: "LAN", Detail: "192.0.2.0/24"}, {ID: "wan", Label: "WAN", Detail: "203.0.113.0/24"}, {ID: "apps", Label: "apps"}, {ID: "private", Label: "private"}, {ID: "edge", Label: "edge"}},
		Nodes:   []webgraph.Node{{ID: "declared", Label: "host-d", Group: "declarations", ContextOnly: true, Detail: "192.0.2.20"}},
		Edges: []webgraph.Edge{
			{From: "proxy", To: "private", Kind: "joins", Membership: true, Tags: []string{"private"}},
			{From: "proxy", To: "edge", Kind: "joins", Membership: true, Tags: []string{"edge"}},
			{From: "proxy", To: "web/main", Label: "www.example.test", Kind: "hop", Tags: []string{"lan"}},
			{From: "proxy", To: "web/alt", Label: "api.example.test", Kind: "hop", Tags: []string{"wan"}},
			{From: "web", To: "proxy/main", Label: "callback", Kind: "hop", Tags: []string{"lan"}},
			{From: "web", To: "database/main", Label: "query", Kind: "local", Tags: []string{"lan"}},
			{From: "worker", To: "database/main", Label: "jobs", Kind: "hop", Tags: []string{"lan"}},
			{From: "proxy", To: "gateway/main", Label: "route", Kind: "hop", Tags: []string{"wan"}},
			{From: "gateway", To: "web/main", Label: "entry", Kind: "hop", Tags: []string{"wan"}},
		},
	}
	for _, component := range []struct{ id, host string }{
		{"proxy", "host-a"}, {"gateway", "host-a"}, {"web", "host-b"},
		{"database", "host-b"}, {"worker", "host-c"}, {"metrics", "host-c"},
	} {
		parent := component.host
		if component.id == "proxy" {
			parent = "apps"
		}
		g.Groups = append(g.Groups, webgraph.Group{ID: component.id, Label: component.id, Parent: parent, Kind: "service", Summary: true})
		for _, port := range []string{"main", "alt"} {
			g.Nodes = append(g.Nodes, webgraph.Node{ID: component.id + "/" + port, Label: port, Group: component.id, Kind: "port", Detail: component.id})
		}
	}
	for i := 0; i < 16; i++ {
		id := fmt.Sprintf("client-%02d", i+1)
		g.Nodes = append(g.Nodes, webgraph.Node{ID: id, Label: id, Kind: "client", Group: "host-c"})
		g.Edges = append(g.Edges, webgraph.Edge{From: id, To: "proxy/main", Label: "access", Kind: "entry", Tags: []string{"lan"}})
	}
	return g
}

func TestPreviewServiceMemberships(t *testing.T) {
	g := previewGraph()
	regions := map[string]bool{}
	for _, group := range g.Groups {
		if group.Region {
			regions[group.ID] = true
		}
	}
	var memberships []string
	for _, group := range g.Groups {
		if group.ID == "proxy" {
			memberships = append(memberships, group.Parent)
		}
	}
	for _, edge := range g.Edges {
		if edge.From == "proxy" && edge.Membership {
			memberships = append(memberships, edge.To)
		}
	}
	if len(memberships) != 3 {
		t.Fatalf("preview memberships = %d, want 3", len(memberships))
	}
	for _, id := range memberships {
		if !regions[id] {
			t.Errorf("membership %q lacks a region", id)
		}
	}
}
