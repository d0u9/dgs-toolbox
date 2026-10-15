package conf

import (
	"testing"

	"github.com/d0u9/rhumb/engine"
	"github.com/d0u9/rhumb/inventory"
)

func TestUsableNodesSkipsMissingHost(t *testing.T) {
	l := engine.Loaded{Inv: &inventory.Root{
		Hosts: map[string]inventory.Host{"host-a": {}},
		Nodes: []inventory.Node{{ID: "node-1", Host: "host-a"}, {ID: "node-2", Host: "host-gone"}, {ID: "node-3"}, {ID: "node-4", Broken: "bad"}},
	}}
	var ids []string
	for _, n := range usableNodes(l) {
		ids = append(ids, n.ID)
	}
	if len(ids) != 2 || ids[0] != "node-1" || ids[1] != "node-3" {
		t.Fatalf("usableNodes = %v, want [node-1 node-3]", ids)
	}
}
