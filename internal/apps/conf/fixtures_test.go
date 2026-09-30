package conf

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dgs-toolbox/internal/config"

	"github.com/d0u9/rhumb/confgen"
	"github.com/d0u9/rhumb/derive"
	"github.com/d0u9/rhumb/inventory"
	"github.com/d0u9/rhumb/secretstore"
)

func configFor(root, secretsDir string) config.Config {
	var c config.Config
	c.Conf.Root = root
	c.Conf.Secrets = secretsDir
	return c
}

func examplesRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	root = filepath.Join(filepath.Dir(file), "..", "..", "..", "examples", "conf")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("examples/conf: %v", err)
	}
	secretsDir = t.TempDir()

	var out bytes.Buffer
	if err := secretAction(strings.NewReader(""), &out, []string{"sync"}, map[string]string{"yes": "true"}, configFor(root, secretsDir)); err != nil {
		t.Fatalf("secret sync: %v\n%s", err, out.String())
	}
	return root, secretsDir
}

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
    hops: [srv/u-node-group-10:main]
`)
	writeFile(t, filepath.Join(root, "networks.yaml"), `
networks: [{name: internet}]
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
