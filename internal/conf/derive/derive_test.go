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
				{ID: "ss-sfo01", Service: "ssserver", Ports: inventory.PortsOf(map[string]int{"main": 38250, "alt": 49217})},
				{ID: "hy2-sfo01", Service: "hysteria2", Ports: inventory.PortsOf(map[string]int{"main": 443})},
				{ID: "bin-sfo01", Service: "microbin", Ports: inventory.PortsOf(map[string]int{"web": 8080})},
			},
		},
		{
			ID:       "jp-tyo-dgo-linux-01",
			Networks: inventory.Networks{"internet": "203.0.113.20"},
			Instances: []inventory.Instance{
				{ID: "hy2-tyo01", Service: "hysteria2", Ports: inventory.PortsOf(map[string]int{"main": 443})},
			},
		},
		{
			ID:       "home-server",
			Networks: inventory.Networks{"home": "192.168.1.10"},
			Instances: []inventory.Instance{
				{ID: "http-home", Service: "httpproxy", Ports: inventory.PortsOf(map[string]int{"proxy": 8118})},
				{ID: "ss-home", Service: "ss-json", Ports: inventory.PortsOf(map[string]int{"local": 1080})},
				{ID: "bin-home", Service: "microbin", Ports: inventory.PortsOf(map[string]int{"web": 8080})},
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
				{ID: "phone-sfo-ssserver-ss-json", Ports: inventory.PortsOf(map[string]int{"socks": 10080, "http": 18080})},
			},
		},
	}

	return &inventory.Root{
		Nodes: nodes,
		Users: map[string]inventory.User{
			"doug":     {Username: "doug", Access: []string{"jp", "sfo", "home-sfo", "bin"}},
			"friend-a": {Username: "yak", Devices: inventory.DevicesNone, Access: []string{"jp", "sfo"}},
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
		"ssserver":         {Auth: confgen.AuthPerPrincipal, Exports: []string{"ss-json"}, Template: "t"},
		"ss-json":          {Auth: confgen.AuthNone, Template: "t"},
		"hysteria2":        {Auth: confgen.AuthPerPrincipal, Exports: []string{"hy2-client"}, Template: "t"},
		"hy2-client":       {Auth: confgen.AuthNone, Template: "t"},
		"httpproxy":        {Auth: confgen.AuthPerPrincipal, Exports: []string{"httpproxy-client"}, Template: "t"},
		"httpproxy-client": {Auth: confgen.AuthNone, Template: "t"},
		"microbin":         {Auth: confgen.AuthNone, Template: "t"}, // reached from a browser: no client.
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
	if e := edgeTo("home-sfo", "ss-home", "local"); e.Address != "127.0.0.1" || e.Network != "" || e.Port != 1080 {
		t.Fatalf("http-home->ss-home = %+v, want 127.0.0.1:1080", e)
	}

	// Different nodes sharing a network: doug's phone and macbook (both
	// `home`) reaching home-server's http-home (also `home`), for home-sfo.
	if e := edgeTo("home-sfo", "http-home", "proxy"); e.Address != "192.168.1.10" || e.Network != "home" || e.Port != 8118 {
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
				{ID: "client-a", Service: "shadowsocks-rust", Ports: inventory.PortsOf(map[string]int{"local": 1080})},
			}},
			{ID: "b", Networks: inventory.Networks{"office": "10.0.0.5"}, Instances: []inventory.Instance{
				{ID: "server-b", Service: "ssserver", Ports: inventory.PortsOf(map[string]int{"main": 443})},
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

func TestDerive_ExportInstances(t *testing.T) {
	m, err := Derive(worked(), workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	ids := make([]string, 0, len(m.ExportInstances))
	for _, ci := range m.ExportInstances {
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
		"macbook-jp-hysteria2-hy2-client", "macbook-sfo-ssserver-ss-json", "macbook-home-sfo-httpproxy-httpproxy-client",
		"phone-jp-hysteria2-hy2-client", "phone-sfo-ssserver-ss-json", "phone-home-sfo-httpproxy-httpproxy-client",
		"yak-default-jp-hysteria2-hy2-client", "yak-default-sfo-ssserver-ss-json",
	}
	sort.Strings(want)
	if len(ids) != len(want) {
		t.Fatalf("ExportInstances = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ExportInstances = %v, want %v", ids, want)
		}
	}

	// A route with no ReachedBy still grants, but derives no instance.
	for _, ci := range m.ExportInstances {
		if ci.Route == "bin" {
			t.Fatalf("ExportInstance %+v derived for a role with no ReachedBy", ci)
		}
	}
}

// TestDerive_ExportInstanceCarriesOverrideValues pins that an authored
// override's own Values reach the derived ExportInstance untouched, the same
// way Ports and Bind already do — the override is what a template's
// service-specific parameters ride on for a derived instance, same as for an
// authored one. See docs/apps/conf/inventory.md#an-instances-own-values.
func TestDerive_ExportInstanceCarriesOverrideValues(t *testing.T) {
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

	var phoneSFO *ExportInstance
	for i := range m.ExportInstances {
		if m.ExportInstances[i].ID == "phone-sfo-ssserver-ss-json" {
			phoneSFO = &m.ExportInstances[i]
		}
	}
	if phoneSFO == nil {
		t.Fatal("phone-sfo-ssserver-ss-link not derived")
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

	// ss-sfo01, port main: the sfo route's entry. Granted to doug — whose
	// two devices name one credential between them, so they are one
	// account — and to yak, who has no device file and holds the same one
	// credential. A port's table is one row per credential, never per
	// machine.
	mainNames := principalNames(m.Principals("ss-sfo01", "main"))
	wantMain := []string{"doug-default", "yak-default"}
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
	wantProxy := []string{"doug-default"}
	if !equal(proxyNames, wantProxy) {
		t.Fatalf("http-home/proxy principals = %v, want %v", proxyNames, wantProxy)
	}

	// bin-sfo01, port web: bin's entry, granted to doug though no client
	// instance renders for it either.
	webNames := principalNames(m.Principals("bin-sfo01", "web"))
	if !equal(webNames, []string{"doug-default"}) {
		t.Fatalf("bin-sfo01/web principals = %v, want doug's two devices", webNames)
	}
}

// TestDerive_ClientRole pins the resolution order at
// docs/apps/conf/inventory.md#which-role-a-client-derives-as: a node's
// client_role wins over a role's own reached_by, and export: none
// derives nothing regardless of it. The grant exists either way.
// TestDerive_ClientNarrowsToOneForm covers what a device's `client` does: the
// service offers several forms, and the device takes one of them. `none`
// takes none, and the grant survives either way.
func TestDerive_ClientNarrowsToOneForm(t *testing.T) {
	inv := worked()
	for i := range inv.Nodes {
		switch inv.Nodes[i].ID {
		case "phone":
			inv.Nodes[i].Export = "ss-link" // one of ssserver's two forms.
		case "macbook":
			inv.Nodes[i].Export = inventory.ExportNone
		}
	}
	manifests := workedManifests()
	ss := manifests["ssserver"]
	ss.Exports = []string{"ss-json", "ss-link"}
	manifests["ssserver"] = ss
	manifests["ss-link"] = confgen.Manifest{Auth: confgen.AuthNone, Template: "t"}

	m, err := Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	var phoneSFO, macbookSFO *ExportInstance
	for i := range m.ExportInstances {
		ci := &m.ExportInstances[i]
		switch ci.ID {
		case "phone-sfo-ssserver-ss-link":
			phoneSFO = ci
		case "macbook-sfo-ssserver-ss-json", "macbook-sfo-ssserver-ss-link":
			macbookSFO = ci
		}
	}
	if phoneSFO == nil {
		t.Fatal("phone-sfo-ssserver-ss-link not derived")
	}
	if phoneSFO.Export != "ss-link" {
		t.Fatalf("phone-sfo-ssserver-ss-link.Service = %q, want the one form the device asked for", phoneSFO.Export)
	}
	for _, ci := range m.ExportInstances {
		if ci.Node == "phone" && ci.Route == "sfo" && ci.Export != "ss-link" {
			t.Fatalf("phone also derived %q, want only the form it asked for", ci.Export)
		}
	}
	if macbookSFO != nil {
		t.Fatalf("macbook derived %+v despite export: none", macbookSFO)
	}

	// The grant exists for doug regardless of export: none.
	names := principalNames(m.Principals("ss-sfo01", "main"))
	if !containsString(names, "doug-default") {
		t.Fatalf("ss-sfo01/main principals = %v, want doug granted even with export: none", names)
	}
}

// TestDerive_ClientOfAPersonWithNoDevice pins the other half of the
// resolution order: with no node to carry one, the person's own `client`
// substitutes, with the same precedence over the service's default.
func TestDerive_ClientOfAPersonWithNoDevice(t *testing.T) {
	inv := worked()
	yak := inv.Users["friend-a"]
	yak.Export = "ss-link"
	inv.Users["friend-a"] = yak

	manifests := workedManifests()
	ss := manifests["ssserver"]
	ss.Exports = []string{"ss-json", "ss-link"}
	manifests["ssserver"] = ss
	manifests["ss-link"] = confgen.Manifest{Auth: confgen.AuthNone, Template: "t"}

	m, err := Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	for _, ci := range m.ExportInstances {
		if ci.ID == "yak-default-sfo-ssserver-ss-link" {
			if ci.Export != "ss-link" {
				t.Fatalf("yak-default-sfo-ssserver-ss-link.Service = %q, want the one form they asked for", ci.Export)
			}
			return
		}
	}
	t.Fatal("yak-default-sfo-ssserver-ss-link not derived")
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

// TestDerive_TwoRoutesIntoOnePortAreOneGrant covers a person granted two
// routes that enter the same port. They get two client configurations, one
// per route, and one credential: the server's account table has no way to
// tell which route a connection came over, and counting the pair twice
// would report one account as two everywhere the list is read.
func TestDerive_TwoRoutesIntoOnePortAreOneGrant(t *testing.T) {
	inv := worked()
	inv.Routes["sfo-again"] = inventory.Route{Hops: []string{"ss-sfo01:main"}}
	doug := inv.Users["doug"]
	doug.Access = append(doug.Access, "sfo-again")
	inv.Users["doug"] = doug

	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	grants := 0
	for _, g := range m.Grants {
		if g.Instance == "ss-sfo01" && g.Port == "main" && g.Principal.ID == "doug/default" {
			grants++
		}
	}
	if grants != 1 {
		t.Fatalf("grants on ss-sfo01:main for one credential = %d, want one however many routes reach it", grants)
	}

	clients := 0
	for _, ci := range m.ExportInstances {
		if ci.Node == "phone" && ci.Export == "ss-json" {
			clients++
		}
	}
	if clients != 2 {
		t.Fatalf("client instances = %d, want one per route", clients)
	}
}

// TestDerive_TwoDevicesOneCredentialShareOneSecret covers the choice the
// model makes about identity: a credential belongs to a person, and a device
// names one. Two devices naming the same one authenticate as the same
// account and hold the same secret, which is what someone using one NAS
// password on a laptop and a phone actually has.
func TestDerive_TwoDevicesOneCredentialShareOneSecret(t *testing.T) {
	inv := worked()
	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	principals := m.Principals("ss-sfo01", "main")
	var doug int
	for _, p := range principals {
		if p.Group == "doug" {
			doug++
			if p.Slot != inventory.DefaultCredential {
				t.Fatalf("slot = %q, want %q", p.Slot, inventory.DefaultCredential)
			}
			if p.Name != "doug-"+inventory.DefaultCredential {
				t.Fatalf("account = %q, want the username and the credential", p.Name)
			}
		}
	}
	if doug != 1 {
		t.Fatalf("%d principals for doug, want 1: both devices name one credential", doug)
	}

	// Each device still renders its own file: they share a secret, not a
	// configuration.
	var files []string
	for _, ci := range m.ExportInstances {
		if ci.Node != "" && ci.Route == "sfo" {
			files = append(files, ci.ID)
		}
	}
	sort.Strings(files)
	if len(files) != 2 {
		t.Fatalf("client instances for route sfo = %v, want one per device", files)
	}
}

// TestDerive_SecondCredentialIsItsOwnPrincipal covers the other direction:
// a device naming a credential of its own holds a separate secret and a
// separate account, which is how a password meant to be revoked on its own
// is kept apart.
func TestDerive_SecondCredentialIsItsOwnPrincipal(t *testing.T) {
	inv := worked()
	user := inv.Users["doug"]
	user.Credentials = map[string]inventory.Credential{inventory.DefaultCredential: {}, "work": {Note: "the office laptop"}}
	inv.Users["doug"] = user
	inv.Nodes = append(inv.Nodes, inventory.Node{
		ID: "work-laptop", Owner: "doug", Reaches: []string{"home"}, Credential: "work",
	})

	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	slots := map[string]string{}
	for _, p := range m.Principals("ss-sfo01", "main") {
		if p.Group == "doug" {
			slots[p.Slot] = p.Name
		}
	}
	if slots["work"] != "doug-work" {
		t.Fatalf("slots = %v, want a work credential accounted as doug-work", slots)
	}
	if slots[inventory.DefaultCredential] != "doug-"+inventory.DefaultCredential {
		t.Fatalf("slots = %v, want the default credential kept apart from it", slots)
	}
}

// A credential is an account because the person declares it. One their
// devices name is carried by those devices; one none of them names is
// carried by the person, who uses it on whatever machine is at hand. Both
// are grants, and each writes its own file.
func TestDerive_CredentialNoDeviceNamesIsTheirsToCarry(t *testing.T) {
	inv := worked()
	inv.Users["doug"] = inventory.User{
		Username: "doug",
		Access:   []string{"sfo"},
		Credentials: map[string]inventory.Credential{
			"default": {Note: "whatever machine is at hand"},
			"mbp":     {Note: "the laptop, revocable on its own"},
		},
	}
	// One device, naming one of the two credentials.
	for i := range inv.Nodes {
		if inv.Nodes[i].ID == "macbook" {
			inv.Nodes[i].Credential = "mbp"
		}
		if inv.Nodes[i].ID == "phone" {
			inv.Nodes[i].Owner = "" // leave one device in play, not two
		}
	}

	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	// Both credentials are accounts on the port the route enters.
	var slots []string
	for _, g := range m.Grants {
		if g.Instance == "ss-sfo01" && g.Port == "main" && g.Principal.Group == "doug" {
			slots = append(slots, g.Principal.Slot)
		}
	}
	sort.Strings(slots)
	if len(slots) != 2 || slots[0] != "default" || slots[1] != "mbp" {
		t.Fatalf("doug's grants on ss-sfo01:main = %v, want both credentials", slots)
	}

	// The device carries the one it names; the person carries the other.
	var ids []string
	for _, ci := range m.ExportInstances {
		if ci.Route == "sfo" && (ci.User == "doug" || ci.Node == "macbook") {
			ids = append(ids, ci.ID)
		}
	}
	sort.Strings(ids)
	want := []string{"doug-default-sfo-ssserver-ss-json", "macbook-sfo-ssserver-ss-json"}
	if len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("export instances for sfo = %v, want %v", ids, want)
	}
	for _, ci := range m.ExportInstances {
		switch ci.ID {
		case "macbook-sfo-ssserver-ss-json":
			if ci.Credential != "mbp" || ci.Node != "macbook" {
				t.Fatalf("%s = %+v, want the device's own credential", ci.ID, ci)
			}
		case "doug-default-sfo-ssserver-ss-json":
			if ci.Credential != "default" || ci.User != "doug" || ci.Node != "" {
				t.Fatalf("%s = %+v, want a file belonging to the person, not a node", ci.ID, ci)
			}
		}
	}
}

// Two devices naming one credential share it, and it is not also written for
// the person: something already carries it.
func TestDerive_CredentialTwoDevicesShareIsNotAlsoThePersons(t *testing.T) {
	inv := worked()
	inv.Users["doug"] = inventory.User{
		Username:    "doug",
		Access:      []string{"sfo"},
		Credentials: map[string]inventory.Credential{"default": {Note: "the laptop and the phone"}},
	}

	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	var grants int
	for _, g := range m.Grants {
		if g.Instance == "ss-sfo01" && g.Port == "main" && g.Principal.Group == "doug" {
			grants++
		}
	}
	if grants != 1 {
		t.Fatalf("grants = %d, want one: two devices naming one credential are one account", grants)
	}
	for _, ci := range m.ExportInstances {
		if ci.User == "doug" && ci.Node == "" {
			t.Fatalf("%s is written for the person, but their devices already carry it", ci.ID)
		}
	}
}

// A credential may open fewer routes than its owner holds. Access is the
// person's, and the credential chooses among what they already have: the
// laptop's password is an account on the line it keeps and on no other, so
// losing the laptop costs one route rather than all of them.
func TestDerive_CredentialNarrowedToFewerRoutes(t *testing.T) {
	inv := worked()
	inv.Users["doug"] = inventory.User{
		Username: "doug",
		Access:   []string{"jp", "sfo"},
		Credentials: map[string]inventory.Credential{
			"default": {Note: "whatever machine is at hand"},
			"mbp":     {Note: "the laptop", Access: []string{"sfo"}},
		},
	}
	for i := range inv.Nodes {
		if inv.Nodes[i].ID == "macbook" {
			inv.Nodes[i].Credential = "mbp"
		}
		if inv.Nodes[i].ID == "phone" {
			inv.Nodes[i].Owner = ""
		}
	}

	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	// The narrowed credential is an account on the port sfo enters and not
	// on the one jp enters, while the unnarrowed one is on both.
	got := map[string][]string{}
	for _, g := range m.Grants {
		if g.Principal.Group == "doug" {
			got[g.Instance] = append(got[g.Instance], g.Principal.Slot)
		}
	}
	for instance, want := range map[string][]string{
		"ss-sfo01":  {"default", "mbp"},
		"hy2-tyo01": {"default"},
	} {
		slots := got[instance]
		sort.Strings(slots)
		if len(slots) != len(want) {
			t.Fatalf("doug's grants on %s = %v, want %v", instance, slots, want)
		}
		for i := range want {
			if slots[i] != want[i] {
				t.Fatalf("doug's grants on %s = %v, want %v", instance, slots, want)
			}
		}
	}

	// The device carrying it takes no file on the route it cannot open.
	for _, ci := range m.ExportInstances {
		if ci.Node == "macbook" && ci.Route == "jp" {
			t.Fatalf("macbook has %s, but its credential does not open jp", ci.ID)
		}
	}
	// Nor does an edge exist for one.
	for _, e := range m.Edges {
		if e.Route == "jp" && e.FromInstance == "macbook-jp-hysteria2-hy2-link" {
			t.Fatalf("edge %s exists on a route its credential does not open", e.FromInstance)
		}
	}
}

func TestDerive_ProfilesWriteADeviceOutOncePerProfile(t *testing.T) {
	inv := worked()
	for i := range inv.Nodes {
		if inv.Nodes[i].ID != "macbook" {
			continue
		}
		inv.Nodes[i].Profiles = map[string]inventory.Profile{
			"singbox": {Export: "ss-json", Values: map[string]any{"local_port": 2080}},
			"browser": {Values: map[string]any{"local_port": 1080, "mode": "tcp_only"}, Access: []string{"sfo", "jp"}},
		}
		inv.Nodes[i].Instances = []inventory.Instance{
			{ID: "macbook-sfo-ssserver-ss-json-browser", Values: map[string]any{"local_port": 7890}},
		}
	}

	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	byID := map[string]ExportInstance{}
	var ids []string
	for _, ci := range m.ExportInstances {
		if ci.Node == "macbook" {
			byID[ci.ID] = ci
			ids = append(ids, ci.ID)
		}
	}
	sort.Strings(ids)
	// singbox narrows to ss-json, which only ssserver offers; browser takes
	// every way but only the two routes its access names.
	want := []string{
		"macbook-jp-hysteria2-hy2-client-browser",
		"macbook-sfo-ssserver-ss-json-browser",
		"macbook-sfo-ssserver-ss-json-singbox",
	}
	if !equal(ids, want) {
		t.Fatalf("macbook export instances = %v, want %v", ids, want)
	}

	singbox := byID["macbook-sfo-ssserver-ss-json-singbox"]
	if singbox.Profile != "singbox" || singbox.Values["local_port"] != 2080 || singbox.Credential != "default" {
		t.Fatalf("singbox = %+v, want profile singbox, local_port 2080, the device's credential", singbox)
	}
	// The override pins one key and leaves the profile's others in place.
	browser := byID["macbook-sfo-ssserver-ss-json-browser"]
	if browser.Values["local_port"] != 7890 || browser.Values["mode"] != "tcp_only" {
		t.Fatalf("browser.Values = %+v, want the override's local_port over the profile's mode", browser.Values)
	}

	// Profiles are files, not accounts: the device still holds one grant.
	if got := principalNames(m.Principals("ss-sfo01", "main")); !equal(got, []string{"doug-default", "yak-default"}) {
		t.Fatalf("ss-sfo01:main principals = %v, want one account for doug however many profiles", got)
	}
}

// TestDerive_ForwarderIsDialedAndTheHopBehindItIsAuthenticatedAgainst pins
// docs/apps/conf/inventory.md#a-service-that-forwards. A relay terminates
// nothing, so a route entering one is written out as the service that ends
// it, the grant belongs to that service's port, and the edge carries both
// ends: what the client dials and what it authenticates against.
func TestDerive_ForwarderIsDialedAndTheHopBehindItIsAuthenticatedAgainst(t *testing.T) {
	inv := worked()
	inv.Nodes[1].Instances = append(inv.Nodes[1].Instances, inventory.Instance{
		ID: "fwd-tyo01", Service: "realm", Ports: inventory.PortsOf(map[string]int{"ss": 40000}),
	})
	inv.Routes["sfo-via-tyo"] = inventory.Route{Hops: []string{"fwd-tyo01:ss", "ss-sfo01:alt"}}
	user := inv.Users["friend-a"]
	user.Access = []string{"sfo-via-tyo"}
	inv.Users["friend-a"] = user

	manifests := workedManifests()
	manifests["realm"] = confgen.Manifest{Auth: confgen.AuthNone, Forwards: true, Template: "t"}

	m, err := Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	// The file is the terminating service's, not the relay's: nobody
	// imports a share URI for a program that reads nothing.
	var found *ExportInstance
	for i := range m.ExportInstances {
		if m.ExportInstances[i].Route == "sfo-via-tyo" {
			found = &m.ExportInstances[i]
			break
		}
	}
	if found == nil {
		t.Fatal("no export instance for the forwarded route")
	}
	if found.Service != "ssserver" {
		t.Fatalf("export instance service = %q, want the service that terminates the chain", found.Service)
	}

	// The edge dials the relay and names the hop the credential is at.
	var edge *Edge
	for i := range m.Edges {
		if m.Edges[i].FromInstance == found.ID {
			edge = &m.Edges[i]
			break
		}
	}
	if edge == nil {
		t.Fatal("no edge out of the export instance")
	}
	if edge.To.Instance != "fwd-tyo01" || edge.Port != 40000 {
		t.Fatalf("edge dials %s:%d, want the relay's own port", edge.To.Instance, edge.Port)
	}
	if edge.Terminal.Instance != "ss-sfo01" || edge.Terminal.Port != "alt" {
		t.Fatalf("edge terminal = %s:%s, want ss-sfo01:alt", edge.Terminal.Instance, edge.Terminal.Port)
	}

	// The grant is on the terminating port. Nothing is granted on the
	// relay: it holds no account table to put anyone in.
	if got := m.Principals("ss-sfo01", "alt"); len(got) == 0 {
		t.Fatal("nobody granted on the port that terminates the chain")
	}
	if got := m.Principals("fwd-tyo01", "ss"); len(got) != 0 {
		t.Fatalf("Principals(fwd-tyo01, ss) = %v, want none on a relay", got)
	}
}

// TestDerive_AForwarderRelayingOnwardHoldsNoCredential is the same rule for a
// relay in the middle of a chain: the instance before it holds the grant, and
// that grant is on the hop the chain ends at.
func TestDerive_AForwarderRelayingOnwardHoldsNoCredential(t *testing.T) {
	inv := worked()
	inv.Nodes[2].Instances = append(inv.Nodes[2].Instances, inventory.Instance{
		ID: "fwd-home", Service: "realm", Ports: inventory.PortsOf(map[string]int{"ss": 40000}),
	})
	inv.Routes["home-sfo"] = inventory.Route{
		Hops: []string{"http-home:proxy", "ss-home:local", "fwd-home:ss", "ss-sfo01:alt"},
	}
	manifests := workedManifests()
	manifests["realm"] = confgen.Manifest{Auth: confgen.AuthNone, Forwards: true, Template: "t"}

	m, err := Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	for _, g := range m.Grants {
		if g.Principal.ID == "fwd-home" {
			t.Fatalf("grant %+v: a relay holds no credential", g)
		}
	}
	var relaying bool
	for _, p := range m.Principals("ss-sfo01", "alt") {
		if p.Kind == PrincipalInstance && p.ID == "ss-home" {
			relaying = true
		}
	}
	if !relaying {
		t.Fatal("ss-home is not granted on ss-sfo01:alt, so nothing it relays through the forwarder authenticates")
	}
}

func TestDerive_PrincipalMakesInstanceDialAsUser(t *testing.T) {
	inv := worked()
	for ni := range inv.Nodes {
		for ii := range inv.Nodes[ni].Instances {
			if inv.Nodes[ni].Instances[ii].ID == "ss-home" {
				inv.Nodes[ni].Instances[ii].Principal = "repeater"
			}
		}
	}
	inv.Users["repeater"] = inventory.User{Devices: inventory.DevicesNone, Export: inventory.ExportNone, Access: []string{"home-sfo"}}

	m, err := Derive(inv, workedManifests())
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	var got *Grant
	for i := range m.Grants {
		g := &m.Grants[i]
		if g.Instance == "ss-sfo01" && g.Port == "alt" && g.Principal.Group == "repeater" {
			got = g
			break
		}
	}
	if got == nil {
		t.Fatal("no grant for repeater on ss-sfo01:alt")
	}
	if got.Principal.Kind != PrincipalUser || got.Principal.Slot != inventory.DefaultCredential || got.Principal.Name != "repeater-default" {
		t.Fatalf("principal = %+v, want repeater's default user credential", got.Principal)
	}
}

// fileServer is one Samba instance reached directly, with the manifest
// naming its accounts as given.
func fileServer(accounts string, users map[string]inventory.User) (*inventory.Root, map[string]confgen.Manifest) {
	inv := &inventory.Root{
		Nodes: []inventory.Node{{
			ID:       "nas",
			Networks: inventory.Networks{"home": "10.0.0.10"},
			Instances: []inventory.Instance{
				{ID: "samba-nas", Service: "samba", Ports: inventory.PortsOf(map[string]int{"smb": 445})},
			},
		}},
		Users:    users,
		Routes:   map[string]inventory.Route{"files": {Hops: []string{"samba-nas:smb"}}},
		Networks: []string{"home"},
	}
	manifests := map[string]confgen.Manifest{
		"samba": {Auth: confgen.AuthPerPrincipal, Accounts: accounts},
	}
	return inv, manifests
}

func accountNames(t *testing.T, inv *inventory.Root, manifests map[string]confgen.Manifest) []string {
	t.Helper()
	m, err := Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	var names []string
	for _, p := range m.Principals("samba-nas", "smb") {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}

// TestDerive_AccountNamedByCredential is the ordinary name: the person and
// the credential, because a credential is what is revocable.
func TestDerive_AccountNamedByCredential(t *testing.T) {
	inv, manifests := fileServer("", map[string]inventory.User{
		"jane": {Devices: inventory.DevicesNone, Access: []string{"files"}},
	})
	if got := accountNames(t, inv, manifests); len(got) != 1 || got[0] != "jane-default" {
		t.Fatalf("accounts = %v, want [jane-default]", got)
	}
}

// TestDerive_AccountNamedByPerson: a service whose accounts are POSIX users
// names them after the person, and honours a username the same way the
// ordinary name does.
func TestDerive_AccountNamedByPerson(t *testing.T) {
	inv, manifests := fileServer(confgen.AccountsPerson, map[string]inventory.User{
		"jane": {Devices: inventory.DevicesNone, Access: []string{"files"}},
		"bob":  {Username: "robert", Devices: inventory.DevicesNone, Access: []string{"files"}},
	})
	got := accountNames(t, inv, manifests)
	if len(got) != 2 || got[0] != "jane" || got[1] != "robert" {
		t.Fatalf("accounts = %v, want [jane robert]", got)
	}
}

// TestDerive_AccountNamedByPersonKeepsTheCredentialsSecret: the name drops
// the credential, the secret does not. Which file holds the value is still
// the credential's, so switching a service to person names regenerates
// nothing.
func TestDerive_AccountNamedByPersonKeepsTheCredentialsSecret(t *testing.T) {
	inv, manifests := fileServer(confgen.AccountsPerson, map[string]inventory.User{
		"jane": {Devices: inventory.DevicesNone, Access: []string{"files"}},
	})
	m, err := Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	p := m.Principals("samba-nas", "smb")
	if len(p) != 1 || p[0].Group != "jane" || p[0].Slot != "default" {
		t.Fatalf("principal = %+v, want jane's default credential", p)
	}
}
