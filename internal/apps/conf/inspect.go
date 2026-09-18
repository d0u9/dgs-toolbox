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
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/scrolllist"
	"dgs-toolbox/internal/tui/text"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The two tabs docs/apps/conf/inspect.md#layout splits the index into.
const (
	tabNodes = iota
	tabUsers
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

	nodes []*nodeGroup
	users []string // sorted user keys

	tab       int
	nodeItems []scrolllist.Item
	userItems []scrolllist.Item
	list      scrolllist.Model

	detailScroll int

	width, height int
}

// InspectData is the loaded, is loaded's exported shape — inspect and export
// read the same bootstrap; see load.go.
type InspectData = loaded

func newInspectModel(rootPath, secretsDir string) InspectModel {
	m := InspectModel{rootPath: rootPath, secretsDir: secretsDir, list: scrolllist.New()}
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

	for key := range l.inv.Users {
		m.users = append(m.users, key)
	}
	sort.Strings(m.users)

	m.nodeItems = nodeTabItems(m.nodes)
	m.userItems = userTabItems(m.l.inv, m.users)
	m.setTab(tabNodes)
	return m
}

func nodeTabItems(nodes []*nodeGroup) []scrolllist.Item {
	var items []scrolllist.Item
	for _, n := range nodes {
		label := "▸ " + n.name
		if n.broken != "" {
			items = append(items, scrolllist.Item{ID: "node:" + n.name, Label: label, Detail: "broken: " + n.broken})
			continue
		}
		items = append(items, scrolllist.Item{ID: "node:" + n.name, Label: label, Detail: plural(len(n.instances), "instance")})
		for _, inst := range n.instances {
			d := inst.detail
			if inst.broken != "" {
				d = "broken: " + inst.broken
			}
			items = append(items, scrolllist.Item{ID: "inst:" + inst.name, Label: "    " + inst.name, Detail: d})
		}
	}
	return items
}

func userTabItems(inv *inventory.Root, keys []string) []scrolllist.Item {
	var items []scrolllist.Item
	for _, key := range keys {
		items = append(items, scrolllist.Item{ID: "user:" + key, Label: key, Detail: userSummary(inv.Users[key])})
	}
	return items
}

// setTab switches the index to tab, resetting the cursor and any detail
// scroll — a tab switch is a fresh view, not a continuation of the last one.
func (m *InspectModel) setTab(tab int) {
	m.tab = tab
	if tab == tabUsers {
		m.list.SetItems(m.userItems)
	} else {
		m.list.SetItems(m.nodeItems)
	}
	m.list.First()
	m.detailScroll = 0
}

// Tabs implements tui.TabContributor.
func (m InspectModel) Tabs() []tui.Tab {
	return []tui.Tab{
		{Label: "Nodes", Active: m.tab == tabNodes},
		{Label: "Users", Active: m.tab == tabUsers},
	}
}

func userSummary(u inventory.User) string {
	kind := "managed"
	if u.Devices == inventory.DevicesUnmanaged {
		kind = "unmanaged"
	}
	return kind + ", access: " + strings.Join(u.Access, ", ")
}

func (m InspectModel) Init() tea.Cmd { return nil }

func (m InspectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tui.TabSelectedMsg:
		if msg.Index >= 0 && msg.Index < tabCount {
			m.setTab(msg.Index)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "[":
			m.setTab((m.tab + tabCount - 1) % tabCount)
			return m, nil
		case "]":
			m.setTab((m.tab + 1) % tabCount)
			return m, nil
		}
		if (m.tab == tabNodes && len(m.nodes) == 0) || (m.tab == tabUsers && len(m.users) == 0) {
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
		}
		return m, nil
	}
	return m, nil
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

	left, right := m.columns()
	height := max(1, m.height-2) // fieldset's own top and bottom border.

	list := m.list
	list.SetSize(left-4, height)
	leftBox := fieldset.ViewFocused("INDEX", text.Fit(list.View(true, titleStyle, mutedStyle), height, left-4), left, true)

	_, id, body, err := m.currentDetail()
	if err != nil {
		body = brokenStyle.Render(err.Error())
	}
	lines := text.Wrapped(body, right-4)
	scroll := min(m.detailScroll, max(0, len(lines)-height))
	end := min(len(lines), scroll+height)
	visible := strings.Join(lines[scroll:end], "\n")
	rightBox := fieldset.View(orNone(id, "nothing selected"), text.Fit(visible, height, right-4), right)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, " ", rightBox)
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
	kind, id, _, _ := m.currentDetail()
	center := plural(len(m.nodes), "node")
	if m.tab == tabUsers {
		center = plural(len(m.users), "user")
	}
	if id != "" {
		center = kind + ": " + id
	}
	return tui.Status{Left: "INSPECT", Center: center, Right: "[/] Tab  ↑↓ Move  ctrl+d/u Scroll detail"}
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
	case "user":
		return renderUserDetail(l, id)
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
	if n.Owner != "" {
		fmt.Fprintf(&b, "owner: %s\n", n.Owner)
	}
	if len(n.Networks) > 0 {
		var names []string
		for name := range n.Networks {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Fprintln(&b, "networks:")
		for _, name := range names {
			fmt.Fprintf(&b, "  %s: %s\n", name, n.Networks[name])
		}
	}
	if len(n.Reaches) > 0 {
		fmt.Fprintf(&b, "reaches: %s\n", strings.Join(n.Reaches, ", "))
	}
	if n.ClientRole != "" {
		fmt.Fprintf(&b, "client_role: %s\n", n.ClientRole)
	}
	if len(n.Instances) > 0 {
		fmt.Fprintln(&b, "\ninstances:")
		for _, inst := range n.Instances {
			if inst.Service == "" && inst.Role == "" {
				continue
			}
			fmt.Fprintf(&b, "  %s (%s / %s)\n", inst.ID, inst.Service, inst.Role)
		}
	}
	return b.String(), nil
}

