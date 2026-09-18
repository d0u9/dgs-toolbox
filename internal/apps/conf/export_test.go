package conf

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/secretstore"
)

// buildExportableRoot is buildRenderableRoot plus a second, unmanaged-user
// target, so an export exercises both a node's directory and a bundle named
// for the person instead.
func buildExportableRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root, secretsDir = buildRenderableRoot(t)

	writeFile(t, filepath.Join(root, "services", "hysteria2", "confgen.yaml"), `
roles:
  server:
    template: templates/server.yaml.tmpl
    defaults: document
    output: config.yaml
    auth: per-principal
    reached_by: link
  link:
    template: templates/link.tmpl
    defaults: element
    output: share.txt
    auth: none
`)
	writeFile(t, filepath.Join(root, "services", "hysteria2", "templates", "link.tmpl"),
		"hysteria2://{{ (upstream).address }}:{{ (upstream).port }}\n")
	writeFile(t, filepath.Join(root, "users.yaml"), `
users:
  friend-a:
    username: yak
    devices: unmanaged
    access: [sfo]
`)
	writeFile(t, filepath.Join(root, "routes.yaml"), `
routes:
  sfo:
    hops: [us-sfo:main]
`)
	writeFile(t, filepath.Join(root, "networks.yaml"), `
networks: [internet]
universal: internet
`)

	inv, err := inventory.Load(root)
	if err != nil {
		t.Fatalf("inventory.Load: %v", err)
	}
	confRoot, err := confgen.Load(root)
	if err != nil {
		t.Fatalf("confgen.Load: %v", err)
	}
	manifests := map[string]confgen.Manifest{}
	for _, svc := range confRoot.Services {
		manifests[svc.Name] = svc.Manifest
	}
	model, err := derive.Derive(inv, manifests)
	if err != nil {
		t.Fatalf("derive.Derive: %v", err)
	}
	implied := secretstore.ImpliedPaths(inv, manifests, model)
	if err := secretstore.Generate(secretsDir, implied, inv, manifests); err != nil {
		t.Fatalf("secretstore.Generate: %v", err)
	}

	return root, secretsDir
}

func TestExportFolder_WritesUnderEachNodeAndUnmanagedUserDirectory(t *testing.T) {
	root, secretsDir := buildExportableRoot(t)
	m := newModel(root, secretsDir)

	instances := m.CheckedInstancesOrAll(t)
	dest := t.TempDir()
	if err := m.ExportFolder(instances, dest); err != nil {
		t.Fatalf("ExportFolder: %v", err)
	}

	want := []string{
		filepath.Join(dest, "srv", "hysteria2", "server", "us-sfo", "config.yaml"),
		filepath.Join(dest, "friend-a", "hysteria2", "link", "yak-sfo", "share.txt"),
	}
	for _, p := range want {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing export file %s: %v", p, err)
		}
	}

	data, err := os.ReadFile(want[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hysteria2://203.0.113.10:443\n" {
		t.Fatalf("share.txt = %q", data)
	}
}

func TestExportFolder_RenderFailureWritesNothing(t *testing.T) {
	root, secretsDir := buildExportableRoot(t)
	// Break the server template so rendering us-sfo fails, after yak-sfo
	// would already have rendered if instances were processed in the other
	// order — CheckedInstancesOrAll sorts, so us-sfo (the broken one) comes
	// first and yak-sfo must still not be written.
	writeFile(t, filepath.Join(root, "services", "hysteria2", "templates", "server.yaml.tmpl"),
		"{{ required .nonexistent \"required field\" }}")
	m := newModel(root, secretsDir)

	instances := m.CheckedInstancesOrAll(t)
	dest := t.TempDir()
	if err := m.ExportFolder(instances, dest); err == nil {
		t.Fatal("ExportFolder: want an error, the template is broken")
	}

	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("dest = %v, want nothing written after a render failure", entries)
	}
}

func TestExportFolder_RefusesAnExistingFile(t *testing.T) {
	root, secretsDir := buildExportableRoot(t)
	m := newModel(root, secretsDir)
	instances := m.CheckedInstancesOrAll(t)

	dest := t.TempDir()
	if err := m.ExportFolder(instances, dest); err != nil {
		t.Fatalf("ExportFolder: %v", err)
	}
	if err := m.ExportFolder(instances, dest); err == nil {
		t.Fatal("ExportFolder: want an error exporting into a destination that already holds these files")
	}
}

func TestExportZip_WritesOneArchiveWithBothEntries(t *testing.T) {
	root, secretsDir := buildExportableRoot(t)
	m := newModel(root, secretsDir)
	instances := m.CheckedInstancesOrAll(t)

	zipPath := filepath.Join(t.TempDir(), "export.zip")
	if err := m.ExportZip(instances, zipPath); err != nil {
		t.Fatalf("ExportZip: %v", err)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("opening zip: %v", err)
	}
	defer zr.Close()

	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{
		"srv/hysteria2/server/us-sfo/config.yaml",
		"friend-a/hysteria2/link/yak-sfo/share.txt",
	} {
		if !names[want] {
			t.Fatalf("zip entries = %v, want %q among them", names, want)
		}
	}
}

func TestExportZip_RenderFailureWritesNothing(t *testing.T) {
	root, secretsDir := buildExportableRoot(t)
	writeFile(t, filepath.Join(root, "services", "hysteria2", "templates", "server.yaml.tmpl"),
		"{{ required .nonexistent \"required field\" }}")
	m := newModel(root, secretsDir)
	instances := m.CheckedInstancesOrAll(t)

	zipPath := filepath.Join(t.TempDir(), "export.zip")
	if err := m.ExportZip(instances, zipPath); err == nil {
		t.Fatal("ExportZip: want an error, the template is broken")
	}
	if _, err := os.Stat(zipPath); err == nil {
		t.Fatal("a zip was written despite the render failure")
	}
}

// CheckedInstancesOrAll checks every checkable instance first — the export
// tests care about what gets exported, not about exercising the tri-state
// checklist, which model_test.go already covers.
func (m Model) CheckedInstancesOrAll(t *testing.T) []string {
	t.Helper()
	for _, n := range m.nodes {
		for _, inst := range n.instances {
			if inst.broken == "" {
				m.checked[inst.name] = true
			}
		}
	}
	return m.CheckedInstances()
}
