package conf

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// examplesRoot is examples/conf with a secrets store generated beside it, so
// a test renders the example inventory the way `dgs conf export` would.
// internal/conf/validate checks the same tree loads, derives and validates;
// this one checks it renders, which is the half a broken template breaks.
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

// renderExamples exports every target the example inventory holds and returns
// the rendered files by the path they were written under.
func renderExamples(t *testing.T) map[string]string {
	t.Helper()
	root, secretsDir := examplesRoot(t)
	dest := t.TempDir()

	var out bytes.Buffer
	if err := exportAction(strings.NewReader(""), &out, []string{"*"}, map[string]string{"to": dest, "yes": "true"}, configFor(root, secretsDir)); err != nil {
		t.Fatalf("export: %v\n%s", err, out.String())
	}

	files := map[string]string{}
	for _, path := range filesUnder(t, dest) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(dest, path)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.ToSlash(rel)] = string(data)
	}
	if len(files) == 0 {
		t.Fatal("the example inventory rendered nothing")
	}
	return files
}

// exampleFile is the one rendered file whose path ends in suffix.
func exampleFile(t *testing.T, files map[string]string, suffix string) string {
	t.Helper()
	var found []string
	for path := range files {
		if strings.HasSuffix(path, suffix) {
			found = append(found, path)
		}
	}
	switch len(found) {
	case 1:
		return files[found[0]]
	case 0:
		paths := make([]string, 0, len(files))
		for path := range files {
			paths = append(paths, path)
		}
		t.Fatalf("no rendered file ends in %q; rendered %v", suffix, paths)
	default:
		t.Fatalf("%d rendered files end in %q: %v", len(found), suffix, found)
	}
	return ""
}

// TestExamples_EveryTargetRenders is the other half of
// internal/conf/validate's example test: a tree that loads and validates can
// still hold a template that will not run, and a broken example is worse than
// none, since examples/conf is what a reader copies.
func TestExamples_EveryTargetRenders(t *testing.T) {
	files := renderExamples(t)
	for path, content := range files {
		if strings.TrimSpace(content) == "" {
			t.Errorf("%s rendered empty", path)
		}
	}
}

// TestExamples_HysteriaMasqueradeComesFromTheInstance pins
// docs/apps/conf/export.md#two-kinds-of-defaults over the whole pipeline: the
// example server sets masquerade_url in its instance's values, and that is
// what reaches the rendered file — not the service default it overrides.
func TestExamples_HysteriaMasqueradeComesFromTheInstance(t *testing.T) {
	files := renderExamples(t)
	got := exampleFile(t, files, "hy2-sfo01/config.yaml")

	if !strings.Contains(got, "url: https://www.example.com/") {
		t.Errorf("rendered config.yaml does not carry the instance's masquerade_url:\n%s", got)
	}
	if strings.Contains(got, "https://example.org/") {
		t.Errorf("rendered config.yaml still carries the service's default masquerade_url:\n%s", got)
	}
	// A default the instance does not override still stands.
	if !strings.Contains(got, "cert: /etc/hysteria/tls/fullchain.pem") {
		t.Errorf("rendered config.yaml lost a default the instance did not override:\n%s", got)
	}
}

// TestExamples_MicrobinAdminIsNotBasicAuth pins that the two switches are two
// things: the example instance opens the administrative interface and leaves
// site-wide basic auth off, and the rendered file says exactly that.
func TestExamples_MicrobinAdminIsNotBasicAuth(t *testing.T) {
	files := renderExamples(t)
	got := exampleFile(t, files, "microbin-sfo01/server.env")

	for _, want := range []string{
		"MICROBIN_ADMIN_USERNAME=webadmin",
		"MICROBIN_ADMIN_PASSWORD=",
		"MICROBIN_UPLOADER_PASSWORD=",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered server.env has no %s:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"MICROBIN_BASIC_AUTH_USERNAME=",
		"MICROBIN_BASIC_AUTH_PASSWORD=",
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("rendered server.env carries %s though auth_enabled is false:\n%s", unwanted, got)
		}
	}
}

// TestExamples_ExportsDialThePortsPublishedName pins
// docs/apps/conf/inventory.md#the-name-a-port-is-published-at from the client
// side: two services on one node answer to names of their own, and each
// export writes its service's name rather than the node's address.
func TestExamples_ExportsDialThePortsPublishedName(t *testing.T) {
	files := renderExamples(t)
	for suffix, want := range map[string]string{
		"phone-sfo-01-ss-ssserver-link/share.txt":            "@ss.example.net:38250#",
		"laptop-sfo-01-ss-ssserver-json-singbox/config.json": `"server": "ss.example.net"`,
		"phone-sfo-01-hy2-hysteria2-link/share.txt":          "@hy2.example.net:443/",
	} {
		got := exampleFile(t, files, suffix)
		if !strings.Contains(got, want) {
			t.Errorf("%s does not carry %q:\n%s", suffix, want, got)
		}
		if strings.Contains(got, "sfo1.example.net") {
			t.Errorf("%s still carries the node's address:\n%s", suffix, got)
		}
	}
}

// TestExamples_ProfilesListenWhereTheirValuesSay pins
// docs/apps/conf/inventory.md#a-device-with-several-profiles: the example
// laptop's two profiles render one configuration each from the same export,
// and each listens on its own profile's port rather than the defaults'.
func TestExamples_ProfilesListenWhereTheirValuesSay(t *testing.T) {
	files := renderExamples(t)
	for suffix, want := range map[string]string{
		"laptop-sfo-01-ss-ssserver-json-singbox/config.json": `"local_port": 2080`,
		"laptop-sfo-01-ss-ssserver-json-browser/config.json": `"local_port": 1080`,
	} {
		if got := exampleFile(t, files, suffix); !strings.Contains(got, want) {
			t.Errorf("%s does not carry %q:\n%s", suffix, want, got)
		}
	}
}
