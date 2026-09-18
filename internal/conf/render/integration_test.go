package render_test

// This is the milestone 6 checkpoint docs/apps/conf/inventory.md asks for:
// "at the end of this milestone one target renders end to end." It wires
// inventory, derive and secretstore together by hand — the way a future
// export command will — and renders one real target through render.Render.

import (
	"strings"
	"testing"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/render"
	"dgs-toolbox/internal/conf/secretstore"
)

func TestEndToEnd_OneTargetRenders(t *testing.T) {
	inv := &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "srv",
				Networks: inventory.Networks{"internet": "203.0.113.10"},
				Instances: []inventory.Instance{
					{ID: "ss-srv", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 38250}},
				},
			},
			{ID: "laptop", Owner: "doug"},
		},
		Users:     map[string]inventory.User{"doug": {Access: []string{"sfo"}}},
		Routes:    map[string]inventory.Route{"sfo": {Hops: []string{"ss-srv:main"}}},
		Networks:  []string{"internet"},
		Universal: "internet",
	}
	manifests := map[string]confgen.Manifest{
		"shadowsocks-rust": {
			Secret: confgen.Secret{Kind: "base64", Bytes: 32},
			Roles: map[string]confgen.Role{
				"server":  {Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust"},
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

	// Render the server side, ss-srv/main: one principal, doug's laptop.
	target := render.Target{Service: "shadowsocks-rust", Role: "server", Instance: "ss-srv"}
	srvNode := inv.Nodes[0]
	srvInst := srvNode.Instances[0]

	var principals []render.Principal
	for _, p := range model.Principals("ss-srv", "main") {
		value, err := secretstore.ReadValue(secretsRoot, secretstore.Path{
			Instance: "ss-srv", Port: "main", Kind: string(p.Kind), Name: p.ID,
		})
		if err != nil {
			t.Fatalf("ReadValue: %v", err)
		}
		principals = append(principals, render.Principal{Name: p.Name, Secret: value})
	}

	out, err := render.Render(render.Input{
		Target:       target,
		Template:     `{{ (node).id }}/{{ (instance).id }}: {{ range principals "main" }}{{ .Name }} {{ end }}`,
		DefaultsKind: confgen.DefaultsDocument,
		Instance:     map[string]any{"id": srvInst.ID, "service": srvInst.Service, "role": srvInst.Role},
		Node:         map[string]any{"id": srvNode.ID},
		Principals:   map[string][]render.Principal{"main": principals},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.HasPrefix(string(out), "srv/ss-srv: doug-laptop") {
		t.Fatalf("out = %q, want it to start with srv/ss-srv: doug-laptop", out)
	}
	if len(principals) != 1 || principals[0].Secret == "" {
		t.Fatalf("principals = %+v, want one principal with a non-empty secret", principals)
	}

	// Render the derived client side too: laptop-sfo, whose upstream is
	// ss-srv:main, resolved to srv's internet address.
	var upstream map[string]any
	for _, e := range model.Edges {
		if e.FromInstance == "laptop-sfo" {
			upstreamSecret, err := secretstore.ReadValue(secretsRoot, secretstore.Path{
				Instance: "ss-srv", Port: "main", Kind: "node", Name: "laptop",
			})
			if err != nil {
				t.Fatalf("ReadValue: %v", err)
			}
			upstream = map[string]any{"address": e.Address, "port": e.Port, "secret": upstreamSecret}
		}
	}
	if upstream == nil {
		t.Fatal("no edge found from laptop-sfo")
	}

	clientOut, err := render.Render(render.Input{
		Target:       render.Target{Service: "shadowsocks-rust", Role: "ss-rust", Instance: "laptop-sfo"},
		Template:     `{{ (upstream).address }}:{{ (upstream).port }}`,
		DefaultsKind: confgen.DefaultsDocument,
		Upstream:     upstream,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(clientOut) != "203.0.113.10:38250" {
		t.Fatalf("clientOut = %q, want 203.0.113.10:38250", clientOut)
	}
}
