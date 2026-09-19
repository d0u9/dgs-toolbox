package validate

import (
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

// validInventory is a small, clean inventory: one server node with two
// ports on one instance, one managed client node, one unmanaged user, and a
// two-hop relay route so rule 8's successor check has something to pin.
func validInventory() *inventory.Root {
	return &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "srv",
				Networks: inventory.Networks{"internet": "203.0.113.10"},
				Instances: []inventory.Instance{
					{ID: "ss-srv", Service: "ssserver", Ports: inventory.PortsOf(map[string]int{"main": 38250, "alt": 49217})},
				},
			},
			{
				ID:       "relay",
				Networks: inventory.Networks{"internet": "203.0.113.20"},
				Instances: []inventory.Instance{
					{ID: "ss-relay", Service: "ssserver", Ports: inventory.PortsOf(map[string]int{"main": 40000})},
				},
			},
			{
				ID:      "laptop",
				Owner:   "doug",
				Reaches: []string{"home"},
				Instances: []inventory.Instance{
					{ID: "laptop-sfo-ssserver-ss-json", Ports: inventory.PortsOf(map[string]int{"local": 1080})},
				},
			},
		},
		Users: map[string]inventory.User{
			"doug": {Username: "doug", Access: []string{"sfo"}},
			"yak":  {Username: "yak", Devices: inventory.DevicesNone, Access: []string{"sfo"}},
		},
		Routes: map[string]inventory.Route{
			"sfo":   {Hops: []string{"ss-srv:main"}},
			"chain": {Hops: []string{"ss-relay:main", "ss-srv:alt"}},
		},
		Networks:  []string{"home", "internet"},
		Universal: "internet",
	}
}

func validManifests() map[string]confgen.Manifest {
	return map[string]confgen.Manifest{
		"ssserver": {Auth: confgen.AuthPerPrincipal, Exports: []string{"ss-json"}, Template: "t"},
		"ss-json":  {Auth: confgen.AuthNone, Template: "t"},
	}
}

// validExports is the exports the fixture's services name, each a rendering
// and nothing more.
func validExports() map[string]confgen.Export {
	return map[string]confgen.Export{
		"ss-link": {Template: "t", Output: "share.txt"},
		"ss-json": {Template: "t", Output: "config.json"},
	}
}

func derived(t *testing.T, inv *inventory.Root, manifests map[string]confgen.Manifest) *derive.Model {
	t.Helper()
	m, err := derive.Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return m
}

func messages(issues []Issue) []string {
	out := make([]string, len(issues))
	for i, iss := range issues {
		out[i] = iss.Message
	}
	return out
}

func containsSubstring(issues []Issue, substr string) bool {
	for _, iss := range issues {
		if strings.Contains(iss.Message, substr) {
			return true
		}
	}
	return false
}

func TestValidate_CleanInventoryHasNoIssues(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if len(got) != 0 {
		t.Fatalf("Validate = %v, want none", messages(got))
	}
}

func TestValidate_DuplicateInstanceID(t *testing.T) {
	inv := validInventory()
	inv.Nodes[1].Instances[0].ID = "ss-srv" // collides with the srv node's instance.
	manifests := validManifests()
	// Derive itself refuses a duplicate instance ID before it can compute
	// anything, so this checks Validate's own pass over the inventory
	// directly rather than routing through a Derive that would never reach
	// it in practice.
	got := Validate(inv, manifests, validExports(), &derive.Model{}, nil)
	if !containsSubstring(got, `instance "ss-srv" is defined more than once`) {
		t.Fatalf("Validate = %v, want a duplicate-instance issue", messages(got))
	}
}

func TestValidate_UnknownServiceAndRole(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances[0].Service = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `service "nonesuch" is not defined`) {
		t.Fatalf("Validate = %v, want an unknown-service issue", messages(got))
	}
}

func TestValidate_UnknownService(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances[0].Service = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `service "nonesuch" is not defined`) {
		t.Fatalf("Validate = %v, want an unknown-service issue", messages(got))
	}
}

