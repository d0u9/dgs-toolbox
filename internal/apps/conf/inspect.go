// Package conf's inspect command answers "what does this configuration
// actually say" from four angles — a node, an instance, a user, a secret —
// per docs/apps/conf/inspect.md. This file is milestone 2: the index and the
// three read-only text views that do not touch a secret's value.
package conf

import (
	"fmt"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/secretstore"
	"dgs-toolbox/internal/conf/target"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/clipboard"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/overlay"
	"dgs-toolbox/internal/tui/scrolllist"
	"dgs-toolbox/internal/tui/text"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The two tabs docs/apps/conf/inspect.md#layout splits the index into.
const (
	tabNodes = iota
	tabUsers
	tabServices
	tabSecrets
	tabCount
)

// InspectModel is dgs conf inspect: a Nodes tab and a Users tab, each an
// index in a left column with the detail of whatever the cursor is on in a
// right column — the two-column, leading-narrow skeleton
// docs/tui.md#shared-column-skeletons names, per
// docs/apps/conf/inspect.md#layout. There is no separate open or closed
// state: moving the cursor is the only navigation within a tab.
type InspectModel struct {
	rootPath, secretsDir string
	loadErr              error

	l InspectData

	nodes      []*nodeGroup
	nodeGroups []*groupRow
	userGroups []*groupRow
	users      []string // sorted user keys
	services   []*serviceGroup
	secrets    secretsModel

	tab          int
	nodeItems    []scrolllist.Item
	userItems    []scrolllist.Item
	serviceItems []scrolllist.Item
	secretItems  []scrolllist.Item
	list         scrolllist.Model

	// nodeRowInstances and userRowInstances run beside nodeItems and
	// userItems: the instances each row stands for, which is what marking
	// that row marks. See docs/apps/conf/export.md#the-page.
	nodeRowInstances [][]string
	userRowInstances [][]string
	// marked is the instances marked for export, by name. It is one set for
	// both tabs, so a node marked on Nodes shows marked on Users too.
	marked map[string]bool
	// exportDir is where the export form opens: conf.export.dir.
	exportDir string
	export    *exportFlow
	// notice is the outcome of the last export, shown in the status bar.
	notice string
	// copy puts text on the clipboard; tests replace it.
	copy func(string) error

	detailScroll int

	// graphErr is why the graph page could not be opened, shown in the
	// status bar rather than over the view: not being able to draw a
	// picture of the inventory does not stop reading it.
	graphErr error

	width, height int
}

// InspectData is the loaded, is loaded's exported shape — inspect and export
// read the same bootstrap; see load.go.
type InspectData = loaded

func newInspectModel(rootPath, secretsDir string) InspectModel {
	m := InspectModel{rootPath: rootPath, secretsDir: secretsDir, list: scrolllist.New(), marked: map[string]bool{}, copy: clipboard.Copy}
	// Every index here is rows of "this thing, and a short qualifier": a
	// role and a machine, a count, a state. Beside the label they read as
	// one row per thing; under it they halve how much of a tree fits.
	m.list.InlineDetail(true)
	if rootPath == "" {
		return m
	}

	l, err := load(rootPath)
	if err != nil {
		m.loadErr = err
		return m
	}
	m.l = l
	m.nodes = buildTree(target.List(l.inv, l.derived))
	fillNodeGroups(l.inv, m.nodes)
	m.nodeGroups = groupTree(m.nodes, l.inv.IsUser, nil)

	for key := range l.inv.Users {
		m.users = append(m.users, key)
	}
	sort.Strings(m.users)

	m.userGroups = userTree(m.nodes, m.l.inv, nil)
	m.userItems, m.userRowInstances = userTabItems(m.userGroups)
	m.services = buildServiceGroups(m.l)
	m.secrets = buildSecrets(m.l, secretsDir)
	m.refresh()
	m.setTab(tabNodes)
	return m
}

// serviceGroup is one service and every instance of it, wherever it runs.
// The Nodes index answers "what runs on this machine"; this answers the
// other direction, "where is this service deployed", which no other view
// gives without reading every node in turn.
type serviceGroup struct {
	name     string
	expanded bool
	broken   string
	// instances are the deployments: one running program each, on a node.
	instances []serviceInstance
}

// serviceInstance is one deployed instance of a service: one process, on one
// node, listening on the ports below it.
type serviceInstance struct {
	id, node string
	ports    []servicePort
	// clients is how many files the routes into this instance produce for
	// people. They are not deployments and do not belong in the tree — a
	// person's files are under that person in the Users tab — but the
	// number belongs beside the thing that serves them.
	clients int
}

// servicePort is one named port of a deployed instance, and who may reach
// it. A port is where an account table lives: two ports of one process are
// two independent sets of credentials, which is why the tree goes this deep
// rather than stopping at the instance.
type servicePort struct {
	name string
	// port carries the number and the transport: 443/udp and 443/tcp are
	// two ports, and a view that printed only the number would call them
	// one.
	port       inventory.Port
	principals []string
}

// buildServiceGroups collects each service's deployed instances and their
// ports. A derived client instance is not deployed — it is a file handed to
// a person, and it belongs under that person — so it is counted here and
// not listed.
func buildServiceGroups(l InspectData) []*serviceGroup {
	byName := map[string]*serviceGroup{}
	var out []*serviceGroup
	group := func(name string) *serviceGroup {
		g, ok := byName[name]
		if !ok {
			g = &serviceGroup{name: name, expanded: true}
			byName[name] = g
			out = append(out, g)
		}
		return g
	}
	// Every service the root declares, so one with no instance yet is still
	// listed rather than silently absent.
	for name, manifest := range l.manifests {
		g := group(name)
		if manifest.Template == "" {
			g.broken = "no template declared"
		}
	}

	clients := map[string]int{} // instance ID -> client files reaching it
	for _, ci := range l.derived.ExportInstances {
		if r, ok := l.inv.Routes[ci.Route]; ok && len(r.Hops) > 0 {
			if hop, err := derive.ParseHop(r.Hops[0]); err == nil {
				clients[hop.Instance]++
			}
		}
	}

	for _, n := range l.inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.Service == "" {
				continue // an override of a derived instance, not one of its own.
			}
			g := group(inst.Service)
			g.instances = append(g.instances, serviceInstance{
				id: inst.ID, node: n.ID,
				ports: portsOf(l, inst.ID, inst.Ports), clients: clients[inst.ID],
			})
		}
	}

	for _, g := range out {
		sort.Slice(g.instances, func(i, j int) bool {
			return g.instances[i].id < g.instances[j].id
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// portsOf is an instance's named ports, each with the principals holding a
// grant on it, sorted by port name so the tree is stable.
func portsOf(l InspectData, instance string, ports inventory.Ports) []servicePort {
	var names []string
	for name := range ports {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]servicePort, 0, len(names))
	for _, name := range names {
		p := servicePort{name: name, port: ports[name]}
		for _, principal := range l.derived.Principals(instance, name) {
			p.principals = append(p.principals, principal.Name)
		}
		sort.Strings(p.principals)
		out = append(out, p)
	}
	return out
}

// serviceTabItems is the Services index: a service, the instances of it
// that are deployed, and under each of those the ports it listens on. It
// goes one level deeper than the Nodes index because a port is where an
// account table lives — two ports of one process are two independent sets
// of credentials, and stopping at the instance would hide that.
func serviceTabItems(services []*serviceGroup) []scrolllist.Item {
	var items []scrolllist.Item
	for _, g := range services {
		foldable := g.broken == "" && len(g.instances) > 0
		label := foldArrow(g.expanded, foldable) + " " + g.name
		if g.broken != "" {
			items = append(items, scrolllist.Item{ID: "service:" + g.name, Label: label, Detail: "broken: " + g.broken})
			continue
		}
		items = append(items, scrolllist.Item{ID: "service:" + g.name, Label: label, Detail: g.counts()})
		if !g.expanded {
			continue
		}
		for i, inst := range g.instances {
			branch, cont := branchMid, trunk
			if i == len(g.instances)-1 {
				branch, cont = branchLast, trunkClosed
			}
			items = append(items, scrolllist.Item{
				ID:     "inst:" + inst.id,
				Label:  "  " + branch + inst.id,
				Detail: "on " + inst.node + inst.clientSuffix(),
			})
			for j, port := range inst.ports {
				portBranch := branchMid
				if j == len(inst.ports)-1 {
					portBranch = branchLast
				}
				items = append(items, scrolllist.Item{
					ID:     "port:" + inst.id + "/" + port.name,
					Label:  "  " + cont + portBranch + port.name,
					Detail: port.summary(),
				})
			}
		}
	}
	return items
}

// summary is who may reach a port. Naming them rather than counting them is
// the point of the row: the account table is short, and the question asked
// of a port is which of them is on it.
func (p servicePort) summary() string {
	who := "nobody yet"
	if len(p.principals) > 0 {
		who = strings.Join(p.principals, ", ")
	}
	number := fmt.Sprintf("%d", p.port.Number)
	if p.port.ProtocolOr() == inventory.ProtocolUDP {
		number += "/" + p.port.Protocol
	}
	return fmt.Sprintf("%s · %s", number, who)
}

// clientSuffix counts the files the routes into this instance produce for
// people. They are not deployments and are not in the tree, and the number
// still belongs beside the instance that serves them.
func (inst serviceInstance) clientSuffix() string {
	if inst.clients == 0 {
		return ""
	}
	return " · " + plural(inst.clients, "file") + " for people"
}

// counts is a service's deployments. A service with one server and forty
// users is one deployment: the forty are files handed out, and counting
// them here would answer a question this index is not asking.
func (g *serviceGroup) counts() string {
	if len(g.instances) == 0 {
		return "deployed nowhere"
	}
	ports, clients := 0, 0
	for _, inst := range g.instances {
		ports += len(inst.ports)
		clients += inst.clients
	}
	out := plural(len(g.instances), "instance") + ", " + plural(ports, "port")
	if clients > 0 {
		out += ", " + plural(clients, "file") + " for people"
	}
	return out
}

// refresh rebuilds the Nodes index from the tree's current fold state and
// feeds it back to the list, keeping the cursor on whatever it was on when
// that row is still visible.
func (m *InspectModel) refresh() {
	selected := ""
	if item, ok := m.list.Selected(); ok {
		selected = item.ID
	}
	m.nodeGroups = groupTree(m.nodes, m.l.inv.IsUser, m.nodeGroups)
	m.nodeItems, m.nodeRowInstances = nodeTabItems(m.nodeGroups)
	m.userGroups = userTree(m.nodes, m.l.inv, m.userGroups)
	m.userItems, m.userRowInstances = userTabItems(m.userGroups)
	m.serviceItems = serviceTabItems(m.services)
	m.secretItems = secretTabItems(m.secrets.groups)
	switch m.tab {
	case tabNodes:
		m.list.SetItems(m.markedItems(m.nodeItems, m.nodeRowInstances))
	case tabUsers:
		m.list.SetItems(m.markedItems(m.userItems, m.userRowInstances))
	case tabServices:
		m.list.SetItems(m.serviceItems)
	case tabSecrets:
		m.list.SetItems(m.secretItems)
	default:
		return
	}
	if selected != "" {
		m.list.SelectID(selected)
	}
}

// foldable returns the fold state of the row under the cursor when it is a
// node or unmanaged user holding instances, and nil otherwise.
func (m *InspectModel) foldable() *bool {
	item, ok := m.list.Selected()
	if !ok {
		return nil
	}
	_, name, _ := strings.Cut(item.ID, ":")
	switch m.tab {
	case tabNodes:
		for _, g := range m.nodeGroups {
			if "group:"+g.name == item.ID && len(g.nodes) > 0 {
				return &g.expanded
			}
		}
		return m.foldableNode(name)
	case tabUsers:
		for _, g := range m.userGroups {
			if "user:"+g.name == item.ID && len(g.nodes) > 0 {
				return &g.expanded
			}
		}
		return m.foldableNode(name)
	case tabServices:
		for _, g := range m.services {
			if g.name == name && g.broken == "" && len(g.instances) > 0 {
				return &g.expanded
			}
		}
	case tabSecrets:
		for _, g := range m.secrets.groups {
			if "secretinst:"+g.instance == item.ID && len(g.ports) > 0 {
				return &g.expanded
			}
		}
	}
	return nil
}

// foldableNode is the fold state of the holder row named by an index ID —
// a node on the Nodes tab, a device or credential on the Users tab, which are
// the same entries seen from two directions.
func (m *InspectModel) foldableNode(name string) *bool {
	for _, n := range m.nodes {
		if n.key == name && n.broken == "" && len(n.instances) > 0 {
			return &n.expanded
		}
	}
	return nil
}

// toParent moves the cursor from an instance row up to the node holding it,
// which is what h does when there is nothing left to fold.
func (m *InspectModel) toParent() {
	item, ok := m.list.Selected()
	if !ok {
		return
	}
	kind, name, _ := strings.Cut(item.ID, ":")
	if kind == "port" {
		// The Services tree is three deep: a port's parent is the instance
		// listening on it.
		instance, _, _ := strings.Cut(name, "/")
		m.list.SelectID("inst:" + instance)
		m.detailScroll = 0
		return
	}
	if kind == "secret" || kind == "secretport" {
		// A credential's parent is its port, and a port's is its instance.
		p := strings.Split(name, "/")
		if kind == "secret" && len(p) >= 2 {
			m.list.SelectID("secretport:" + p[0] + "/" + p[1])
		} else if len(p) >= 1 {
			m.list.SelectID("secretinst:" + p[0])
		}
		m.detailScroll = 0
		return
	}
	if kind != "inst" {
		return
	}
	if m.tab == tabServices {
		for _, g := range m.services {
			for _, inst := range g.instances {
				if kind == "inst" && inst.id == name {
					m.list.SelectID("service:" + g.name)
					m.detailScroll = 0
					return
				}
			}
		}
		return
	}
	for _, n := range m.nodes {
		for _, inst := range n.instances {
			if inst.name != name {
				continue
			}
			id := "node:" + n.name
			if n.user {
				id = "cred:" + n.key
			}
			m.list.SelectID(id)
			m.detailScroll = 0
			return
		}
	}
}

// Tree drawing for the Nodes index. The list component gives every visible
// row the same one-based number, so depth has to come from the label: an
// instance hangs off its node on a branch, and its detail line continues the
// trunk past it. Without them a node and its instances read as one flat run
// of equals, which is what they are to the list and is not what they are.
const (
	branchMid   = "├─ "
	branchLast  = "└─ "
	trunk       = "│  "
	trunkClosed = "   "
)

// nodeTabItems is the Nodes index: whose machines these are, the machines,
// and what runs on each. Only machines — what is rendered for a person with
// no device file has no node file behind it and belongs on the Users tab,
// which answers the other question.
func nodeTabItems(groups []*groupRow) ([]scrolllist.Item, [][]string) {
	var items []scrolllist.Item
	var rows [][]string
	for _, g := range groups {
		nodes := realNodes(g.nodes)
		if len(nodes) == 0 {
			continue
		}
		indent := ""
		if g.name != "" {
			what := "provider"
			if g.isUser {
				what = "devices"
			}
			items = append(items, scrolllist.Item{
				ID:     "group:" + g.name,
				Label:  foldArrow(g.expanded, true) + " " + g.name,
				Detail: what + fieldSeparator + plural(len(nodes), "node"),
			})
			rows = append(rows, instancesOf(nodes))
			if !g.expanded {
				continue
			}
			indent = "  "
		}
		more, moreRows := treeRows(nodes, indent, "instance")
		items, rows = append(items, more...), append(rows, moreRows...)
	}
	return items, rows
}

// userTabItems is the Users index: one row per person, the devices and
// credentials their files hang off, and the files themselves. A device and a
// credential sit at one level because they answer one question — which of a
// person's identities this file was rendered for.
func userTabItems(groups []*groupRow) ([]scrolllist.Item, [][]string) {
	var items []scrolllist.Item
	var rows [][]string
	for _, g := range groups {
		items = append(items, scrolllist.Item{
			ID:     "user:" + g.name,
			Label:  foldArrow(g.expanded, len(g.nodes) > 0) + " " + g.name,
			Detail: plural(len(g.nodes), holderWord(g.nodes)),
		})
		rows = append(rows, instancesOf(g.nodes))
		if !g.expanded {
			continue
		}
		more, moreRows := treeRows(g.nodes, "  ", "file")
		items, rows = append(items, more...), append(rows, moreRows...)
	}
	return items, rows
}

// instancesOf is every instance held by nodes, in tree order.
func instancesOf(nodes []*nodeGroup) []string {
	var out []string
	for _, n := range nodes {
		for _, inst := range n.instances {
			out = append(out, inst.name)
		}
	}
	return out
}

// fillNodeGroups copies each node's group and owner off the inventory, which
// the tree itself does not read.
func fillNodeGroups(inv *inventory.Root, nodes []*nodeGroup) {
	for _, n := range nodes {
		for _, in := range inv.Nodes {
			if in.ID == n.key && in.Broken == "" {
				n.group, n.owner = in.Group, in.Owner
			}
		}
	}
}

// realNodes are the entries with a node file behind them.
func realNodes(nodes []*nodeGroup) []*nodeGroup {
	var out []*nodeGroup
	for _, n := range nodes {
		if !n.user {
			out = append(out, n)
		}
	}
	return out
}

// holderWord names what a person's files hang off: devices, credentials, or
// both, since someone may have a device file and still hold a credential no
// device names.
func holderWord(nodes []*nodeGroup) string {
	devices, credentials := false, false
	for _, n := range nodes {
		if n.user {
			credentials = true
		} else {
			devices = true
		}
	}
	switch {
	case devices && credentials:
		return "holder"
	case credentials:
		return "credential"
	}
	return "device"
}

// treeRows is the two levels under a group: each holder, and the instances
// under it while it is expanded. Beside each row it returns the instances
// that row stands for.
func treeRows(nodes []*nodeGroup, indent, unit string) ([]scrolllist.Item, [][]string) {
	var items []scrolllist.Item
	var rows [][]string
	for _, n := range nodes {
		// An unmanaged user's holder row is the person's own credential, and
		// its detail is the person — but it is not the group row above it,
		// which is that same person. Two rows keyed "user:doug" made folding
		// one fold the other, so the holder row is keyed by what it draws.
		kind, what := "node", ""
		if n.user {
			kind, what = "cred", "credential, "
		} else if n.owner != "" {
			// Whose device this is, said on the row itself: a phone named
			// `phone` says nothing on its own, and the group header scrolls
			// out of sight.
			what = n.owner + "'s device, "
		}
		foldable := n.broken == "" && len(n.instances) > 0
		label := indent + foldArrow(n.expanded, foldable) + " " + n.name
		if n.broken != "" {
			items = append(items, scrolllist.Item{ID: kind + ":" + n.key, Label: label, Detail: "broken: " + n.broken})
			rows = append(rows, nil)
			continue
		}
		items = append(items, scrolllist.Item{ID: kind + ":" + n.key, Label: label, Detail: what + plural(len(n.instances), unit)})
		rows = append(rows, instancesOf([]*nodeGroup{n}))
		if !n.expanded {
			continue
		}
		for i, inst := range n.instances {
			branch := branchMid
			if i == len(n.instances)-1 {
				branch = branchLast
			}
			d := inst.detail
			if inst.broken != "" {
				d = "broken: " + inst.broken
			}
			items = append(items, scrolllist.Item{
				ID:     "inst:" + inst.name,
				Label:  indent + "  " + branch + inst.label,
				Detail: d,
			})
			rows = append(rows, []string{inst.name})
		}
	}
	return items, rows
}

// setTab switches the index to tab, resetting the cursor and any detail
// scroll — a tab switch is a fresh view, not a continuation of the last one.
func (m *InspectModel) setTab(tab int) {
	m.tab = tab
	switch tab {
	case tabUsers:
		m.list.SetItems(m.markedItems(m.userItems, m.userRowInstances))
		m.list.HideNumbers(true)
	case tabServices:
		m.list.SetItems(m.serviceItems)
		m.list.HideNumbers(true)
	case tabSecrets:
		m.list.SetItems(m.secretItems)
		m.list.HideNumbers(true)
	default:
		m.list.SetItems(m.markedItems(m.nodeItems, m.nodeRowInstances))
		m.list.HideNumbers(true)
	}
	m.list.First()
	m.detailScroll = 0
}

// Tabs implements tui.TabContributor.
func (m InspectModel) Tabs() []tui.Tab {
	return []tui.Tab{
		{Label: "Nodes", Active: m.tab == tabNodes},
		{Label: "Users", Active: m.tab == tabUsers},
		{Label: "Services", Active: m.tab == tabServices},
		{Label: "Secrets", Active: m.tab == tabSecrets},
	}
}

func (m InspectModel) Init() tea.Cmd { return nil }

func (m InspectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.export != nil && m.export.picking {
			w, h := m.pickerSize()
			m.export.picker.SetSize(w-4, h-6)
		}
		return m, nil
	case graphOpenedMsg:
		m.graphErr = msg.err
		return m, nil
	case exportDoneMsg:
		m.finishExport(msg)
		return m, nil
	case exportCopiedMsg:
		m.finishCopy(msg)
		return m, nil
	case tui.TabSelectedMsg:
		// An open export is about the tab it started on; a click on another
		// tab behind it would change the page it is drawn over.
		if m.export != nil {
			return m, nil
		}
		if msg.Index >= 0 && msg.Index < tabCount {
			m.setTab(msg.Index)
		}
		return m, nil
	case tea.KeyMsg:
		if m.export != nil {
			return m.updateExport(msg)
		}
		switch msg.String() {
		case "[":
			m.setTab((m.tab + tabCount - 1) % tabCount)
			return m, nil
		case "]":
			m.setTab((m.tab + 1) % tabCount)
			return m, nil
		}
		if (m.tab == tabNodes && len(m.nodes) == 0) ||
			(m.tab == tabUsers && len(m.users) == 0) ||
			(m.tab == tabServices && len(m.services) == 0) ||
			(m.tab == tabSecrets && len(m.secrets.groups) == 0) {
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			m.list.Move(-1)
			m.detailScroll = 0
		case "down", "j":
			m.list.Move(1)
			m.detailScroll = 0
		case "g", "home":
			m.list.First()
			m.detailScroll = 0
		case "G", "end":
			m.list.Last()
			m.detailScroll = 0
		case "ctrl+d", "pgdown":
			m.detailScroll += max(1, (m.height-2)/2)
		case "ctrl+u", "pgup":
			m.detailScroll = max(0, m.detailScroll-max(1, (m.height-2)/2))
		// The fold keys are the File Explorer's, the same ones the export
		// tree uses: docs/apps/conf/inspect.md#layout says folding follows
		// that component rather than inventing its own.
		case "right", "l":
			if fold := m.foldable(); fold != nil {
				*fold = true
				m.refresh()
			}
		case "o":
			if fold := m.foldable(); fold != nil {
				*fold = !*fold
				m.refresh()
			}
		case "left", "h":
			if fold := m.foldable(); fold != nil && *fold {
				*fold = false
				m.refresh()
			} else {
				m.toParent()
			}
		case " ":
			m.toggleMark()
		case "a":
			m.toggleMarkAll()
		case "x":
			m.startExport()
		case "t":
			// t for topology. g is the first row and w folds everything,
			// both from the File Explorer's keys, so the graph takes the
			// letter of what it draws.
			//
			// The graph is the whole inventory at once, which no index can
			// be: an index is a tree, and what this shows is the edges
			// between its branches.
			return m, m.openGraph()
		case "w":
			switch m.tab {
			case tabNodes:
				for _, n := range m.nodes {
					n.expanded = false
				}
			case tabServices:
				for _, g := range m.services {
					g.expanded = false
				}
			case tabSecrets:
				for _, g := range m.secrets.groups {
					g.expanded = false
				}
			}
			m.refresh()
			m.list.First()
			m.detailScroll = 0
		}
		return m, nil
	}
	return m.updateExportMsg(msg)
}

// currentDetail renders whatever the index's cursor is on. It is computed on
// every call rather than cached: nothing here touches disk, and a stale
// cache would be one more thing to invalidate correctly.
func (m InspectModel) currentDetail() (kind, id, body string, err error) {
	item, ok := m.list.Selected()
	if !ok {
		return "", "", "", nil
	}
	kind, id, _ = strings.Cut(item.ID, ":")
	if kind == "secret" {
		body, err = renderSecretDetail(m.secrets, m.l, id)
		return kind, id, body, err
	}
	body, err = renderDetail(m.l, kind, id)
	return kind, id, body, err
}

func (m InspectModel) View() string {
	if m.rootPath == "" {
		return m.centered(titleStyle.Render("dgs conf inspect") + "\n\n" +
			mutedStyle.Render("conf.root is not configured"))
	}
	if m.loadErr != nil {
		return m.centered(titleStyle.Render("dgs conf inspect") + "\n\n" +
			brokenStyle.Render(m.loadErr.Error()))
	}
	if m.tab == tabUsers && len(m.users) == 0 {
		return m.centered(titleStyle.Render("dgs conf inspect") + "\n\n" +
			mutedStyle.Render("no user found"))
	}
	if m.tab == tabNodes && len(m.nodes) == 0 {
		return m.centered(titleStyle.Render("dgs conf inspect") + "\n\n" +
			mutedStyle.Render("no node found"))
	}
	if m.tab == tabServices && len(m.services) == 0 {
		return m.centered(titleStyle.Render("dgs conf inspect") + "\n\n" +
			mutedStyle.Render("no service found"))
	}
	if m.tab == tabSecrets && m.secrets.loadErr != nil {
		return m.centered(titleStyle.Render("dgs conf inspect") + "\n\n" +
			brokenStyle.Render(m.secrets.loadErr.Error()))
	}
	if m.tab == tabSecrets && len(m.secrets.groups) == 0 {
		return m.centered(titleStyle.Render("dgs conf inspect") + "\n\n" +
			mutedStyle.Render("the inventory implies no credential"))
	}

	left, right := m.columns()
	height := max(1, m.height-2) // fieldset's own top and bottom border.

	list := m.list
	list.SetSize(left-4, height)
	indexTitle := "NODES"
	switch m.tab {
	case tabUsers:
		indexTitle = "USERS"
	case tabServices:
		indexTitle = "SERVICES"
	case tabSecrets:
		indexTitle = "SECRETS"
	}
	leftBox := fieldset.ViewFocused(indexTitle, text.Fit(list.View(true, titleStyle, mutedStyle), height, left-4), left, true)

	_, id, body, err := m.currentDetail()
	if err != nil {
		body = brokenStyle.Render(err.Error())
	}
	lines := text.Hanging(body, right-4)
	scroll := min(m.detailScroll, max(0, len(lines)-height))
	end := min(len(lines), scroll+height)
	visible := strings.Join(lines[scroll:end], "\n")
	rightBox := fieldset.View(orNone(id, "nothing selected"), text.Fit(visible, height, right-4), right)

	page := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, " ", rightBox)
	if m.export != nil {
		return overlay.Place(page, m.exportView(), m.width, m.height)
	}
	return page
}

