package conf

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/cred/publish"
)

// migrateWizard collects explicit choices and writes a read-only rollout
// report. Empty answers keep the value shown in brackets; EOF cancels.
func migrateWizard(in io.Reader, out io.Writer, root string, global config.Config) error {
	l, err := load(root)
	if err != nil {
		return err
	}
	if len(l.inv.Nodes) == 0 {
		return fmt.Errorf("no nodes found in %s", root)
	}
	reader := bufio.NewReader(in)
	fmt.Fprintln(out, "Node migration wizard (preview only; inventory and secrets are not changed)")
	fmt.Fprintln(out, "Available nodes:")
	for _, node := range l.inv.Nodes {
		if node.Broken == "" {
			fmt.Fprintf(out, "  %s\n", node.ID)
		}
	}
	oldID, err := migrationAnswer(reader, out, "Move which node", "")
	if err != nil {
		return err
	}
	var nodeFound bool
	var nodeIndex int
	for i, node := range l.inv.Nodes {
		if node.ID == oldID && node.Broken == "" {
			nodeFound, nodeIndex = true, i
			break
		}
	}
	if !nodeFound {
		return fmt.Errorf("node %q not found", oldID)
	}
	node := l.inv.Nodes[nodeIndex]
	newID, err := migrationAnswer(reader, out, "New node ID", oldID)
	if err != nil {
		return err
	}
	networks := make([]string, 0, len(node.Networks))
	for name := range node.Networks {
		networks = append(networks, name)
	}
	sort.Strings(networks)
	var networkSpecs []string
	for _, name := range networks {
		fmt.Fprintf(out, "Network %s on %s:\n", name, oldID)
		newName, err := migrationAnswer(reader, out, "  New network name", name)
		if err != nil {
			return err
		}
		address, err := migrationAnswer(reader, out, "  New address", node.Networks[name])
		if err != nil {
			return err
		}
		if newName == name && address == node.Networks[name] {
			continue
		}
		spec := "from=" + name
		if newName != name {
			spec += ",to=" + newName
		}
		if address != node.Networks[name] {
			spec += ",address=" + address
		}
		networkSpecs = append(networkSpecs, spec)
	}
	var instanceSpecs []string
	var publishedSpecs []string
	instanceIDs := map[string]bool{}
	for _, inst := range node.Instances {
		if inst.Service == "" {
			continue
		}
		instanceIDs[inst.ID] = true
		newName, err := migrationAnswer(reader, out, "New instance ID for "+inst.ID, inst.ID)
		if err != nil {
			return err
		}
		if newName != inst.ID {
			instanceSpecs = append(instanceSpecs, "from="+inst.ID+",to="+newName)
		}
		portNames := make([]string, 0, len(inst.Ports))
		for name := range inst.Ports {
			if inst.Ports[name].Published != "" {
				portNames = append(portNames, name)
			}
		}
		sort.Strings(portNames)
		for _, port := range portNames {
			oldName := inst.Ports[port].Published
			newName, err := migrationAnswer(reader, out, "Published name for "+inst.ID+":"+port, oldName)
			if err != nil {
				return err
			}
			if newName != oldName {
				publishedSpecs = append(publishedSpecs, "instance="+inst.ID+",port="+port+",to="+newName)
			}
		}
	}
	routeNames := make([]string, 0, len(l.inv.Routes))
	for name, route := range l.inv.Routes {
		for _, raw := range route.Hops {
			hop, err := derive.ParseHop(raw)
			if err == nil && instanceIDs[hop.Instance] {
				routeNames = append(routeNames, name)
				break
			}
		}
	}
	sort.Strings(routeNames)
	var routeSpecs []string
	for _, name := range routeNames {
		newName, err := migrationAnswer(reader, out, "New route name for "+name, name)
		if err != nil {
			return err
		}
		if newName != name {
			routeSpecs = append(routeSpecs, "from="+name+",to="+newName)
		}
	}
	if oldID == newID && len(networkSpecs) == 0 && len(instanceSpecs) == 0 && len(routeSpecs) == 0 && len(publishedSpecs) == 0 {
		return fmt.Errorf("no changes selected")
	}
	encNetworks, _ := json.Marshal(networkSpecs)
	encInstances, _ := json.Marshal(instanceSpecs)
	encRoutes, _ := json.Marshal(routeSpecs)
	encPublished, _ := json.Marshal(publishedSpecs)
	flags := map[string]string{
		"node":    "from=" + oldID + ",to=" + newID,
		"network": string(encNetworks), "instance": string(encInstances), "route": string(encRoutes), "published": string(encPublished),
	}
	report, err := buildMigrationReport(flags, global)
	if err != nil {
		return err
	}
	path, err := migrationAnswer(reader, out, "Save report to path (Enter prints only)", "")
	if err != nil {
		return err
	}
	if path != "" {
		if err := publish.Create(path, report, 0o600, 0o700); err != nil {
			return fmt.Errorf("saving migration report: %w", err)
		}
		fmt.Fprintf(out, "Report saved: %s\n", path)
	}
	_, err = out.Write(report)
	return err
}

func buildMigrationReport(flags map[string]string, global config.Config) ([]byte, error) {
	var report bytes.Buffer
	fmt.Fprintln(&report, "Upgrade report (proposed; no files changed)")
	if err := migrateAction(nil, &report, []string{"node"}, flags, global); err != nil {
		return nil, err
	}
	fmt.Fprintln(&report, "Operator checklist:")
	fmt.Fprintln(&report, "  [ ] Review the inventory references, instance IDs, service-file matches and implied secret paths above.")
	fmt.Fprintln(&report, "  [ ] Make the approved inventory and secret-path edits; keep backups for rollback.")
	fmt.Fprintln(&report, "  [ ] Run dgs conf --check, then export the listed changed targets.")
	fmt.Fprintln(&report, "  [ ] Install or redeploy those outputs on their destination nodes; verify affected routes.")
	fmt.Fprintln(&report, "  [ ] Review DNS separately, then verify from each relevant network.")
	fmt.Fprintln(&report, "  [ ] Retire the old node only after the new endpoints work.")
	fmt.Fprintln(&report, "Rollback: restore inventory and secret backups, export and redeploy the old targets, and revert any DNS changes.")
	return report.Bytes(), nil
}

func migrationAnswer(reader *bufio.Reader, out io.Writer, prompt, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(out, "%s: ", prompt)
	} else {
		fmt.Fprintf(out, "%s [%s]: ", prompt, defaultValue)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("migration wizard cancelled before %s: %w", prompt, err)
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		answer = defaultValue
	}
	return answer, nil
}
