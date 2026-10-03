package conf

import (
	"bytes"

	"dgs-toolbox/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --bundle refuses the destinations and flags that belong to other exports,
// and --download is only an answer to --bundle.
func TestExportAction_BundleFlags(t *testing.T) {
	g := configFor(buildExportableRoot(t))
	for _, c := range []struct {
		flags map[string]string
		want  string
	}{
		{map[string]string{"bundle": "true", "zip": "x.zip"}, "does not take --zip"},
		{map[string]string{"bundle": "true", "to": "-"}, "cannot write to stdout"},
		{map[string]string{"bundle": "true", "format": "yaml"}, "--bundle does not"},
		{map[string]string{"bundle": "true", "to": t.TempDir(), "download": "later"}, `unknown --download "later"`},
		{map[string]string{"to": t.TempDir(), "download": "build"}, "they need --bundle"},
		{map[string]string{"to": t.TempDir(), "services": t.TempDir()}, "they need --bundle"},
	} {
		err := exportAction(strings.NewReader(""), &bytes.Buffer{}, []string{"node:srv"}, c.flags, g)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("flags %v: err = %v, want %q", c.flags, err, c.want)
		}
	}
}

// A bundle that cannot be built stops the export before anything is written.
func TestExportAction_BundleRefusedWritesNothing(t *testing.T) {
	g := configFor(buildExportableRoot(t))
	dest := t.TempDir()
	flags := map[string]string{"bundle": "true", "to": dest, "yes": "true"}
	err := exportAction(strings.NewReader(""), &bytes.Buffer{}, []string{"node:srv"}, flags, g)
	if err == nil || !strings.Contains(err.Error(), "no deploy definition") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Fatalf("refused export wrote %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dest, "srv")); err == nil {
		t.Fatal("refused export wrote a bundle")
	}
}

// writeBundleExport writes one instance's export, as an export would leave
// it, for a service only a definitions directory knows.
func writeBundleExport(t *testing.T) (export, services string) {
	t.Helper()
	export = t.TempDir()
	os.WriteFile(filepath.Join(export, "app.conf"), []byte("port = 8080\n"), 0o600)
	os.WriteFile(filepath.Join(export, "manifest.yaml"), []byte(`schema: 1
node: node-1
instance: node-1-demo
service: demo
runtime: host
files:
  - path: app.conf
`), 0o600)
	services = t.TempDir()
	os.WriteFile(filepath.Join(services, "demo.yaml"), []byte(`binary:
  name: demo
  release:
    github: example/demo
    version: latest
    url: demo-{target}.tar.gz
    targets: {linux/amd64: x86_64}
command: ["{bin}/demo", "-c", "{conf}/app.conf"]
`), 0o644)
	return export, services
}

// dgs conf bundle builds what rhumb deploy build builds, reading the
// definitions conf.services names, and --services overrides it.
func TestBundleAction_UsesConfServices(t *testing.T) {
	export, services := writeBundleExport(t)
	bin := filepath.Join(t.TempDir(), "demo")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	flags := map[string]string{"platform": "linux/amd64", "bin": bin}

	var g config.Config
	flags["to"] = filepath.Join(t.TempDir(), "b")
	if err := bundleAction(nil, &bytes.Buffer{}, []string{export}, flags, g); err == nil || !strings.Contains(err.Error(), "no deploy definition") {
		t.Fatalf("without services: err = %v", err)
	}

	g.Conf.Services = services
	var out bytes.Buffer
	if err := bundleAction(nil, &out, []string{export}, flags, g); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ctl", "bin/demo", "conf/app.conf", "manifest.yaml"} {
		if _, err := os.Stat(filepath.Join(flags["to"], f)); err != nil {
			t.Errorf("bundle lacks %s", f)
		}
	}
	if !strings.Contains(out.String(), "ctl install") {
		t.Errorf("output = %q", out.String())
	}

	g.Conf.Services = t.TempDir()
	flags["services"] = services
	flags["to"] = filepath.Join(t.TempDir(), "b")
	if err := bundleAction(nil, &bytes.Buffer{}, []string{export}, flags, g); err != nil {
		t.Fatalf("--services did not override conf.services: %v", err)
	}
}

func TestBundleAction_Refusals(t *testing.T) {
	export, services := writeBundleExport(t)
	g := config.Config{}
	g.Conf.Services = services
	if err := bundleAction(nil, &bytes.Buffer{}, []string{export}, map[string]string{}, g); err == nil || !strings.Contains(err.Error(), "--to") {
		t.Errorf("no --to: err = %v", err)
	}
	flags := map[string]string{"to": filepath.Join(t.TempDir(), "b"), "download": "later"}
	if err := bundleAction(nil, &bytes.Buffer{}, []string{export}, flags, g); err == nil || !strings.Contains(err.Error(), `unknown --download "later"`) {
		t.Errorf("bad --download: err = %v", err)
	}
}

// bundle-gc lists a shim whose bundle was deleted, leaves one whose bundle is
// there, and removes only with --yes.
func TestBundleGCAction(t *testing.T) {
	home := t.TempDir()
	old := userHome
	userHome = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHome = old })

	bundles := filepath.Join(home, "bundles")
	kept := filepath.Join(bundles, "kept")
	os.MkdirAll(kept, 0o755)
	os.WriteFile(filepath.Join(kept, "ctl"), []byte("#!/bin/sh\n"), 0o755)
	shims := filepath.Join(home, ".local", "bin")
	os.MkdirAll(shims, 0o755)
	gone := filepath.Join(shims, "demo-gone")
	os.WriteFile(gone, []byte("#!/bin/sh\n# rhumb-bundle: "+filepath.Join(bundles, "gone")+"\n"), 0o755)
	os.WriteFile(filepath.Join(shims, "demo-kept"), []byte("#!/bin/sh\n# rhumb-bundle: "+kept+"\n"), 0o755)

	var out bytes.Buffer
	if err := bundleGCAction(nil, &out, nil, map[string]string{}, config.Config{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo-gone") || strings.Contains(out.String(), "demo-kept") || !strings.Contains(out.String(), "--yes") {
		t.Fatalf("listing = %q", out.String())
	}
	if _, err := os.Stat(gone); err != nil {
		t.Fatal("listing removed the shim")
	}

	if err := bundleGCAction(nil, &bytes.Buffer{}, nil, map[string]string{"yes": "true"}, config.Config{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Fatal("--yes left the shim")
	}
	out.Reset()
	bundleGCAction(nil, &out, nil, map[string]string{}, config.Config{})
	if !strings.Contains(out.String(), "nothing left behind") {
		t.Fatalf("after removal = %q", out.String())
	}
}