// A `export` naming a form none of the services this device reaches offers
// writes nothing at all, silently. There is no separate "not in exports/"
// case: a service's exports are the directories it holds, so a name the
// service does not offer is the only way to get this wrong.
func TestValidate_NodeExportNotInExportsDir(t *testing.T) {
	inv := validInventory()
	inv.Nodes[2].Export = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `node "laptop": export "nonesuch" is not one of the ways ssserver is written out`) {
		t.Fatalf("Validate = %v, want a bad export issue", messages(got))
	}
}

func TestValidate_HopNamesMissingInstanceOrPort(t *testing.T) {
	inv := validInventory()
	inv.Routes["broken-instance"] = inventory.Route{Hops: []string{"nonesuch:main"}}
	inv.Routes["broken-port"] = inventory.Route{Hops: []string{"ss-srv:nonesuch"}}
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `names an instance that does not exist`) {
		t.Fatalf("Validate = %v, want a missing-instance issue", messages(got))
	}
	if !containsSubstring(got, `has no port "nonesuch"`) {
		t.Fatalf("Validate = %v, want a missing-port issue", messages(got))
	}
}

func TestValidate_ReservedSelfPort(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances[0].Ports["self"] = inventory.Port{Number: 1234}
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `port "self" is reserved`) {
		t.Fatalf("Validate = %v, want a reserved-port issue", messages(got))
	}
}

func TestValidate_UnknownRouteInAccessAndUnknownOwner(t *testing.T) {
	inv := validInventory()
	inv.Users["doug"] = inventory.User{Username: "doug", Access: []string{"sfo", "nonesuch"}}
	inv.Nodes[2].Owner = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `access names route "nonesuch"`) {
		t.Fatalf("Validate = %v, want an unknown-route issue", messages(got))
	}
	if !containsSubstring(got, `owner "nonesuch" is not a user`) {
		t.Fatalf("Validate = %v, want an unknown-owner issue", messages(got))
	}
}

func TestValidate_OverrideMatchesNothingDerived(t *testing.T) {
	inv := validInventory()
	inv.Nodes[2].Instances[0].ID = "laptop-typo-route"
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `overrides nothing this node derives`) {
		t.Fatalf("Validate = %v, want an unmatched-override issue", messages(got))
	}
}

func TestValidate_OverrideSettingServiceOrRole(t *testing.T) {
	inv := validInventory()
	inv.Nodes[2].Instances[0].Service = "shadowsocks-rust"
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `sets service or role; an override may only set ports and bind`) {
		t.Fatalf("Validate = %v, want an invalid-override issue", messages(got))
	}
}

func TestValidate_DifferentSuccessorsForOneNonTerminalHop(t *testing.T) {
	inv := validInventory()
	inv.Nodes[1].Instances = append(inv.Nodes[1].Instances, inventory.Instance{
		ID: "ss-relay-b", Service: "ssserver", Ports: inventory.PortsOf(map[string]int{"main": 40001}),
	})
	inv.Routes["chain2"] = inventory.Route{Hops: []string{"ss-relay:main", "ss-relay-b:main"}}
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `has different successors in routes`) {
		t.Fatalf("Validate = %v, want a rule-based-routing issue", messages(got))
	}
}

func TestValidate_RouteNamesOneInstanceTwice(t *testing.T) {
	inv := validInventory()
	inv.Routes["loop"] = inventory.Route{Hops: []string{"ss-relay:main", "ss-srv:alt", "ss-relay:main"}}
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `names instance "ss-relay" twice`) {
		t.Fatalf("Validate = %v, want a repeated-instance issue", messages(got))
	}
}

func TestValidate_CarriedCredentialRouteNotEnteringOnInternet(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Networks = inventory.Networks{"home": "192.168.1.5"} // srv no longer has an internet address.
	// The relay->srv edge in "chain" would now fail to resolve too, which
	// Derive treats as fatal; this rule reads only the inventory, so check
	// it directly rather than through a Derive that would never get here.
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), &derive.Model{}, nil)
	if !containsSubstring(got, `user "yak": route "sfo" enters "ss-srv", which has no address on the universal network`) {
		t.Fatalf("Validate = %v, want a carried-credential-not-on-internet issue", messages(got))
	}
}

