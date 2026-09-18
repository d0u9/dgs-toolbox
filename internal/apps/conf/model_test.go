package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildRoot is a generator root with one service (hysteria2, server only, no
// reached_by — nothing derives from it, so the tree holds exactly the
// authored instances), a node with two good instances, and a node whose file
// will not parse.
func buildRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "services", "hysteria2", "confgen.yaml"), `
secret:
  kind: base64
  bytes: 32
roles:
  server:
    template: templates/server.yaml.tmpl
    defaults: element
    output: config.yaml
    auth: per-principal
`)
	writeFile(t, filepath.Join(dir, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: us-sfo
    service: hysteria2
    role: server
    ports:
      main: 443
  - id: jp-tyo
    service: hysteria2
    role: server
    ports:
      main: 443
`)
	writeFile(t, filepath.Join(dir, "nodes", "bad.yaml"), "id: [unterminated\n")
	return dir
}

// press sends each key as runes — enough for the single-character keys
// (space, x, o, w, g) these tests use. Named keys (arrows, home, end) go
// through pressKey instead.
func press(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		m = next.(Model)
	}
	return m
}

// pressKey sends a single named key (e.g. "up", "down", "left", "right").
func pressKey(m Model, kt tea.KeyType) Model {
	next, _ := m.Update(tea.KeyMsg{Type: kt})
	return next.(Model)
}

func TestNewModel_NoRootConfigured(t *testing.T) {
	m := newModel("", "")
	view := m.View()
	if !strings.Contains(view, "no generator root configured") {
		t.Fatalf("View() = %q, want the no-root message", view)
	}
	status := m.Status()
	if status.Center != "no generator root" {
		t.Fatalf("Status().Center = %q", status.Center)
	}
}

func TestNewModel_LoadErrorIsShown(t *testing.T) {
	m := newModel(filepath.Join(t.TempDir(), "does-not-exist"), "")
	if m.loadErr == nil {
		t.Fatal("loadErr = nil, want an error for a missing root")
	}
	if !strings.Contains(m.View(), m.loadErr.Error()) {
		t.Fatalf("View() does not contain the load error")
	}
}

func TestNewModel_BuildsRowsForNodesAndInstances(t *testing.T) {
	m := newModel(buildRoot(t), "")
	m.width, m.height = 80, 24

	var ids []string
	for _, r := range m.rows {
		ids = append(ids, r.id)
	}
	want := []string{
		"node:nodes/bad.yaml",
		"node:srv",
		"inst:srv/jp-tyo",
		"inst:srv/us-sfo",
	}
	if len(ids) != len(want) {
		t.Fatalf("rows = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("rows[%d] = %q, want %q (rows: %v)", i, ids[i], want[i], ids)
		}
	}
}

func TestModel_BrokenNodeCannotBeChecked(t *testing.T) {
	m := newModel(buildRoot(t), "")
	m.width, m.height = 80, 24
	if !m.list.SelectID("node:nodes/bad.yaml") {
		t.Fatal("SelectID: row not found")
	}
	m2 := press(m, " ")
	total, _ := m2.counts()
	if total != 2 {
		t.Fatalf("checked count = %d, want 2 (bad.yaml excluded, its two siblings not)", total)
	}
	for _, r := range m2.rows {
		if r.id == "node:nodes/bad.yaml" && r.checkbox != "" {
			t.Fatal("a broken node became checkable")
		}
	}
}

func TestModel_CheckingAnInstancePropagatesUpAndCountsIt(t *testing.T) {
	m := newModel(buildRoot(t), "")
	m.width, m.height = 80, 24
	m.list.SelectID("inst:srv/us-sfo")
	m = press(m, " ")

	if !m.checked["us-sfo"] {
		t.Fatal("checking the instance row did not check its target")
	}

	total, selected := m.counts()
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if selected != 1 {
		t.Fatalf("selected = %d, want 1", selected)
	}

	var nodeBox string
	for _, r := range m.rows {
		if r.id == "node:srv" {
			nodeBox = r.checkbox
		}
	}
	if nodeBox != "[-]" {
		t.Fatalf("srv node checkbox = %q, want [-] (partial)", nodeBox)
	}
}

func TestModel_CheckingNodeChecksEveryCheckableInstanceUnderIt(t *testing.T) {
	m := newModel(buildRoot(t), "")
	m.width, m.height = 80, 24
	m.list.SelectID("node:srv")
	m = press(m, " ")

	if !m.checked["us-sfo"] || !m.checked["jp-tyo"] {
		t.Fatal("checking the node did not check both instances")
	}

	var nodeBox string
	for _, r := range m.rows {
		if r.id == "node:srv" {
			nodeBox = r.checkbox
		}
	}
	if nodeBox != "[x]" {
		t.Fatalf("srv node checkbox = %q, want [x] (all checkable checked)", nodeBox)
	}

	// Toggling again unchecks everything under it.
	m = press(m, " ")
	if m.checked["us-sfo"] || m.checked["jp-tyo"] {
		t.Fatal("toggling a fully checked node did not clear it")
	}
}

func TestModel_FoldingHidesChildrenWithoutLosingChecks(t *testing.T) {
	m := newModel(buildRoot(t), "")
	m.width, m.height = 80, 24
	m.list.SelectID("inst:srv/us-sfo")
	m = press(m, " ")

	m.list.SelectID("node:srv")
	m = pressKey(m, tea.KeyLeft) // collapse the node

	for _, r := range m.rows {
		if strings.HasPrefix(r.id, "inst:srv/") {
			t.Fatalf("instance row %q still visible after collapsing its node", r.id)
		}
	}
	if !m.checked["us-sfo"] {
		t.Fatal("collapsing the node lost the check")
	}

	m = pressKey(m, tea.KeyRight) // expand it again
	found := false
	for _, r := range m.rows {
		if r.id == "inst:srv/us-sfo" {
			found = true
		}
	}
	if !found {
		t.Fatal("expanding the node did not bring the instance row back")
	}
}

// TestNewModel_UnmanagedUserIsATopLevelEntry is milestone 8's own
// requirement: the tree's top level is nodes, with unmanaged users holding
// their derived instances as their own entries, since they have no node.
func TestNewModel_UnmanagedUserIsATopLevelEntry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "services", "hysteria2", "confgen.yaml"), `
roles:
  server:
    template: templates/server.yaml.tmpl
    defaults: document
    output: config.yaml
    auth: per-principal
    reached_by: link
  link:
    template: templates/link.tmpl
    output: share.txt
    auth: none
`)
	writeFile(t, filepath.Join(dir, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: hy2-srv
    service: hysteria2
    role: server
    ports:
      main: 443
`)
	writeFile(t, filepath.Join(dir, "users.yaml"), `
users:
  friend-a:
    username: yak
    devices: unmanaged
    access: [jp]
`)
	writeFile(t, filepath.Join(dir, "routes.yaml"), `
routes:
  jp:
    hops: [hy2-srv:main]
`)
	writeFile(t, filepath.Join(dir, "networks.yaml"), `
networks: [internet]
universal: internet
`)

	m := newModel(dir, "")
	m.width, m.height = 80, 24

	var ids []string
	for _, r := range m.rows {
		ids = append(ids, r.id)
	}
	want := []string{"node:friend-a", "inst:friend-a/yak-jp", "node:srv", "inst:srv/hy2-srv"}
	if len(ids) != len(want) {
		t.Fatalf("rows = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("rows[%d] = %q, want %q (rows: %v)", i, ids[i], want[i], ids)
		}
	}
}

func TestModel_LeftOnLeafMovesToParent(t *testing.T) {
	m := newModel(buildRoot(t), "")
	m.width, m.height = 80, 24
	m.list.SelectID("inst:srv/us-sfo")
	m = pressKey(m, tea.KeyLeft)

	item, ok := m.list.Selected()
	if !ok || item.ID != "node:srv" {
		t.Fatalf("cursor = %+v, want node:srv", item)
	}
}
