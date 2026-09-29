package conf

import (
	"strings"
	"testing"

	"dgs-toolbox/internal/conf/inventory"
)

func TestMigrationDNSReviewNamesProxyIngressWithoutClaimingBackendIsDNSTarget(t *testing.T) {
	before := loaded{inv: &inventory.Root{
		Nodes: []inventory.Node{
			{ID: "proxy", Networks: inventory.Networks{"internet": "198.51.100.5"}, Instances: []inventory.Instance{{ID: "gateway", Ports: inventory.Ports{"web": {Number: 443}}}}},
			{ID: "network-4", Networks: inventory.Networks{"home": "10.0.1.4"}, Instances: []inventory.Instance{{ID: "vault", Service: "vaultwarden", Ports: inventory.Ports{"web": {Number: 8080, Published: "vault.example.test"}}}}},
			{ID: "unrelated", Networks: inventory.Networks{"internet": "198.51.100.9"}, Instances: []inventory.Instance{{ID: "other", Ports: inventory.Ports{"web": {Number: 80, Published: "other.example.test"}}}}},
		},
		Routes: map[string]inventory.Route{"vault": {Hops: []string{"gateway:web", "vault:web"}}, "other": {Hops: []string{"other:web"}}},
	}}
	after := loaded{inv: &inventory.Root{
		Nodes: []inventory.Node{
			before.inv.Nodes[0],
			{ID: "network-8", Networks: inventory.Networks{"network-8": "10.0.1.8"}, Instances: before.inv.Nodes[1].Instances},
			before.inv.Nodes[2],
		},
		Routes: before.inv.Routes,
	}}
	rep := &migrationReport{}
	migrationDNSReview(rep, before, after, "network-4", "network-8", true, nil)
	got := rep.dnsMarkdown()
	for _, want := range []string{
		"**`vault.example.test`** — `vaultwarden/vault` on port `web`",
		"- Ingress: route `vault` via `proxy` → route `vault` via `proxy`",
		"- Ingress `proxy`: `internet 198.51.100.5` → `internet 198.51.100.5`",
		"- Backend candidate `network-4 → network-8`: `home 10.0.1.4` → `network-8 10.0.1.8`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "other.example.test") || strings.Contains(got, "DNS change required") {
		t.Fatalf("report asserted unrelated or unverified DNS work:\n%s", got)
	}
}

func TestMigrationDNSReviewListsPublishedNameWhenIngressNodeMoves(t *testing.T) {
	before := loaded{inv: &inventory.Root{
		Nodes: []inventory.Node{
			{ID: "network-4", Networks: inventory.Networks{"internet": "203.0.113.4"}, Instances: []inventory.Instance{{ID: "proxy", Ports: inventory.Ports{"https": {Number: 443}}}}},
			{ID: "backend", Networks: inventory.Networks{"home": "10.0.1.20"}, Instances: []inventory.Instance{{ID: "site", Service: "site", Ports: inventory.Ports{"web": {Number: 8080, Published: "site.example.test"}}}}},
		},
		Routes: map[string]inventory.Route{"site": {Hops: []string{"proxy:https", "site:web"}}},
	}}
	after := loaded{inv: &inventory.Root{
		Nodes:  []inventory.Node{{ID: "network-8", Networks: inventory.Networks{"internet": "203.0.113.8"}, Instances: before.inv.Nodes[0].Instances}, before.inv.Nodes[1]},
		Routes: before.inv.Routes,
	}}
	rep := &migrationReport{}
	migrationDNSReview(rep, before, after, "network-4", "network-8", true, nil)
	got := rep.dnsMarkdown()
	for _, want := range []string{"**`site.example.test`**", "- Ingress: route `site` via `network-4` → route `site` via `network-8`", "- Ingress `network-4 → network-8`: `internet 203.0.113.4` → `internet 203.0.113.8`"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}