// CapturesShellKey keeps Esc and q inside an open export, where Esc steps
// back and q may be typed into the destination.
func (m InspectModel) CapturesShellKey(key string) bool {
	if m.export != nil && m.export.picking {
		return key == "esc" || (key == "q" && m.export.picker.CapturesText())
	}
	return m.export != nil && (key == "esc" || key == "q")
}

// columns is the "Two columns, leading narrow" skeleton docs/tui.md#shared-column-skeletons
// names: left a third of the width and capped at 90 cells, right the rest.
func (m InspectModel) columns() (left, right int) {
	available := max(3, m.width-1) // the one-cell gutter JoinHorizontal inserts.
	left = min(90, available/3)
	return left, available - left
}

func orNone(id, placeholder string) string {
	if id == "" {
		return placeholder
	}
	return id
}

func (m InspectModel) centered(content string) string {
	return lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, content)
}

func (m InspectModel) Status() tui.Status {
	if m.rootPath == "" || m.loadErr != nil {
		return tui.Status{Left: "INSPECT", Center: "no generator root", Right: "q Quit"}
	}
	// What the cursor is on is already the detail pane's own legend, so the
	// centre carries what the whole tab amounts to instead — for Secrets,
	// whether the store is in step, which is the reason that tab exists.
	center := plural(len(m.nodes), "node")
	switch m.tab {
	case tabUsers:
		center = plural(len(m.users), "user")
	case tabServices:
		center = plural(len(m.services), "service")
	case tabSecrets:
		center = m.secrets.secretsStatus()
	}
	right := "[/] Tab  h/l Fold  w Fold all  t Graph"
	switch m.tab {
	case tabNodes, tabUsers:
		right = "[/] Tab  Space Mark  a All  x Export  t Graph"
		if n := m.markedCount(); n > 0 {
			center += " · " + plural(n, "instance") + " marked"
		}
	}
	if m.export != nil {
		switch {
		case m.export.picking:
			right = "Enter Choose  Esc Back"
		case m.export.stage == exportConfirm:
			right = "Tab Switch  Enter Select  Esc Back"
		case m.export.stage == exportShow:
			right = "Tab Next file  ↑↓ Scroll  c Copy  Esc Back"
		default:
			right = "n Next  Esc Cancel"
		}
	}
	if m.notice != "" {
		center = m.notice
	}
	if m.graphErr != nil {
		right = "graph: " + m.graphErr.Error()
	}
	return tui.Status{Left: "INSPECT", Center: center, Right: right}
}

