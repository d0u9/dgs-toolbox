package conf

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// buildRenderableRoot is buildRoot plus the template and defaults files a
// preview actually needs to render something.
func buildRenderableRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "services", "hysteria2", "confgen.yaml"), `
roles:
  server:
    template: templates/server.yaml.tmpl
    defaults: document
    output: config.yaml
    auth: per-principal
`)
	writeFile(t, filepath.Join(root, "services", "hysteria2", "templates", "server.yaml.tmpl"),
		"listen: {{ .listen }} on {{ (node).id }}\n")
	writeFile(t, filepath.Join(root, "services", "hysteria2", "server", "defaults.yaml"), "listen: :443\n")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: us-sfo
    service: hysteria2
    role: server
    ports:
      main: 443
`)

	secretsDir = t.TempDir()
	return root, secretsDir
}

// buildCombineOwnRoot is a server role with auth: per-principal and
// combine_own: psk, reached by a managed client, so upstreamFor has an edge
// to resolve and a combine_own to look up.
func buildCombineOwnRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "confgen.yaml"), `
roles:
  server:
    template: templates/server.json.tmpl
    defaults: element
    output: config.json
    auth: per-principal
    reached_by: ss-rust
    own: [psk]
    combine_own: psk
  ss-rust:
    template: templates/client.json.tmpl
    defaults: element
    output: config.json
    auth: none
`)
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "templates", "server.json.tmpl"), "{}\n")
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "server", "defaults.yaml"), "{}\n")
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "templates", "client.json.tmpl"),
		"password: {{ (upstream).own }}:{{ (upstream).secret }}\n")
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "ss-rust", "defaults.yaml"), "{}\n")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: shadowsocks-rust
    role: server
    ports:
      main: 38250
`)
	writeFile(t, filepath.Join(root, "nodes", "laptop.yaml"), `
id: laptop
owner: doug
`)
	writeFile(t, filepath.Join(root, "users.yaml"), `
users:
  doug:
    access: [sfo]
`)
	writeFile(t, filepath.Join(root, "routes.yaml"), `
routes:
  sfo:
    hops: [ss-srv:main]
`)
	writeFile(t, filepath.Join(root, "networks.yaml"), `
networks: [internet]
universal: internet
`)

	secretsDir = t.TempDir()
	writeFile(t, filepath.Join(secretsDir, "ss-srv", "main", "node", "laptop"), "user-psk")
	return root, secretsDir
}

func TestPreview_UpstreamCombinesOwnWithThePrincipalsSecret(t *testing.T) {
	root, secretsDir := buildCombineOwnRoot(t)
	writeFile(t, filepath.Join(secretsDir, "ss-srv", "own", "psk"), "server-psk")

	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:laptop/laptop-sfo")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if m.preview.err != nil {
		t.Fatalf("preview error: %v", m.preview.err)
	}
	got := strings.Join(m.preview.lines, "\n")
	want := "password: server-psk:user-psk"
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

// TestPreview_UpstreamMissingCombineOwnNamesIt covers the destination role
// declaring combine_own: psk with no own/psk secret on disk yet — the
// client cannot render a correct password silently, so this must fail
// naming the missing secret rather than rendering one half of it.
func TestPreview_UpstreamMissingCombineOwnNamesIt(t *testing.T) {
	root, secretsDir := buildCombineOwnRoot(t)
	// own/psk deliberately not written.

	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:laptop/laptop-sfo")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if m.preview.err == nil {
		t.Fatal("preview.err = nil, want an error naming the missing own secret")
	}
	if !strings.Contains(m.preview.err.Error(), "psk") {
		t.Fatalf("preview.err = %v, want it to name \"psk\"", m.preview.err)
	}
}

func TestPreview_RendersTheSameWayExportWould(t *testing.T) {
	root, secretsDir := buildRenderableRoot(t)
	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:srv/us-sfo")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if m.preview.err != nil {
		t.Fatalf("preview error: %v", m.preview.err)
	}
	got := strings.Join(m.preview.lines, "\n")
	want := "listen: :443 on srv"
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

