// This file maps the inventory's topology onto the generic shape
// internal/webgraph draws. The Container/Shape/Edge vocabulary belongs here
// and stays here: webgraph never learns what a node, an instance or a route
// is. See docs/apps/conf/inspect.md#the-connectivity-graph.
package conf

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/topology"
	"dgs-toolbox/internal/desktop"
	"dgs-toolbox/internal/webgraph"

	tea "github.com/charmbracelet/bubbletea"
)

// Graph kinds. They are this design's words, used to colour the picture and
// to fill its key.
const (
	kindPort    = "port"
	kindClient  = "client"
	kindProcess = "process"
	kindGroup   = "group"
	kindNode    = "node"
	kindHop     = "hop"
	kindEntry   = "entry"
	kindLocal   = "local"
)

// buildGraph turns one loaded inventory into the picture.
//
// The unit of the picture is a port, not an instance: a port is what holds
// principals, what a grant is written against and what a secret belongs to,
// so a server listening on two of them is two shapes and a line to one of
// them says which. A process box groups the ports one running program serves
// — two instances rendered into one configuration file — and a node box
// groups the processes on one machine. A line carries the name of the
// principal that crosses it, because the port it lands on already says the
// rest.
func buildGraph(l InspectData, title string) webgraph.Graph {
	t := topology.Build(l.inv, l.derived)

	g := webgraph.Graph{
		Title: title,
		Legend: map[string]string{
			kindPort:    "one listening port: its own principals, grants and secrets",
			kindClient:  "a configuration rendered for a person, dialling out",
			kindProcess: "one running program, serving the ports it holds",
			kindGroup:   "whose machines these are: a person's devices, or a provider",
			kindNode:    "one machine, drawn as itself when the picture is too small for its ports",
			kindEntry:   "a person's device reaching the port it was granted",
			kindHop:     "one hop of a route to the next, across machines",
			kindLocal:   "one hop of a route to the next, on the same machine",
		},
	}

	// A node's own line says where it can be reached and what it can reach:
	// the two facts every address on an edge below was chosen from.
	// The outermost box is whose machines these are — a person's devices, or
	// whoever hosts the servers — which is the directory the node file sits
	// in and not a field anyone writes twice.
	seenGroup := map[string]bool{}
	addGroup := func(group string) {
		if group == "" || seenGroup[group] {
			return
		}
		seenGroup[group] = true
		detail := ""
		if _, isUser := l.inv.Users[group]; isUser {
			detail = "devices"
		}
		g.Groups = append(g.Groups, webgraph.Group{
			ID: groupBox(group), Label: group, Detail: detail, Kind: kindGroup,
		})
	}
	for _, c := range t.Containers {
		addGroup(c.Group)
	}
	for _, c := range t.Containers {
		parts := nodeNetworks(l, c.ID)
		label := c.ID
		// The group box already says whose this is, so the node keeps the
		// part of its name the group does not repeat.
		if c.Group != "" {
			label = strings.TrimPrefix(label, c.Group+"-")
		}
		// The machine is the level the small picture is drawn at: zoomed
		// out, this box shuts and the lines between the ports inside it
		// become lines between machines.
		g.Groups = append(g.Groups, webgraph.Group{
			ID: c.ID, Label: label, Detail: strings.Join(parts, " · "), Parent: groupBox(c.Group), Kind: kindNode, Collapse: true,
		})
	}

	// A client instance is drawn as the route it was derived for, inside a
	// box for the device and a box for whose device that is. Its own ID
	// repeats the principal those boxes already name, and pushes the route
	// — the only part a reader is tracing — to the end of a string that
	// then reads like a machine.
	clientOf := map[string]derive.ExportInstance{}
	for _, ci := range l.derived.ExportInstances {
		clientOf[ci.ID] = ci
	}

	// source is the shape an instance's outgoing edges leave from: its
	// process box when it has ports of its own, and its single shape when
	// it has none.
	source := map[string]string{}
	seenProcess := map[string]bool{}

	// collapse is for a box that stands for a machine rather than a process
	// on one: an unmanaged user's device has no node file to be a container,
	// so its box is made here and shuts like a machine's does.
	addProcess := func(nodeID, procID, label string, collapse bool) {
		if seenProcess[procID] {
			return
		}
		seenProcess[procID] = true
		g.Groups = append(g.Groups, webgraph.Group{
			ID: procID, Label: label, Parent: nodeID, Kind: kindProcess, Collapse: collapse,
		})
	}

	for _, sh := range t.Shapes {
		inst, node, ok := findInstance(l, sh.Instance)
		ports := inventory.Ports{}
		if ok {
			ports = inst.Ports
		}
		procID := sh.Instance
		procLabel := sh.Instance
		if ok && inst.Process != "" {
			procID = node + "/" + inst.Process
			procLabel = inst.Process
		}

		kind := kindPort
		if !ok {
			kind = kindClient
		}

		if len(ports) == 0 {
			// Nothing listens: one shape for the whole instance, which is
			// what a client dialling out is.
			label, box := sh.Instance, sh.Container
			if ci, isClient := clientOf[sh.Instance]; isClient {
				label = ci.Route
				if box == "" {
					// A person with no node file has no device box, so
					// the box is the credential this file authenticates
					// with — the same name the secrets tree files it
					// under.
					box = ci.User + "/" + inventory.DefaultCredential
					addProcess(groupBox(ci.User), box, inventory.DefaultCredential, true)
					addGroup(ci.User)
				}
			}
			g.Nodes = append(g.Nodes, webgraph.Node{
				ID:      sh.Instance,
				Label:   label,
				Group:   box,
				Kind:    kind,
				Detail:  sh.Service,
				Tooltip: instanceTooltip(l, sh),
			})
			source[sh.Instance] = sh.Instance
			continue
		}

		addProcess(sh.Container, procID, procLabel, false)
		source[sh.Instance] = procID
		for _, port := range sortedPortNames(ports) {
			g.Nodes = append(g.Nodes, webgraph.Node{
				ID:      portShape(sh.Instance, port),
				Label:   portLabel(port, ports[port]),
				Group:   procID,
				Kind:    kind,
				Detail:  sh.Instance,
				Tooltip: portTooltip(l, sh, port, ports[port]),
			})
		}
	}

	// A line lands on the port it reaches and carries the name of what
	// crosses it: the person's account on that port, or the relay instance
	// dialling through.
	container := map[string]string{}
	for _, sh := range t.Shapes {
		container[sh.Instance] = sh.Container
	}
	authored := map[string]bool{}
	for _, n := range l.inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.Service != "" {
				authored[inst.ID] = true
			}
		}
	}
	for _, e := range t.Edges {
		kind := kindHop
		switch {
		case !authored[e.From]:
			kind = kindEntry
		case container[e.From] != "" && container[e.From] == container[e.To]:
			kind = kindLocal
		}
		from := source[e.From]
		if from == "" {
			from = e.From
		}
		label := publishedOn(l, e)
		if label == "" {
			label = principalOn(l, e)
		}
		g.Edges = append(g.Edges, webgraph.Edge{
			From:  from,
			To:    portShape(e.To, e.ToPort),
			Label: label,
			Kind:  kind,
		})
	}
	return g
}