func TestValidate_DuplicateAccountNameOnOnePort(t *testing.T) {
	inv := validInventory()
	// An account name is the username and the credential. A second person
	// whose username and credential spell the same pair collides on
	// ss-srv's main port, and the server's table would carry two rows a
	// reader cannot tell apart.
	inv.Users["clash"] = inventory.User{
		Username: "doug", Devices: inventory.DevicesNone, Access: []string{"sfo"},
	}
	manifests := validManifests()
	model := derived(t, inv, manifests)
	got := Validate(inv, manifests, validExports(), model, nil)
	if !containsSubstring(got, `account "doug-default" is rendered by more than one principal`) {
		t.Fatalf("Validate = %v, want a duplicate-account issue", messages(got))
	}
}

func TestValidate_TwoInstancesBindTheSameAddressAndPort(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances = append(inv.Nodes[0].Instances, inventory.Instance{
		ID: "other-srv", Service: "ssserver", Ports: inventory.PortsOf(map[string]int{"main": 38250}),
	})
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `bind the same address, port and protocol`) {
		t.Fatalf("Validate = %v, want a bind-collision issue", messages(got))
	}
}

// TestValidate_SamePortDifferentProtocolIsNotACollision covers the case the
// check exists to allow: a QUIC service on 443/udp beside a web server on
// 443/tcp is two real listeners, not one machine bound twice.
func TestValidate_SamePortDifferentProtocolIsNotACollision(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances = append(inv.Nodes[0].Instances, inventory.Instance{
		ID: "quic-srv", Service: "ssserver",
		Ports: inventory.Ports{"main": {Number: 38250, Protocol: inventory.ProtocolUDP}},
	})
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if containsSubstring(got, `bind the same address, port and protocol`) {
		t.Fatalf("Validate = %v, want no collision: one is udp and one is tcp", messages(got))
	}
}

func TestValidate_UniversalNotInNetworksList(t *testing.T) {
	inv := validInventory()
	inv.Universal = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), &derive.Model{}, nil)
	if !containsSubstring(got, `universal "nonesuch" does not name a network in the list`) {
		t.Fatalf("Validate = %v, want a bad-universal issue", messages(got))
	}
}

func TestValidate_UserExportNotInExportsDir(t *testing.T) {
	inv := validInventory()
	yak := inv.Users["yak"]
	yak.Export = "nonesuch"
	inv.Users["yak"] = yak
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `user "yak": export "nonesuch" is not one of the ways ssserver is written out`) {
		t.Fatalf("Validate = %v, want a bad export issue", messages(got))
	}
}

func TestValidate_StalePrevious(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	previous := map[string]time.Time{
		"ss-srv/main/node/laptop": time.Now().Add(-8 * 24 * time.Hour),
	}
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), previous)
	if !containsSubstring(got, `ss-srv/main/node/laptop.previous`) || !containsSubstring(got, "seven-day limit") {
		t.Fatalf("Validate = %v, want a stale-.previous issue", messages(got))
	}
}

func TestValidate_RecentPreviousIsNotAnIssue(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	previous := map[string]time.Time{
		"ss-srv/main/node/laptop": time.Now().Add(-6 * 24 * time.Hour),
	}
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), previous)
	if containsSubstring(got, "seven-day limit") {
		t.Fatalf("Validate = %v, want no stale-.previous issue for a six-day-old file", messages(got))
	}
}

// TestValidate_PortHandsOutUnknownSecret covers a port handing out a name
// the service never declared: a value the client never receives, with
// nothing in the rendered file to say it was meant to be there.
func TestValidate_PortHandsOutUnknownSecret(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	ss := manifests["ssserver"]
	ss.Self = confgen.SelfDecls{"psk": {Set: true}}
	manifests["ssserver"] = ss

	inst := &inv.Nodes[0].Instances[0]
	port := inst.Ports["main"]
	port.Self = []string{"nonesuch.main"}
	inst.Ports["main"] = port

	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `hands out "nonesuch"`) {
		t.Fatalf("Validate = %v, want a port handing out an undeclared secret", messages(got))
	}

	port.Self = []string{"psk.main"}
	inst.Ports["main"] = port
	got = Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if containsSubstring(got, "hands out") {
		t.Fatalf("Validate = %v, want no issue once the name is declared", messages(got))
	}
}

