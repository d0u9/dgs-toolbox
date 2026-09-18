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

	m.userItems = userTabItems(m.l.inv, m.users)
	m.refresh()
	m.setTab(tabNodes)
	return m
}

// refresh rebuilds the Nodes index from the tree's current fold state and
// feeds it back to the list, keeping the cursor on whatever it was on when
// that row is still visible.
func (m *InspectModel) refresh() {
	selected := ""
	if item, ok := m.list.Selected(); ok {
		selected = item.ID
	}
	m.nodeItems = nodeTabItems(m.nodes)
	if m.tab == tabNodes {
		m.list.SetItems(m.nodeItems)
		if selected != "" {
			m.list.SelectID(selected)
		}
	}
}

// foldable returns the fold state of the row under the cursor when it is a
// node or unmanaged user holding instances, and nil otherwise.
func (m *InspectModel) foldable() *bool {
	if m.tab != tabNodes {
		return nil
	}
	item, ok := m.list.Selected()
	if !ok {
		return nil
	}
	_, name, _ := strings.Cut(item.ID, ":")
	for _, n := range m.nodes {
		if n.name == name && n.broken == "" && len(n.instances) > 0 {
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
	if kind != "inst" {
		return
	}
	for _, n := range m.nodes {
		for _, inst := range n.instances {
			if inst.name != name {
				continue
			}
			id := "node:" + n.name
			if n.user {
				id = "user:" + n.name
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

// nodeTabItems is the Nodes index: one row per node, each followed by the
// instances on it while it is expanded. An unmanaged user has no node file
// and appears here only because the instances derived for them have nowhere
// else to sit, so its row reads as a user and opens the user view — asking
// for a node detail under that name is asking for a file that does not
// exist.
func nodeTabItems(nodes []*nodeGroup) []scrolllist.Item {
	var items []scrolllist.Item
	for _, n := range nodes {
		kind, what := "node", ""
		if n.user {
			kind, what = "user", "unmanaged user, "
		}
		foldable := n.broken == "" && len(n.instances) > 0
		label := foldArrow(n.expanded, foldable) + " " + n.name
		if n.broken != "" {
			items = append(items, scrolllist.Item{ID: kind + ":" + n.name, Label: label, Detail: "  broken: " + n.broken})
			continue
		}
		items = append(items, scrolllist.Item{ID: kind + ":" + n.name, Label: label, Detail: "  " + what + plural(len(n.instances), "instance")})
		if !n.expanded {
			continue
		}
		for i, inst := range n.instances {
			branch, cont := branchMid, trunk
			if i == len(n.instances)-1 {
				branch, cont = branchLast, trunkClosed
			}
			d := inst.detail
			if inst.broken != "" {
				d = "broken: " + inst.broken
			}
			items = append(items, scrolllist.Item{
				ID:     "inst:" + inst.name,
				Label:  "  " + branch + inst.name,
				Detail: "  " + cont + "   " + d,
			})
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
		m.list.HideNumbers(false)
	} else {
		m.list.SetItems(m.nodeItems)
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
		case "w":
			for _, n := range m.nodes {
				n.expanded = false
			}
			m.refresh()
			m.list.First()
			m.detailScroll = 0
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
	indexTitle := "NODES"
	if m.tab == tabUsers {
		indexTitle = "USERS"
	}
	leftBox := fieldset.ViewFocused(indexTitle, text.Fit(list.View(true, titleStyle, mutedStyle), height, left-4), left, true)

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
	right := "[/] Tab  ↑↓ Move  ctrl+d/u Scroll"
	if m.tab == tabNodes {
		right = "[/] Tab  ↑↓ Move  h/l Fold  w Fold all"
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
	// Everything that runs here, authored and derived alike. An authored
	// entry with no service is an override of a derived instance, not an
	// instance of its own, so it is listed once, from the derivation that
	// gives it its service and role.
	type row struct{ id, detail string }
	var rows []row
	for _, inst := range n.Instances {
		if inst.Service == "" && inst.Role == "" {
			continue
		}
		rows = append(rows, row{inst.ID, inst.Service + " / " + inst.Role})
	}
	for _, ci := range l.derived.ClientInstances {
		if ci.Node == id {
			rows = append(rows, row{ci.ID, ci.Service + " / " + ci.Role + "  (derived)"})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	if len(rows) > 0 {
		fmt.Fprintln(&b, "\ninstances:")
		for _, r := range rows {
			fmt.Fprintf(&b, "  %-24s %s\n", r.id, r.detail)
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
		// The service's own parameters, opaque to dgs — see
		// docs/apps/conf/inventory.md#an-instances-own-values. They are
		// printed as written, since naming a key without its value says
		// nothing a reader could not get from the node file.
		var names []string
		for name := range values {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Fprintln(&b, "values:")
		for _, name := range names {
			fmt.Fprintf(&b, "  %s: %v\n", name, values[name])
		}
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
		fmt.Fprintf(&b, "\nhop of: %s\n", strings.Join(routes, ", "))
	}
	for _, ci := range l.derived.ClientInstances {
		if ci.ID == id {
			// A derived client enters a route rather than being a hop of
			// one, so the scan above never finds it.
			fmt.Fprintf(&b, "\nderived for route: %s\n", ci.Route)
		}
	}

	// Every secret this instance holds, by path and never by value: one per
	// principal on a per-principal port, plus the role's own list. The paths
	// are the real ones docs/apps/conf/inventory.md#secrets derives, so a
	// reader can go straight to the file.
	r := l.manifests[service].Roles[role]
	var secretLines []string
	if r.Auth == confgen.AuthPerPrincipal {
		var names []string
		for name := range ports {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, port := range names {
			for _, p := range l.derived.Principals(id, port) {
				path := secretstore.Path{Instance: id, Port: port, Kind: string(p.Kind), Name: p.ID}
				secretLines = append(secretLines, fmt.Sprintf("  %-48s %s", path.String(), p.Name))
			}
		}
	}
	for _, name := range r.Own {
		path := secretstore.Path{Instance: id, Port: secretstore.OwnPort, Name: name}
		note := "own"
		if name == r.CombineOwn {
			note = "own, combined into every client"
		}
		secretLines = append(secretLines, fmt.Sprintf("  %-48s %s", path.String(), note))
	}
	if len(secretLines) > 0 {
		fmt.Fprintf(&b, "\nsecrets (%s, values not shown):\n", plural(len(secretLines), "file"))
		fmt.Fprintln(&b, strings.Join(secretLines, "\n"))
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
	if name := u.UsernameOr(id); name != id {
		fmt.Fprintf(&b, "username: %s\n", name)
	}
	if u.ClientRole != "" {
		fmt.Fprintf(&b, "client_role: %s\n", u.ClientRole)
	}

	// What each granted route reaches, so a route name means something
	// here: the entry hop is where this person's traffic goes in.
	fmt.Fprintln(&b, "\naccess:")
	for _, name := range u.Access {
		r, ok := l.inv.Routes[name]
		if !ok || len(r.Hops) == 0 {
			fmt.Fprintf(&b, "  %-12s (no such route)\n", name)
			continue
		}
		fmt.Fprintf(&b, "  %-12s enters %s\n", name, r.Hops[0])
	}

	if u.Devices == inventory.DevicesUnmanaged {
		fmt.Fprintln(&b, "\ndevices: unmanaged — one credential per route, no node file")
		writeUserInstances(&b, l, id, "", u.Access, "  ")
	} else {
		var owned []string
		for _, n := range l.inv.Nodes {
			if n.Owner == id {
				owned = append(owned, n.ID)
			}
		}
		sort.Strings(owned)
		fmt.Fprintf(&b, "\ndevices: managed — %s, one credential each\n", plural(len(owned), "device"))
		for _, node := range owned {
			fmt.Fprintf(&b, "  %s\n", node)
			writeUserInstances(&b, l, "", node, u.Access, "    ")
		}
	}

	// Every credential this person holds, by path. The account name is what
	// the server's own table calls them, which is the user's username for
	// an unmanaged user and "<node>-<route>"'s device for a managed one.
	var lines []string
	for _, g := range l.derived.Grants {
		switch {
		case g.Principal.Kind == derive.PrincipalUser && g.Principal.ID == id:
		case g.Principal.Kind == derive.PrincipalNode && nodeOwner(l.inv, g.Principal.ID) == id:
		default:
			continue
		}
		path := secretstore.Path{Instance: g.Instance, Port: g.Port, Kind: string(g.Principal.Kind), Name: g.Principal.ID}
		lines = append(lines, fmt.Sprintf("  %-44s %s", path.String(), g.Principal.Name))
	}
	sort.Strings(lines)
	if len(lines) > 0 {
		fmt.Fprintf(&b, "\ngrants (%s, values not shown):\n", plural(len(lines), "credential"))
		fmt.Fprintln(&b, strings.Join(lines, "\n"))
	}
	return b.String(), nil
}

// writeUserInstances lists what each granted route derives for one unmanaged
// user or one managed device — one line per route, whether or not it derives
// anything. A route whose entry role declares no reached_by renders no client
// file, and the person still holds a credential for it: listing only the
// instances that exist would make the two cases look like one route missing.
// See docs/apps/conf/inventory.md#what-is-derived.
func writeUserInstances(b *strings.Builder, l InspectData, user, node string, access []string, indent string) {
	for _, route := range access {
		var found *derive.ClientInstance
		for i := range l.derived.ClientInstances {
			ci := &l.derived.ClientInstances[i]
			if ci.Route != route {
				continue
			}
			if (user != "" && ci.User == user) || (node != "" && ci.Node == node) {
				found = ci
			}
		}
		if found != nil {
			fmt.Fprintf(b, "%s%-12s %-28s %s / %s\n", indent, route, found.ID, found.Service, found.Role)
			continue
		}
		fmt.Fprintf(b, "%s%-12s %-28s %s\n", indent, route, "—", noClientReason(l, route))
	}
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
			if l.manifests[inst.Service].Roles[inst.Role].ReachedBy == "" {
				return fmt.Sprintf("no client file (%s/%s declares no reached_by)", inst.Service, inst.Role)
			}
		}
	}
	return "no client file"
}

func nodeOwner(inv *inventory.Root, nodeID string) string {
	for _, n := range inv.Nodes {
		if n.ID == nodeID {
			return n.Owner
		}
	}
	return ""
}
