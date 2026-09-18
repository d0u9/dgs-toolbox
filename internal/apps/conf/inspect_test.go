package conf

import (
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/tui"

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
	if !strings.Contains(view, "INDEX") {
		t.Fatalf("View() = %q, want the left column's fieldset legend", view)
	}
	if !strings.Contains(view, "ss-srv") {
		t.Fatalf("View() = %q, want the selected instance's detail on the right", view)
	}
}