// TestValidate_PortNamesASetWithoutItsKey covers the two halves of the same
// mistake: a set handed out without saying which of its values, and a
// single value handed out with a key it does not have.
func TestValidate_PortNamesASetWithoutItsKey(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	ss := manifests["ssserver"]
	ss.Self = confgen.SelfDecls{"psk": {Set: true}, "tls_key": {}}
	manifests["ssserver"] = ss

	inst := &inv.Nodes[0].Instances[0]
	port := inst.Ports["main"]
	port.Self = []string{"psk"}
	inst.Ports["main"] = port
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, "which is a set: name one of its keys") {
		t.Fatalf("Validate = %v, want a set named without a key", messages(got))
	}

	port.Self = []string{"tls_key.main"}
	inst.Ports["main"] = port
	got = Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, "is one value and takes no key") {
		t.Fatalf("Validate = %v, want a single value named with a key", messages(got))
	}
}

// TestValidate_InstanceSelfOutsideTheServiceList covers the one way an
// instance's narrowed `self` can be wrong in the other direction: a name the
// service never declared is a credential sync would generate and nothing
// accounts for.
func TestValidate_InstanceSelfOutsideTheServiceList(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	ss := manifests["ssserver"]
	ss.Self = confgen.SelfDecls{"psk": {Set: true}}
	manifests["ssserver"] = ss
	inv.Nodes[0].Instances[0].Self = map[string][]string{"psk": {"main"}, "nonesuch": nil}

	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `self "nonesuch" is not one of "ssserver"'s own secrets`) {
		t.Fatalf("Validate = %v, want a self outside the service's list issue", messages(got))
	}
}

// TestValidate_InstanceSelfKeysOnASingleValue covers keys written against a
// name the service declares as one value: they name files sync neither
// generates nor reports.
func TestValidate_InstanceSelfKeysOnASingleValue(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	ss := manifests["ssserver"]
	ss.Self = confgen.SelfDecls{"tls_key": {}}
	manifests["ssserver"] = ss
	inv.Nodes[0].Instances[0].Self = map[string][]string{"tls_key": {"main"}}

	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, "takes no keys") {
		t.Fatalf("Validate = %v, want a keyed single value issue", messages(got))
	}
}

// `devices: none` asserts there is no node file for this person. One owned
// by them says the opposite, and the pair is what tells a deliberate absence
// apart from a misplaced file.
func TestValidate_DevicesNoneBesideANodeFile(t *testing.T) {
	inv := validInventory()
	yak := inv.Users["yak"]
	yak.Devices = inventory.DevicesNone
	inv.Users["yak"] = yak
	inv.Nodes = append(inv.Nodes, inventory.Node{ID: "yak-phone", Owner: "yak"})

	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `user "yak": devices: none says this inventory holds no node file for them, but yak-phone is theirs`) {
		t.Fatalf("Validate = %v, want the contradiction named", messages(got))
	}
}

// A credential narrows the routes its owner already holds. Naming one they do
// not hold would grant access from the wrong place — access belongs to the
// person — so it is reported with the routes they do have.
func TestValidate_CredentialNarrowsToARouteItsOwnerDoesNotHold(t *testing.T) {
	inv := validInventory()
	inv.Users["doug"] = inventory.User{
		Username: "doug",
		Access:   []string{"sfo"},
		Credentials: map[string]inventory.Credential{
			"default": {},
			"mbp":     {Note: "the laptop", Access: []string{"nonesuch"}},
		},
	}
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `credential "mbp" names route "nonesuch"`) {
		t.Fatalf("Validate = %v, want a credential narrowed past its owner's access", messages(got))
	}
}

