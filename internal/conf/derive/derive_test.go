package derive

import (
	"sort"
	"testing"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/inventory"
)

// worked builds the inventory in docs/apps/conf/inventory.md, in full: the
// nodes under "Nodes", "What is derived" and the "Naming" convention's own
// bin-sfo01, plus users.yaml and routes.yaml as written there.
func worked() *inventory.Root {
	nodes := []inventory.Node{
		{
			ID:       "us-sfo-dgo-linux-01",
			Networks: inventory.Networks{"internet": "203.0.113.10"},
			Instances: []inventory.Instance{
				{ID: "ss-sfo01", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 38250, "alt": 49217}},
				{ID: "hy2-sfo01", Service: "hysteria2", Role: "server", Ports: map[string]int{"main": 443}},
				{ID: "bin-sfo01", Service: "microbin", Role: "server", Ports: map[string]int{"web": 8080}},
			},
		},
		{
			ID:       "jp-tyo-dgo-linux-01",
			Networks: inventory.Networks{"internet": "203.0.113.20"},
			Instances: []inventory.Instance{
				{ID: "hy2-tyo01", Service: "hysteria2", Role: "server", Ports: map[string]int{"main": 443}},
			},
		},
		{
			ID:       "home-server",
			Networks: inventory.Networks{"home": "192.168.1.10"},
			Instances: []inventory.Instance{
				{ID: "http-home", Service: "httpproxy", Role: "server", Ports: map[string]int{"proxy": 8118}},
				{ID: "ss-home", Service: "shadowsocks-rust", Role: "ss-rust", Ports: map[string]int{"local": 1080}},
				{ID: "bin-home", Service: "microbin", Role: "server", Ports: map[string]int{"web": 8080}},
			},
		},
		{
			ID:      "macbook",
			Owner:   "doug",
			Reaches: []string{"home"},
		},
		{
			ID:      "phone",
			Owner:   "doug",
			Reaches: []string{"home"},
			Instances: []inventory.Instance{
				{ID: "phone-sfo", Ports: map[string]int{"socks": 10080, "http": 18080}},
			},
		},
	}

	return &inventory.Root{
		Nodes: nodes,
		Users: map[string]inventory.User{
			"doug":     {Username: "doug", Access: []string{"jp", "sfo", "home-sfo", "bin"}},
			"friend-a": {Username: "yak", Devices: inventory.DevicesUnmanaged, Access: []string{"jp", "sfo"}},
		},
		Routes: map[string]inventory.Route{
			"jp":       {Hops: []string{"hy2-tyo01:main"}},
			"sfo":      {Hops: []string{"ss-sfo01:main"}},
			"home-sfo": {Hops: []string{"http-home:proxy", "ss-home:local", "ss-sfo01:alt"}},
			"bin":      {Hops: []string{"bin-sfo01:web"}},
		},
		Networks:  []string{"home", "internet"},
		Universal: "internet",
	}
}