func renderInstanceDetail(l InspectData, id string) (string, error) {
	// A real, authored instance.
	for _, n := range l.inv.Nodes {
		for _, inst := range n.Instances {
			if inst.ID != id || inst.Service == "" {
				continue
			}
			return textInstanceDetail(l, id, inst.Service, inst.Role, n.ID, "", inst.Ports, inst.Values)
		}
	}
	// A derived client instance.
	for _, ci := range l.derived.ClientInstances {
		if ci.ID != id {
			continue
		}
		return textInstanceDetail(l, id, ci.Service, ci.Role, ci.Node, ci.User, ci.Ports, ci.Values)
	}
	return "", fmt.Errorf("instance %q not found", id)
}

func textInstanceDetail(l InspectData, id, service, role, node, user string, ports map[string]int, values map[string]any) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "service: %s\nrole: %s\n", service, role)
	if node != "" {
		fmt.Fprintf(&b, "node: %s\n", node)
	}
	if user != "" {
		fmt.Fprintf(&b, "owned by: %s (unmanaged)\n", user)
	}
	if len(ports) > 0 {
		var names []string
		for name := range ports {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Fprintln(&b, "ports:")
		for _, name := range names {
			fmt.Fprintf(&b, "  %s: %d\n", name, ports[name])
		}
	}
	if len(values) > 0 {
		fmt.Fprintln(&b, "values: (template's own — see docs/apps/conf/inventory.md#an-instances-own-values)")
	}

	var routes []string
	for name, r := range l.inv.Routes {
		for _, raw := range r.Hops {
			if hop, err := derive.ParseHop(raw); err == nil && hop.Instance == id {
				routes = append(routes, name)
			}
		}
	}
	sort.Strings(routes)
	if len(routes) > 0 {
		fmt.Fprintf(&b, "\nroutes: %s\n", strings.Join(routes, ", "))
	}

	auth, combineOwn := roleOf(l.manifests, service, role)
	if len(ports) > 0 && auth == confgen.AuthPerPrincipal {
		fmt.Fprintln(&b, "\nsecrets (structure only — Enter on a port to reveal, once milestone 3 lands):")
		var names []string
		for name := range ports {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, port := range names {
			var principalNames []string
			for _, p := range l.derived.Principals(id, port) {
				principalNames = append(principalNames, p.Name)
			}
			fmt.Fprintf(&b, "  %s: %s\n", secretstore.Path{Instance: id, Port: port, Kind: "*", Name: "*"}.String(), strings.Join(principalNames, ", "))
		}
		if combineOwn != "" {
			fmt.Fprintf(&b, "  %s\n", secretstore.Path{Instance: id, Port: secretstore.OwnPort, Name: combineOwn}.String())
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
	if upstream != nil {
		fmt.Fprintf(&b, "\nupstream: %s:%s (%s:%d)\n", upstream.To.Instance, upstream.To.Port, upstream.Address, upstream.Port)
	}

	return b.String(), nil
}

// roleOf looks up a service's role declaration.
func roleOf(manifests map[string]confgen.Manifest, service, role string) (auth, combineOwn string) {
	r := manifests[service].Roles[role]
	return r.Auth, r.CombineOwn
}

func renderUserDetail(l InspectData, id string) (string, error) {
	u, ok := l.inv.Users[id]
	if !ok {
		return "", fmt.Errorf("user %q not found", id)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "username: %s\n", u.UsernameOr(id))
	if u.Devices == inventory.DevicesUnmanaged {
		fmt.Fprintln(&b, "devices: unmanaged — one credential, no node")
	} else {
		fmt.Fprintln(&b, "devices: managed")
		var owned []string
		for _, n := range l.inv.Nodes {
			if n.Owner == id {
				owned = append(owned, n.ID)
			}
		}
		sort.Strings(owned)
		if len(owned) > 0 {
			fmt.Fprintf(&b, "  %s\n", strings.Join(owned, ", "))
		}
	}
	if u.ClientRole != "" {
		fmt.Fprintf(&b, "client_role: %s\n", u.ClientRole)
	}
	fmt.Fprintf(&b, "access: %s\n", strings.Join(u.Access, ", "))

	var derivedIDs []string
	for _, ci := range l.derived.ClientInstances {
		if ci.User == id || (ci.Node != "" && nodeOwner(l.inv, ci.Node) == id) {
			derivedIDs = append(derivedIDs, ci.ID)
		}
	}
	sort.Strings(derivedIDs)
	if len(derivedIDs) > 0 {
		fmt.Fprintf(&b, "\nderived instances: %s\n", strings.Join(derivedIDs, ", "))
	}
	return b.String(), nil
}

func nodeOwner(inv *inventory.Root, nodeID string) string {
	for _, n := range inv.Nodes {
		if n.ID == nodeID {
			return n.Owner
		}
	}
	return ""
}
