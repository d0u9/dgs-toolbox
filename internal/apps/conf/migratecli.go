package conf

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/validate"
	"dgs-toolbox/internal/config"
)

// migrateAction is the read-only first milestone of node migration. It uses
// the same inventory, derivation and validation as conf inspect and check.
func migrateAction(in io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	return migrateActionSnapshot(in, out, args, flags, global, nil)
}

// migrateActionSnapshot lets the TUI inspect the same validated before/after
// inventory used by the report. Returning true stops before report rendering.
func migrateActionSnapshot(in io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config, snapshot func(before, after loaded) bool) error {
	if len(args) != 1 || args[0] != "node" {
		return fmt.Errorf("usage: dgs conf migrate node [--node from=<old>,to=<new> --network ... --instance ... --route ... --published ...]")
	}
	root := global.ConfRoot()
	if root == "" {
		return fmt.Errorf("conf.root is not configured")
	}
	if flags["node"] == "" {
		for _, name := range []string{"network", "instance", "route", "published"} {
			if flags[name] != "" && flags[name] != "[]" {
				return fmt.Errorf("--node is required when using --%s; omit all change flags to start the wizard", name)
			}
		}
		return migrateWizard(in, out, root, global)
	}
	oldID, newID, err := parseNodeChange(flags["node"])
	if err != nil {
		return err
	}
	networkChanges, err := parseNetworkChanges(flags["network"])
	if err != nil {
		return err
	}
	instanceChanges, err := parseInstanceChanges(flags["instance"])
	if err != nil {
		return err
	}
	routeChanges, err := parseRouteChanges(flags["route"])
	if err != nil {
		return err
	}
	publishedChanges, err := parsePublishedChanges(flags["published"])
	if err != nil {
		return err
	}
	if oldID == newID && len(networkChanges) == 0 && len(instanceChanges) == 0 && len(routeChanges) == 0 && len(publishedChanges) == 0 {
		return fmt.Errorf("give a new node ID, network name, or address")
	}

	l, err := load(root)
	if err != nil {
		return err
	}
	if broken := brokenFiles(l.inv); len(broken) != 0 {
		return fmt.Errorf("current inventory is broken:\n  %s", strings.Join(broken, "\n  "))
	}
	if issues := validate.Validate(l.inv, l.manifests, l.exports, l.derived, nil); len(issues) != 0 {
		return fmt.Errorf("current inventory has %d validation problems; run dgs conf --check first: %s", len(issues), issues[0].Message)
	}

	// Copy the containers that can change. No change to this preview can
	// reach the loaded snapshot, which is the source for the before/after diff.
	inv := *l.inv
	inv.Nodes = append([]inventory.Node(nil), l.inv.Nodes...)
	index := -1
	for i := range inv.Nodes {
		if inv.Nodes[i].ID == oldID {
			index = i
		}
		if newID != oldID && inv.Nodes[i].ID == newID {
			return fmt.Errorf("node %q already exists", newID)
		}
	}
	if index < 0 {
		return fmt.Errorf("node %q not found", oldID)
	}
	before := inv.Nodes[index]
	// Validate the whole set against the original names before any rename.
	// This rejects ambiguous chains and swaps such as A -> B, B -> C.
	for _, change := range networkChanges {
		if _, ok := before.Networks[change.From]; !ok {
			return fmt.Errorf("node %q has no address on network %q", oldID, change.From)
		}
		if change.From != change.To {
			for _, name := range l.inv.Networks {
				if name == change.To {
					return fmt.Errorf("network %q already exists", change.To)
				}
			}
		}
	}
	renamedRefs := map[string][]string{}
	for _, change := range networkChanges {
		if change.From != change.To {
			renamedRefs[change.From], err = renameInventoryNetwork(&inv, change.From, change.To)
			if err != nil {
				return err
			}
		}
	}
	// Always detach the target node's networks before changing its address.
	current := inv.Nodes[index].Networks
	inv.Nodes[index].Networks = make(inventory.Networks, len(current))
	for network, address := range current {
		inv.Nodes[index].Networks[network] = address
	}
	for _, change := range networkChanges {
		if change.Address == "" {
			continue
		}
		if _, ok := current[change.To]; !ok {
			return fmt.Errorf("node %q has no address on network %q", oldID, change.To)
		}
		inv.Nodes[index].Networks[change.To] = change.Address
	}
	inv.Nodes[index].ID = newID
	publishedRefs, err := renameMigrationPublished(&inv, index, publishedChanges)
	if err != nil {
		return err
	}
	instanceRefs, err := renameMigrationInstances(&inv, index, instanceChanges)
	if err != nil {
		return err
	}
	routeRefs, err := renameMigrationRoutes(&inv, routeChanges)
	if err != nil {
		return err
	}

	after, err := derive.Derive(&inv, l.manifests)
	if err != nil {
		return fmt.Errorf("target inventory cannot derive routes: %w", err)
	}
	if issues := validate.Validate(&inv, l.manifests, l.exports, after, nil); len(issues) != 0 {
		return fmt.Errorf("target inventory has %d validation problems: %s", len(issues), issues[0].Message)
	}
	newLoaded := l
	newLoaded.inv, newLoaded.derived = &inv, after
	if snapshot != nil && snapshot(l, newLoaded) {
		return nil
	}

	fmt.Fprintf(out, "Node migration preview: %s -> %s\nInventory: %s\n", oldID, newID, before.Path)
	if oldID != newID && strings.Contains(before.Path, oldID) {
		fmt.Fprintf(out, "  manual review: inventory filename retains old node ID: %s\n", before.Path)
	}
	for _, change := range networkChanges {
		if change.From != change.To {
			fmt.Fprintf(out, "Network: %s -> %s\n", change.From, change.To)
			for _, ref := range renamedRefs[change.From] {
				fmt.Fprintf(out, "  reference: %s\n", ref)
			}
		}
		if change.Address != "" {
			fmt.Fprintf(out, "Address: %s %s -> %s\n", change.To, before.Networks[change.From], change.Address)
		}
	}
	for _, change := range instanceChanges {
		fmt.Fprintf(out, "Instance: %s -> %s\n", change.From, change.To)
		for _, ref := range instanceRefs[change.From] {
			fmt.Fprintf(out, "  reference: %s\n", ref)
		}
	}
	for _, change := range routeChanges {
		fmt.Fprintf(out, "Route: %s -> %s\n", change.From, change.To)
		for _, ref := range routeRefs[change.From] {
			fmt.Fprintf(out, "  reference: %s\n", ref)
		}
		fmt.Fprintf(out, "  manual review: opaque instance values/deploy and service templates may use %q as a key\n", change.From)
	}
	for _, change := range publishedChanges {
		fmt.Fprintf(out, "Published name: %s:%s %s -> %s\n", change.Instance, change.Port, publishedRefs[change.key()], change.To)
		fmt.Fprintln(out, "  DNS review: check authoritative records and affected clients; this preview does not edit DNS")
	}

	// The edge key denotes the same route connection across a node rename.
	// A derived client ID starts with its node ID, so normalize that prefix.
	oldEdges := map[string]derive.Edge{}
	for _, edge := range l.derived.Edges {
		oldEdges[edgeKey(edge, oldID, newID)] = edge
	}
	newEdges := map[string]derive.Edge{}
	for _, edge := range after.Edges {
		newEdges[edgeKey(edge, newID, newID)] = edge
	}
	keys := make([]string, 0, len(oldEdges)+len(newEdges))
	seen := map[string]bool{}
	for key := range oldEdges {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range newEdges {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	changed := 0
	for _, key := range keys {
		old, was := oldEdges[key]
		new, is := newEdges[key]
		if !was || !is || old.Address != new.Address || old.Network != new.Network || old.Port != new.Port {
			if changed == 0 {
				fmt.Fprintln(out, "Changed route edges:")
			}
			changed++
			fmt.Fprintf(out, "  %s: %s -> %s\n", key, edgeAddress(old, was), edgeAddress(new, is))
		}
	}
	if changed == 0 {
		fmt.Fprintln(out, "Changed route edges: none")
	}
	compareMigrationTargets(out, l, newLoaded, root, global.ConfSecrets(), oldID, newID)
	compareMigrationSecrets(out, l, newLoaded)
	if err := reportServiceReferences(out, root, oldID, before, networkChanges, instanceChanges, routeChanges); err != nil {
		return err
	}
	fmt.Fprintln(out, "No inventory or secret files were changed. External DNS records were not checked.")
	return nil
}

func edgeKey(edge derive.Edge, sourceID, targetID string) string {
	from := edge.FromInstance
	if from == "" {
		from = edge.From.Instance + ":" + edge.From.Port
	} else if sourceID != targetID && strings.HasPrefix(from, sourceID+"-") {
		from = targetID + strings.TrimPrefix(from, sourceID)
	}
	return edge.Route + " " + from + " -> " + edge.To.Instance + ":" + edge.To.Port
}

func edgeAddress(edge derive.Edge, exists bool) string {
	if !exists {
		return "(absent)"
	}
	return fmt.Sprintf("%s:%d [%s]", edge.Address, edge.Port, edge.Network)
}

type networkChange struct {
	From    string
	To      string
	Address string
}

func parseNodeChange(spec string) (string, string, error) {
	fields := map[string]string{}
	for _, part := range strings.Split(spec, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || value == "" || (key != "from" && key != "to") || fields[key] != "" {
			return "", "", fmt.Errorf("--node: want from=<old>,to=<new>")
		}
		fields[key] = value
	}
	if fields["from"] == "" || fields["to"] == "" {
		return "", "", fmt.Errorf("--node: want from=<old>,to=<new>")
	}
	return fields["from"], fields["to"], nil
}

// parseNetworkChanges keeps each repeated --network occurrence together.
// The named fields avoid positional pairing between several networks.
func parseNetworkChanges(encoded string) ([]networkChange, error) {
	if encoded == "" {
		return nil, nil
	}
	var specs []string
	if err := json.Unmarshal([]byte(encoded), &specs); err != nil {
		return nil, fmt.Errorf("--network values: %w", err)
	}
	changes := make([]networkChange, 0, len(specs))
	fromSeen, toSeen := map[string]bool{}, map[string]bool{}
	for _, spec := range specs {
		fields := map[string]string{}
		for _, part := range strings.Split(spec, ",") {
			key, value, ok := strings.Cut(part, "=")
			if !ok || value == "" || (key != "from" && key != "to" && key != "address") || fields[key] != "" {
				return nil, fmt.Errorf("--network %q: want from=<old>,to=<new>,address=<new address>", spec)
			}
			fields[key] = value
		}
		change := networkChange{From: fields["from"], To: fields["to"], Address: fields["address"]}
		if change.From == "" || (change.To == "" && change.Address == "") {
			return nil, fmt.Errorf("--network %q: give from and at least to or address", spec)
		}
		if change.To == "" {
			change.To = change.From
		}
		if fromSeen[change.From] || toSeen[change.To] {
			return nil, fmt.Errorf("--network %q: network given more than once", spec)
		}
		fromSeen[change.From], toSeen[change.To] = true, true
		changes = append(changes, change)
	}
	return changes, nil
}

// renameInventoryNetwork changes every schema-defined reference in the
// in-memory snapshot. It does not infer meaning from opaque values or text.
func renameInventoryNetwork(inv *inventory.Root, oldName, newName string) ([]string, error) {
	found := false
	for _, name := range inv.Networks {
		if name == newName {
			return nil, fmt.Errorf("network %q already exists", newName)
		}
		if name == oldName {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("network %q is not in networks.yaml", oldName)
	}
	inv.Networks = append([]string(nil), inv.Networks...)
	refs := []string{"networks.yaml: networks"}
	for i, name := range inv.Networks {
		if name == oldName {
			inv.Networks[i] = newName
		}
	}
	if inv.Universal == oldName {
		inv.Universal = newName
		refs = append(refs, "networks.yaml: universal")
	}
	for i := range inv.Nodes {
		node := &inv.Nodes[i]
		if address, ok := node.Networks[oldName]; ok {
			if _, collision := node.Networks[newName]; collision {
				return nil, fmt.Errorf("%s already names network %q", node.Path, newName)
			}
			copied := make(inventory.Networks, len(node.Networks))
			for name, value := range node.Networks {
				copied[name] = value
			}
			delete(copied, oldName)
			copied[newName] = address
			node.Networks = copied
			refs = append(refs, node.Path+": networks."+oldName)
		}
		for j, name := range node.Reaches {
			if name == oldName {
				node.Reaches = append([]string(nil), node.Reaches...)
				node.Reaches[j] = newName
				refs = append(refs, node.Path+": reaches")
				break
			}
		}
	}
	if len(inv.Users) > 0 {
		users := make(map[string]inventory.User, len(inv.Users))
		for key, user := range inv.Users {
			users[key] = user
		}
		inv.Users = users
		for key, user := range inv.Users {
			credentials := make(map[string]inventory.Credential, len(user.Credentials))
			for name, value := range user.Credentials {
				credentials[name] = value
			}
			for credentialName, credential := range user.Credentials {
				for i, name := range credential.Reaches {
					if name == oldName {
						credential.Reaches = append([]string(nil), credential.Reaches...)
						credential.Reaches[i] = newName
						credentials[credentialName] = credential
						refs = append(refs, "users.yaml: users."+key+".credentials."+credentialName+".reaches")
						break
					}
				}
			}
			user.Credentials = credentials
			inv.Users[key] = user
		}
	}
	sort.Strings(refs)
	return refs, nil
}
