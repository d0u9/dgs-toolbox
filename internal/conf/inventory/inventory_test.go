package inventory

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ParsesNodesUsersRoutesAndNetworks(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, NodesDir, "us-sfo-dgo-linux-01.yaml"), `
id: us-sfo-dgo-linux-01

networks:
  internet: 203.0.113.10

instances:
  - id: ss-sfo01
    service: shadowsocks-rust
    role: server
    ports:
      main: 38250
      alt: 49217
`)
	writeFile(t, filepath.Join(root, NodesDir, "macbook.yaml"), `
id: macbook
owner: doug
reaches: [home]
`)
	writeFile(t, filepath.Join(root, UsersFilename), `
users:
  doug:
    access: [sfo]
  friend-a:
    username: yak
    devices: unmanaged
    client_role: link
    access: [sfo]
`)
	writeFile(t, filepath.Join(root, RoutesFilename), `
routes:
  sfo:
    hops: [ss-sfo01:main]
`)
	writeFile(t, filepath.Join(root, NetworksFilename), `
networks: [home, internet]
universal: internet
`)

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Nodes) != 2 {
		t.Fatalf("Nodes = %d, want 2: %+v", len(got.Nodes), got.Nodes)
	}
	// Sorted by path/file name: macbook.yaml before us-sfo-dgo-linux-01.yaml.
	macbook, sfo := got.Nodes[0], got.Nodes[1]

	if macbook.ID != "macbook" || macbook.Owner != "doug" {
		t.Fatalf("macbook = %+v", macbook)
	}
	if macbook.Broken != "" {
		t.Fatalf("macbook.Broken = %q, want empty", macbook.Broken)
	}
	if len(macbook.Networks) != 0 {
		t.Fatalf("macbook.Networks = %+v, want none (it has no address anywhere)", macbook.Networks)
	}
	if len(macbook.Reaches) != 1 || macbook.Reaches[0] != "home" {
		t.Fatalf("macbook.Reaches = %+v, want [home]", macbook.Reaches)
	}

	if sfo.ID != "us-sfo-dgo-linux-01" {
		t.Fatalf("sfo.ID = %q", sfo.ID)
	}
	if sfo.Networks["internet"] != "203.0.113.10" {
		t.Fatalf(`sfo.Networks["internet"] = %q`, sfo.Networks["internet"])
	}
	if len(sfo.Instances) != 1 {
		t.Fatalf("sfo.Instances = %+v, want 1", sfo.Instances)
	}
	inst := sfo.Instances[0]
	if inst.ID != "ss-sfo01" || inst.Service != "shadowsocks-rust" || inst.Role != "server" {
		t.Fatalf("inst = %+v", inst)
	}
	if inst.Ports["main"] != 38250 || inst.Ports["alt"] != 49217 {
		t.Fatalf("inst.Ports = %+v", inst.Ports)
	}

	if got.UsersBroken != "" {
		t.Fatalf("UsersBroken = %q, want empty", got.UsersBroken)
	}
	if len(got.Users) != 2 {
		t.Fatalf("Users = %d, want 2: %+v", len(got.Users), got.Users)
	}
	if got.Users["friend-a"].Devices != DevicesUnmanaged {
		t.Fatalf("friend-a.Devices = %q, want %q", got.Users["friend-a"].Devices, DevicesUnmanaged)
	}
	if got.Users["doug"].Devices != "" {
		t.Fatalf("doug.Devices = %q, want empty (managed is the default)", got.Users["doug"].Devices)
	}
	if got.Users["doug"].UsernameOr("doug") != "doug" {
		t.Fatalf(`doug.UsernameOr("doug") = %q, want "doug" (falls back to the map key)`, got.Users["doug"].UsernameOr("doug"))
	}
	if got.Users["friend-a"].UsernameOr("friend-a") != "yak" {
		t.Fatalf(`friend-a.UsernameOr("friend-a") = %q, want "yak"`, got.Users["friend-a"].UsernameOr("friend-a"))
	}
	if got.Users["friend-a"].ClientRole != "link" {
		t.Fatalf("friend-a.ClientRole = %q, want %q", got.Users["friend-a"].ClientRole, "link")
	}

	if got.RoutesBroken != "" {
		t.Fatalf("RoutesBroken = %q, want empty", got.RoutesBroken)
	}
	if len(got.Routes) != 1 || len(got.Routes["sfo"].Hops) != 1 {
		t.Fatalf("Routes = %+v", got.Routes)
	}
	if got.Routes["sfo"].Hops[0] != "ss-sfo01:main" {
		t.Fatalf("Hops[0] = %q", got.Routes["sfo"].Hops[0])
	}

	if got.NetworksBroken != "" {
		t.Fatalf("NetworksBroken = %q, want empty", got.NetworksBroken)
	}
	if len(got.Networks) != 2 || got.Networks[0] != "home" || got.Networks[1] != "internet" {
		t.Fatalf("Networks = %+v", got.Networks)
	}
	if got.Universal != "internet" {
		t.Fatalf("Universal = %q, want %q", got.Universal, "internet")
	}
}

