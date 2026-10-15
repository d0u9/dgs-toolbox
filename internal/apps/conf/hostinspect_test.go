package conf

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func buildBoundHostRoot(t *testing.T) (string, string) {
	t.Helper()
	root, secrets := buildExportableRoot(t)
	path := filepath.Join(root, "nodes", "srv.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var node map[string]any
	if err := yaml.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	node["host"] = "host-a"
	data, err = yaml.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
	writeFile(t, filepath.Join(root, "hosts.yaml"), `hosts:
  host-a:
    names:
      device.example.test: [{interface: wired, address: main}]
    interfaces:
      wired:
        mac: "02:00:00:00:00:11"
        addresses:
          main: {network: internet, ip: 203.0.113.10, dhcp_reservation: true, names: [wired.example.test]}
          extra: {network: backend, ip: 203.0.113.12}
      wifi:
        mac: "02:00:00:00:00:12"
        addresses:
          main: {network: internet, ip: 203.0.113.11}
    endpoints:
      internet: {from: {interface: wired, address: main}}
      edge: {address: host-a.example.net}
  printer:
    interfaces:
      lan:
        mac: "02:00:00:00:00:13"
        addresses:
          main: {network: internet, ip: 203.0.113.20}
  host-b:
    names:
      host-b.dev.example.test: [{endpoint: internet}]
    endpoints:
      internet: {address: host-b.example.test}
  host-c:
    interfaces:
      lan: {mac: "02:00:00:00:00:14"}
`)
	writeFile(t, filepath.Join(root, "networks.yaml"), "networks: [{name: internet}, {name: edge}, {name: backend}]\nuniversal: internet\n")
	return root, secrets
}

func TestHostNodeDetailAndGraphInventory(t *testing.T) {
	root, secrets := buildBoundHostRoot(t)
	m := newInspectModel(root, secrets)
	if m.loadErr != nil {
		t.Fatal(m.loadErr)
	}
	detail, err := renderNodeDetail(m.l, "srv")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"host-a", "hosts.yaml", "wired", "wifi", "203.0.113.10", "203.0.113.11", "203.0.113.12", "02:00:00:00:00:11", "02:00:00:00:00:12", "wired/main", "DHCP reservation", "wired.example.test", "device name device.example.test"} {
		if !strings.Contains(detail, want) {
			t.Errorf("node detail missing %q: %s", want, detail)
		}
	}
	g := buildGraph(m.l, "test")
	seen := map[string]bool{}
	for _, n := range g.Nodes {
		if n.ID == "hosts:host-a" {
			t.Fatal("bound host duplicated")
		}
		if strings.HasPrefix(n.ID, "hosts:") {
			seen[n.ID] = true
			want := map[string]string{"hosts:printer": "203.0.113.20", "hosts:host-b": "host-b.example.test", "hosts:host-c": "02:00:00:00:00:14"}[n.ID]
			if want != "" && !strings.Contains(n.Tooltip, want) {
				t.Errorf("host tooltip %s missing %s: %s", n.ID, want, n.Tooltip)
			}
			if n.ID == "hosts:host-b" && (!strings.Contains(n.Tooltip, "host-b.dev.example.test") || !strings.Contains(n.Tooltip, "endpoint internet")) {
				t.Error("public device name reference missing from graph detail")
			}
			if n.ID != "hosts:host-c" && !slices.Contains(n.Tags, "net:internet") {
				t.Errorf("missing host network tag: %+v", n)
			}
		}
	}
	for _, id := range []string{"hosts:printer", "hosts:host-b", "hosts:host-c"} {
		if !seen[id] {
			t.Errorf("host absent: %s", id)
		}
	}
	for _, group := range g.Groups {
		if group.ID == "srv" {
			for _, want := range []string{"wired", "wifi", "203.0.113.12", "02:00:00:00:00:12"} {
				if !strings.Contains(group.Detail, want) {
					t.Errorf("bound group missing %s", want)
				}
			}
			if !slices.Contains(group.Tags, "net:internet") {
				t.Error("bound group missing network tag")
			}
		}
	}
}