func workedManifests() map[string]confgen.Manifest {
	return map[string]confgen.Manifest{
		"shadowsocks-rust": {Roles: map[string]confgen.Role{
			"server":  {Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust"},
			"ss-rust": {Auth: confgen.AuthNone},
		}},
		"hysteria2": {Roles: map[string]confgen.Role{
			"server": {Auth: confgen.AuthPerPrincipal, ReachedBy: "client"},
		}},
		"httpproxy": {Roles: map[string]confgen.Role{
			"server": {Auth: confgen.AuthPerPrincipal, ReachedBy: "client"},
			"client": {Auth: confgen.AuthNone},
		}},
		"microbin": {Roles: map[string]confgen.Role{
			"server": {Auth: confgen.AuthNone}, // reached from a browser: no ReachedBy.
		}},
	}
}

func TestDerive_AddressResolution(t *testing.T) {
	m, err := Derive(worked(), workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	edgeTo := func(route, toInstance, toPort string) *Edge {
		for i := range m.Edges {
			e := &m.Edges[i]
			if e.Route == route && e.To.Instance == toInstance && e.To.Port == toPort {
				return e
			}
		}
		t.Fatalf("no edge for route %q to %s:%s", route, toInstance, toPort)
		return nil
	}

	// Same node: home-sfo's http-home -> ss-home edge, both on home-server.
	if e := edgeTo("home-sfo", "ss-home", "local"); e.Address != "127.0.0.1" || e.Port != 1080 {
		t.Fatalf("http-home->ss-home = %+v, want 127.0.0.1:1080", e)
	}

	// Different nodes sharing a network: doug's phone and macbook (both
	// `home`) reaching home-server's http-home (also `home`), for home-sfo.
	if e := edgeTo("home-sfo", "http-home", "proxy"); e.Address != "192.168.1.10" || e.Port != 8118 {
		t.Fatalf("phone/macbook->http-home = %+v, want 192.168.1.10:8118", e)
	}
}

// TestDerive_NoSharedNetworkIsAnError pins the third address case. Every
// node reaches "internet" implicitly, so this needs a downstream with no
// address on "internet" and no address on any network the upstream reaches
// either — here office, a network the preference order does not even name.
func TestDerive_NoSharedNetworkIsAnError(t *testing.T) {
	inv := &inventory.Root{
		Nodes: []inventory.Node{
			{ID: "a", Reaches: []string{"home"}, Instances: []inventory.Instance{
				{ID: "client-a", Service: "shadowsocks-rust", Role: "client", Ports: map[string]int{"local": 1080}},
			}},
			{ID: "b", Networks: inventory.Networks{"office": "10.0.0.5"}, Instances: []inventory.Instance{
				{ID: "server-b", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 443}},
			}},
		},
		Routes: map[string]inventory.Route{
			"chain": {Hops: []string{"client-a:local", "server-b:main"}},
		},
		Networks:  []string{"home", "internet"},
		Universal: "internet",
	}
	if _, err := Derive(inv, workedManifests()); err == nil {
		t.Fatal("Derive: want an error for nodes sharing no network")
	}
}

func TestDerive_ClientInstances(t *testing.T) {
	m, err := Derive(worked(), workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	ids := make([]string, 0, len(m.ClientInstances))
	for _, ci := range m.ClientInstances {
		ids = append(ids, ci.ID)
	}
	sort.Strings(ids)

	// doug: managed, owns macbook and phone, access [jp, sfo, home-sfo, bin].
	// jp, sfo and home-sfo all derive a client (their entry roles all declare
	// ReachedBy); bin's entry is microbin, no ReachedBy, so it does not. Each
	// of macbook and phone gets one client instance per route that derives.
	// yak: unmanaged, access [jp, sfo]: one client instance per route, no
	// node.
	want := []string{
		"macbook-jp", "macbook-sfo", "macbook-home-sfo",
		"phone-jp", "phone-sfo", "phone-home-sfo",
		"yak-jp", "yak-sfo",
	}
	sort.Strings(want)
	if len(ids) != len(want) {
		t.Fatalf("ClientInstances = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ClientInstances = %v, want %v", ids, want)
		}
	}

	// A route with no ReachedBy still grants, but derives no instance.
	for _, ci := range m.ClientInstances {
		if ci.Route == "bin" {
			t.Fatalf("ClientInstance %+v derived for a role with no ReachedBy", ci)
		}
	}
}

// TestDerive_ClientInstanceCarriesOverrideValues pins that an authored
// override's own Values reach the derived ClientInstance untouched, the same
// way Ports and Bind already do — the override is what a template's
// service-specific parameters ride on for a derived instance, same as for an
// authored one. See docs/apps/conf/inventory.md#an-instances-own-values.
func TestDerive_ClientInstanceCarriesOverrideValues(t *testing.T) {
	inv := worked()
	for i := range inv.Nodes {
		if inv.Nodes[i].ID != "phone" {
			continue
		}
		inv.Nodes[i].Instances[0].Values = map[string]any{"note": "office SIM"}
	}

	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	var phoneSFO *ClientInstance
	for i := range m.ClientInstances {
		if m.ClientInstances[i].ID == "phone-sfo" {
			phoneSFO = &m.ClientInstances[i]
		}
	}
	if phoneSFO == nil {
		t.Fatal("phone-sfo not derived")
	}
	if phoneSFO.Values["note"] != "office SIM" {
		t.Fatalf("phoneSFO.Values = %+v, want the override's own values", phoneSFO.Values)
	}
}

func TestDerive_GrantsAndPrincipalTables(t *testing.T) {
	m, err := Derive(worked(), workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	// ss-sfo01, port main: the sfo route's entry. Granted to doug's two
	// managed devices (rendered doug-macbook, doug-phone) and yak
	// (unmanaged, rendered yak) — both kinds of principal on one port.
	mainNames := principalNames(m.Principals("ss-sfo01", "main"))
	wantMain := []string{"doug-macbook", "doug-phone", "yak"}
	if !equal(mainNames, wantMain) {
		t.Fatalf("ss-sfo01/main principals = %v, want %v", mainNames, wantMain)
	}

	// ss-sfo01, port alt: reached only by the non-terminal hop ss-home, in
	// the home-sfo route — an instance-kind principal, a wholly separate
	// table from the same instance's main port.
	altNames := principalNames(m.Principals("ss-sfo01", "alt"))
	wantAlt := []string{"ss-home"}
	if !equal(altNames, wantAlt) {
		t.Fatalf("ss-sfo01/alt principals = %v, want %v", altNames, wantAlt)
	}
	if m.Principals("ss-sfo01", "alt")[0].Kind != PrincipalInstance {
		t.Fatalf("ss-sfo01/alt principal Kind = %q, want %q", m.Principals("ss-sfo01", "alt")[0].Kind, PrincipalInstance)
	}

	// http-home, port proxy: home-sfo's entry, granted to doug (both
	// devices) though no client instance renders for it.
	proxyNames := principalNames(m.Principals("http-home", "proxy"))
	wantProxy := []string{"doug-macbook", "doug-phone"}
	if !equal(proxyNames, wantProxy) {
		t.Fatalf("http-home/proxy principals = %v, want %v", proxyNames, wantProxy)
	}

	// bin-sfo01, port web: bin's entry, granted to doug though no client
	// instance renders for it either.
	webNames := principalNames(m.Principals("bin-sfo01", "web"))
	if !equal(webNames, []string{"doug-macbook", "doug-phone"}) {
		t.Fatalf("bin-sfo01/web principals = %v, want doug's two devices", webNames)
	}
}

// TestDerive_ClientRole pins the resolution order at
// docs/apps/conf/inventory.md#which-role-a-client-derives-as: a node's
// client_role wins over a role's own reached_by, and client_role: none
// derives nothing regardless of it. The grant exists either way.
func TestDerive_ClientRole(t *testing.T) {
	inv := worked()
	for i := range inv.Nodes {
		switch inv.Nodes[i].ID {
		case "phone":
			inv.Nodes[i].ClientRole = "link" // overrides reached_by: ss-rust.
		case "macbook":
			inv.Nodes[i].ClientRole = inventory.ClientRoleNone
		}
	}
	manifests := workedManifests()
	manifests["shadowsocks-rust"].Roles["link"] = confgen.Role{Auth: confgen.AuthNone}

	m, err := Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	var phoneSFO, macbookSFO *ClientInstance
	for i := range m.ClientInstances {
		ci := &m.ClientInstances[i]
		switch ci.ID {
		case "phone-sfo":
			phoneSFO = ci
		case "macbook-sfo":
			macbookSFO = ci
		}
	}
	if phoneSFO == nil {
		t.Fatal("phone-sfo not derived")
	}
	if phoneSFO.Role != "link" {
		t.Fatalf("phone-sfo.Role = %q, want %q (client_role wins over reached_by)", phoneSFO.Role, "link")
	}
	if macbookSFO != nil {
		t.Fatalf("macbook-sfo derived despite client_role: none: %+v", macbookSFO)
	}

	// The grant exists for macbook regardless of client_role: none.
	names := principalNames(m.Principals("ss-sfo01", "main"))
	if !containsString(names, "doug-macbook") {
		t.Fatalf("ss-sfo01/main principals = %v, want doug-macbook granted even with client_role: none", names)
	}
}

// TestDerive_UnmanagedClientRole pins the other half of the resolution
// order: an unmanaged user has no node, so its own client_role substitutes,
// with the same precedence over reached_by.
func TestDerive_UnmanagedClientRole(t *testing.T) {
	inv := worked()
	yak := inv.Users["friend-a"]
	yak.ClientRole = "link"
	inv.Users["friend-a"] = yak

	manifests := workedManifests()
	manifests["shadowsocks-rust"].Roles["link"] = confgen.Role{Auth: confgen.AuthNone}

	m, err := Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	for _, ci := range m.ClientInstances {
		if ci.ID == "yak-sfo" {
			if ci.Role != "link" {
				t.Fatalf("yak-sfo.Role = %q, want %q", ci.Role, "link")
			}
			return
		}
	}
	t.Fatal("yak-sfo not derived")
}

func principalNames(ps []Principal) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
