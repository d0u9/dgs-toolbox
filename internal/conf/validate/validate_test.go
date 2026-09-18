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
					{ID: "ss-srv", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 38250, "alt": 49217}},
				},
			},
			{
				ID:       "relay",
				Networks: inventory.Networks{"internet": "203.0.113.20"},
				Instances: []inventory.Instance{
					{ID: "ss-relay", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 40000}},
				},
			},
			{
				ID:      "laptop",
				Owner:   "doug",
				Reaches: []string{"home"},
				Instances: []inventory.Instance{
					{ID: "laptop-sfo", Ports: map[string]int{"local": 1080}},
				},
			},
		},
		Users: map[string]inventory.User{
			"doug": {Username: "doug", Access: []string{"sfo"}},
			"yak":  {Username: "yak", Devices: inventory.DevicesUnmanaged, Access: []string{"sfo"}},
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
		"shadowsocks-rust": {Roles: map[string]confgen.Role{
			"server":  {Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust"},
			"ss-rust": {Auth: confgen.AuthNone},
		}},
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
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
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
	got := Validate(inv, manifests, &derive.Model{}, nil)
	if !containsSubstring(got, `instance "ss-srv" is defined more than once`) {
		t.Fatalf("Validate = %v, want a duplicate-instance issue", messages(got))
	}
}

func TestValidate_UnknownServiceAndRole(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances[0].Service = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `service "nonesuch" is not defined`) {
		t.Fatalf("Validate = %v, want an unknown-service issue", messages(got))
	}
}

func TestValidate_UnknownRole(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances[0].Role = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `has no role "nonesuch"`) {
		t.Fatalf("Validate = %v, want an unknown-role issue", messages(got))
	}
}

func TestValidate_ReachedByNamesUnknownRole(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	manifests["shadowsocks-rust"].Roles["server"] = confgen.Role{Auth: confgen.AuthPerPrincipal, ReachedBy: "nonesuch"}
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `reached_by "nonesuch" is not a role of this service`) {
		t.Fatalf("Validate = %v, want a bad reached_by issue", messages(got))
	}
}

func TestValidate_ClientRoleNotARoleOfTheService(t *testing.T) {
	inv := validInventory()
	inv.Nodes[2].ClientRole = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `client_role "nonesuch" is not a role of "shadowsocks-rust"`) {
		t.Fatalf("Validate = %v, want a bad client_role issue", messages(got))
	}
}

func TestValidate_HopNamesMissingInstanceOrPort(t *testing.T) {
	inv := validInventory()
	inv.Routes["broken-instance"] = inventory.Route{Hops: []string{"nonesuch:main"}}
	inv.Routes["broken-port"] = inventory.Route{Hops: []string{"ss-srv:nonesuch"}}
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `names an instance that does not exist`) {
		t.Fatalf("Validate = %v, want a missing-instance issue", messages(got))
	}
	if !containsSubstring(got, `has no port "nonesuch"`) {
		t.Fatalf("Validate = %v, want a missing-port issue", messages(got))
	}
}

func TestValidate_ReservedOwnPort(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances[0].Ports["own"] = 1234
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `port "own" is reserved`) {
		t.Fatalf("Validate = %v, want a reserved-port issue", messages(got))
	}
}

func TestValidate_UnknownRouteInAccessAndUnknownOwner(t *testing.T) {
	inv := validInventory()
	inv.Users["doug"] = inventory.User{Username: "doug", Access: []string{"sfo", "nonesuch"}}
	inv.Nodes[2].Owner = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
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
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `overrides nothing this node derives`) {
		t.Fatalf("Validate = %v, want an unmatched-override issue", messages(got))
	}
}

func TestValidate_OverrideSettingServiceOrRole(t *testing.T) {
	inv := validInventory()
	inv.Nodes[2].Instances[0].Service = "shadowsocks-rust"
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `sets service or role; an override may only set ports and bind`) {
		t.Fatalf("Validate = %v, want an invalid-override issue", messages(got))
	}
}

func TestValidate_DifferentSuccessorsForOneNonTerminalHop(t *testing.T) {
	inv := validInventory()
	inv.Nodes[1].Instances = append(inv.Nodes[1].Instances, inventory.Instance{
		ID: "ss-relay-b", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 40001},
	})
	inv.Routes["chain2"] = inventory.Route{Hops: []string{"ss-relay:main", "ss-relay-b:main"}}
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `has different successors in routes`) {
		t.Fatalf("Validate = %v, want a rule-based-routing issue", messages(got))
	}
}

