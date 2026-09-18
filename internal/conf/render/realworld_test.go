package render_test

// This exercises the exact template text migrated into
// ~/.dot/conf/05-confgen/services/shadowsocks-rust — the multi-port,
// combine_own case docs/apps/conf/inventory.md#a-shared-identity-alongside-a-principals-own
// describes, rendered against fixture data shaped like that inventory rather
// than a minimal one. It is a regression test for the migration, not a
// generic feature test — see internal/conf/render/integration_test.go for
// that.

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/render"
	"dgs-toolbox/internal/conf/secretstore"
)

const ssServerTemplate = `{{- define "port" -}}
{{- $port := . -}}
{{- $userList := slice -}}
{{- range $p := principals $port.name -}}
  {{- $userList = append $userList (dict "name" $p.Name "password" $p.Secret) -}}
{{- end -}}
{{- merge (dict "server_port" $port.number "password" (secret "psk") "users" $userList) (defaults) | toJSON -}}
{{- end -}}
{
  "servers": [
    {{- $first := true -}}
    {{- range $name, $number := (instance).ports -}}
      {{- if not $first }},{{ end -}}
      {{ template "port" (dict "name" $name "number" $number) }}
      {{- $first = false -}}
    {{- end -}}
  ]
}
`

const ssServerDefaults = `server: "::"
method: 2022-blake3-aes-256-gcm
mode: tcp_and_udp
timeout: 300
no_delay: true
keep_alive: 15
udp_timeout: 300
udp_max_associations: 4096
fast_open: true
users: []
`

const ssClientTemplate = `{{- $password := printf "%s:%s" (upstream).own (upstream).secret -}}
{{- $server := merge (dict "server" (upstream).address "server_port" (upstream).port "password" $password) (defaults) -}}
{{- $ports := dict -}}
{{- if has (instance) "ports" -}}
  {{- $ports = (instance).ports -}}
{{- end -}}
{{- if eq (len $ports) 0 -}}
  {{- $ports = dict "socks" 1080 -}}
{{- end -}}
{{- $locals := slice -}}
{{- range $name, $number := $ports -}}
  {{- $proto := "socks" -}}
  {{- $mode := "tcp_and_udp" -}}
  {{- if eq $name "http" -}}
    {{- $proto = "http" -}}
    {{- $mode = "tcp_only" -}}
  {{- end -}}
  {{- $locals = append $locals (dict "local_address" "127.0.0.1" "local_port" $number "protocol" $proto "mode" $mode) -}}
{{- end -}}
{
  "servers": [{{ $server | toJSON }}],
  "locals": [
    {{- $first := true -}}
    {{- range $l := $locals -}}
      {{- if not $first }},{{ end }}{{ $l | toJSON }}
      {{- $first = false -}}
    {{- end -}}
  ]
}
`

const ssClientDefaults = `method: 2022-blake3-aes-256-gcm
mode: tcp_and_udp
timeout: 300
no_delay: true
keep_alive: 15
udp_timeout: 300
fast_open: true
`

