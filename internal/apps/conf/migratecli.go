package conf

import (
	"bytes"
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

// migrateAction builds the report for a plan the Migrate tab encodes as
// flags; "apply" writes it through the same validated plan. It is not a
// command-line action: the TUI is the only interface.
func migrateAction(in io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	if flags["apply"] == "true" {
		return migrateApplyAction(in, out, args, flags, global)
	}
	if flags["yes"] == "true" {
		return fmt.Errorf("confirmation given without apply")
	}
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
		return fmt.Errorf("migration plan names no source node")
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

	if _, err := parseMigrationScenario(flags["scenario"]); err != nil {
		return err
	}
	rep := &migrationReport{OldID: oldID, NewID: newID, Root: root, Inventory: before.Path, Apply: flags["apply"] == "true"}
	if oldID != newID {
		rep.Edits = append(rep.Edits, migrationChangeRow{Kind: "Node ID", Before: oldID, After: newID})
	}
	for _, change := range networkChanges {
		if change.From != change.To {
			rep.Edits = append(rep.Edits, migrationChangeRow{Kind: "Network name", Before: change.From, After: change.To, Refs: renamedRefs[change.From]})
		}
		if change.Address != "" {
			rep.Edits = append(rep.Edits, migrationChangeRow{Kind: "Address on " + change.To, Before: before.Networks[change.From], After: change.Address})
		}
	}
	renames := map[string]string{}
	for _, change := range instanceChanges {
		renames[change.From] = change.To
		rep.Edits = append(rep.Edits, migrationChangeRow{Kind: "Instance ID", Before: change.From, After: change.To, Refs: instanceRefs[change.From]})
	}
	for _, change := range routeChanges {
		rep.Edits = append(rep.Edits, migrationChangeRow{Kind: "Route name", Before: change.From, After: change.To, Refs: routeRefs[change.From]})
		rep.NodeMatches = append(rep.NodeMatches, migrationTextMatch{Path: "routes.yaml", Kind: "route key", Term: change.From,
			Text: "opaque instance values, deploy settings and service templates may use this route name as a key"})
	}
	for _, change := range publishedChanges {
		rep.Edits = append(rep.Edits, migrationChangeRow{Kind: "Published name " + change.Instance + ":" + change.Port, Before: publishedRefs[change.key()], After: change.To})
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
	keys := map[string]bool{}
	for key := range oldEdges {
		keys[key] = true
	}
	for key := range newEdges {
		keys[key] = true
	}
	for _, key := range sortedKeys(keys) {
		old, was := oldEdges[key]
		new, is := newEdges[key]
		if !was || !is || old.Address != new.Address || old.Network != new.Network || old.Port != new.Port {
			rep.Edges = append(rep.Edges, migrationEdgeChange{Key: key, Before: edgeAddress(old, was), After: edgeAddress(new, is)})
		}
	}
	rep.Work = compareMigrationTargets(rep, l, newLoaded, root, global.ConfSecrets(), oldID, newID)
	compareMigrationSecrets(rep, l, newLoaded)
	migrationDNSReview(rep, l, newLoaded, oldID, newID, oldID != newID || len(networkChanges) != 0, instanceChanges)
	if err := reportServiceReferences(rep, root, oldID, before, networkChanges, instanceChanges, routeChanges); err != nil {
		return err
	}
	rep.Procedure = buildMigrationProcedure(l, newLoaded, oldID, newID, root, global.ConfSecrets(), flags, renames)
	_, err = out.Write(rep.markdown())
	return err
}

func buildMigrationReport(flags map[string]string, global config.Config) ([]byte, error) {
	var report bytes.Buffer
	if err := migrateAction(nil, &report, []string{"node"}, flags, global); err != nil {
		return nil, err
	}
	return report.Bytes(), nil
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
	if info, ok := inv.NetworkInfo[oldName]; ok {
		infos := make(map[string]inventory.Network, len(inv.NetworkInfo))
		for name, value := range inv.NetworkInfo {
			infos[name] = value
		}
		delete(infos, oldName)
		info.Name = newName
		infos[newName] = info
		inv.NetworkInfo = infos
	}
	if inv.Universal == oldName {
		inv.Universal = newName
		refs = append(refs, "networks.yaml: universal")
	}
	if len(inv.Hosts) > 0 {
		hosts := make(map[string]inventory.Host, len(inv.Hosts))
		for key, host := range inv.Hosts {
			if host.Network == oldName {
				host.Network = newName
				refs = append(refs, "hosts.yaml: hosts."+key+".network")
			}
			hosts[key] = host
		}
		inv.Hosts = hosts
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
			if mac, ok := node.MACs[oldName]; ok {
				macs := make(map[string]string, len(node.MACs))
				for name, value := range node.MACs {
					macs[name] = value
				}
				delete(macs, oldName)
				macs[newName] = mac
				node.MACs = macs
			}
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
