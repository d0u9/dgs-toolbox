// Package topology turns an inventory and its derivation into the
// connectivity graph docs/apps/conf/inspect.md#the-connectivity-graph
// describes: one container per node, one shape per instance, one edge per
// resolved connection. It reads no filesystem and knows nothing of the TUI
// or the web page that render it.
package topology

import (
	"sort"

	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

// Container is one node, holding every instance that runs on it. An
// unmanaged user's derived instances belong to no container; see Shape.
type Container struct {
	ID    string
	Owner string
}

// Shape is one instance, real or derived — one node of the graph.
type Shape struct {
	Instance string
	Service  string
	Role     string
	// Container is the node this shape runs on, or empty for an unmanaged
	// user's derived instance, which has none.
	Container string
	// Owner is the user this shape's principal identity belongs to: a
	// managed node's owner, or an unmanaged user's own key. Empty for an
	// instance nobody owns — an authored server.
	Owner string
}

// Edge is one resolved connection between two shapes.
type Edge struct {
	Route   string
	From    string
	To      string
	Address string
	Port    int
}

// Graph is the whole connectivity picture one inventory and its derivation
// produce.
type Graph struct {
	Containers []Container
	Shapes     []Shape
	Edges      []Edge
}

// Build computes Graph from inv and model. Broken nodes contribute no
// container and no shapes — the same way they contribute no target to
// target.List.
func Build(inv *inventory.Root, model *derive.Model) *Graph {
	g := &Graph{}

	owner := map[string]string{}
	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		owner[n.ID] = n.Owner
		g.Containers = append(g.Containers, Container{ID: n.ID, Owner: n.Owner})

		for _, inst := range n.Instances {
			if inst.Service == "" && inst.Role == "" {
				continue // an override, not a shape of its own — see target.List.
			}
			g.Shapes = append(g.Shapes, Shape{
				Instance:  inst.ID,
				Service:   inst.Service,
				Role:      inst.Role,
				Container: n.ID,
				Owner:     n.Owner,
			})
		}
	}

	for _, ci := range model.ClientInstances {
		shapeOwner := ci.User
		if shapeOwner == "" {
			shapeOwner = owner[ci.Node]
		}
		g.Shapes = append(g.Shapes, Shape{
			Instance:  ci.ID,
			Service:   ci.Service,
			Role:      ci.Role,
			Container: ci.Node,
			Owner:     shapeOwner,
		})
	}

	for _, e := range model.Edges {
		from := e.FromInstance
		if from == "" {
			from = e.From.Instance
		}
		g.Edges = append(g.Edges, Edge{
			Route:   e.Route,
			From:    from,
			To:      e.To.Instance,
			Address: e.Address,
			Port:    e.Port,
		})
	}

	sort.Slice(g.Containers, func(i, j int) bool { return g.Containers[i].ID < g.Containers[j].ID })
	sort.Slice(g.Shapes, func(i, j int) bool { return g.Shapes[i].Instance < g.Shapes[j].Instance })
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		return g.Edges[i].To < g.Edges[j].To
	})
	return g
}