// Narrowing to a route the person does hold is ordinary and reports nothing.
func TestValidate_CredentialNarrowedWithinItsOwnersAccessIsFine(t *testing.T) {
	inv := validInventory()
	user := inv.Users["doug"]
	user.Credentials = map[string]inventory.Credential{
		"default": {},
		"mbp":     {Note: "the laptop", Access: []string{user.Access[0]}},
	}
	inv.Users["doug"] = user
	manifests := validManifests()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if containsSubstring(got, `credential "mbp"`) {
		t.Fatalf("Validate = %v, want nothing reported for a credential within its owner's access", messages(got))
	}
}

// TestValidate_UpstreamSharedFromAPortThatHandsOutNothing covers rule 17.
// Needing the hop's shared secrets is the dialling service's own
// declaration, so nothing about the port it reaches makes it true. A port
// handing out none renders an empty list and a credential built half from
// it, which authenticates nothing and looks complete in the file.
func TestValidate_UpstreamSharedFromAPortThatHandsOutNothing(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	ss := manifests["ssserver"]
	ss.Self = confgen.SelfDecls{"psk": {Set: true}}
	ss.Upstream = confgen.UpstreamDecls{confgen.UpstreamShared: {}}
	manifests["ssserver"] = ss

	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `instance "ss-relay" needs the shared secrets of ss-srv:alt`) {
		t.Fatalf("Validate = %v, want the relay's unmet need named", messages(got))
	}

	// The port it dials now hands one out, which is the only thing that
	// can settle it.
	inst := &inv.Nodes[0].Instances[0]
	port := inst.Ports["alt"]
	port.Self = []string{"psk.alt"}
	inst.Ports["alt"] = port
	got = Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if containsSubstring(got, "needs the shared secrets") {
		t.Fatalf("Validate = %v, want no issue once the port hands one out", messages(got))
	}
}

// TestValidate_UpstreamSharedUndeclaredIsNotAnIssue is the other half: a
// service that never asked is not owed anything, however little the hop it
// dials hands out. A reverse proxy in front of a web service is this case.
func TestValidate_UpstreamSharedUndeclaredIsNotAnIssue(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()

	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if containsSubstring(got, "needs the shared secrets") {
		t.Fatalf("Validate = %v, want nothing owed to a service that did not ask", messages(got))
	}
}

// fanOutInventory puts a proxy in front of two web services, each on its own
// route, which is the shape rule 8 used to reject outright.
func fanOutInventory() (*inventory.Root, map[string]confgen.Manifest) {
	inv := validInventory()
	inv.Nodes[0].Instances = append(inv.Nodes[0].Instances,
		inventory.Instance{ID: "proxy", Service: "caddy", Ports: inventory.PortsOf(map[string]int{"https": 443})},
		inventory.Instance{ID: "vault", Service: "web", Ports: inventory.Ports{
			"web": {Number: 8222, Published: "vault.example.com"},
		}},
		inventory.Instance{ID: "bin", Service: "web", Ports: inventory.Ports{
			"web": {Number: 8080, Published: "clip.example.com"},
		}},
	)
	inv.Routes["vault"] = inventory.Route{Hops: []string{"proxy:https", "vault:web"}}
	inv.Routes["clip"] = inventory.Route{Hops: []string{"proxy:https", "bin:web"}}

	manifests := validManifests()
	manifests["caddy"] = confgen.Manifest{Auth: confgen.AuthNone, Downstreams: confgen.DownstreamsMany, Template: "t"}
	manifests["web"] = confgen.Manifest{Auth: confgen.AuthNone, Template: "t"}
	return inv, manifests
}

func TestValidate_FanOutIsNotRuleBasedRouting(t *testing.T) {
	inv, manifests := fanOutInventory()
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if len(got) != 0 {
		t.Fatalf("Validate = %v, want none", messages(got))
	}
}

// Without the declaration the same two routes are rule 8's original error,
// and the message points at the declaration rather than leaving a reader to
// hunt for a rule set they never wrote.
func TestValidate_TwoSuccessorsWithoutFanOut(t *testing.T) {
	inv, manifests := fanOutInventory()
	manifests["caddy"] = confgen.Manifest{Auth: confgen.AuthNone, Template: "t"}
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `instance "proxy" has different successors in routes "clip" and "vault"`) {
		t.Fatalf("Validate = %v, want a successor issue", messages(got))
	}
	if !containsSubstring(got, "downstreams: many") {
		t.Fatalf("Validate = %v, want the error to name the declaration", messages(got))
	}
}

