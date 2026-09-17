package confgen

import (
	"os"
	"path/filepath"
	"testing"
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

func TestLoad_DiscoversServiceRolesAndInstances(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "hysteria2", ManifestFilename), `
secrets: hysteria2.yaml
roles:
  server:
    template: templates/server.json.tmpl
    defaults: element
    output: config.json
  client:
    template: templates/client.json.tmpl
    defaults: document
    output: config.json
`)
	writeFile(t, filepath.Join(root, "hysteria2", "server", "defaults.yaml"), "listen: :443\n")
	writeFile(t, filepath.Join(root, "hysteria2", "server", "us-sfo-dgo-linux-01.yaml"), "server: sfo\n")
	writeFile(t, filepath.Join(root, "hysteria2", "server", "jp-tyo-dgo-linux-01.yaml"), "server: tyo\n")

	// A scratch folder without a manifest is skipped.
	writeFile(t, filepath.Join(root, "scratch", "notes.yaml"), "todo: yes\n")

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Services) != 1 {
		t.Fatalf("Services = %d, want 1 (scratch should be skipped): %+v", len(got.Services), got.Services)
	}
	svc := got.Services[0]
	if svc.Name != "hysteria2" {
		t.Fatalf("Name = %q, want hysteria2", svc.Name)
	}
	if svc.Broken != "" {
		t.Fatalf("Broken = %q, want empty", svc.Broken)
	}
	if svc.Manifest.Secrets != "hysteria2.yaml" {
		t.Fatalf("Secrets = %q", svc.Manifest.Secrets)
	}
	if len(svc.Roles) != 2 {
		t.Fatalf("Roles = %d, want 2", len(svc.Roles))
	}

	server := svc.Roles[1] // sorted: client, server
	if server.Name != "server" {
		t.Fatalf("Roles[1].Name = %q, want server", server.Name)
	}
	if server.Role.Defaults != DefaultsElement {
		t.Fatalf("Defaults = %q, want %q", server.Role.Defaults, DefaultsElement)
	}
	if len(server.Instances) != 2 {
		t.Fatalf("Instances = %d, want 2 (defaults.yaml excluded): %+v", len(server.Instances), server.Instances)
	}
	if server.Instances[0].Name != "jp-tyo-dgo-linux-01" || server.Instances[1].Name != "us-sfo-dgo-linux-01" {
		t.Fatalf("Instances = %+v, want sorted by name", server.Instances)
	}
	for _, inst := range server.Instances {
		if inst.Broken != "" {
			t.Fatalf("Instance %q Broken = %q, want empty", inst.Name, inst.Broken)
		}
	}

	client := svc.Roles[0]
	if client.Name != "client" {
		t.Fatalf("Roles[0].Name = %q, want client", client.Name)
	}
	if len(client.Instances) != 0 {
		t.Fatalf("client Instances = %d, want 0 (role directory absent)", len(client.Instances))
	}
}

func TestLoad_BrokenManifestIsReportedNotFatal(t *testing.T) {
	root := t.TempDir()

	// Unknown key in the manifest.
	writeFile(t, filepath.Join(root, "microbin", ManifestFilename), `
secrets: microbin.yaml
roles:
  server:
    template: templates/server.env.tmpl
    defaults: document
    output: server.env
    typo_field: oops
`)
	// A second, valid service, so one broken manifest does not abort discovery.
	writeFile(t, filepath.Join(root, "shadowsocks-rust", ManifestFilename), `
secrets: shadowsocks-rust.yaml
roles:
  server:
    template: templates/server.json.tmpl
    defaults: element
    output: config.json
`)
	writeFile(t, filepath.Join(root, "shadowsocks-rust", "server", "us-sfo-dgo-linux-01.yaml"), "servers: []\n")

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Services) != 2 {
		t.Fatalf("Services = %d, want 2", len(got.Services))
	}

	microbin := got.Services[0]
	if microbin.Name != "microbin" {
		t.Fatalf("Services[0].Name = %q, want microbin", microbin.Name)
	}
	if microbin.Broken == "" {
		t.Fatal("Broken = empty, want unknown-field error")
	}
	if len(microbin.Roles) != 0 {
		t.Fatalf("Roles = %d, want 0 for a broken manifest", len(microbin.Roles))
	}

	ss := got.Services[1]
	if ss.Broken != "" {
		t.Fatalf("shadowsocks-rust Broken = %q, want empty", ss.Broken)
	}
	if len(ss.Roles) != 1 || len(ss.Roles[0].Instances) != 1 {
		t.Fatalf("shadowsocks-rust Roles = %+v", ss.Roles)
	}
}

func TestLoad_BrokenInstanceIsListedWithParseError(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "hysteria2", ManifestFilename), `
secrets: hysteria2.yaml
roles:
  server:
    template: templates/server.json.tmpl
    defaults: element
    output: config.json
`)
	writeFile(t, filepath.Join(root, "hysteria2", "server", "good.yaml"), "server: sfo\n")
	// Invalid YAML.
	writeFile(t, filepath.Join(root, "hysteria2", "server", "bad-syntax.yaml"), "server: [unterminated\n")
	// Valid YAML, but not a mapping.
	writeFile(t, filepath.Join(root, "hysteria2", "server", "bad-shape.yaml"), "- one\n- two\n")

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	svc := got.Services[0]
	instances := svc.Roles[0].Instances
	byName := map[string]Instance{}
	for _, inst := range instances {
		byName[inst.Name] = inst
	}

	if len(instances) != 3 {
		t.Fatalf("Instances = %d, want 3 (broken instances still listed): %+v", len(instances), instances)
	}
	if byName["good"].Broken != "" {
		t.Fatalf("good.Broken = %q, want empty", byName["good"].Broken)
	}
	if byName["bad-syntax"].Broken == "" {
		t.Fatal("bad-syntax.Broken = empty, want parse error")
	}
	if byName["bad-shape"].Broken == "" {
		t.Fatal("bad-shape.Broken = empty, want not-a-mapping error")
	}
}
