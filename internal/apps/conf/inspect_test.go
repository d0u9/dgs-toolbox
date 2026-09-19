package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
)

// buildInspectRoot is a generator root with one server node whose instance
// listens on two named ports, one managed client device, one unmanaged
// user, and a broken node — enough to exercise every view's text. The
// second port has no route into it on purpose: an instance's ports are its
// own, and one nothing reaches yet is a listener with an empty account
// table rather than a mistake.
func buildInspectRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "services", "ssserver", "confgen.yaml"), `
secret:
  kind: base64
  bytes: 32
template: templates/config.json.tmpl
defaults: element
output: config.json
auth: per-principal
self:
  psk: {set: true}
`)
	writeFile(t, filepath.Join(dir, "services", "ssserver", "exports", "ss-json", "confgen.yaml"), `
template: templates/config.json.tmpl
defaults: element
output: config.json
`)
	writeFile(t, filepath.Join(dir, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: ssserver
    ports:
      main: {port: 38250, self: [psk.main]}
      alt: {port: 49217, self: [psk.alt]}
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
    devices: none
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
		ids = append(ids, "node:"+n.key)
		for _, inst := range n.instances {
			ids = append(ids, "inst:"+inst.name)
		}
	}
	for _, u := range m.users {
		ids = append(ids, "user:"+u)
	}

	want := map[string]bool{
		"node:srv": true, "inst:ss-srv": true,
		"node:laptop": true, "inst:laptop-sfo-ssserver-ss-json": true,
		"node:nodes/bad.yaml": true,
		// yak is unmanaged, so it appears twice: once as buildTree's
		// pseudo-node holding its derived instance (matching export's own
		// tree, on the Nodes tab), and once on the Users tab.
		"node:yak": true, "inst:yak-default-sfo-ssserver-ss-json": true,
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
	if strings.HasPrefix(id, "service:") {
		m.setTab(tabServices)
	}
	if !m.list.SelectID(id) {
		t.Fatalf("SelectID(%q): row not found", id)
	}
	kind, gotID, body, err := m.currentDetail()
	if err != nil {
		t.Fatalf("currentDetail(%q): %v", id, err)
	}
	if kind+":"+gotID != id {
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
	if !strings.Contains(body, "ssserver") || !strings.Contains(body, "server") {
		t.Fatalf("instance detail = %q, want the service", body)
	}
	if !strings.Contains(body, "sfo") {
		t.Fatalf("instance detail = %q, want the route it is a hop of", body)
	}
	if !strings.Contains(body, "ss-srv/main/") {
		t.Fatalf("instance detail = %q, want its secret path structure, no value", body)
	}
	if !strings.Contains(body, "ss-srv/self/psk") {
		t.Fatalf("instance detail = %q, want the shared path listed", body)
	}
	if strings.Contains(body, "-----BEGIN") {
		t.Fatal("instance detail leaked something that looks like a secret value")
	}
}

func TestInspect_DerivedExportInstanceDetailShowsUpstream(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	_, body := selectDetail(t, m, "inst:laptop-sfo-ssserver-ss-json")

	if !strings.Contains(body, "upstream ss-srv:main") {
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
	if !strings.Contains(body, "1 device") || !strings.Contains(body, "laptop") {
		t.Fatalf("user detail = %q, want the device count and the owned node", body)
	}
	if !strings.Contains(body, "laptop-sfo-ssserver-ss-json") {
		t.Fatalf("user detail = %q, want the derived instance", body)
	}
}

func TestInspect_UnmanagedUserDetail(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24
	_, body := selectDetail(t, m, "user:yak")

	if !strings.Contains(body, "no node file") || !strings.Contains(body, "carries default") {
		t.Fatalf("user detail = %q, want the absent node file and the carried credential", body)
	}
	if !strings.Contains(body, "yak-default-sfo-ssserver-ss-json") {
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
	root, secrets := buildInspectRoot(t), buildSecretsDir(t)
	m := newInspectModel(root, secrets)
	m.width, m.height = 80, 24

	tabs := m.Tabs()
	if len(tabs) != tabCount || tabs[0].Label != "Nodes" || !tabs[0].Active || tabs[1].Active {
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

	m = pressInspect(t, m, "]")
	tabs = m.Tabs()
	if !tabs[tabServices].Active {
		t.Fatalf("Tabs() after ]] = %+v, want Services active", tabs)
	}
	item, ok = m.list.Selected()
	if !ok || !strings.HasPrefix(item.ID, "service:") {
		t.Fatalf("selected row after switching to Services = %+v, want a service: row", item)
	}

	m = pressInspect(t, m, "]")
	if !m.Tabs()[tabSecrets].Active {
		t.Fatalf("Tabs() after ]]] = %+v, want Secrets active", m.Tabs())
	}
	item, ok = m.list.Selected()
	if !ok || !strings.HasPrefix(item.ID, "secret") {
		t.Fatalf("selected row after switching to Secrets = %+v, want a secret row", item)
	}

	m = pressInspect(t, m, "]")
	if !m.Tabs()[tabNodes].Active {
		t.Fatal("] past the last tab did not wrap round to Nodes")
	}
	m = pressInspect(t, m, "[")
	if !m.Tabs()[tabSecrets].Active {
		t.Fatal("[ from Nodes did not wrap round to Secrets")
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

// TestInspect_NodesHoldsOnlyMachines covers the split between the two trees.
// The Nodes index is machines: a person with no device file has no node file
// behind them, so nothing of theirs appears there. What is rendered for them
// is on the Users index, which answers the other question.
func TestInspect_NodesHoldsOnlyMachines(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 80, 24

	for _, it := range m.nodeItems {
		if strings.HasSuffix(it.ID, ":yak") {
			t.Fatalf("the Nodes index holds %q, and yak has no node file", it.ID)
		}
		if strings.Contains(it.Detail, "credential") {
			t.Fatalf("Nodes row %q reads %q, and a machine is not a credential", it.ID, it.Detail)
		}
	}

	var row *scrolllist.Item
	for i := range m.userItems {
		if m.userItems[i].ID == "user:yak" {
			row = &m.userItems[i]
		}
	}
	if row == nil {
		t.Fatal("the Users index has no entry for yak")
	}
	if !strings.Contains(row.Detail, "credential") {
		t.Fatalf("row detail = %q, want what their files hang off", row.Detail)
	}
	if strings.Contains(row.Detail, "node") {
		t.Fatalf("row detail = %q, want no mention of a node for someone with no node file", row.Detail)
	}

	kind, body := selectDetail(t, m, "user:yak")
	if kind != "user" {
		t.Fatalf("kind = %q, want user", kind)
	}
	if !strings.Contains(body, "no node file") {
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

	if !strings.Contains(body, "laptop-sfo-ssserver-ss-json") {
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
	if !strings.Contains(body, "yak-default-sfo-ssserver-ss-json") {
		t.Fatalf("user detail = %q, want the instance derived for the route", body)
	}
	if !strings.Contains(body, "ss-srv/main/yak/default") {
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
	instance := strings.Index(body, "laptop-sfo-ssserver-ss-json")
	if device < 0 || instance < 0 || instance < device {
		t.Fatalf("user detail = %q, want the device and then what it derives", body)
	}
	if !strings.Contains(body, "doug-default") {
		t.Fatalf("user detail = %q, want the account name the server sees", body)
	}
}

// TestInspect_UsersIndexKeepsItsNumbers covers the two indexes wanting
// opposite answers: the Nodes index is a tree and hides them, while the
// Users index is a flat collection where a number is what makes a row easy
// to point at.
func TestInspect_BothTreesHideTheirNumbers(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24

	if strings.Contains(m.View(), " 1 ▾") == false && strings.Contains(m.View(), "▾") == false {
		t.Fatal("the Nodes index lost its disclosure markers")
	}
	if m.list.LabelOffset() != 2 {
		t.Fatalf("Nodes LabelOffset = %d, want no number column", m.list.LabelOffset())
	}

	// The Users index is a tree too now, so it hides its numbers the same
	// way. See docs/scroll-lists.md#line-numbers.
	m.setTab(tabUsers)
	if m.list.LabelOffset() != 2 {
		t.Fatalf("Users LabelOffset = %d, want no number column on a tree", m.list.LabelOffset())
	}
}

// TestInspect_ServicesIndexIsDeployedInstancesAndTheirPorts covers what
// the Services index is for: where a service runs. A derived client
// instance is a file handed to a person, not a deployment, and it belongs
// under that person in the Users tab — listing it here made one Shadowsocks
// server serving five people look like six instances of the service.
func TestInspect_ServicesIndexIsDeployedInstancesAndTheirPorts(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.setTab(tabServices)

	if indexHas(m.serviceItems, "inst:yak-default-sfo-ssserver-ss-json") {
		t.Fatalf("index = %+v, want no client file among the deployments", m.serviceItems)
	}
	if !indexHas(m.serviceItems, "inst:ss-srv") {
		t.Fatalf("index = %+v, want the deployed instance", m.serviceItems)
	}

	row := itemByID(t, m.serviceItems, "service:ssserver")
	if !strings.Contains(row.Detail, "1 instance") || !strings.Contains(row.Detail, "2 ports") {
		t.Fatalf("service row = %q, want one deployment and its ports counted", row.Detail)
	}
	if !strings.Contains(row.Detail, "file") {
		t.Fatalf("service row = %q, want the files for people counted separately", row.Detail)
	}
}

// TestInspect_ServicesIndexGoesDownToPorts covers the third level. A port
// is where an account table lives: two ports of one process are two
// independent sets of credentials, and an index stopping at the instance
// cannot show which people are on which.
func TestInspect_ServicesIndexGoesDownToPorts(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.setTab(tabServices)

	main := itemByID(t, m.serviceItems, "port:ss-srv/main")
	if !strings.Contains(main.Detail, "38250") {
		t.Fatalf("port row = %q, want the number it listens on", main.Detail)
	}
	if !strings.Contains(main.Detail, "yak-default") || !strings.Contains(main.Detail, "doug-default") {
		t.Fatalf("port row = %q, want the account names on this port", main.Detail)
	}

	alt := itemByID(t, m.serviceItems, "port:ss-srv/alt")
	if !strings.Contains(alt.Detail, "nobody yet") {
		t.Fatalf("port row = %q, want a port nothing reaches to say so", alt.Detail)
	}
}

// TestInspect_PortDetailIsItsOwnAccountTable covers a port view. The
// credentials on one port say nothing about the port beside it, so the
// view is per port rather than per instance.
func TestInspect_PortDetailIsItsOwnAccountTable(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.setTab(tabServices)
	_, body := selectDetail(t, m, "port:ss-srv/main")

	for _, want := range []string{"38250", "ss-srv", "srv", "sfo", "yak-default", "ss-srv/main/yak/default"} {
		if !strings.Contains(body, want) {
			t.Fatalf("port detail = %q, want it to name %q", body, want)
		}
	}
	if strings.Contains(body, "ss-srv/alt") {
		t.Fatal("the port view showed another port's credentials")
	}

	_, empty := selectDetail(t, m, "port:ss-srv/alt")
	if !strings.Contains(empty, "nobody holds a grant") {
		t.Fatalf("port detail = %q, want a port nothing reaches to say so", empty)
	}
}

// TestInspect_ServiceDetailShowsManifestAndDeployment covers the service
// view holding the half of the manifest no other view has room for — a
// role's template, output name and own secrets — beside where each role is
// deployed, or how many files it renders when it is a client role.
func TestInspect_ServiceDetailShowsManifestAndDeployment(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.setTab(tabServices)
	_, body := selectDetail(t, m, "service:ssserver")

	for _, want := range []string{
		"per-principal",
		"written out as ss-json",
		"templates/config.json.tmpl",
		"config.json",
		"psk",
		"ss-srv on srv",
		"38250",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("service detail = %q, want it to name %q", body, want)
		}
	}
}

// TestInspect_ServicesIndexFolds covers the Services index folding the same
// way the Nodes index does — it is the same shape, so it takes the same
// keys rather than a second set.
func TestInspect_ServicesIndexFolds(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.setTab(tabServices)
	m.list.SelectID("service:ssserver")

	folded := pressInspect(t, m, "h")
	if indexHas(folded.serviceItems, "inst:ss-srv") {
		t.Fatalf("index = %+v, want the service's instances hidden once folded", folded.serviceItems)
	}
	reopened := pressInspect(t, folded, "l")
	if !indexHas(reopened.serviceItems, "inst:ss-srv") {
		t.Fatalf("index = %+v, want them back once unfolded", reopened.serviceItems)
	}
}

// TestInspect_LeftFromAPortGoesToItsInstance covers h on the third level.
// The Services tree is one deeper than the Nodes tree, so leaving a child
// has one more step than it used to.
func TestInspect_LeftFromAPortGoesToItsInstance(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.setTab(tabServices)
	m.list.SelectID("port:ss-srv/main")

	up := pressInspect(t, m, "h")
	item, ok := up.list.Selected()
	if !ok || item.ID != "inst:ss-srv" {
		t.Fatalf("selected = %+v, want the instance listening on that port", item)
	}

	up = pressInspect(t, up, "h")
	item, ok = up.list.Selected()
	if !ok || item.ID != "service:ssserver" {
		t.Fatalf("selected = %+v, want the service holding that instance", item)
	}
}

// buildSecretsDir is a secrets store for buildInspectRoot's inventory, with
// one of each state the Secrets tab distinguishes: a credential in step, one
// the inventory implies and the store does not hold, one the store holds and
// nothing implies, and one mid-rotation.
func buildSecretsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, value string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ss-srv/main/yak/default", "in-step")
	write("ss-srv/main/yak/default.previous", "rotating")
	write("ss-srv/self/psk", "in-step")
	write("ss-srv/main/user/nobody", "orphan")
	// ss-srv/main/doug/default is deliberately absent: it is what the
	// inventory implies and the store does not hold.
	return dir
}

// TestInspect_SecretsTabComparesBothDirections covers the Secrets tab's
// reason to exist. Every other view reads the inventory alone; this one
// reads the store too, and the two ways they can disagree are each silent
// everywhere else — a path with no file renders nothing, and a file nothing
// implies is a credential sync will never regenerate.
func TestInspect_SecretsTabComparesBothDirections(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), buildSecretsDir(t))
	m.width, m.height = 100, 24
	m.setTab(tabSecrets)

	if m.secrets.loadErr != nil {
		t.Fatalf("buildSecrets: %v", m.secrets.loadErr)
	}
	if m.secrets.present == 0 || m.secrets.missing == 0 || m.secrets.orphaned == 0 {
		t.Fatalf("counts = present %d, missing %d, orphaned %d — want one of each",
			m.secrets.present, m.secrets.missing, m.secrets.orphaned)
	}

	missing := itemByID(t, m.secretItems, "secret:ss-srv/main/doug/default")
	if !strings.Contains(missing.Detail, "missing") {
		t.Fatalf("row = %q, want a path the store does not hold to say so", missing.Detail)
	}
	orphaned := itemByID(t, m.secretItems, "secret:ss-srv/main/user/nobody")
	if !strings.Contains(orphaned.Detail, "orphaned") {
		t.Fatalf("row = %q, want a file nothing implies to say so", orphaned.Detail)
	}
	rotating := itemByID(t, m.secretItems, "secret:ss-srv/main/yak/default")
	if !strings.Contains(rotating.Detail, ".previous") {
		t.Fatalf("row = %q, want the rotation leftover named", rotating.Detail)
	}

	status := m.Status().Center
	if !strings.Contains(status, "missing") || !strings.Contains(status, "orphaned") {
		t.Fatalf("status = %q, want what is out of step", status)
	}
}

// TestInspect_SecretDetailNeverReadsTheValue covers the one thing this view
// must not do. Revealing a credential is its own milestone; a view that
// printed values on the way past would put every one of them in the
// terminal's scrollback.
func TestInspect_SecretDetailNeverReadsTheValue(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), buildSecretsDir(t))
	m.width, m.height = 100, 24
	m.setTab(tabSecrets)

	body, err := renderSecretDetail(m.secrets, m.l, "ss-srv/main/yak/default")
	if err != nil {
		t.Fatalf("renderSecretDetail: %v", err)
	}
	if strings.Contains(body, "in-step") || strings.Contains(body, "rotating") {
		t.Fatalf("secret detail = %q, want no value in it", body)
	}
	if !strings.Contains(body, "ss-srv/main/yak/default") {
		t.Fatalf("secret detail = %q, want the path it is addressed by", body)
	}
	if !strings.Contains(body, "yak-default") {
		t.Fatalf("secret detail = %q, want the account it belongs to", body)
	}
}

// TestInspect_SecretsTabWithoutAStore covers conf.secrets left unset. The
// inventory still implies every path; there is simply nothing to compare
// against, and saying so beats an empty tree.
func TestInspect_SecretsTabWithoutAStore(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24
	m.setTab(tabSecrets)

	if m.secrets.loadErr == nil {
		t.Fatal("buildSecrets accepted an unset conf.secrets")
	}
	if !strings.Contains(m.View(), "conf.secrets is not configured") {
		t.Fatalf("View() = %q, want it to name what is unset", m.View())
	}
}

// TestInspect_ServicesIndexHoldsOnlyPrograms covers the split between the two
// directories. A service is a program a node deploys; a way of handing a
// credential to a person is an export, and nothing runs one, so it is not in
// this index. The service names the ways it is written out instead.
func TestInspect_ServicesIndexHoldsOnlyPrograms(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.width, m.height = 100, 24

	for _, it := range m.serviceItems {
		if it.ID == "service:ss-json" {
			t.Fatal("the Services index holds ss-json, which nothing deploys")
		}
	}

	_, body := selectDetail(t, m, "service:ssserver")
	if !strings.Contains(body, "written out as ss-json") {
		t.Fatalf("service detail = %q, want the ways it is written out", body)
	}
	if !strings.Contains(body, "written out for people") {
		t.Fatalf("service detail = %q, want how many files that produced", body)
	}
}