func TestValidate_RouteNamesOneInstanceTwice(t *testing.T) {
	inv := validInventory()
	inv.Routes["loop"] = inventory.Route{Hops: []string{"ss-relay:main", "ss-srv:alt", "ss-relay:main"}}
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `names instance "ss-relay" twice`) {
		t.Fatalf("Validate = %v, want a repeated-instance issue", messages(got))
	}
}

func TestValidate_UnmanagedUserRouteNotEnteringOnInternet(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Networks = inventory.Networks{"home": "192.168.1.5"} // srv no longer has an internet address.
	// The relay->srv edge in "chain" would now fail to resolve too, which
	// Derive treats as fatal; this rule reads only the inventory, so check
	// it directly rather than through a Derive that would never get here.
	manifests := validManifests()
	got := Validate(inv, manifests, &derive.Model{}, nil)
	if !containsSubstring(got, `(unmanaged): route "sfo" enters "ss-srv", which has no address on the universal network`) {
		t.Fatalf("Validate = %v, want an unmanaged-not-on-internet issue", messages(got))
	}
}

func TestValidate_DuplicateAccountNameOnOnePort(t *testing.T) {
	inv := validInventory()
	// A second unmanaged user with the same username as an existing managed
	// device's rendered account collides on ss-srv's main port.
	inv.Users["doug-laptop-clash"] = inventory.User{Username: "doug-laptop", Devices: inventory.DevicesUnmanaged, Access: []string{"sfo"}}
	manifests := validManifests()
	model := derived(t, inv, manifests)
	got := Validate(inv, manifests, model, nil)
	if !containsSubstring(got, `account "doug-laptop" is rendered by more than one principal`) {
		t.Fatalf("Validate = %v, want a duplicate-account issue", messages(got))
	}
}

func TestValidate_TwoInstancesBindTheSameAddressAndPort(t *testing.T) {
	inv := validInventory()
	inv.Nodes[0].Instances = append(inv.Nodes[0].Instances, inventory.Instance{
		ID: "other-srv", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 38250},
	})
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `bind the same address and port`) {
		t.Fatalf("Validate = %v, want a bind-collision issue", messages(got))
	}
}

func TestValidate_UniversalNotInNetworksList(t *testing.T) {
	inv := validInventory()
	inv.Universal = "nonesuch"
	manifests := validManifests()
	got := Validate(inv, manifests, &derive.Model{}, nil)
	if !containsSubstring(got, `universal "nonesuch" does not name a network in the list`) {
		t.Fatalf("Validate = %v, want a bad-universal issue", messages(got))
	}
}

func TestValidate_UnmanagedUserClientRoleNotARoleOfTheService(t *testing.T) {
	inv := validInventory()
	yak := inv.Users["yak"]
	yak.ClientRole = "nonesuch"
	inv.Users["yak"] = yak
	manifests := validManifests()
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `user "yak": client_role "nonesuch" is not a role of "shadowsocks-rust"`) {
		t.Fatalf("Validate = %v, want a bad unmanaged client_role issue", messages(got))
	}
}

func TestValidate_StalePrevious(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	previous := map[string]time.Time{
		"ss-srv/main/node/laptop": time.Now().Add(-8 * 24 * time.Hour),
	}
	got := Validate(inv, manifests, derived(t, inv, manifests), previous)
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
	got := Validate(inv, manifests, derived(t, inv, manifests), previous)
	if containsSubstring(got, "seven-day limit") {
		t.Fatalf("Validate = %v, want no stale-.previous issue for a six-day-old file", messages(got))
	}
}

// TestValidate_CombineOwnNotInOwnList covers a combine_own naming a secret
// the role's own list does not hold: sync would neither generate that file
// nor report it, so every client deriving from the role renders without the
// shared identity it needs.
func TestValidate_CombineOwnNotInOwnList(t *testing.T) {
	inv := validInventory()
	manifests := validManifests()
	manifests["shadowsocks-rust"].Roles["server"] = confgen.Role{Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust", CombineOwn: "psk"}
	got := Validate(inv, manifests, derived(t, inv, manifests), nil)
	if !containsSubstring(got, `combine_own "psk" is not in this role's own list`) {
		t.Fatalf("Validate = %v, want a combine_own outside the own list issue", messages(got))
	}

	manifests["shadowsocks-rust"].Roles["server"] = confgen.Role{Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust", CombineOwn: "psk", Own: []string{"psk"}}
	got = Validate(inv, manifests, derived(t, inv, manifests), nil)
	if containsSubstring(got, "combine_own") {
		t.Fatalf("Validate = %v, want no combine_own issue once psk is listed", messages(got))
	}
}
