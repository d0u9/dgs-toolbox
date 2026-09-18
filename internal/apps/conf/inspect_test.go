package conf

import (
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
)

// buildInspectRoot is a generator root with one server node (two ports, one
// per-principal), one managed client device, one unmanaged user, and a
// broken node — enough to exercise all four views' text.
func buildInspectRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "services", "shadowsocks-rust", "confgen.yaml"), `
secret:
  kind: base64
  bytes: 32
roles:
  server:
    template: templates/server.json.tmpl
    defaults: element
    output: config.json
    auth: per-principal
    reached_by: ss-rust
    own: [psk]
    combine_own: psk
  ss-rust:
    template: templates/client.json.tmpl
    defaults: element
    output: config.json
    auth: none
`)
	writeFile(t, filepath.Join(dir, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: shadowsocks-rust
    role: server
    ports:
      main: 38250
`)
	writeFile(t, filepath.Join(dir, "nodes", "laptop.yaml"), `
id: laptop
owner: doug
`)
	writeFile(t, filepath.Join(dir, "nodes", "bad.yaml"), "id: [unterminated\n")
	writeFile(t, filepath.Join(dir, "users.yaml"), `
users:
  doug:
    access: [sfo]
  yak:
    devices: unmanaged
    access: [sfo]
`)
	writeFile(t, filepath.Join(dir, "routes.yaml"), `
routes:
  sfo:
    hops: [ss-srv:main]
`)
	writeFile(t, filepath.Join(dir, "networks.yaml"), `
networks: [internet]
universal: internet
`)
	return dir
}

func TestInspect_NoRootConfigured(t *testing.T) {
	m := newInspectModel("", "")
	if !strings.Contains(m.View(), "conf.root is not configured") {
		t.Fatalf("View() = %q, want the no-root message", m.View())
	}
	if m.Status().Center != "no generator root" {
		t.Fatalf("Status().Center = %q", m.Status().Center)
	}
}

func TestInspect_LoadErrorIsShown(t *testing.T) {
	m := newInspectModel(filepath.Join(t.TempDir(), "does-not-exist"), "")
	if m.loadErr == nil {
		t.Fatal("loadErr = nil, want an error for a missing root")
	}
	if !strings.Contains(m.View(), m.loadErr.Error()) {
		t.Fatal("View() does not contain the load error")
	}
}

func TestInspect_IndexListsNodesInstancesAndUsers(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24

	var ids []string
	for _, n := range m.nodes {
		ids = append(ids, "node:"+n.name)
		for _, inst := range n.instances {
			ids = append(ids, "inst:"+inst.name)
		}
	}
	for _, u := range m.users {
		ids = append(ids, "user:"+u)
	}

	want := map[string]bool{
		"node:srv": true, "inst:ss-srv": true,
		"node:laptop": true, "inst:laptop-sfo": true,
		"node:nodes/bad.yaml": true,
		// yak is unmanaged, so it appears twice: once as buildTree's
		// pseudo-node holding its derived instance (matching export's own
		// tree, on the Nodes tab), and once on the Users tab.
		"node:yak": true, "inst:yak-sfo": true,
		"user:doug": true, "user:yak": true,
	}
	if len(ids) != len(want) {
		t.Fatalf("index ids = %v, want %v distinct entries", ids, len(want))
	}
	for _, id := range ids {
		if !want[id] {
			t.Fatalf("unexpected id %q in index", id)
		}
	}
}

// select moves to the tab id belongs to and its row, returning the resulting
// detail — the way the two-column view reads it: live, from wherever the
// cursor sits.
func selectDetail(t *testing.T, m InspectModel, id string) (kind, body string) {
	t.Helper()
	if strings.HasPrefix(id, "user:") {
		m.setTab(tabUsers)
	}
	if !m.list.SelectID(id) {
		t.Fatalf("SelectID(%q): row not found", id)
	}
	kind, gotID, body, err := m.currentDetail()
	if err != nil {
		t.Fatalf("currentDetail(%q): %v", id, err)
	}
	if "node:"+gotID != id && "inst:"+gotID != id && "user:"+gotID != id {
		t.Fatalf("currentDetail id = %q, want it to match selection %q", gotID, id)
	}
	return kind, body
}

func TestInspect_NodeDetailShowsNetworksAndInstances(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	kind, body := selectDetail(t, m, "node:srv")

	if kind != "node" {
		t.Fatalf("kind = %q, want node", kind)
	}
	if !strings.Contains(body, "203.0.113.10") {
		t.Fatalf("node detail = %q, want the network address", body)
	}
	if !strings.Contains(body, "ss-srv") {
		t.Fatalf("node detail = %q, want its instance listed", body)
	}
}

func TestInspect_InstanceDetailShowsRouteAndSecretStructure(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	kind, body := selectDetail(t, m, "inst:ss-srv")

	if kind != "inst" {
		t.Fatalf("kind = %q, want inst", kind)
	}
	if !strings.Contains(body, "shadowsocks-rust") || !strings.Contains(body, "server") {
		t.Fatalf("instance detail = %q, want service and role", body)
	}
	if !strings.Contains(body, "sfo") {
		t.Fatalf("instance detail = %q, want the route it is a hop of", body)
	}
	if !strings.Contains(body, "ss-srv/main/") {
		t.Fatalf("instance detail = %q, want its secret path structure, no value", body)
	}
	if !strings.Contains(body, "ss-srv/own/psk") {
		t.Fatalf("instance detail = %q, want the combine_own path listed", body)
	}
	if strings.Contains(body, "-----BEGIN") {
		t.Fatal("instance detail leaked something that looks like a secret value")
	}
}

func TestInspect_DerivedClientInstanceDetailShowsUpstream(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	_, body := selectDetail(t, m, "inst:laptop-sfo")

	if !strings.Contains(body, "upstream: ss-srv:main") {
		t.Fatalf("instance detail = %q, want its upstream", body)
	}
}

func TestInspect_ManagedUserDetailShowsDevicesAndDerived(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	kind, body := selectDetail(t, m, "user:doug")

	if kind != "user" {
		t.Fatalf("kind = %q, want user", kind)
	}
	if !strings.Contains(body, "managed") || !strings.Contains(body, "laptop") {
		t.Fatalf("user detail = %q, want managed and the owned node", body)
	}
	if !strings.Contains(body, "laptop-sfo") {
		t.Fatalf("user detail = %q, want the derived instance", body)
	}
}

func TestInspect_UnmanagedUserDetail(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	_, body := selectDetail(t, m, "user:yak")

	if !strings.Contains(body, "unmanaged") {
		t.Fatalf("user detail = %q, want unmanaged", body)
	}
	if !strings.Contains(body, "yak-sfo") {
		t.Fatalf("user detail = %q, want yak's derived instance", body)
	}
}

func TestInspect_BrokenNodeDetailShowsParseError(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	_, body := selectDetail(t, m, "node:nodes/bad.yaml")

	if !strings.Contains(body, "broken:") {
		t.Fatalf("broken node detail = %q, want its parse error", body)
	}
}

func TestInspect_TabsStartOnNodesAndBracketsCycle(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24

	tabs := m.Tabs()
	if len(tabs) != 2 || tabs[0].Label != "Nodes" || !tabs[0].Active || tabs[1].Active {
		t.Fatalf("Tabs() = %+v, want Nodes active first", tabs)
	}
	if _, ok := m.list.Selected(); !ok {
		t.Fatal("no row selected on the Nodes tab at startup")
	}

	m = pressInspect(t, m, "]")
	tabs = m.Tabs()
	if tabs[0].Active || !tabs[1].Active {
		t.Fatalf("Tabs() after ] = %+v, want Users active", tabs)
	}
	item, ok := m.list.Selected()
	if !ok || !strings.HasPrefix(item.ID, "user:") {
		t.Fatalf("selected row after switching to Users = %+v, want a user: row", item)
	}

	m = pressInspect(t, m, "[")
	tabs = m.Tabs()
	if !tabs[0].Active {
		t.Fatal("] then [ did not return to Nodes")
	}
}

func TestInspect_TabSelectedMsgSwitchesTabs(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24

	next, _ := m.Update(tui.TabSelectedMsg{Index: tabUsers})
	m = next.(InspectModel)
	if !m.Tabs()[1].Active {
		t.Fatal("TabSelectedMsg{Index: tabUsers} did not activate the Users tab")
	}
}

func pressInspect(t *testing.T, m InspectModel, key string) InspectModel {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return next.(InspectModel)
}

func TestInspect_ViewRendersBothColumns(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.list.SelectID("inst:ss-srv")

	view := m.View()
	if !strings.Contains(view, "NODES") {
		t.Fatalf("View() = %q, want the left column's fieldset legend naming the active tab", view)
	}
	if !strings.Contains(view, "ss-srv") {
		t.Fatalf("View() = %q, want the selected instance's detail on the right", view)
	}
}

// TestInspect_UnmanagedUserRowOpensTheUserView covers the Nodes index's one
// entry that is not a node. An unmanaged user has no node file, and appears
// there only because the instances derived for them have nowhere else to
// sit; a row asking for a node detail under that name asked for a file that
// does not exist, and showed an error where every other row showed content.
func TestInspect_UnmanagedUserRowOpensTheUserView(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24

	var row *scrolllist.Item
	for i := range m.nodeItems {
		if strings.HasSuffix(m.nodeItems[i].ID, ":yak") {
			row = &m.nodeItems[i]
		}
	}
	if row == nil {
		t.Fatal("the Nodes index has no entry for the unmanaged user yak")
	}
	if row.ID != "user:yak" {
		t.Fatalf("row ID = %q, want user:yak — an unmanaged user is not a node", row.ID)
	}
	if !strings.Contains(row.Detail, "unmanaged user") {
		t.Fatalf("row detail = %q, want it to say what this entry is", row.Detail)
	}

	kind, body := selectDetail(t, m, "user:yak")
	if kind != "user" {
		t.Fatalf("kind = %q, want user", kind)
	}
	if !strings.Contains(body, "unmanaged") {
		t.Fatalf("detail = %q, want the user view", body)
	}
}

// TestInspect_NodeDetailListsDerivedInstances covers a client node, whose
// instances are all derived: the authored entries on it are overrides with
// no service or role of their own, so listing only those printed an
// "instances:" heading with nothing under it.
func TestInspect_NodeDetailListsDerivedInstances(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	_, body := selectDetail(t, m, "node:laptop")

	if !strings.Contains(body, "laptop-sfo") {
		t.Fatalf("node detail = %q, want the instance derived for it listed", body)
	}
	if !strings.Contains(body, "(derived)") {
		t.Fatalf("node detail = %q, want a derived instance marked as one", body)
	}
}

// TestInspect_NodesIndexDrawsInstancesOnBranches covers the Nodes index
// reading as a tree. The list component numbers every visible row the same
// way, so a node and its instances are equals to it; the branch characters
// are the only thing carrying depth, and the detail line under an instance
// has to continue the trunk or the run comes apart.
func TestInspect_NodesIndexDrawsInstancesOnBranches(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24

	var node, instance *scrolllist.Item
	for i := range m.nodeItems {
		switch m.nodeItems[i].ID {
		case "node:srv":
			node = &m.nodeItems[i]
		case "inst:ss-srv":
			instance = &m.nodeItems[i]
		}
	}
	if node == nil || instance == nil {
		t.Fatalf("index = %+v, want a node row and its instance row", m.nodeItems)
	}
	if !strings.HasPrefix(node.Label, "▾ ") {
		t.Fatalf("node label = %q, want an open disclosure marker", node.Label)
	}
	if !strings.Contains(instance.Label, branchLast) && !strings.Contains(instance.Label, branchMid) {
		t.Fatalf("instance label = %q, want it hung off a branch", instance.Label)
	}
}

// TestInspect_FoldingHidesAndShowsInstances covers the disclosure marker
// doing something. It was drawn as a static "▸" with no key bound to it, so
// it said a node could be folded and nothing folded it.
func TestInspect_FoldingHidesAndShowsInstances(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.list.SelectID("node:srv")

	folded := pressInspect(t, m, "h")
	if indexHas(folded.nodeItems, "inst:ss-srv") {
		t.Fatalf("index = %+v, want srv's instances hidden once folded", folded.nodeItems)
	}
	if !strings.HasPrefix(itemByID(t, folded.nodeItems, "node:srv").Label, "▸ ") {
		t.Fatal("a folded node still shows an open disclosure marker")
	}

	reopened := pressInspect(t, folded, "l")
	if !indexHas(reopened.nodeItems, "inst:ss-srv") {
		t.Fatalf("index = %+v, want srv's instances back once unfolded", reopened.nodeItems)
	}

	all := pressInspect(t, reopened, "w")
	for _, it := range all.nodeItems {
		if strings.HasPrefix(it.ID, "inst:") {
			t.Fatalf("index = %+v, want w to fold every node", all.nodeItems)
		}
	}
}

// TestInspect_FoldingLeftFromAnInstanceGoesToItsNode covers h on a row with
// nothing to fold, which is how the File Explorer's keys leave a child.
func TestInspect_FoldingLeftFromAnInstanceGoesToItsNode(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.list.SelectID("inst:ss-srv")

	up := pressInspect(t, m, "h")
	item, ok := up.list.Selected()
	if !ok || item.ID != "node:srv" {
		t.Fatalf("selected = %+v, want the node holding ss-srv", item)
	}
}

func indexHas(items []scrolllist.Item, id string) bool {
	for _, it := range items {
		if it.ID == id {
			return true
		}
	}
	return false
}

func itemByID(t *testing.T, items []scrolllist.Item, id string) scrolllist.Item {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("index has no %q", id)
	return scrolllist.Item{}
}

// TestInspect_UserDetailNamesEveryGrantedRoute covers a route that derives
// no client instance. hysteria2's server role declares no reached_by, so a
// person granted a route into it holds a credential and gets no file; the
// view listed only the instances that exist, which made the route look
// missing rather than answered.
func TestInspect_UserDetailNamesEveryGrantedRoute(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	_, body := selectDetail(t, m, "user:yak")

	if !strings.Contains(body, "enters ss-srv:main") {
		t.Fatalf("user detail = %q, want each granted route's entry hop", body)
	}
	if !strings.Contains(body, "yak-sfo") {
		t.Fatalf("user detail = %q, want the instance derived for the route", body)
	}
	if !strings.Contains(body, "ss-srv/main/user/yak") {
		t.Fatalf("user detail = %q, want the credential this person holds, by path", body)
	}
	if strings.Contains(body, "username: yak") {
		t.Fatal("user detail printed a username equal to the identifier, which says nothing")
	}
}

// TestInspect_UserDetailListsDevicesWithWhatEachDerives covers a managed
// user: docs/apps/conf/inspect.md asks for their devices and the instance
// each derives, which means the two together rather than a device list and
// a separate instance list the reader has to join by hand.
func TestInspect_UserDetailListsDevicesWithWhatEachDerives(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	_, body := selectDetail(t, m, "user:doug")

	device := strings.Index(body, "laptop")
	instance := strings.Index(body, "laptop-sfo")
	if device < 0 || instance < 0 || instance < device {
		t.Fatalf("user detail = %q, want the device and then what it derives", body)
	}
	if !strings.Contains(body, "doug-laptop") {
		t.Fatalf("user detail = %q, want the account name the server sees", body)
	}
}

// TestInspect_UsersIndexKeepsItsNumbers covers the two indexes wanting
// opposite answers: the Nodes index is a tree and hides them, while the
// Users index is a flat collection where a number is what makes a row easy
// to point at.
func TestInspect_UsersIndexKeepsItsNumbers(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24

	if strings.Contains(m.View(), " 1 ▾") == false && strings.Contains(m.View(), "▾") == false {
		t.Fatal("the Nodes index lost its disclosure markers")
	}
	if m.list.LabelOffset() != 2 {
		t.Fatalf("Nodes LabelOffset = %d, want no number column", m.list.LabelOffset())
	}

	m.setTab(tabUsers)
	if m.list.LabelOffset() == 2 {
		t.Fatal("the Users index lost its numbers, which a flat list keeps")
	}
	if !strings.Contains(m.View(), "1 ") {
		t.Fatalf("View() = %q, want numbered rows on the Users index", m.View())
	}
}
