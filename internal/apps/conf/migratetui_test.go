package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMigrationTableEditsCanBeRevisitedAndReported(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	path := filepath.Join(root, "nodes", "srv.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := newInspectModel(root, secrets)
	m.width, m.height = 110, 30
	m.setTab(tabMigrate)
	if m.migration.mode != "plans" || !strings.Contains(m.View(), "MIGRATION PLANS") {
		t.Fatalf("migration tab did not open plan list: %s", m.View())
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.migration.mode != "scenario" || !strings.Contains(m.View(), "SELECT SCENARIO") {
		t.Fatal("new migration did not ask for a scenario")
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.migration.mode != "nodes" || m.migration.scenario != migrationRelocate {
		t.Fatal("new migration did not open node list")
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.migration.mode != "table" || len(m.migration.rows) == 0 {
		t.Fatalf("node selection did not open edit table: %+v", m.migration)
	}
	if !strings.Contains(m.View(), "◆ NODE") || !strings.Contains(m.View(), "◆ NETWORKS") {
		t.Fatalf("migration sections missing: %s", m.View())
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.CapturesShellKey("q") || !m.CapturesShellKey("esc") {
		t.Fatal("text editor did not retain shell keys")
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyCtrlU})
	for _, ch := range "srv08" {
		pressMigration(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.migration.rows[0].after; got != "srv08" {
		t.Fatalf("edited node ID = %q", got)
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyDown})
	if m.migration.selectedRow() != 1 {
		t.Fatal("Down stopped on a section heading instead of the next editable row")
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyUp})
	if m.migration.selectedRow() != 0 || m.migration.rows[0].after != "srv08" {
		t.Fatal("moving away and back lost the edit")
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.migration.mode != "report" || !strings.Contains(string(m.migration.report), "# Node migration: `srv` → `srv08`") {
		t.Fatalf("report = %s; notice = %s", m.migration.report, m.migration.notice)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("TUI changed inventory: %v", err)
	}
}

func TestMigrationTableSaveReportDoesNotOverwrite(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = migrationRelocate
	m.selectNode("srv")
	for _, section := range []string{"NODE", "NETWORKS", "INSTANCES", "ROUTES"} {
		found := false
		// The section names are visible in the table, while the data rows
		// remain the only keyboard stops.
		if strings.Contains(m.view(120, 40), "◆ "+section) {
			found = true
		}
		if !found {
			t.Errorf("section %s missing", section)
		}
	}
	m.rows[0].after = "srv08"
	m.generateReport()
	if m.mode != "report" {
		t.Fatalf("report mode = %q, notice = %q", m.mode, m.notice)
	}
	path := filepath.Join(t.TempDir(), "migration.txt")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.save(path)
	if !strings.Contains(m.notice, "already exists") {
		t.Fatalf("notice = %q", m.notice)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "existing" {
		t.Fatalf("saved file changed: %q, %v", data, err)
	}
}

func TestMigrationTableMouseSelectsRowsButNotSections(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	m := newInspectModel(root, secrets)
	m.width, m.height = 120, 40
	m.setTab(tabMigrate)
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	m.View() // establish the scroll list's viewport size
	if m.migration.selectedRow() != 0 {
		t.Fatal("first editable row was not selected")
	}
	// A section rule takes a row but is not selectable.
	clickMigration(&m, 1, 4)
	if m.migration.selectedRow() != 0 {
		t.Fatal("clicking a section changed selection")
	}
	clickMigration(&m, 1, 5)
	if m.migration.selectedRow() != 1 {
		t.Fatalf("click selected row %d, want first network", m.migration.selectedRow())
	}
	item, ok := m.migration.list.Selected()
	if !ok || !strings.HasPrefix(item.Label, "  ") || !strings.HasPrefix(item.Detail, "    ") {
		t.Fatalf("row values are not indented: %+v", item)
	}
	m.migration.generateReport()
	clickMigration(&m, 1, 3)
	if m.migration.mode != "table" || m.migration.selectedRow() != 0 {
		t.Fatal("clicking a row did not return from report to its editable table row")
	}
}

func clickMigration(m *InspectModel, x, y int) {
	next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	*m = next.(InspectModel)
}

func TestMigrationReportScrollReturnsFromBottomAndWheelFollowsPane(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = migrationRelocate
	m.selectNode("srv")
	m.mode = "report"
	m.report = []byte(strings.Repeat("A report line that wraps across this pane and needs scrolling.\n", 40))
	width, height := 90, 16
	m.view(width, height)
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}, width, height)
	bottom := m.reportScroll
	if bottom <= 0 {
		t.Fatal("report did not scroll")
	}
	m.update(tea.KeyMsg{Type: tea.KeyUp}, width, height)
	if m.reportScroll != bottom-1 {
		t.Fatalf("up from bottom = %d, want %d", m.reportScroll, bottom-1)
	}
	left, _ := migrationColumns(width)
	m.updateMouse(tea.MouseMsg{X: left + 2, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp}, width, height)
	if m.reportScroll != bottom-4 {
		t.Fatalf("report wheel = %d", m.reportScroll)
	}
	selected := m.selectedRow()
	m.updateMouse(tea.MouseMsg{X: 2, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}, width, height)
	if m.selectedRow() == selected || m.reportScroll != bottom-4 {
		t.Fatal("table wheel did not stay in its pane")
	}
}

func TestMigrationReportCopiesWithCAndY(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	m := newInspectModel(root, secrets)
	m.migration.mode = "report"
	m.migration.report = []byte("migration report")
	var copied string
	m.migration.copy = func(value string) error { copied = value; return nil }
	for _, key := range []string{"c", "y"} {
		copied = ""
		cmd := m.migration.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}, 120, 40)
		if cmd == nil {
			t.Fatalf("%s did not return a clipboard command", key)
		}
		msg := cmd()
		updated, _ := m.Update(msg)
		m = updated.(InspectModel)
		if copied != "migration report" || !strings.Contains(m.migration.notice, "report copied") {
			t.Fatalf("%s copied %q, notice %q", key, copied, m.migration.notice)
		}
	}
}

func TestMigrationTableCollectsAllEditableKinds(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	path := filepath.Join(root, "nodes", "srv.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, strings.Replace(string(data), "main: 443", "main: {port: 443, published: old.example.test}", 1))
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = migrationRelocate
	m.selectNode("srv")
	if !strings.Contains(m.view(120, 40), "◆ PUBLISHED") {
		t.Fatal("published section is missing")
	}
	choices := map[string]string{
		"node": "srv08", "network": "wan", "address": "203.0.113.8", "instance": "u-node-group-1008", "published": "new.example.test", "route": "sfo08",
	}
	for i := range m.rows {
		if value, ok := choices[m.rows[i].kind]; ok {
			m.rows[i].after = value
		}
	}
	m.generateReport()
	if m.mode != "report" {
		t.Fatalf("report failed: %s", m.notice)
	}
	for _, want := range []string{"| Network name | `internet` | `wan` |", "| Address on wan | `203.0.113.10` | `203.0.113.8` |", "| Instance ID | `u-node-group-10` | `u-node-group-1008` |", "| Published name u-node-group-10:main | `old.example.test` | `new.example.test` |", "| Route name | `sfo` | `sfo08` |"} {
		if !strings.Contains(string(m.report), want) {
			t.Errorf("report missing %q:\n%s", want, m.report)
		}
	}
}

func TestMigrationTableShowsDerivedSecretsWithoutPersistingThem(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = migrationRelocate
	m.selectNode("srv")
	var oldSecret string
	for _, row := range m.rows {
		if row.kind == "secret" {
			oldSecret = row.before
			break
		}
	}
	if oldSecret == "" || !strings.Contains(m.view(120, 40), "◆ SECRETS") {
		t.Fatal("derived secrets missing from migration table")
	}
	for i := range m.rows {
		if m.rows[i].kind == "instance" {
			m.rows[i].after = "u-node-group-1008"
		}
	}
	m.refreshSecretRows()
	var changed bool
	for i, row := range m.rows {
		if row.kind != "secret" {
			continue
		}
		if row.before != row.after {
			changed = true
		}
		m.list.SelectID(fmt.Sprintf("row:%d", i))
		m.update(tea.KeyMsg{Type: tea.KeyEnter}, 120, 40)
		if m.editing() {
			t.Fatal("derived secret opened an editor")
		}
	}
	if !changed {
		t.Fatal("instance rename did not update any derived secret path")
	}
	for _, row := range migrationPlanFromTable(m).Rows {
		if row.Kind == "secret" {
			t.Fatal("derived secret persisted as an editable input")
		}
	}
}

func TestMigrationPlanCreateOpenEditAndRejectStaleInventory(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = migrationRelocate
	m.selectNode("srv")
	m.rows[0].after = "srv08"
	m.savePlan("move-home")
	if m.planName != "move-home" || m.planDigest == nil {
		t.Fatalf("plan was not saved: %s", m.notice)
	}
	path, _ := migrationPlanPath(m.planDir, "move-home")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "to: srv08") {
		t.Fatalf("saved plan = %s, %v", data, err)
	}
	reopened := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	reopened.openPlan("move-home")
	if reopened.mode != "table" || reopened.rows[0].after != "srv08" {
		t.Fatalf("open lost edit: %s", reopened.notice)
	}
	reopened.rows[0].after = "srv09"
	reopened.savePlan("move-home")
	if reopened.notice == "" || !strings.Contains(reopened.notice, "Saved:") {
		t.Fatalf("edit was not saved: %s", reopened.notice)
	}
	// An external edit must not be silently overwritten by a stale TUI.
	m.rows[0].after = "srv10"
	m.savePlan("move-home")
	if !strings.Contains(m.notice, "changed on disk") {
		t.Fatalf("stale save was accepted: %s", m.notice)
	}
	nodePath := filepath.Join(root, "nodes", "srv.yaml")
	node, err := os.ReadFile(nodePath)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, nodePath, strings.Replace(string(node), "203.0.113.10", "203.0.113.11", 1))
	stale := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	stale.openPlan("move-home")
	if stale.mode != "plans" || !strings.Contains(stale.notice, "inventory has changed") {
		t.Fatalf("stale inventory was accepted: %s", stale.notice)
	}
}

func TestMigrationPlanRejectsUnsafeNameAndVersion(t *testing.T) {
	if _, err := migrationPlanPath(t.TempDir(), "../escape"); err == nil {
		t.Fatal("unsafe name accepted")
	}
	root, secrets := buildExportableRoot(t)
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = migrationRelocate
	m.selectNode("srv")
	p := migrationPlanFromTable(m)
	p.Version = 99
	if err := applyMigrationPlan(m, p); err == nil {
		t.Fatal("unknown version accepted")
	}
}

func TestMigrationPlanCanOpenAndSaveOutsideDefaultDirectory(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	outside := t.TempDir()
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = migrationRelocate
	m.selectNode("srv")
	m.rows[0].after = "srv08"
	m.outputDir = outside
	m.savePlan("move")
	path := filepath.Join(outside, "move.yaml")
	if m.planPath != path {
		t.Fatalf("saved path = %q", m.planPath)
	}
	ymlPath := filepath.Join(outside, "move.yml")
	if err := os.Rename(path, ymlPath); err != nil {
		t.Fatal(err)
	}
	reopened := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	reopened.openPlanPath(ymlPath)
	if reopened.mode != "table" || reopened.rows[0].after != "srv08" {
		t.Fatalf("external plan open: %s", reopened.notice)
	}
	reopened.rows[0].after = "srv09"
	reopened.outputDir = outside
	reopened.savePlan("move")
	if reopened.planPath != ymlPath {
		t.Fatalf("save changed extension: %q", reopened.planPath)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected new YAML file: %v", err)
	}
}

func TestMigrationTUIStartsNewPlanAndReopensIt(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	m := newInspectModel(root, secrets)
	m.width, m.height = 110, 30
	m.setTab(tabMigrate)
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter}) // New migration
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter}) // Scenario
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter}) // Source node
	m.migration.rows[0].after = "srv08"
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.migration.picking != "plan-dir" {
		t.Fatal("new plan did not open File Explorer")
	}
	m.migration.acceptPicked(filepath.Join(root, "migrations"))
	if m.migration.mode != "plan-name" || !m.migration.editing() {
		t.Fatal("new plan did not ask for a name")
	}
	for _, ch := range "home-move" {
		pressMigration(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.migration.planName != "home-move" {
		t.Fatalf("plan name = %q, notice = %q", m.migration.planName, m.migration.notice)
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.migration.mode != "plans" {
		t.Fatal("Esc did not return to plan list")
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.migration.mode != "table" || m.migration.rows[0].after != "srv08" {
		t.Fatalf("saved plan did not reopen: %s", m.migration.notice)
	}
}

func TestMigrationTUIWarnsBeforeDiscardingUnsavedPlan(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	m := newInspectModel(root, secrets)
	m.setTab(tabMigrate)
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEnter})
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.migration.mode != "table" || !strings.Contains(m.migration.notice, "Unsaved plan") {
		t.Fatal("first Esc discarded an unsaved plan")
	}
	pressMigration(&m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.migration.mode != "plans" {
		t.Fatal("second Esc did not discard the plan")
	}
}

func mustLoadMigration(t *testing.T, root string) loaded {
	t.Helper()
	l, err := load(root)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func pressMigration(m *InspectModel, key tea.KeyMsg) {
	next, _ := m.Update(key)
	*m = next.(InspectModel)
}

func TestMigrationReportAppliesOnlyAfterConfirmation(t *testing.T) {
	root, secrets := buildExportableRoot(t)
	path := filepath.Join(root, "nodes", "srv.yaml")
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = migrationReplace
	m.selectNode("srv")
	m.rows[0].after = "srv08"
	m.generateReport()
	if m.mode != "report" || !strings.Contains(string(m.report), "**Scenario: replace with a new machine.**") {
		t.Fatalf("report failed: %s\n%s", m.notice, m.report)
	}
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}, 120, 40)
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, 120, 40)
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "srv08") {
		t.Fatal("declined apply changed the node file")
	}
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}, 120, 40)
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, 120, 40)
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "id: srv08") {
		t.Fatalf("apply did not write the node file: %s; notice %s", data, m.notice)
	}
	if !strings.Contains(string(m.report), "## Apply result") || !strings.Contains(string(m.report), ".dgs-migration-backup-") {
		t.Fatalf("apply result missing:\n%s", m.report)
	}
}
