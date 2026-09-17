package target

import (
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/conf/confgen"
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

func TestList_FlattensServicesRolesAndInstances(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hysteria2", confgen.ManifestFilename), `
secrets: hysteria2.yaml
roles:
  server:
    template: templates/server.yaml.tmpl
    defaults: element
    output: config.yaml
`)
	writeFile(t, filepath.Join(dir, "hysteria2", "server", "us-sfo-dgo-linux-01.yaml"), "server: sfo\n")
	writeFile(t, filepath.Join(dir, "hysteria2", "server", "bad.yaml"), "server: [unterminated\n")

	root, err := confgen.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	statuses := List(root)
	if len(statuses) != 2 {
		t.Fatalf("List = %d, want 2: %+v", len(statuses), statuses)
	}

	byName := map[string]Status{}
	for _, s := range statuses {
		byName[s.Target.String()] = s
	}

	ok, exists := byName["hysteria2/server/us-sfo-dgo-linux-01"]
	if !exists {
		t.Fatalf("missing good target: %+v", statuses)
	}
	if ok.Broken != "" {
		t.Fatalf("Broken = %q, want empty", ok.Broken)
	}

	broken, exists := byName["hysteria2/server/bad"]
	if !exists {
		t.Fatalf("missing broken target: %+v", statuses)
	}
	if broken.Broken == "" {
		t.Fatal("Broken = empty, want a parse error")
	}
}