func TestPreview_EscClosesAndReturnsToTheTree(t *testing.T) {
	root, secretsDir := buildRenderableRoot(t)
	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:srv/us-sfo")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if !m.CapturesShellKey("esc") {
		t.Fatal("CapturesShellKey(esc) = false while previewing, want true")
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(Model)
	if m.preview != nil {
		t.Fatal("esc did not close the preview")
	}
	if m.CapturesShellKey("esc") {
		t.Fatal("CapturesShellKey(esc) = true once the preview is closed")
	}
}

// TestPreview_RotationEmitsBothCurrentAndPreviousAccounts pins
// docs/apps/conf/inventory.md#rotation: a role rendering an account table
// emits both the current and previous value as two accounts for one
// principal, until the .previous file is deleted.
func TestPreview_RotationEmitsBothCurrentAndPreviousAccounts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "confgen.yaml"), `
roles:
  server:
    template: templates/server.tmpl
    defaults: document
    output: config.json
    auth: per-principal
`)
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "templates", "server.tmpl"),
		"{{ range principals \"main\" }}{{ .Name }}={{ .Secret }} {{ end }}")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: shadowsocks-rust
    role: server
    ports:
      main: 38250
`)
	writeFile(t, filepath.Join(root, "nodes", "laptop.yaml"), `
id: laptop
owner: doug
`)
	writeFile(t, filepath.Join(root, "users.yaml"), "users:\n  doug:\n    access: [sfo]\n")
	writeFile(t, filepath.Join(root, "routes.yaml"), "routes:\n  sfo:\n    hops: [ss-srv:main]\n")
	writeFile(t, filepath.Join(root, "networks.yaml"), "networks: [internet]\nuniversal: internet\n")

	secretsDir := t.TempDir()
	current := filepath.Join(secretsDir, "ss-srv", "main", "node", "laptop")
	writeFile(t, current, "new-secret")
	writeFile(t, current+".previous", "old-secret")

	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:srv/ss-srv")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if m.preview.err != nil {
		t.Fatalf("preview error: %v", m.preview.err)
	}
	got := strings.Join(m.preview.lines, "\n")
	want := "doug-laptop=new-secret doug-laptop=old-secret "
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

// TestPreview_DisruptiveRotationOmitsThePreviousAccount pins the other half:
// a role declaring rotation: disruptive never gets a second account, since
// its template cannot emit two for one principal.
func TestPreview_DisruptiveRotationOmitsThePreviousAccount(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "confgen.yaml"), `
roles:
  server:
    template: templates/server.tmpl
    defaults: document
    output: config.json
    auth: per-principal
    rotation: disruptive
`)
	writeFile(t, filepath.Join(root, "services", "shadowsocks-rust", "templates", "server.tmpl"),
		"{{ range principals \"main\" }}{{ .Name }}={{ .Secret }} {{ end }}")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: shadowsocks-rust
    role: server
    ports:
      main: 38250
`)
	writeFile(t, filepath.Join(root, "nodes", "laptop.yaml"), `
id: laptop
owner: doug
`)
	writeFile(t, filepath.Join(root, "users.yaml"), "users:\n  doug:\n    access: [sfo]\n")
	writeFile(t, filepath.Join(root, "routes.yaml"), "routes:\n  sfo:\n    hops: [ss-srv:main]\n")
	writeFile(t, filepath.Join(root, "networks.yaml"), "networks: [internet]\nuniversal: internet\n")

	secretsDir := t.TempDir()
	current := filepath.Join(secretsDir, "ss-srv", "main", "node", "laptop")
	writeFile(t, current, "new-secret")
	writeFile(t, current+".previous", "old-secret")

	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:srv/ss-srv")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if m.preview.err != nil {
		t.Fatalf("preview error: %v", m.preview.err)
	}
	got := strings.Join(m.preview.lines, "\n")
	want := "doug-laptop=new-secret "
	if got != want {
		t.Fatalf("preview = %q, want %q (disruptive: no previous account)", got, want)
	}
}

func TestPreview_BrokenNodeCannotBePreviewed(t *testing.T) {
	m := newModel(buildRoot(t), "")
	m.width, m.height = 80, 24
	m.list.SelectID("node:nodes/bad.yaml")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	if m.preview != nil {
		t.Fatal("previewing a broken node opened a preview")
	}
}
