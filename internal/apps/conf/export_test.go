package conf

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/secretstore"
	"dgs-toolbox/internal/conf/target"
)

// buildExportableRoot is buildRenderableRoot plus a second, unmanaged-user
// target, so an export exercises both a node's directory and a bundle named
// for the person instead.
func buildExportableRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root, secretsDir = buildRenderableRoot(t)

	writeFile(t, filepath.Join(root, "services", "hysteria2", "confgen.yaml"), `
template: templates/server.yaml.tmpl
defaults: document
output: config.yaml
auth: per-principal
`)
	writeFile(t, filepath.Join(root, "services", "hysteria2", "exports", "link", "confgen.yaml"), `
template: templates/link.tmpl
defaults: element
output: share.txt
`)
	writeFile(t, filepath.Join(root, "services", "hysteria2", "exports", "link", "templates", "link.tmpl"),
		"hysteria2://{{ (upstream).address }}:{{ (upstream).port }}\n")
	writeFile(t, filepath.Join(root, "users.yaml"), `
users:
  friend-a:
    username: yak
    devices: none
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
	r, instances := exportAll(t, root, secretsDir)
	dest := t.TempDir()
	if err := r.ExportFolder(instances, dest, false); err != nil {
		t.Fatalf("ExportFolder: %v", err)
	}

	want := []string{
		filepath.Join(dest, "srv", "hysteria2", "us-sfo", "config.yaml"),
		filepath.Join(dest, "friend-a", "hysteria2-link", "yak-default-sfo-hysteria2-link", "share.txt"),
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
	// Break the server template so rendering us-sfo fails, after yak-default-sfo-hysteria2-link
	// would already have rendered if instances were processed in the other
	// order — exportAll sorts, so us-sfo (the broken one) comes
	// first and yak-default-sfo-hysteria2-link must still not be written.
	writeFile(t, filepath.Join(root, "services", "hysteria2", "templates", "server.yaml.tmpl"),
		"{{ required .nonexistent \"required field\" }}")
	r, instances := exportAll(t, root, secretsDir)
	dest := t.TempDir()
	if err := r.ExportFolder(instances, dest, false); err == nil {
		t.Fatal("ExportFolder: want an error, the template is broken")
	}

	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("dest = %v, want nothing written after a render failure", entries)
	}
}

func TestExportFolder_RefusesAnExistingFile(t *testing.T) {
	root, secretsDir := buildExportableRoot(t)
	r, instances := exportAll(t, root, secretsDir)

	dest := t.TempDir()
	if err := r.ExportFolder(instances, dest, false); err != nil {
		t.Fatalf("ExportFolder: %v", err)
	}
	if err := r.ExportFolder(instances, dest, false); err == nil {
		t.Fatal("ExportFolder: want an error exporting into a destination that already holds these files")
	}
}

func TestExportZip_WritesOneArchiveWithBothEntries(t *testing.T) {
	root, secretsDir := buildExportableRoot(t)
	r, instances := exportAll(t, root, secretsDir)

	zipPath := filepath.Join(t.TempDir(), "export.zip")
	if err := r.ExportZip(instances, zipPath, false); err != nil {
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
		"srv/hysteria2/us-sfo/config.yaml",
		"friend-a/hysteria2-link/yak-default-sfo-hysteria2-link/share.txt",
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
	r, instances := exportAll(t, root, secretsDir)

	zipPath := filepath.Join(t.TempDir(), "export.zip")
	if err := r.ExportZip(instances, zipPath, false); err == nil {
		t.Fatal("ExportZip: want an error, the template is broken")
	}
	if _, err := os.Stat(zipPath); err == nil {
		t.Fatal("a zip was written despite the render failure")
	}
}

// exportAll loads root and returns its renderer with every instance that is
// not broken, sorted — the export tests care about what gets exported, not
// about how the instances were chosen.
func exportAll(t *testing.T, root, secretsDir string) (renderer, []string) {
	t.Helper()
	l, err := load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var instances []string
	for _, n := range buildTree(target.List(l.inv, l.derived)) {
		for _, inst := range n.instances {
			if inst.broken == "" {
				instances = append(instances, inst.name)
			}
		}
	}
	sort.Strings(instances)
	return renderer{l: l, rootPath: root, secretsDir: secretsDir}, instances
}

func TestIndentJSON(t *testing.T) {
	got := string(indentJSON([]byte(`{"b":1,"a":[1,2]}`)))
	want := "{\n  \"b\": 1,\n  \"a\": [\n    1,\n    2\n  ]\n}\n"
	if got != want {
		t.Fatalf("indentJSON = %q, want %q", got, want)
	}
	if got := string(indentJSON([]byte("not json"))); got != "not json" {
		t.Fatalf("indentJSON(invalid) = %q, want it unchanged", got)
	}
}