func TestRealWorld_ShadowsocksServerCombinesOwnAcrossTwoPorts(t *testing.T) {
	inv := &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "us-sfo-dgo-linux-01",
				Networks: inventory.Networks{"internet": "203.0.113.10"},
				Instances: []inventory.Instance{
					{ID: "ss-sfo01", Service: "shadowsocks-rust", Role: "server",
						Ports: map[string]int{"main": 38250, "relay": 52146}},
				},
			},
			{ID: "macbook", Owner: "doug"},
		},
		Users: map[string]inventory.User{
			"doug":            {Access: []string{"sfo"}},
			"jane":            {Devices: inventory.DevicesUnmanaged, Access: []string{"sfo"}},
			"cn-relay":        {Devices: inventory.DevicesUnmanaged, Access: []string{"sfo-relay"}},
			"default-account": {Username: "default", Devices: inventory.DevicesUnmanaged, Access: []string{"sfo"}},
		},
		Routes: map[string]inventory.Route{
			"sfo":       {Hops: []string{"ss-sfo01:main"}},
			"sfo-relay": {Hops: []string{"ss-sfo01:relay"}},
		},
		Networks:  []string{"internet"},
		Universal: "internet",
	}
	manifests := map[string]confgen.Manifest{
		"shadowsocks-rust": {
			Secret: confgen.Secret{Kind: "base64", Bytes: 32},
			Roles: map[string]confgen.Role{
				"server":  {Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust", CombineOwn: "psk", Rotation: confgen.RotationDisruptive},
				"ss-rust": {Auth: confgen.AuthNone},
			},
		},
	}

	model, err := derive.Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	secretsRoot := t.TempDir()
	implied := secretstore.ImpliedPaths(inv, manifests, model)
	if err := secretstore.Generate(secretsRoot, implied, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	renderPort := func(port string) map[string]any {
		var principals []render.Principal
		for _, p := range model.Principals("ss-sfo01", port) {
			v, err := secretstore.ReadValue(secretsRoot, secretstore.Path{
				Instance: "ss-sfo01", Port: port, Kind: string(p.Kind), Name: p.ID,
			})
			if err != nil {
				t.Fatalf("ReadValue: %v", err)
			}
			principals = append(principals, render.Principal{Name: p.Name, Secret: v})
		}
		own, err := secretstore.ReadOwn(secretsRoot, "ss-sfo01")
		if err != nil {
			t.Fatalf("ReadOwn: %v", err)
		}
		out, err := render.Render(render.Input{
			Target:       render.Target{Service: "shadowsocks-rust", Role: "server", Instance: "ss-sfo01"},
			Template:     ssServerTemplate,
			Defaults:     []byte(ssServerDefaults),
			DefaultsKind: confgen.DefaultsElement,
			Instance:     map[string]any{"id": "ss-sfo01", "service": "shadowsocks-rust", "role": "server", "ports": map[string]int{"main": 38250, "relay": 52146}},
			Principals:   map[string][]render.Principal{port: principals},
			Own:          own,
		})
		if err != nil {
			t.Fatalf("Render(%s): %v", port, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("Render(%s) produced invalid JSON: %v\n%s", port, err, out)
		}
		return doc
	}

	main := renderPort("main")
	servers, ok := main["servers"].([]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("main servers = %#v, want 2 entries (main and relay port both rendered per port call)", main["servers"])
	}
	// This call rendered only the "main" port's principals: exactly one
	// entry should carry a non-empty "users" list, and both entries must
	// share the same server-wide password, since combine_own is per
	// instance, not per port.
	firstPassword := servers[0].(map[string]any)["password"]
	for i, s := range servers {
		entry := s.(map[string]any)
		if entry["password"] != firstPassword {
			t.Fatalf("entry %d password = %v, want %v (own/psk shared across ports)", i, entry["password"], firstPassword)
		}
		if entry["method"] != "2022-blake3-aes-256-gcm" {
			t.Fatalf("entry %d missing defaults merge: %+v", i, entry)
		}
	}
	usersOnMain := servers[0].(map[string]any)["users"].([]any)
	if len(usersOnMain) != 3 {
		t.Fatalf("main port users = %v, want doug-macbook, jane and default (all granted route sfo)", usersOnMain)
	}
	names := map[string]bool{}
	for _, u := range usersOnMain {
		names[u.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"doug-macbook", "jane", "default"} {
		if !names[want] {
			t.Fatalf("main port users = %v, missing %q", usersOnMain, want)
		}
	}

	// Now the client side: doug's macbook connecting to ss-sfo01:main.
	var edge *derive.Edge
	for i := range model.Edges {
		if model.Edges[i].FromInstance == "macbook-sfo" {
			edge = &model.Edges[i]
		}
	}
	if edge == nil {
		t.Fatal("no edge for macbook-sfo")
	}
	ownVal, err := secretstore.ReadOwn(secretsRoot, "ss-sfo01")
	if err != nil {
		t.Fatal(err)
	}
	principalSecret, err := secretstore.ReadValue(secretsRoot, secretstore.Path{
		Instance: "ss-sfo01", Port: "main", Kind: "node", Name: "macbook",
	})
	if err != nil {
		t.Fatal(err)
	}
	clientOut, err := render.Render(render.Input{
		Target:       render.Target{Service: "shadowsocks-rust", Role: "ss-rust", Instance: "macbook-sfo"},
		Template:     ssClientTemplate,
		Defaults:     []byte(ssClientDefaults),
		DefaultsKind: confgen.DefaultsElement,
		Instance:     map[string]any{"id": "macbook-sfo", "service": "shadowsocks-rust", "role": "ss-rust"},
		Upstream:     map[string]any{"address": edge.Address, "port": edge.Port, "secret": principalSecret, "own": ownVal["psk"]},
	})
	if err != nil {
		t.Fatalf("Render(client): %v", err)
	}
	var clientDoc struct {
		Servers []map[string]any `json:"servers"`
		Locals  []map[string]any `json:"locals"`
	}
	if err := json.Unmarshal(clientOut, &clientDoc); err != nil {
		t.Fatalf("client output invalid JSON: %v\n%s", err, clientOut)
	}
	if len(clientDoc.Servers) != 1 {
		t.Fatalf("client servers = %+v, want 1", clientDoc.Servers)
	}
	wantPassword := ownVal["psk"] + ":" + principalSecret
	if clientDoc.Servers[0]["password"] != wantPassword {
		t.Fatalf("client password = %v, want own:principal = %q", clientDoc.Servers[0]["password"], wantPassword)
	}
	if len(clientDoc.Locals) != 1 || clientDoc.Locals[0]["local_port"] != float64(1080) {
		t.Fatalf("client locals = %+v, want one fallback socks listener on 1080 (macbook has no port override)", clientDoc.Locals)
	}

	// phone overrides ports: socks/http both named explicitly.
	phoneOut, err := render.Render(render.Input{
		Target:       render.Target{Service: "shadowsocks-rust", Role: "ss-rust", Instance: "phone-sfo"},
		Template:     ssClientTemplate,
		Defaults:     []byte(ssClientDefaults),
		DefaultsKind: confgen.DefaultsElement,
		Instance:     map[string]any{"id": "phone-sfo", "ports": map[string]int{"socks": 10080, "http": 18080}},
		Upstream:     map[string]any{"address": edge.Address, "port": edge.Port, "secret": principalSecret, "own": ownVal["psk"]},
	})
	if err != nil {
		t.Fatalf("Render(phone client): %v", err)
	}
	var phoneDoc struct {
		Locals []map[string]any `json:"locals"`
	}
	if err := json.Unmarshal(phoneOut, &phoneDoc); err != nil {
		t.Fatalf("phone output invalid JSON: %v\n%s", err, phoneOut)
	}
	if len(phoneDoc.Locals) != 2 {
		t.Fatalf("phone locals = %+v, want socks and http", phoneDoc.Locals)
	}
	byProto := map[string]map[string]any{}
	for _, l := range phoneDoc.Locals {
		byProto[l["protocol"].(string)] = l
	}
	if byProto["socks"]["local_port"] != float64(10080) || byProto["http"]["local_port"] != float64(18080) {
		t.Fatalf("phone locals = %+v, want socks:10080 http:18080", phoneDoc.Locals)
	}
	if byProto["http"]["mode"] != "tcp_only" {
		t.Fatalf("http local mode = %v, want tcp_only", byProto["http"]["mode"])
	}
}

const hy2ServerTemplate = `{{- $userpass := dict -}}
{{- range $p := principals "main" -}}
  {{- $userpass = merge (dict $p.Name $p.Secret) $userpass -}}
{{- end -}}
{{- $config := merge (dict
      "listen" (printf ":%d" (index (instance).ports "main"))
      "auth" (dict "type" "userpass" "userpass" $userpass)
    ) . -}}
{{- $config = omit $config "id" "service" "role" "ports" "bind" "values" -}}
{{ $config | toYAML }}
`

const hy2ServerDefaults = `tls:
  cert: /etc/hysteria/tls/fullchain.pem
  key: /etc/hysteria/tls/privkey.pem
ignoreClientBandwidth: false
speedTest: false
masquerade:
  type: proxy
  proxy:
    url: https://matrix-au.org/
    rewriteHost: true
`

func TestRealWorld_Hysteria2ServerListensAndAuthenticates(t *testing.T) {
	instance := map[string]any{
		"id": "hy2-sfo01", "service": "hysteria2", "role": "server",
		"ports": map[string]int{"main": 443},
	}
	out, err := render.Render(render.Input{
		Target:       render.Target{Service: "hysteria2", Role: "server", Instance: "hy2-sfo01"},
		Template:     hy2ServerTemplate,
		Defaults:     []byte(hy2ServerDefaults),
		DefaultsKind: confgen.DefaultsDocument,
		Instance:     instance,
		Principals: map[string][]render.Principal{
			"main": {
				{Name: "doug-macbook", Secret: "secret-doug"},
				{Name: "jane", Secret: "secret-jane"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output invalid YAML: %v\n%s", err, out)
	}
	if doc["listen"] != ":443" {
		t.Fatalf("listen = %v, want :443 (derived from instance.ports.main, not hardcoded in defaults)", doc["listen"])
	}
	if doc["id"] != nil || doc["ports"] != nil {
		t.Fatalf("output leaked instance bookkeeping keys: %+v", doc)
	}
	auth, ok := doc["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth = %#v, want a mapping", doc["auth"])
	}
	userpass, ok := auth["userpass"].(map[string]any)
	if !ok || userpass["doug-macbook"] != "secret-doug" || userpass["jane"] != "secret-jane" {
		t.Fatalf("auth.userpass = %#v", auth["userpass"])
	}
	masquerade, ok := doc["masquerade"].(map[string]any)
	if !ok {
		t.Fatalf("masquerade missing from defaults merge: %+v", doc)
	}
	proxy, ok := masquerade["proxy"].(map[string]any)
	if !ok || proxy["url"] != "https://matrix-au.org/" {
		t.Fatalf("masquerade.proxy = %#v", masquerade["proxy"])
	}
}

const microbinServerTemplate = `{{- $values := dict -}}
{{- if has (instance) "values" -}}
  {{- $values = (instance).values -}}
{{- end -}}
{{- $env := omit . "id" "service" "role" "ports" "bind" "values" -}}
{{- $env = merge (dict
      "MICROBIN_BIND" (instance).bind
      "MICROBIN_PORT" (index (instance).ports "main")
    ) $env -}}
{{- if index $values "public_path" -}}
  {{- $env = merge (dict "MICROBIN_PUBLIC_PATH" (index $values "public_path")) $env -}}
{{- end -}}
{{- if index $values "auth_enabled" -}}
  {{- $env = merge (dict
        "MICROBIN_BASIC_AUTH_USERNAME" (required (secret "auth_username") "missing MicroBin auth username")
        "MICROBIN_BASIC_AUTH_PASSWORD" (required (secret "auth_password") "missing MicroBin auth password")
        "MICROBIN_ADMIN_USERNAME" (required (secret "admin_username") "missing MicroBin admin username")
        "MICROBIN_ADMIN_PASSWORD" (required (secret "admin_password") "missing MicroBin admin password")
      ) $env -}}
{{- end -}}
{{- if index $values "upload_enabled" -}}
  {{- $env = merge (dict
        "MICROBIN_READONLY" true
        "MICROBIN_UPLOADER_PASSWORD" (required (secret "upload_password") "missing MicroBin uploader password")
      ) $env -}}
{{- end -}}
{{- range $name, $value := $env }}
{{ $name }}={{ $value | toJSON }}
{{ end -}}
`

const microbinServerDefaults = `MICROBIN_PRIVATE: true
MICROBIN_DEFAULT_PRIVACY: secret
MICROBIN_DATA_DIR: /var/lib/microbin
`

func TestRealWorld_MicroBinServerEmitsOwnSecretsWhenEnabled(t *testing.T) {
	instance := map[string]any{
		"id": "bin-sfo01", "service": "microbin", "role": "server",
		"bind": "127.0.0.1", "ports": map[string]int{"main": 8080},
		"values": map[string]any{
			"public_path":    "https://clip.matrix-au.org/",
			"auth_enabled":   true,
			"upload_enabled": true,
		},
	}
	own := map[string]string{
		"auth_username":   "clip",
		"auth_password":   "auth-pw",
		"admin_username":  "admin",
		"admin_password":  "admin-pw",
		"upload_password": "upload-pw",
	}
	out, err := render.Render(render.Input{
		Target:       render.Target{Service: "microbin", Role: "server", Instance: "bin-sfo01"},
		Template:     microbinServerTemplate,
		Defaults:     []byte(microbinServerDefaults),
		DefaultsKind: confgen.DefaultsDocument,
		Instance:     instance,
		Own:          own,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	env := parseEnvLines(string(out))
	if env["MICROBIN_BIND"] != `"127.0.0.1"` || env["MICROBIN_PORT"] != "8080" {
		t.Fatalf("env = %+v, want bind/port from the instance, not defaults", env)
	}
	if env["MICROBIN_PUBLIC_PATH"] != `"https://clip.matrix-au.org/"` {
		t.Fatalf("MICROBIN_PUBLIC_PATH = %v", env["MICROBIN_PUBLIC_PATH"])
	}
	if env["MICROBIN_BASIC_AUTH_PASSWORD"] != `"auth-pw"` || env["MICROBIN_ADMIN_PASSWORD"] != `"admin-pw"` {
		t.Fatalf("env = %+v, want auth/admin secrets emitted", env)
	}
	if env["MICROBIN_UPLOADER_PASSWORD"] != `"upload-pw"` || env["MICROBIN_READONLY"] != "true" {
		t.Fatalf("env = %+v, want upload secret and readonly emitted", env)
	}
	if env["MICROBIN_PRIVATE"] != "true" {
		t.Fatalf("env = %+v, want the shared default MICROBIN_PRIVATE to survive", env)
	}
}

// TestRealWorld_MicroBinServerSkipsOwnSecretsWhenDisabled is the case that
// matters most for this template: an instance with no "values" at all must
// not touch Own or panic on a missing map, matching the same real-world
// hazard already found and fixed for the Shadowsocks client above.
func TestRealWorld_MicroBinServerSkipsOwnSecretsWhenDisabled(t *testing.T) {
	instance := map[string]any{
		"id": "bin-test", "service": "microbin", "role": "server",
		"bind": "0.0.0.0", "ports": map[string]int{"main": 8080},
	}
	out, err := render.Render(render.Input{
		Target:       render.Target{Service: "microbin", Role: "server", Instance: "bin-test"},
		Template:     microbinServerTemplate,
		Defaults:     []byte(microbinServerDefaults),
		DefaultsKind: confgen.DefaultsDocument,
		Instance:     instance,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	env := parseEnvLines(string(out))
	if _, ok := env["MICROBIN_BASIC_AUTH_PASSWORD"]; ok {
		t.Fatalf("env = %+v, want no auth secrets when values is absent entirely", env)
	}
	if _, ok := env["MICROBIN_PUBLIC_PATH"]; ok {
		t.Fatalf("env = %+v, want no public path when values is absent", env)
	}
}

func parseEnvLines(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}
