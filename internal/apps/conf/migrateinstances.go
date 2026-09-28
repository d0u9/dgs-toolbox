package conf

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

type instanceChange struct{ From, To string }

func parseInstanceChanges(encoded string) ([]instanceChange, error) {
	if encoded == "" {
		return nil, nil
	}
	var specs []string
	if err := json.Unmarshal([]byte(encoded), &specs); err != nil {
		return nil, fmt.Errorf("--instance values: %w", err)
	}
	var changes []instanceChange
	fromSeen, toSeen := map[string]bool{}, map[string]bool{}
	for _, spec := range specs {
		fields := map[string]string{}
		for _, part := range strings.Split(spec, ",") {
			key, value, ok := strings.Cut(part, "=")
			if !ok || value == "" || (key != "from" && key != "to") || fields[key] != "" {
				return nil, fmt.Errorf("--instance %q: want from=<old>,to=<new>", spec)
			}
			fields[key] = value
		}
		if fields["from"] == "" || fields["to"] == "" || strings.ContainsAny(fields["to"], "/:\\") {
			return nil, fmt.Errorf("--instance %q: want from=<old>,to=<new> with a plain instance ID", spec)
		}
		if fromSeen[fields["from"]] || toSeen[fields["to"]] {
			return nil, fmt.Errorf("--instance %q: instance given more than once", spec)
		}
		fromSeen[fields["from"]], toSeen[fields["to"]] = true, true
		if fields["from"] != fields["to"] {
			changes = append(changes, instanceChange{fields["from"], fields["to"]})
		}
	}
	return changes, nil
}

// renameMigrationInstances changes authored IDs and typed route-hop references
// in the preview snapshot. Opaque values, paths and secrets remain manual work.
func renameMigrationInstances(inv *inventory.Root, nodeIndex int, changes []instanceChange) (map[string][]string, error) {
	refs := map[string][]string{}
	if len(changes) == 0 {
		return refs, nil
	}
	node := &inv.Nodes[nodeIndex]
	all := map[string]bool{}
	for _, n := range inv.Nodes {
		for _, inst := range n.Instances {
			all[inst.ID] = true
		}
	}
	changesByID := map[string]string{}
	for _, change := range changes {
		found := false
		for _, inst := range node.Instances {
			if inst.ID == change.From && inst.Service != "" {
				found = true
				refs[change.From] = append(refs[change.From], inst.Path+": id")
				if strings.Contains(filepath.Base(inst.Path), change.From) {
					refs[change.From] = append(refs[change.From], "manual review: filename "+inst.Path+" retains old instance ID")
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("node %q has no authored instance %q", node.ID, change.From)
		}
		if all[change.To] {
			return nil, fmt.Errorf("instance %q already exists", change.To)
		}
		changesByID[change.From] = change.To
	}
	node.Instances = append([]inventory.Instance(nil), node.Instances...)
	for i := range node.Instances {
		if to := changesByID[node.Instances[i].ID]; to != "" {
			node.Instances[i].ID = to
		}
	}
	inv.Routes = cloneRoutes(inv.Routes)
	for name, route := range inv.Routes {
		for i, raw := range route.Hops {
			hop, err := derive.ParseHop(raw)
			if err != nil {
				return nil, err
			}
			if to := changesByID[hop.Instance]; to != "" {
				route.Hops[i] = to + ":" + hop.Port
				refs[hop.Instance] = append(refs[hop.Instance], "routes.yaml: "+name+".hops")
			}
		}
		inv.Routes[name] = route
	}
	for from := range refs {
		sort.Strings(refs[from])
	}
	return refs, nil
}

func cloneRoutes(routes map[string]inventory.Route) map[string]inventory.Route {
	copyOf := make(map[string]inventory.Route, len(routes))
	for name, route := range routes {
		copyOf[name] = inventory.Route{Hops: append([]string(nil), route.Hops...)}
	}
	return copyOf
}