// portLabel names one port: its name, its number, and its transport when
// that is not the TCP a reader assumes.
func portLabel(name string, p inventory.Port) string {
	if p.ProtocolOr() == inventory.ProtocolUDP {
		return fmt.Sprintf("%s %d/%s", name, p.Number, p.Protocol)
	}
	return fmt.Sprintf("%s %d", name, p.Number)
}

// sortedPortNames is the port names of one instance, in a stable order.
func sortedPortNames(ports inventory.Ports) []string {
	out := make([]string, 0, len(ports))
	for name := range ports {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// groupBox is the box ID for one group of nodes. It is prefixed so a group
// named after a user cannot collide with a node of the same name.
func groupBox(group string) string {
	if group == "" {
		return ""
	}
	return "group:" + group
}

// portShape is the shape ID for one port of one instance. Two instances of
// one process keep their own IDs here: the process is a box around them, not
// a renaming.
func portShape(instance, port string) string { return instance + ":" + port }

// findInstance returns the authored instance with this ID and the node it
// runs on. A derived client instance is not authored and has neither.
func findInstance(l InspectData, id string) (inventory.Instance, string, bool) {
	for _, n := range l.inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.ID == id && inst.Service != "" {
				return inst, n.ID, true
			}
		}
	}
	return inventory.Instance{}, "", false
}

// principalOn names what crosses one edge: the account a person's device
// holds on the port it lands on, or the instance relaying through it.
// publishedOn is the name a fan-out edge arrived at: the `published` of the
// port it lands on. It is what tells one of a reverse proxy's lines from the
// next, so it is the one thing on such an edge worth reading — the proxy
// itself holds no credential, and repeating its instance name on every line
// out of it says nothing the shapes at either end do not.
//
// Only for an edge out of a service that declares it fans out. An ordinary
// edge into a published port still carries its principal, which is the
// thing crossing it.
func publishedOn(l InspectData, e topology.Edge) string {
	from := instanceByID(l, e.From)
	if from == nil || !l.manifests[from.Service].FansOut() {
		return ""
	}
	to := instanceByID(l, e.To)
	if to == nil {
		return ""
	}
	return to.Ports[e.ToPort].Published
}