// renderDetail is the whole of milestone 2's three text views. It reads only
// what load already holds — no filesystem, no secretstore reads yet; a
// secret's own value is milestone 3.
func renderDetail(l InspectData, kind, id string) (string, error) {
	switch kind {
	case "node":
		return renderNodeDetail(l, id)
	case "inst":
		return renderInstanceDetail(l, id)
	case "user", "cred":
		// An unmanaged user's credential row stands for the person: there is
		// no file behind it, and the person is what it has to say.
		return renderUserDetail(l, id)
	case "service":
		return renderServiceDetail(l, id)
	case "port":
		return renderPortDetail(l, id)
	case "secretinst", "secretport":
		// A grouping row in the Secrets tree; its children carry the
		// detail, and a heading has none of its own.
		return "", nil
	default:
		return "", fmt.Errorf("inspect: unknown kind %q", kind)
	}
}

func renderNodeDetail(l InspectData, id string) (string, error) {
	var n *inventory.Node
	for i := range l.inv.Nodes {
		// A broken node has no ID — target.List and buildTree index it by
		// its file path instead, the same fallback as target.go's
		// valueOr(n.ID, n.Path).
		key := l.inv.Nodes[i].ID
		if key == "" {
			key = l.inv.Nodes[i].Path
		}
		if key == id {
			n = &l.inv.Nodes[i]
		}
	}
	if n == nil {
		return "", fmt.Errorf("node %q not found", id)
	}
	var b strings.Builder
	if n.Broken != "" {
		fmt.Fprintf(&b, "broken: %s\n", n.Broken)
		return b.String(), nil
	}
	line(&b,
		field("owner", n.Owner),
		field("reaches", strings.Join(n.Reaches, ", ")),
		field("export", n.Export),
	)
	line(&b, field("networks", pairsInline(n.Networks)))
	if n.Owner != "" {
		credential := l.inv.Users[n.Owner].Credentials[n.CredentialOr()]
		line(&b, field("credential", n.CredentialOr()), field("note", credential.Note))
	}
	// Everything that runs here, authored and derived alike. An authored
	// entry with no service is an override of a derived instance, not an
	// instance of its own, so it is listed once, from the derivation that
	// gives it its service and role.
	var rows [][]string
	for _, inst := range n.Instances {
		if inst.Service == "" {
			continue
		}
		rows = append(rows, []string{inst.ID, inst.Service})
	}
	for _, ci := range l.derived.ExportInstances {
		if ci.Node == id {
			rows = append(rows, []string{ci.ID, ci.Export + "  (derived)"})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	section(&b, plural(len(rows), "instance"), columns(rows))
	return b.String(), nil
}

func renderInstanceDetail(l InspectData, id string) (string, error) {
	// A real, authored instance.
	for _, n := range l.inv.Nodes {
		for _, inst := range n.Instances {
			if inst.ID != id || inst.Service == "" {
				continue
			}
			return textInstanceDetail(l, id, inst.Service, n.ID, "", inst, inst.Values)
		}
	}
	// A derived client instance.
	for _, ci := range l.derived.ExportInstances {
		if ci.ID != id {
			continue
		}
		return textInstanceDetail(l, id, ci.Export, ci.Node, ci.User, inventory.Instance{Ports: ci.Ports}, ci.Values)
	}
	return "", fmt.Errorf("instance %q not found", id)
}

func textInstanceDetail(l InspectData, id, service, node, user string, inst inventory.Instance, values map[string]any) (string, error) {
	var b strings.Builder
	ports := inst.Ports

	where := field("on", node)
	if user != "" {
		where = field("for", user+" (unmanaged)")
	}
	// What delivers the process, written only when it is not a host one.
	// It is here for the same reason the graph badges it: it says how to
	// read the bind under it — see
	// docs/apps/conf/inventory.md#what-runs-the-process.
	runtime := ""
	if inst.Containerised() {
		runtime = inst.RuntimeOr()
	}
	line(&b, fields(service, where, field("runs in", runtime)))

	var portParts []string
	for _, name := range sortedPortNames(ports) {
		portParts = append(portParts, portLabel(name, ports[name]))
	}
	line(&b, field("ports", strings.Join(portParts, fieldSeparator)))

	// The service's own parameters, opaque to dgs — see
	// docs/apps/conf/inventory.md#an-instances-own-values. They are printed
	// as written, since naming a key without its value says nothing a
	// reader could not get from the node file.
	var valueParts []string
	for _, name := range sortedAnyKeys(values) {
		valueParts = append(valueParts, fmt.Sprintf("%s %v", name, values[name]))
	}
	line(&b, field("values", strings.Join(valueParts, fieldSeparator)))

	var routes []string
	for name, r := range l.inv.Routes {
		for _, raw := range r.Hops {
			if hop, err := derive.ParseHop(raw); err == nil && hop.Instance == id {
				routes = append(routes, name)
			}
		}
	}
	sort.Strings(routes)
	derivedFor := ""
	for _, ci := range l.derived.ExportInstances {
		if ci.ID == id {
			// A derived client enters a route rather than being a hop of
			// one, so the scan above never finds it.
			derivedFor = ci.Route
		}
	}

	var upstream *derive.Edge
	for i := range l.derived.Edges {
		e := &l.derived.Edges[i]
		from := e.FromInstance
		if from == "" {
			from = e.From.Instance
		}
		if from == id {
			upstream = e
		}
	}
	upstreamText := ""
	if upstream != nil {
		upstreamText = fmt.Sprintf("%s:%s (%s:%d)", upstream.To.Instance, upstream.To.Port, upstream.Address, upstream.Port)
	}
	line(&b,
		field("hop of", strings.Join(routes, ", ")),
		field("derived for", derivedFor),
		field("upstream", upstreamText),
	)

	// Every secret this instance holds, by path and never by value: one per
	// principal on a per-principal port, plus the role's own list. The paths
	// are the real ones docs/apps/conf/inventory.md#secrets derives, so a
	// reader can go straight to the file.
	r := l.manifests[service]
	var secretRows [][]string
	if r.Auth == confgen.AuthPerPrincipal {
		for _, port := range sortedPortNames(ports) {
			for _, principal := range l.derived.Principals(id, port) {
				path := secretstore.Path{Instance: id, Port: port, Group: principal.Group, Name: principal.Slot}
				secretRows = append(secretRows, []string{path.String(), principal.Name})
			}
		}
	}
	for _, path := range selfPathsOf(r, id, inst) {
		note := "self"
		if who := portsHandingOut(inst, path.Name, path.Key); len(who) > 0 {
			note = "handed to everything granted on " + strings.Join(who, ", ")
		}
		secretRows = append(secretRows, []string{path.String(), note})
	}
	section(&b, plural(len(secretRows), "secret")+", values not shown", columns(secretRows))

	return b.String(), nil
}

// sortedAnyKeys keeps map iteration out of the views, which
// would otherwise reorder a detail pane between two redraws of the same row.

func sortedAnyKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func renderUserDetail(l InspectData, id string) (string, error) {
	u, ok := l.inv.Users[id]
	if !ok {
		return "", fmt.Errorf("user %q not found", id)
	}
	var b strings.Builder

	// The username is what the services see, and defaults to the user's own
	// identifier — see docs/apps/conf/inventory.md#users. Printing it when
	// the two are equal says nothing; printing it when they differ is the
	// whole point of the field.
	username := ""
	if name := u.UsernameOr(id); name != id {
		username = name
	}
	var owned []string
	named := map[string]bool{}
	for _, n := range l.inv.Nodes {
		if n.Owner == id {
			owned = append(owned, n.ID)
			named[n.CredentialOr()] = true
		}
	}
	sort.Strings(owned)
	// A credential none of their devices names is one they carry themselves,
	// on whatever machine is at hand. Someone with no device file is that
	// case for every credential they keep.
	var carried []string
	for _, credential := range u.CredentialNames() {
		if !named[credential] {
			carried = append(carried, credential)
		}
	}
	kind := plural(len(owned), "device")
	if u.Devices == inventory.DevicesNone {
		kind = "none declared, no node file"
	}
	if len(carried) > 0 {
		kind += ", carries " + strings.Join(carried, ", ")
	}
	line(&b,
		field("username", username),
		field("devices", kind),
		field("export", u.Export),
	)

	// What each granted route reaches, so a route name means something
	// here: the entry hop is where this person's traffic goes in.
	var accessRows [][]string
	for _, name := range u.Access {
		r, ok := l.inv.Routes[name]
		if !ok || len(r.Hops) == 0 {
			accessRows = append(accessRows, []string{name, "(no such route)"})
			continue
		}
		accessRows = append(accessRows, []string{name, "enters " + r.Hops[0]})
	}
	section(&b, plural(len(accessRows), "route")+" granted", columns(accessRows))

	if len(carried) > 0 {
		section(&b, "carried by hand, renders", columns(userInstanceRows(l, id, "", u.Access)))
	}
	{
		for _, node := range owned {
			heading := node + " renders"
			if c := nodeByID(l.inv, node).CredentialOr(); c != inventory.DefaultCredential {
				heading = node + " · " + c + " renders"
			}
			section(&b, heading, columns(userInstanceRows(l, "", node, u.Access)))
		}
	}

	// Every credential this person holds, by path. The account name is what
	// the server's own table calls it: the username and the credential,
	// always both.
	var grantRows [][]string
	seenGrant := map[string]bool{}
	for _, g := range l.derived.Grants {
		if g.Principal.Kind != derive.PrincipalUser || g.Principal.Group != id {
			continue
		}
		path := secretstore.Path{Instance: g.Instance, Port: g.Port, Group: g.Principal.Group, Name: g.Principal.Slot}
		if seenGrant[path.String()] {
			continue
		}
		seenGrant[path.String()] = true
		grantRows = append(grantRows, []string{path.String(), g.Principal.Name})
	}
	sort.Slice(grantRows, func(i, j int) bool { return grantRows[i][0] < grantRows[j][0] })
	section(&b, plural(len(grantRows), "credential")+", values not shown", columns(grantRows))
	return b.String(), nil
}

// userInstanceRows is what each granted route derives for one unmanaged
// user or one managed device — one row per route, whether or not it derives
// anything. A route whose entry role declares no reached_by renders no client
// file, and the person still holds a credential for it: listing only the
// instances that exist would make the two cases look like one route missing.
// See docs/apps/conf/inventory.md#what-is-derived.
func userInstanceRows(l InspectData, user, node string, access []string) [][]string {
	var rows [][]string
	for _, route := range access {
		var found *derive.ExportInstance
		for i := range l.derived.ExportInstances {
			ci := &l.derived.ExportInstances[i]
			if ci.Route != route {
				continue
			}
			if (user != "" && ci.User == user) || (node != "" && ci.Node == node) {
				found = ci
			}
		}
		if found != nil {
			rows = append(rows, []string{route, found.ID, found.Export})
			continue
		}
		rows = append(rows, []string{route, "—", noClientReason(l, route)})
	}
	return rows
}

// noClientReason says why a granted route derives no client instance, which
// is a fact about the service it enters rather than something missing here.
func noClientReason(l InspectData, route string) string {
	r, ok := l.inv.Routes[route]
	if !ok || len(r.Hops) == 0 {
		return "(no such route)"
	}
	hop, err := derive.ParseHop(r.Hops[0])
	if err != nil {
		return "(unreadable entry hop)"
	}
	for _, n := range l.inv.Nodes {
		for _, inst := range n.Instances {
			if inst.ID != hop.Instance {
				continue
			}
			if len(l.manifests[inst.Service].Exports) == 0 {
				return fmt.Sprintf("no file (%s declares no exports)", inst.Service)
			}
		}
	}
	return "no client file"
}

// nodeByID is the node with that ID, or a zero node — which has no
// profiles, so a caller looping over ProfileNames gets the one unnamed
// identity and behaves as it would for a device with none.
func nodeByID(inv *inventory.Root, id string) inventory.Node {
	for _, n := range inv.Nodes {
		if n.ID == id {
			return n
		}
	}
	return inventory.Node{}
}

// renderServiceDetail is the service view: what the manifest declares, role
// by role, and where every instance of it runs. The manifest half is the
// part no other view shows — a role's template, output name and own secrets
// are what someone deploying the service needs and what a node detail,
// which is about one machine, has no place for.
func renderServiceDetail(l InspectData, id string) (string, error) {
	manifest, ok := l.manifests[id]
	if !ok {
		return "", fmt.Errorf("service %q not found", id)
	}
	var b strings.Builder

	secretShape := "printable random string (none declared)"
	if manifest.Secret.Kind != "" {
		secretShape = manifest.Secret.Kind
		if manifest.Secret.Bytes > 0 {
			secretShape += fmt.Sprintf(", %d bytes", manifest.Secret.Bytes)
		}
	}
	line(&b, field("", l.serviceDirs[id]), field("secret", secretShape))

	r := manifest

	// The service's own line carries what it is; the lines under it carry
	// what it produces.
	rotation := ""
	if r.Rotation != "" {
		rotation = r.Rotation + ", rotating drops the connection"
	}
	written := ""
	if len(r.Exports) > 0 {
		written = "written out as " + strings.Join(r.Exports, ", ")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, fields(
		orNone(r.Auth, confgen.AuthNone+" (not declared)"),
		written,
		field("rotation", rotation),
	))

	render := r.Template
	if r.Defaults != "" {
		render += " (" + r.Defaults + " defaults)"
	}
	line(&b, "  "+fields(field("renders", render), field("as", r.Output)))

	if len(r.Self) > 0 {
		var described []string
		for _, name := range r.Self.Names() {
			decl := r.Self[name]
			switch {
			case decl.Set && len(decl.Fields) > 0:
				described = append(described, name+" (a set of "+strings.Join(decl.FieldNames(), "+")+")")
			case decl.Set:
				described = append(described, name+" (a set)")
			case len(decl.Fields) > 0:
				described = append(described, name+" ("+strings.Join(decl.FieldNames(), "+")+")")
			default:
				described = append(described, name)
			}
		}
		line(&b, "  "+field("its own secrets", strings.Join(described, ", ")))
	}

	// A service both runs somewhere and produces files for the people
	// granted a route into it. The two are separate counts, and a server
	// with no route into it yet has the first without the second.
	if rendered := filesOf(l, id); rendered > 0 {
		line(&b, "  "+plural(rendered, "file")+" written out for people")
	}
	var instances []serviceInstance
	for _, g := range buildServiceGroups(l) {
		if g.name == id {
			instances = g.instances
		}
	}
	if len(instances) == 0 {
		line(&b, "  deployed nowhere yet")
		return b.String(), nil
	}
	for _, inst := range instances {
		fmt.Fprintf(&b, "  %s on %s\n", inst.id, inst.node)
		var rows [][]string
		for _, port := range inst.ports {
			rows = append(rows, []string{port.name, port.summary()})
		}
		for _, row := range columns(rows) {
			fmt.Fprintf(&b, "    %s\n", row)
		}
	}
	return b.String(), nil
}

// filesOf counts the files written out for people from routes entering this
// service — one per export instance whose route enters one of its instances.
func filesOf(l InspectData, service string) int {
	n := 0
	for _, inst := range instancesOfService(l, service) {
		for _, ci := range l.derived.ExportInstances {
			if r, ok := l.inv.Routes[ci.Route]; ok && len(r.Hops) > 0 {
				if hop, err := derive.ParseHop(r.Hops[0]); err == nil && hop.Instance == inst {
					n++
				}
			}
		}
	}
	return n
}

// instancesOfService is every authored instance of one service.
func instancesOfService(l InspectData, service string) []string {
	var out []string
	for _, n := range l.inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.Service == service {
				out = append(out, inst.ID)
			}
		}
	}
	return out
}