// Rule 17: a downstream with no published name leaves the proxy nothing to
// match a site block on.
func TestValidate_FanOutDownstreamWithoutPublished(t *testing.T) {
	inv, manifests := fanOutInventory()
	inv.Nodes[0].Instances[2].Ports["web"] = inventory.Port{Number: 8222}
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `instance "proxy" reaches vault:web in route "vault", which declares no published name`) {
		t.Fatalf("Validate = %v, want a missing-published issue", messages(got))
	}
}

// Rule 18: a proxy tells its downstreams apart by name alone, so two ports
// behind it cannot share one.
func TestValidate_PublishedNameSharedBehindAProxy(t *testing.T) {
	inv, manifests := fanOutInventory()
	inv.Nodes[0].Instances[3].Ports["web"] = inventory.Port{Number: 8080, Published: "vault.example.com"}
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `published name "vault.example.com" is declared by bin:web and vault:web, and a proxy in front of one cannot tell them apart`) {
		t.Fatalf("Validate = %v, want a duplicate-published issue", messages(got))
	}
}

// Rule 18: two ports no proxy fronts may share a name, as two services on one
// machine share its DNS name, as long as their number or transport differs.
func TestValidate_PublishedNameSharedOnDifferentPorts(t *testing.T) {
	inv, manifests := fanOutInventory()
	inv.Routes = map[string]inventory.Route{}
	inv.Nodes[0].Instances[2].Ports["web"] = inventory.Port{Number: 8222, Published: "host.example.com"}
	inv.Nodes[0].Instances[3].Ports["web"] = inventory.Port{Number: 8222, Protocol: inventory.ProtocolUDP, Published: "host.example.com"}
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if containsSubstring(got, `published name "host.example.com"`) {
		t.Fatalf("Validate = %v, want no duplicate-published issue", messages(got))
	}
}

// Rule 18: a name resolves to one machine, so ports on two nodes cannot share
// it even on different numbers — a client of the second would dial the first.
func TestValidate_PublishedNameSharedAcrossNodes(t *testing.T) {
	inv, manifests := fanOutInventory()
	inv.Routes = map[string]inventory.Route{}
	bin := inv.Nodes[0].Instances[3]
	inv.Nodes[0].Instances = inv.Nodes[0].Instances[:3]
	bin.Ports = inventory.Ports{"web": {Number: 443, Protocol: inventory.ProtocolUDP, Published: "host.example.com"}}
	inv.Nodes[0].Instances[2].Ports["web"] = inventory.Port{Number: 8222, Published: "host.example.com"}
	other := inv.Nodes[0]
	other.ID, other.Instances = "other", []inventory.Instance{bin}
	inv.Nodes = append(inv.Nodes, other)
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `published name "host.example.com" is declared by bin:web on other and vault:web on `+inv.Nodes[0].ID+`, and one name reaches one machine`) {
		t.Fatalf("Validate = %v, want a duplicate-published issue", messages(got))
	}
}

// Rule 18: on one number and transport, nobody dialing the name can tell the
// two ports apart.
func TestValidate_PublishedNameSharedOnOnePort(t *testing.T) {
	inv, manifests := fanOutInventory()
	inv.Routes = map[string]inventory.Route{}
	inv.Nodes[0].Instances[2].Ports["web"] = inventory.Port{Number: 8222, Published: "host.example.com"}
	inv.Nodes[0].Instances[3].Ports["web"] = inventory.Port{Number: 8222, Published: "host.example.com"}
	got := Validate(inv, manifests, validExports(), derived(t, inv, manifests), nil)
	if !containsSubstring(got, `published name "host.example.com" is declared by bin:web and vault:web, both on 8222/tcp`) {
		t.Fatalf("Validate = %v, want a duplicate-published issue", messages(got))
	}
}
