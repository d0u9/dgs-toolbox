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
template: templates/server.yaml.tmpl
defaults: document
output: config.yaml
auth: per-principal
`)
	writeFile(t, filepath.Join(root, "services", "hysteria2", "templates", "server.yaml.tmpl"),
		"listen: {{ .listen }} on {{ (node).id }}\n")
	writeFile(t, filepath.Join(root, "services", "hysteria2", "defaults.yaml"), "listen: :443\n")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: us-sfo
    service: hysteria2
    ports:
      main: 443
`)

	secretsDir = t.TempDir()
	return root, secretsDir
}

// buildSharedRoot is a server role with auth: per-principal whose port
// hands out a psk, reached by a managed client, so upstreamFor has an edge
// to resolve and a handed-out secret to look up.
func buildSharedRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "services", "ssserver", "confgen.yaml"), `
template: templates/server.json.tmpl
defaults: element
output: config.json
auth: per-principal
self:
  psk: {set: true}
`)
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "confgen.yaml"), `
template: templates/client.json.tmpl
defaults: element
output: config.json
upstream:
  shared: {}
`)
	writeFile(t, filepath.Join(root, "services", "ssserver", "templates", "server.json.tmpl"), "{}\n")
	writeFile(t, filepath.Join(root, "services", "ssserver", "defaults.yaml"), "{}\n")
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "templates", "client.json.tmpl"),
		"password: {{ join \":\" (upstream).shared }}:{{ (upstream).secret }}\n")
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "defaults.yaml"), "{}\n")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: ssserver
    ports:
      main: {port: 38250, self: [psk.main]}
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
	writeFile(t, filepath.Join(secretsDir, "ss-srv", "main", "doug", "default"), "user-psk")
	return root, secretsDir
}

func TestPreview_UpstreamCombinesOwnWithThePrincipalsSecret(t *testing.T) {
	root, secretsDir := buildSharedRoot(t)
	writeFile(t, filepath.Join(secretsDir, "ss-srv", "self", "psk", "main"), "server-psk")

	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:laptop/laptop-sfo-ssserver-ss-json")

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

// TestPreview_UpstreamMissingSharedNamesIt covers the destination port
// handing out psk.main with no such secret on disk yet — the
// client cannot render a correct password silently, so this must fail
// naming the missing secret rather than rendering one half of it.
func TestPreview_UpstreamMissingSharedNamesIt(t *testing.T) {
	root, secretsDir := buildSharedRoot(t)
	// own/psk deliberately not written.

	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:laptop/laptop-sfo-ssserver-ss-json")

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

// TestPreview_UpstreamSharedOnlyReachesWhatDeclaredIt covers the case a
// reverse proxy in front of a web service makes: the hop it dials hands a
// secret to everyone granted on it, and the proxy still has no business
// reading it. A secret belongs to the instance it is filed under, and it
// crosses to another only because the program dialling declared it needs
// it — the port it reaches never decides that on its own.
func TestPreview_UpstreamSharedOnlyReachesWhatDeclaredIt(t *testing.T) {
	root, secretsDir := buildSharedRoot(t)
	writeFile(t, filepath.Join(secretsDir, "ss-srv", "self", "psk", "main"), "server-psk")
	// The same export, with its declaration taken away and a template that
	// asks anyway. What it renders is what a template naming something it
	// never declared gets: nothing.
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "confgen.yaml"), `
template: templates/client.json.tmpl
defaults: element
output: config.json
`)

	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:laptop/laptop-sfo-ssserver-ss-json")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if m.preview.err != nil {
		t.Fatalf("preview error: %v", m.preview.err)
	}
	got := strings.Join(m.preview.lines, "\n")
	if strings.Contains(got, "server-psk") {
		t.Fatalf("preview = %q, want it to hold no secret of the hop it dials", got)
	}
	if want := "password: :user-psk"; got != want {
		t.Fatalf("preview = %q, want %q", got, want)
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
	writeFile(t, filepath.Join(root, "services", "ssserver", "confgen.yaml"), `
template: templates/server.tmpl
defaults: document
output: config.json
auth: per-principal
`)
	writeFile(t, filepath.Join(root, "services", "ssserver", "templates", "server.tmpl"),
		"{{ range principals \"main\" }}{{ .Name }}={{ .Secret }} {{ end }}")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: ssserver
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
	current := filepath.Join(secretsDir, "ss-srv", "main", "doug", "default")
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
	want := "doug-default=new-secret doug-default=old-secret "
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

// TestPreview_DisruptiveRotationOmitsThePreviousAccount pins the other half:
// a role declaring rotation: disruptive never gets a second account, since
// its template cannot emit two for one principal.
func TestPreview_DisruptiveRotationOmitsThePreviousAccount(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "services", "ssserver", "confgen.yaml"), `
template: templates/server.tmpl
defaults: document
output: config.json
auth: per-principal
rotation: disruptive
`)
	writeFile(t, filepath.Join(root, "services", "ssserver", "templates", "server.tmpl"),
		"{{ range principals \"main\" }}{{ .Name }}={{ .Secret }} {{ end }}")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: ssserver
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
	current := filepath.Join(secretsDir, "ss-srv", "main", "doug", "default")
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
	want := "doug-default=new-secret "
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