// renderPortDetail is one named port of one instance, addressed as
// "<instance>/<port>". A port is the unit an account table belongs to, so
// this is where "who can reach this, and with which credential" is
// answered — the instance above it holds several of these, independently.
func renderPortDetail(l InspectData, id string) (string, error) {
	instance, port, ok := strings.Cut(id, "/")
	if !ok {
		return "", fmt.Errorf("port %q is not <instance>/<port>", id)
	}

	var found *inventory.Instance
	var node string
	for _, n := range l.inv.Nodes {
		for i := range n.Instances {
			if n.Instances[i].ID == instance {
				found, node = &n.Instances[i], n.ID
			}
		}
	}
	if found == nil {
		return "", fmt.Errorf("instance %q not found", instance)
	}
	number, ok := found.Ports[port]
	if !ok {
		return "", fmt.Errorf("instance %q has no port %q", instance, port)
	}

	var routes []string
	for name, r := range l.inv.Routes {
		for _, raw := range r.Hops {
			if hop, err := derive.ParseHop(raw); err == nil && hop.Instance == instance && hop.Port == port {
				routes = append(routes, name)
			}
		}
	}
	sort.Strings(routes)

	var b strings.Builder
	line(&b, fmt.Sprintf("%s on %s", portLabel(port, number), node), field("bind", found.Bind))
	line(&b,
		field("instance", fmt.Sprintf("%s (%s)", instance, found.Service)),
		field("routes", strings.Join(routes, ", ")),
	)

	// The account table, which is this port's alone: another port of the
	// same process has its own, with its own credentials.
	principals := l.derived.Principals(instance, port)
	if len(principals) == 0 {
		line(&b, "\nnobody holds a grant on this port")
		return b.String(), nil
	}
	sort.Slice(principals, func(i, j int) bool { return principals[i].Name < principals[j].Name })
	var rows [][]string
	for _, principal := range principals {
		path := secretstore.Path{Instance: instance, Port: port, Group: principal.Group, Name: principal.Slot}
		rows = append(rows, []string{principal.Name, path.String()})
	}
	section(&b, plural(len(principals), "account")+", values not shown", columns(rows))
	return b.String(), nil
}

