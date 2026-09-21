package conf

import (
	"path/filepath"
	"strings"
	"testing"
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

	p := renderPreview(t, root, secretsDir, "laptop-sfo-ssserver-ss-json")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
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

	p := renderPreview(t, root, secretsDir, "laptop-sfo-ssserver-ss-json")
	if p.err == nil {
		t.Fatal("preview.err = nil, want an error naming the missing own secret")
	}
	if !strings.Contains(p.err.Error(), "psk") {
		t.Fatalf("preview.err = %v, want it to name \"psk\"", p.err)
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

	p := renderPreview(t, root, secretsDir, "laptop-sfo-ssserver-ss-json")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
	if strings.Contains(got, "server-psk") {
		t.Fatalf("preview = %q, want it to hold no secret of the hop it dials", got)
	}
	if want := "password: :user-psk"; got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

// TestPreview_UpstreamValuesReachTheClientThatDeclaredThem pins
// docs/apps/conf/inventory.md#what-a-service-needs-from-its-upstream for
// `values`: a parameter the two ends have to agree on is written on the
// instance that listens, and the client's file is rendered from there rather
// than from a second copy of it.
func TestPreview_UpstreamValuesReachTheClientThatDeclaredThem(t *testing.T) {
	root, secretsDir := buildSharedRoot(t)
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "confgen.yaml"), `
template: templates/client.json.tmpl
defaults: element
output: config.json
upstream:
  values: {}
`)
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "templates", "client.json.tmpl"),
		"hop: {{ (upstream).values.port_hopping }}\n")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: ssserver
    ports:
      main: {port: 38250, self: [psk.main]}
    values:
      port_hopping: "20000-25000"
`)

	p := renderPreview(t, root, secretsDir, "laptop-sfo-ssserver-ss-json")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
	if want := "hop: 20000-25000"; got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

// TestPreview_UpstreamValuesOnlyReachWhatDeclaredIt is the same rule `shared`
// has: the hop's values cross to another instance because the program
// dialling declared it needs them, never because the instance listening holds
// them.
func TestPreview_UpstreamValuesOnlyReachWhatDeclaredIt(t *testing.T) {
	root, secretsDir := buildSharedRoot(t)
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "confgen.yaml"), `
template: templates/client.json.tmpl
defaults: element
output: config.json
`)
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "templates", "client.json.tmpl"),
		"hop: {{ (upstream).values.port_hopping }}\n")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: ssserver
    ports:
      main: {port: 38250, self: [psk.main]}
    values:
      port_hopping: "20000-25000"
`)

	p := renderPreview(t, root, secretsDir, "laptop-sfo-ssserver-ss-json")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
	if strings.Contains(got, "20000-25000") {
		t.Fatalf("preview = %q, want it to hold no value of the hop it dials", got)
	}
}

