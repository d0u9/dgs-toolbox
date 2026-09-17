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

func buildRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hysteria2", "confgen.yaml"), `
secrets: hysteria2.yaml
roles:
  server:
    template: templates/server.yaml.tmpl
    defaults: element
    output: config.yaml
  client:
    template: templates/client.yaml.tmpl
    defaults: document
    output: config.yaml
`)
	writeFile(t, filepath.Join(dir, "hysteria2", "server", "us-sfo.yaml"), "server: sfo\n")
	writeFile(t, filepath.Join(dir, "hysteria2", "server", "jp-tyo.yaml"), "server: tyo\n")
	writeFile(t, filepath.Join(dir, "hysteria2", "server", "bad.yaml"), "server: [unterminated\n")
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
	m := newModel("")
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
	m := newModel(filepath.Join(t.TempDir(), "does-not-exist"))
	if m.loadErr == nil {
		t.Fatal("loadErr = nil, want an error for a missing root")
	}
	if !strings.Contains(m.View(), m.loadErr.Error()) {
		t.Fatalf("View() does not contain the load error")
	}
}

func TestNewModel_BuildsRowsForServicesRolesAndInstances(t *testing.T) {
	m := newModel(buildRoot(t))
	m.width, m.height = 80, 24

	var ids []string
	for _, r := range m.rows {
		ids = append(ids, r.id)
	}
	want := []string{
		"svc:hysteria2",
		"role:hysteria2/client",
		"role:hysteria2/server",
		"inst:hysteria2/server/bad",
		"inst:hysteria2/server/jp-tyo",
		"inst:hysteria2/server/us-sfo",
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

func TestModel_BrokenInstanceCannotBeChecked(t *testing.T) {
	m := newModel(buildRoot(t))
	m.width, m.height = 80, 24
	if !m.list.SelectID("inst:hysteria2/server/bad") {
		t.Fatal("SelectID: row not found")
	}
	m2 := press(m, " ")
	if _, checked := m2.checked["hysteria2/server/bad"]; checked {
		t.Fatal("a broken instance became checked")
	}
	_, total := m2.counts()
	if total != 0 {
		t.Fatalf("checked count = %d, want 0", total)
	}
}

func TestModel_CheckingAnInstancePropagatesUpAndCountsIt(t *testing.T) {
	m := newModel(buildRoot(t))
	m.width, m.height = 80, 24
	m.list.SelectID("inst:hysteria2/server/us-sfo")
	m = press(m, " ")

	if !m.checked["hysteria2/server/us-sfo"] {
		t.Fatal("checking the instance row did not check its target")
	}

	total, selected := m.counts()
	if total != 2 {
		t.Fatalf("total = %d, want 2 (bad.yaml excluded)", total)
	}
	if selected != 1 {
		t.Fatalf("selected = %d, want 1", selected)
	}

	var serverBox string
	for _, r := range m.rows {
		if r.id == "role:hysteria2/server" {
			serverBox = r.checkbox
		}
	}
	if serverBox != "[-]" {
		t.Fatalf("server role checkbox = %q, want [-] (partial)", serverBox)
	}
}

func TestModel_CheckingRoleChecksEveryCheckableInstanceUnderIt(t *testing.T) {
	m := newModel(buildRoot(t))
	m.width, m.height = 80, 24
	m.list.SelectID("role:hysteria2/server")
	m = press(m, " ")

	if !m.checked["hysteria2/server/us-sfo"] || !m.checked["hysteria2/server/jp-tyo"] {
		t.Fatal("checking the role did not check both good instances")
	}
	if m.checked["hysteria2/server/bad"] {
		t.Fatal("checking the role checked the broken instance")
	}

	var serverBox string
	for _, r := range m.rows {
		if r.id == "role:hysteria2/server" {
			serverBox = r.checkbox
		}
	}
	if serverBox != "[x]" {
		t.Fatalf("server role checkbox = %q, want [x] (all checkable checked)", serverBox)
	}

	// Toggling again unchecks everything under it.
	m = press(m, " ")
	if m.checked["hysteria2/server/us-sfo"] || m.checked["hysteria2/server/jp-tyo"] {
		t.Fatal("toggling a fully checked role did not clear it")
	}
}

func TestModel_FoldingHidesChildrenWithoutLosingChecks(t *testing.T) {
	m := newModel(buildRoot(t))
	m.width, m.height = 80, 24
	m.list.SelectID("inst:hysteria2/server/us-sfo")
	m = press(m, " ")

	m.list.SelectID("role:hysteria2/server")
	m = pressKey(m, tea.KeyLeft) // collapse the server role

	for _, r := range m.rows {
		if strings.HasPrefix(r.id, "inst:hysteria2/server/") {
			t.Fatalf("instance row %q still visible after collapsing its role", r.id)
		}
	}
	if !m.checked["hysteria2/server/us-sfo"] {
		t.Fatal("collapsing the role lost the check")
	}

	m = pressKey(m, tea.KeyRight) // expand it again
	found := false
	for _, r := range m.rows {
		if r.id == "inst:hysteria2/server/us-sfo" {
			found = true
		}
	}
	if !found {
		t.Fatal("expanding the role did not bring the instance row back")
	}
}

func TestModel_LeftOnLeafMovesToParent(t *testing.T) {
	m := newModel(buildRoot(t))
	m.width, m.height = 80, 24
	m.list.SelectID("inst:hysteria2/server/us-sfo")
	m = pressKey(m, tea.KeyLeft)

	item, ok := m.list.Selected()
	if !ok || item.ID != "role:hysteria2/server" {
		t.Fatalf("cursor = %+v, want role:hysteria2/server", item)
	}
}