// TestLoad_InstanceValuesAreOpaque covers an instance's own values: dgs
// parses them as an arbitrary mapping and does not interpret their shape —
// nested structure and a list both come through unchanged, for a template to
// read by name. See docs/apps/conf/inventory.md#an-instances-own-values.
func TestLoad_InstanceValuesAreOpaque(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, NodesDir, "srv.yaml"), `
id: srv
networks:
  internet: 203.0.113.10
instances:
  - id: hy2-sfo01
    service: hysteria2
    role: server
    ports:
      main: 443
    values:
      masquerade:
        type: proxy
        proxy:
          url: https://example.org/
          rewriteHost: true
      tags: [prod, sfo]
`)

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Nodes) != 1 || len(got.Nodes[0].Instances) != 1 {
		t.Fatalf("got = %+v", got)
	}
	values := got.Nodes[0].Instances[0].Values
	masquerade, ok := values["masquerade"].(map[string]any)
	if !ok {
		t.Fatalf("values[masquerade] = %#v, want a nested mapping", values["masquerade"])
	}
	proxy, ok := masquerade["proxy"].(map[string]any)
	if !ok || proxy["url"] != "https://example.org/" {
		t.Fatalf("values[masquerade][proxy] = %#v", masquerade["proxy"])
	}
	tags, ok := values["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "prod" {
		t.Fatalf("values[tags] = %#v, want [prod sfo]", values["tags"])
	}
}

func TestLoad_MissingInventoryFilesAreEmptyNotErrors(t *testing.T) {
	root := t.TempDir()

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Nodes) != 0 || got.Users != nil || got.Routes != nil || got.Networks != nil {
		t.Fatalf("got = %+v, want an empty inventory", got)
	}
	if got.UsersBroken != "" || got.RoutesBroken != "" || got.NetworksBroken != "" {
		t.Fatalf("got = %+v, want no Broken set for files that simply don't exist", got)
	}
}

func TestLoad_MissingRootIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("Load: want an error for a missing root")
	}
}

func TestLoad_BrokenNodeIsListedWithParseError(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, NodesDir, "good.yaml"), "id: good\nreaches: [home]\n")
	// Invalid YAML.
	writeFile(t, filepath.Join(root, NodesDir, "bad-syntax.yaml"), "id: [unterminated\n")
	// Unknown key.
	writeFile(t, filepath.Join(root, NodesDir, "bad-key.yaml"), "id: bad-key\ntypo_field: oops\n")
	// A network entry that is neither a mapping nor a list.
	writeFile(t, filepath.Join(root, NodesDir, "bad-networks.yaml"), "id: bad-networks\nnetworks: home\n")

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Nodes) != 4 {
		t.Fatalf("Nodes = %d, want 4 (broken nodes still listed): %+v", len(got.Nodes), got.Nodes)
	}

	byPath := map[string]Node{}
	for _, n := range got.Nodes {
		byPath[filepath.Base(n.Path)] = n
	}

	if byPath["good.yaml"].Broken != "" {
		t.Fatalf("good.Broken = %q, want empty", byPath["good.yaml"].Broken)
	}
	if byPath["bad-syntax.yaml"].Broken == "" {
		t.Fatal("bad-syntax.Broken = empty, want parse error")
	}
	if byPath["bad-key.yaml"].Broken == "" {
		t.Fatal("bad-key.Broken = empty, want unknown-field error")
	}
	if byPath["bad-networks.yaml"].Broken == "" {
		t.Fatal("bad-networks.Broken = empty, want a networks shape error")
	}
}

func TestLoad_BrokenTopLevelFileIsReported(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, UsersFilename), "users: [not, a, mapping]\n")
	writeFile(t, filepath.Join(root, RoutesFilename), "routes:\n  sfo:\n    hops: [ss-sfo01:main]\n    typo: oops\n")
	writeFile(t, filepath.Join(root, NetworksFilename), "networks: {not: a-list}\n")

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.UsersBroken == "" {
		t.Fatal("UsersBroken = empty, want an error")
	}
	if got.RoutesBroken == "" {
		t.Fatal("RoutesBroken = empty, want an error")
	}
	if got.NetworksBroken == "" {
		t.Fatal("NetworksBroken = empty, want an error")
	}
}