// TestPreview_ForwardedRouteDialsTheRelayAndAuthenticatesAtTheExit pins
// docs/apps/conf/inventory.md#a-service-that-forwards: a client's file is
// built from both ends of a forwarded chain — the relay's address and port,
// the exit's account, secret and shared secrets — and `exit` carries what
// belongs to the far end and cannot be taken from the near one, such as the
// name a certificate was issued for.
func TestPreview_ForwardedRouteDialsTheRelayAndAuthenticatesAtTheExit(t *testing.T) {
	root, secretsDir := buildSharedRoot(t)
	writeFile(t, filepath.Join(secretsDir, "ss-srv", "self", "psk", "main"), "server-psk")
	writeFile(t, filepath.Join(root, "services", "realm", "confgen.yaml"), `
template: templates/config.toml.tmpl
defaults: document
output: config.toml
auth: none
forwards: true
`)
	writeFile(t, filepath.Join(root, "services", "realm", "templates", "config.toml.tmpl"), "{}\n")
	writeFile(t, filepath.Join(root, "services", "realm", "defaults.yaml"), "{}\n")
	writeFile(t, filepath.Join(root, "services", "ssserver", "exports", "ss-json", "templates", "client.json.tmpl"),
		"server: {{ or (upstream).published (upstream).address }}:{{ (upstream).port }}\n"+
			"sni: {{ (upstream).exit.published }}\n"+
			"password: {{ join \":\" (upstream).shared }}:{{ (upstream).secret }}\n")
	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: ss-srv
    service: ssserver
    ports:
      main: {port: 38250, published: exit.example.com, self: [psk.main]}
`)
	writeFile(t, filepath.Join(root, "nodes", "relay.yaml"), `
id: relay
networks:
  internet: 203.0.113.20
instances:
  - id: fwd-relay
    service: realm
    ports:
      ss: {port: 40000, published: relay.example.net}
`)
	writeFile(t, filepath.Join(root, "routes.yaml"), `
routes:
  sfo:
    hops: [fwd-relay:ss, ss-srv:main]
`)

	p := renderPreview(t, root, secretsDir, "laptop-sfo-ssserver-ss-json")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
	want := "server: relay.example.net:40000\n" +
		"sni: exit.example.com\n" +
		"password: server-psk:user-psk"
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

// TestPreview_AForwarderReadsAnAddressAndNoCredential is the relay's own
// side of the same rule: it dials a port that authenticates every principal
// separately and still holds no account there, because nothing granted it
// one. Reading a secret for it would fail on a file that does not exist.
func TestPreview_AForwarderReadsAnAddressAndNoCredential(t *testing.T) {
	root, secretsDir := buildSharedRoot(t)
	writeFile(t, filepath.Join(secretsDir, "ss-srv", "self", "psk", "main"), "server-psk")
	writeFile(t, filepath.Join(root, "services", "realm", "confgen.yaml"), `
template: templates/config.toml.tmpl
defaults: document
output: config.toml
auth: none
forwards: true
`)
	writeFile(t, filepath.Join(root, "services", "realm", "defaults.yaml"), "{}\n")
	writeFile(t, filepath.Join(root, "services", "realm", "templates", "config.toml.tmpl"),
		"remote = {{ printf \"%s:%v\" (upstream).address (upstream).port | toJSON }}\n"+
			"secret = {{ toJSON (upstream).secret }}\n")
	writeFile(t, filepath.Join(root, "nodes", "relay.yaml"), `
id: relay
networks:
  internet: 203.0.113.20
instances:
  - id: fwd-relay
    service: realm
    ports:
      ss: {port: 40000}
`)
	writeFile(t, filepath.Join(root, "routes.yaml"), `
routes:
  sfo:
    hops: [fwd-relay:ss, ss-srv:main]
`)

	p := renderPreview(t, root, secretsDir, "fwd-relay")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
	want := "remote = \"203.0.113.10:38250\"\nsecret = \"\""
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

func TestPreview_RendersTheSameWayExportWould(t *testing.T) {
	root, secretsDir := buildRenderableRoot(t)
	p := renderPreview(t, root, secretsDir, "us-sfo")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
	want := "listen: :443 on srv"
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
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

	p := renderPreview(t, root, secretsDir, "ss-srv")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
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

	p := renderPreview(t, root, secretsDir, "ss-srv")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
	want := "doug-default=new-secret "
	if got != want {
		t.Fatalf("preview = %q, want %q (disruptive: no previous account)", got, want)
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

func previewOf(t *testing.T, root, secretsDir, instance string) string {
	t.Helper()
	p := renderPreview(t, root, secretsDir, instance)
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	return strings.Join(p.lines, "\n")
}

// A fan-out instance renders one entry per route through it, ordered by
// route name, with each downstream's address resolved the way any other edge
// is — loopback here, since the two ends share a node.
func TestPreview_FanOutRendersEveryDownstream(t *testing.T) {
	root, secretsDir := buildFanOutRoot(t)
	got := previewOf(t, root, secretsDir, "proxy")
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
	got := previewOf(t, root, secretsDir, "vault")
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
	p := renderPreview(t, root, secretsDir, "us-sfo")
	if p.err != nil {
		t.Fatalf("preview error: %v", p.err)
	}
	got := strings.Join(p.lines, "\n")
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

type rendered struct {
	lines []string
	err   error
}

// renderPreview renders one instance the way export and inspect do.
func renderPreview(t *testing.T, root, secretsDir, instance string) rendered {
	t.Helper()
	l, err := load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	r := renderer{l: l, rootPath: root, secretsDir: secretsDir}
	out, err := r.renderTarget(instance)
	if err != nil {
		return rendered{err: err}
	}
	return rendered{lines: strings.Split(strings.TrimRight(string(out), "\n"), "\n")}
}

// buildPublishedEdgesRoot has one published web port and three instances
// dialing it: one on the same node, one across a private network, and one
// that shares nothing with it but the universal network.
func buildPublishedEdgesRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "services", "fwd", "confgen.yaml"), `
template: templates/fwd.tmpl
defaults: document
output: fwd.txt
auth: none
`)
	writeFile(t, filepath.Join(root, "services", "fwd", "templates", "fwd.tmpl"),
		"{{ or (upstream).published (upstream).address }}")
	writeFile(t, filepath.Join(root, "services", "fwd", "defaults.yaml"), "{}\n")
	writeFile(t, filepath.Join(root, "services", "web", "confgen.yaml"), `
template: templates/web.tmpl
defaults: document
output: web.txt
auth: none
`)
	writeFile(t, filepath.Join(root, "services", "web", "templates", "web.tmpl"), "web\n")
	writeFile(t, filepath.Join(root, "services", "web", "defaults.yaml"), "{}\n")

	writeFile(t, filepath.Join(root, "nodes", "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
  home: 192.168.1.10
instances:
  - id: web
    service: web
    ports:
      web: {port: 8443, published: web.example.com}
  - id: fwd-local
    service: fwd
    ports:
      in: 1081
`)
	writeFile(t, filepath.Join(root, "nodes", "lan.yaml"), `
id: lan
networks:
  home: 192.168.1.20
instances:
  - id: fwd-lan
    service: fwd
    ports:
      in: 1081
`)
	writeFile(t, filepath.Join(root, "nodes", "far.yaml"), `
id: far
networks:
  internet: 198.51.100.7
instances:
  - id: fwd-far
    service: fwd
    ports:
      in: 1081
`)
	writeFile(t, filepath.Join(root, "routes.yaml"), `
routes:
  local:
    hops: [fwd-local:in, web:web]
  lan:
    hops: [fwd-lan:in, web:web]
  far:
    hops: [fwd-far:in, web:web]
`)
	writeFile(t, filepath.Join(root, "networks.yaml"), `
networks: [home, internet]
universal: internet
`)
	writeFile(t, filepath.Join(root, "users.yaml"), "users: {}\n")
	return root, t.TempDir()
}

// (upstream).published is given only on an edge resolved on the universal
// network, where the name is taken to resolve. A same-node or private-network
// edge keeps the address it was given, so `or published address` falls back
// to loopback or the LAN address rather than sending the client out to the
// public name.
func TestPreview_UpstreamPublishedOnlyOnTheUniversalNetwork(t *testing.T) {
	root, secretsDir := buildPublishedEdgesRoot(t)
	for instance, want := range map[string]string{
		"fwd-local": "127.0.0.1",
		"fwd-lan":   "192.168.1.10",
		"fwd-far":   "web.example.com",
	} {
		if got := previewOf(t, root, secretsDir, instance); got != want {
			t.Errorf("%s = %q, want %q", instance, got, want)
		}
	}
}