// buildFanOutRoot is a reverse proxy in front of two web services, each on
// its own route. The web service authenticates nobody, which is the case a
// proxy always reaches: there is no credential under its port to read.
func buildFanOutRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "services", "caddy", "confgen.yaml"), `
template: templates/Caddyfile.tmpl
defaults: document
output: Caddyfile
auth: none
downstreams: many
`)
	writeFile(t, filepath.Join(root, "services", "caddy", "templates", "Caddyfile.tmpl"),
		"{{ range downstreams }}{{ .Published }} -> {{ .Address }}:{{ .Number }}\n{{ end }}")
	writeFile(t, filepath.Join(root, "services", "caddy", "defaults.yaml"), "{}\n")

	writeFile(t, filepath.Join(root, "services", "web", "confgen.yaml"), `
template: templates/server.env.tmpl
defaults: document
output: server.env
auth: none
`)
	writeFile(t, filepath.Join(root, "services", "web", "templates", "server.env.tmpl"),
		`DOMAIN=https://{{ published "web" }}`+"\n")
	writeFile(t, filepath.Join(root, "services", "web", "defaults.yaml"), "{}\n")

	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: proxy
    service: caddy
    ports:
      https: 443
  - id: vault
    service: web
    bind: 127.0.0.1
    ports:
      web: {port: 8222, published: vault.example.com}
  - id: bin
    service: web
    bind: 127.0.0.1
    ports:
      web: {port: 8080, published: clip.example.com}
`)
	writeFile(t, filepath.Join(root, "routes.yaml"), `
routes:
  vault:
    hops: [proxy:https, vault:web]
  clip:
    hops: [proxy:https, bin:web]
`)
	writeFile(t, filepath.Join(root, "networks.yaml"), `
networks: [internet]
universal: internet
`)
	writeFile(t, filepath.Join(root, "users.yaml"), "users: {}\n")

	return root, t.TempDir()
}

func previewOf(t *testing.T, root, secretsDir, id string) string {
	t.Helper()
	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID(id)

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if m.preview.err != nil {
		t.Fatalf("preview error: %v", m.preview.err)
	}
	return strings.Join(m.preview.lines, "\n")
}

// A fan-out instance renders one entry per route through it, ordered by
// route name, with each downstream's address resolved the way any other edge
// is — loopback here, since the two ends share a node.
func TestPreview_FanOutRendersEveryDownstream(t *testing.T) {
	root, secretsDir := buildFanOutRoot(t)
	got := previewOf(t, root, secretsDir, "inst:srv/proxy")
	want := "clip.example.com -> 127.0.0.1:8080\nvault.example.com -> 127.0.0.1:8222"
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

// The service behind the proxy renders the same name the proxy matches on,
// which is what keeps its own absolute URLs from disagreeing with the site
// block in front of it.
func TestPreview_PublishedReachesTheServiceBehind(t *testing.T) {
	root, secretsDir := buildFanOutRoot(t)
	got := previewOf(t, root, secretsDir, "inst:srv/vault")
	want := "DOMAIN=https://vault.example.com"
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

// buildValuesRoot is a document-defaults service whose instance overrides one
// of the defaults in its own values, and whose template also reads a key only
// the defaults carry.
func buildValuesRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "services", "hysteria2", "confgen.yaml"), `
template: templates/server.yaml.tmpl
defaults: document
output: config.yaml
auth: per-principal
`)
	writeFile(t, filepath.Join(root, "services", "hysteria2", "templates", "server.yaml.tmpl"),
		"masquerade: {{ .masquerade_url }}\ncert: {{ .tls_cert }}\nid: {{ has . \"id\" }}\n")
	writeFile(t, filepath.Join(root, "services", "hysteria2", "defaults.yaml"),
		"masquerade_url: https://example.org/\ntls_cert: /etc/hysteria/tls/fullchain.pem\n")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: us-sfo
    service: hysteria2
    ports:
      main: {port: 443, protocol: udp}
    values:
      masquerade_url: https://www.example.com/
`)

	secretsDir = t.TempDir()
	return root, secretsDir
}

// TestPreview_InstanceValuesOverrideDocumentDefaults pins
// docs/apps/conf/export.md#two-kinds-of-defaults: for a document-shaped
// defaults file, an instance's values merge over the whole document. The
// instance's own keys — id, service, bind, ports — are not part of it: they
// are what dgs itself needs and are read through the instance function, and
// values is the only place a node file may write a service's own settings.
func TestPreview_InstanceValuesOverrideDocumentDefaults(t *testing.T) {
	root, secretsDir := buildValuesRoot(t)
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
	want := strings.Join([]string{
		// The instance's value wins,
		"masquerade: https://www.example.com/",
		// a default it does not override still stands,
		"cert: /etc/hysteria/tls/fullchain.pem",
		// and the instance's own id is not a key of the document.
		"id: false",
	}, "\n")
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}