// instanceByID finds an authored instance in the inventory.
func instanceByID(l InspectData, id string) *inventory.Instance {
	for _, n := range l.inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for i := range n.Instances {
			if n.Instances[i].ID == id {
				return &n.Instances[i]
			}
		}
	}
	return nil
}

func principalOn(l InspectData, e topology.Edge) string {
	for _, ci := range l.derived.ExportInstances {
		if ci.ID != e.From {
			continue
		}
		if ci.User != "" {
			return l.inv.Users[ci.User].Account(ci.User, ci.Credential)
		}
		for _, n := range l.inv.Nodes {
			if n.ID != ci.Node || n.Broken != "" {
				continue
			}
			return l.inv.Users[n.Owner].Account(n.Owner, ci.Credential)
		}
		return ci.ID
	}
	return e.From
}

// nodeNetworks describes one node's two different statements about networks:
// `networks` is where others can reach it, `reaches` is where it can open a
// connection. NAT is why they cannot be one field, and why both belong on the
// picture.
func nodeNetworks(l InspectData, nodeID string) []string {
	for _, n := range l.inv.Nodes {
		if n.ID != nodeID || n.Broken != "" {
			continue
		}
		names := make([]string, 0, len(n.Networks))
		for name := range n.Networks {
			names = append(names, name)
		}
		sort.Strings(names)
		var out []string
		for _, name := range names {
			out = append(out, name+" "+n.Networks[name])
		}
		if len(n.Reaches) > 0 {
			out = append(out, "reaches "+strings.Join(n.Reaches, ", "))
		}
		return out
	}
	return nil
}

// portTooltip is what hovering one port shows: the machine it listens on and
// every principal holding a grant there, by name. No value from the secrets
// store reaches the page.
func portTooltip(l InspectData, sh topology.Shape, port string, p inventory.Port) string {
	lines := []string{fmt.Sprintf("%s %s on %s", sh.Instance, portLabel(port, p), sh.Container)}
	var who []string
	for _, p := range l.derived.Principals(sh.Instance, port) {
		who = append(who, p.Name)
	}
	if len(who) > 0 {
		lines = append(lines, fmt.Sprintf("%d granted: %s", len(who), strings.Join(who, ", ")))
	} else {
		lines = append(lines, "nothing holds a grant here")
	}
	return strings.Join(lines, "\n")
}

// instanceTooltip is what hovering a shape shows: where it runs, what it
// listens on, and who holds a grant on each port — by name. No value from
// the secrets store reaches the page.
func instanceTooltip(l InspectData, sh topology.Shape) string {
	var lines []string
	switch {
	case sh.Container != "":
		lines = append(lines, "on "+sh.Container)
	case sh.Owner != "":
		lines = append(lines, "for "+sh.Owner+" (unmanaged)")
	}
	for _, n := range l.inv.Nodes {
		for _, inst := range n.Instances {
			if inst.ID != sh.Instance {
				continue
			}
			for _, port := range sortedPortNames(inst.Ports) {
				var who []string
				for _, p := range l.derived.Principals(sh.Instance, port) {
					who = append(who, p.Name)
				}
				line := portLabel(port, inst.Ports[port])
				if len(who) > 0 {
					line += ": " + strings.Join(who, ", ")
				}
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// graphServer is the one page per process, the same way geo/gpx serves one:
// reopening the command reuses it rather than failing to bind.
var graphServer webgraph.Server

// openGraph starts the graph page if it is not already running and opens it
// in the browser. The graph is rebuilt per request from the root on disk, so
// a page reloaded after an edit shows the edit.
func (m InspectModel) openGraph() tea.Cmd {
	root := m.rootPath
	return func() tea.Msg {
		if root == "" {
			return graphOpenedMsg{err: fmt.Errorf("conf.root is not configured")}
		}
		url, err := graphServer.Start(graphAddr, func() (webgraph.Graph, error) {
			l, err := load(root)
			if err != nil {
				return webgraph.Graph{}, err
			}
			return buildGraph(l, "conf · "+filepath.Base(root)), nil
		})
		if err != nil {
			return graphOpenedMsg{err: err}
		}
		return graphOpenedMsg{err: desktop.Open(url)}
	}
}

// graphAddr is the address the graph page listens on. Loopback only: it
// holds the shape of the whole inventory, which is not a thing to serve to
// the network by default.
const graphAddr = "127.0.0.1:0"

// graphOpenedMsg reports what happened when the page was asked for.
type graphOpenedMsg struct{ err error }
