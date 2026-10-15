package conf

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	rcli "github.com/d0u9/rhumb/cli"
)

func TestHostMigrationRowsFlagsAndPlanRoundTrip(t *testing.T) {
	root, secrets := buildBoundHostRoot(t)
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = rcli.MigrationRelocate
	m.selectNode("srv")
	foundBackend := false
	for _, row := range m.rows {
		if row.kind == "network" && row.network == "backend" {
			foundBackend = true
		}
	}
	if !foundBackend {
		t.Error("interface-only network missing rename row")
	}
	changes := map[string]string{"host-ip": "203.0.113.30", "host-mac": "02:00:00:00:00:31", "host-target": "wifi/main", "host-endpoint": "host-a.example.org"}
	seen := map[string]bool{}
	for i := range m.rows {
		r := &m.rows[i]
		if to, ok := changes[r.kind]; ok && !seen[r.kind] {
			r.after = to
			seen[r.kind] = true
		}
	}
	for kind := range changes {
		if !seen[kind] {
			t.Errorf("missing editable %s row", kind)
		}
	}
	var edits []rcli.HostChange
	if err := json.Unmarshal([]byte(m.flags()["host-edit"]), &edits); err != nil {
		t.Fatalf("host flags: %v; %v", err, m.flags())
	}
	if len(edits) != 4 {
		t.Fatalf("host edits = %+v", edits)
	}
	kinds := map[string]bool{}
	for _, e := range edits {
		kinds[e.Kind] = true
		if e.From == "" || e.To == "" {
			t.Errorf("missing comparison guard: %+v", e)
		}
		if e.Kind == "ip" && (e.Interface == "" || e.Address == "") {
			t.Errorf("IP edit missing identity: %+v", e)
		}
	}
	for _, kind := range []string{"ip", "mac", "target", "endpoint"} {
		if !kinds[kind] {
			t.Errorf("missing encoded %s", kind)
		}
	}
	plan := migrationPlanFromTable(m)
	if plan.Version != 1 {
		t.Fatalf("plan version = %d", plan.Version)
	}
	path := filepath.Join(t.TempDir(), "plan.yaml")
	if _, err := writeMigrationPlan(path, plan, nil); err != nil {
		t.Fatal(err)
	}
	saved, _, err := readMigrationPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	if err := applyMigrationPlan(reopened, saved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.flags(), reopened.flags()) {
		t.Fatalf("plan changed meaning: %v / %v", m.flags(), reopened.flags())
	}
	saved.SourceHost = "different-host"
	if err := applyMigrationPlan(reopened, saved); err == nil || !(strings.Contains(err.Error(), "new plan") || strings.Contains(err.Error(), "recreate")) {
		t.Fatalf("changed host binding must request recreation: %v", err)
	}
}

func TestHostChangeAfterReportRefusesApply(t *testing.T) {
	root, secrets := buildBoundHostRoot(t)
	m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
	m.scenario = rcli.MigrationRelocate
	m.selectNode("srv")
	for i := range m.rows {
		if m.rows[i].kind == "host-ip" {
			m.rows[i].after = "203.0.113.31"
			break
		}
	}
	m.generateReport()
	if m.mode != "report" {
		t.Fatalf("preview: %s", m.notice)
	}
	path := filepath.Join(root, "hosts.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]byte("# changed after preview\n"), data...)
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	m.applyPlan()
	if !strings.Contains(m.notice, "stale") {
		t.Fatalf("stale preview accepted: %s", m.notice)
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, changed) {
		t.Fatal("stale apply overwrote external change")
	}
}