// selfPathsOf is every secret path an instance's own secrets imply, in the
// order a reader walks the tree: one per name, per key of a set, per field
// of a record. It mirrors secretstore.ImpliedPaths, which computes the same
// set for the whole inventory.
func selfPathsOf(manifest confgen.Manifest, id string, inst inventory.Instance) []secretstore.Path {
	keys := inst.SelfKeys()
	var out []secretstore.Path
	for _, name := range inst.SelfNames(manifest.Self.Names()) {
		decl := manifest.Self[name]
		leaves := func(key string) {
			fields := decl.FieldNames()
			if len(fields) == 0 {
				out = append(out, secretstore.Path{Instance: id, Port: secretstore.SelfPort, Name: name, Key: key})
				return
			}
			for _, field := range fields {
				out = append(out, secretstore.Path{Instance: id, Port: secretstore.SelfPort, Name: name, Key: key, Field: field})
			}
		}
		if !decl.Set {
			leaves("")
			continue
		}
		for _, key := range keys[name] {
			leaves(key)
		}
	}
	return out
}

// portsHandingOut names the ports of an instance that hand one of its own
// secrets to everything granted on them, so a reader sees which credential
// travels and which stays on the machine.
func portsHandingOut(inst inventory.Instance, name, key string) []string {
	want := name
	if key != "" {
		want = name + "." + key
	}
	var out []string
	for portName, port := range inst.Ports {
		for _, ref := range port.Self {
			if ref == want {
				out = append(out, portName)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}
