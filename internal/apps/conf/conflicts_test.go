package conf

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rcli "github.com/d0u9/rhumb/cli"
	"github.com/d0u9/rhumb/engine"
	"gopkg.in/yaml.v3"
)

const sharedContainerProcess = `id: node-1
runtime: docker
containers:
  - {name: apps, subnet: 10.0.1.0/24}
processes:
  program-a: {service: example}
instances:
  - id: app-a
    service: example
    process: program-a
    containers: {apps: 10.0.1.5}
    ports: {main: 8080}
  - id: app-b
    service: example
    process: program-a
    containers: {apps: 10.0.1.5}
    ports: {other: 8443}
`

const bridgeContainerNode = "id: node-1\nhost: host-a\ncontainers: [{name: apps, subnet: 10.0.0.0/24, gateway: 10.0.0.1}]\n"

func conflictCheckRoot(t *testing.T, node string) (string, string) {
	t.Helper()
	root, secrets := t.TempDir(), t.TempDir()
	for name, body := range map[string]string{
		"networks.yaml": "networks: [{name: lan, subnet: 10.0.0.0/24, gateway: 10.0.0.1}]\n",
		"hosts.yaml": `hosts:
  host-a:
    interfaces:
      bridge:
        addresses:
          a: {network: lan, ip: 10.0.0.1}
  host-b:
    endpoints:
      lan: {address: 10.0.0.20}
`,
		"nodes/node-1.yaml":                       node,
		"services/example/confgen.yaml":           "auth: none\ndefaults: document\ntemplate: templates/example.tmpl\noutput: config.txt\n",
		"services/example/templates/example.tmpl": "example configuration\n",
	} {
		writeFile(t, filepath.Join(root, name), body)
	}
	return root, secrets
}

func TestCheckReportNetworkConflicts(t *testing.T) {
	for _, tc := range []struct{ name, node, want string }{
		{"shared process", sharedContainerProcess, ""},
		{"process address mismatch", strings.Replace(sharedContainerProcess, "{apps: 10.0.1.5}", "{apps: 10.0.1.6}", 1), "differ in container addresses"},
		{"wildcard bind", `id: node-1
instances:
  - {id: app-a, service: example, bind: "0.0.0.0", ports: {main: 443}}
  - {id: app-b, service: example, bind: "192.0.2.1", ports: {main: 443}}
`, "bind overlapping addresses"},
		{"equivalent IPv6 bind", `id: node-1
instances:
  - {id: app-a, service: example, bind: "2001:db8::1", ports: {main: 443}}
  - {id: app-b, service: example, bind: "2001:0db8:0:0:0:0:0:1", ports: {main: 443}}
`, "bind the same address"},
		{"equivalent IPv6 gateway", `id: node-1
runtime: docker
containers:
  - {name: apps, subnet: "2001:db8::/64", gateway: "2001:db8::1"}
instances:
  - {id: app-a, service: example, containers: {apps: "2001:0db8:0:0:0:0:0:1"}}
`, "is the network's gateway"},
		{"host bridge gateway", bridgeContainerNode, ""},
		{"endpoint is not local ownership", "id: node-1\nhost: host-b\ncontainers: [{name: apps, subnet: 10.0.0.0/24}]\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, secrets := conflictCheckRoot(t, tc.node)
			var out bytes.Buffer
			err := writeCheckReport(&out, configFor(root, secrets))
			if tc.want == "" {
				if err != nil || !strings.Contains(out.String(), "no problem found") {
					t.Fatalf("Check: %v\n%s", err, out.String())
				}
			} else {
				var problems rcli.ErrProblems
				if !errors.As(err, &problems) || !strings.Contains(out.String(), tc.want) {
					t.Fatalf("Check: %v\n%s\nwant %q", err, out.String(), tc.want)
				}
			}
			data, err := os.ReadFile(filepath.Join(root, "nodes", "node-1.yaml"))
			if err != nil || string(data) != tc.node {
				t.Fatal("check changed inventory")
			}
		})
	}
}

func TestMigrationPreviewUsesNetworkValidation(t *testing.T) {
	for _, tc := range []struct{ name, node, want string }{
		{"shared process can migrate", sharedContainerProcess, ""},
		{"host bridge gateway can migrate", bridgeContainerNode, ""},
		{"process mismatch blocks migration", strings.Replace(sharedContainerProcess, "{apps: 10.0.1.5}", "{apps: 10.0.1.6}", 1), "differ in container addresses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, secrets := conflictCheckRoot(t, tc.node)
			m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
			m.scenario = rcli.MigrationRelocate
			m.selectNode("node-1")
			m.rows[0].after = "node-2"
			m.generateReport()
			if tc.want == "" {
				if m.mode != "report" {
					t.Fatalf("preview: %s", m.notice)
				}
			} else if m.mode == "report" || !strings.Contains(m.notice, tc.want) {
				t.Fatalf("preview mode=%s notice=%s, want %q", m.mode, m.notice, tc.want)
			}
			data, err := os.ReadFile(filepath.Join(root, "nodes", "node-1.yaml"))
			if err != nil || string(data) != tc.node {
				t.Fatal("preview changed inventory")
			}
		})
	}
}

func TestCheckReportContainerConflictCount(t *testing.T) {
	for _, independent := range []string{"app-0", "app-z"} {
		t.Run(independent, func(t *testing.T) {
			node := sharedContainerProcess + "  - id: " + independent + "\n    service: example\n    containers: {apps: 10.0.1.5}\n    ports: {extra: 9443}\n"
			root, secrets := conflictCheckRoot(t, node)
			var out bytes.Buffer
			err := writeCheckReport(&out, configFor(root, secrets))
			var problems rcli.ErrProblems
			if !errors.As(err, &problems) || problems.Count != 1 || strings.Count(out.String(), "both hold") != 1 {
				t.Fatalf("Check: %v\n%s\nwant one conflict for two runtime owners", err, out.String())
			}
		})
	}
}

func TestExportNormalizedHostMappings(t *testing.T) {
	root, secrets := conflictCheckRoot(t, `id: node-1
host: host-b
runtime: docker
containers: [{name: apps, subnet: 10.0.1.0/24}]
instances:
  - {id: app-a, service: example, ports: {main: 443}}
`)
	writeFile(t, filepath.Join(root, "networks.yaml"), "networks: [{name: lan-a}, {name: lan-b}, {name: lan-c}]\n")
	writeFile(t, filepath.Join(root, "hosts.yaml"), `hosts:
  host-b:
    endpoints:
      lan-a: {address: "2001:db8::1"}
      lan-b: {address: "2001:0db8:0:0:0:0:0:1"}
      lan-c: {address: "2001:db8::2"}
`)
	var report bytes.Buffer
	cfg := configFor(root, secrets)
	if err := writeCheckReport(&report, cfg); err != nil {
		t.Fatalf("Check: %v\n%s", err, &report)
	}
	destination := t.TempDir()
	if err := exportAction(strings.NewReader(""), &report, []string{"*"}, map[string]string{"to": destination, "yes": "true"}, cfg); err != nil {
		t.Fatalf("Export: %v\n%s", err, &report)
	}
	data, err := os.ReadFile(filepath.Join(destination, "node-1", "example", "app-a", engine.ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest engine.DeployManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Ports) != 1 || manifest.Ports[0].Port != 443 ||
		len(manifest.Ports[0].Bind) != 2 || manifest.Ports[0].Bind[0] != "2001:db8::1" || manifest.Ports[0].Bind[1] != "2001:db8::2" {
		t.Fatalf("exported port mappings = %+v, want normalized, deduplicated addresses with the distinct address retained", manifest.Ports)
	}
}

func TestMigrationApplyPreservesContainerManifest(t *testing.T) {
	for _, tc := range []struct {
		name, node, target, address string
		ports                       int
		toNode                      string
		renameMember                bool
	}{
		{"shared process", sharedContainerProcess, "program-a", "10.0.1.5", 2, "node-2", false},
		{"process member rename", sharedContainerProcess, "program-a", "10.0.1.5", 2, "node-1", true},
		{"node and process member rename", sharedContainerProcess, "program-a", "10.0.1.5", 2, "node-2", true},
		{"host bridge gateway", bridgeContainerNode + "runtime: docker\ninstances:\n  - {id: app-a, service: example, containers: {apps: 10.0.0.5}, ports: {main: 443}}\n", "app-a", "10.0.0.5", 1, "node-2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, secrets := conflictCheckRoot(t, tc.node)
			hostsBefore, err := os.ReadFile(filepath.Join(root, "hosts.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			m := newMigrationTable(mustLoadMigration(t, root), root, secrets)
			m.scenario = rcli.MigrationRelocate
			m.selectNode("node-1")
			m.rows[0].after = tc.toNode
			if tc.renameMember {
				found := false
				for i := range m.rows {
					if m.rows[i].kind == "instance" && m.rows[i].before == "app-a" {
						m.rows[i].after, found = "app-c", true
					}
				}
				if !found {
					t.Fatal("missing member rename row")
				}
			}
			m.generateReport()
			if m.mode != "report" {
				t.Fatalf("preview: %s", m.notice)
			}
			m.applyPlan()
			if m.reportDigest != "" || !strings.HasPrefix(m.notice, "Applied;") {
				t.Fatalf("apply: %s", m.notice)
			}
			if tc.toNode != "node-1" {
				if _, err := os.Stat(filepath.Join(root, "nodes", "node-1.yaml")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("old node still present: %v", err)
				}
			}
			hostsAfter, err := os.ReadFile(filepath.Join(root, "hosts.yaml"))
			if err != nil || !bytes.Equal(hostsBefore, hostsAfter) {
				t.Fatal("node rename changed host interface facts")
			}
			cfg := configFor(root, secrets)
			var report bytes.Buffer
			if err := writeCheckReport(&report, cfg); err != nil {
				t.Fatalf("Check after apply: %v\n%s", err, &report)
			}
			destination := t.TempDir()
			if err := exportAction(strings.NewReader(""), &report, []string{"*"}, map[string]string{"to": destination, "yes": "true"}, cfg); err != nil {
				t.Fatalf("Export after apply: %v\n%s", err, &report)
			}
			targets, err := os.ReadDir(filepath.Join(destination, tc.toNode, "example"))
			if err != nil || len(targets) != 1 || targets[0].Name() != tc.target {
				t.Fatalf("exported targets = %v, err = %v", targets, err)
			}
			data, err := os.ReadFile(filepath.Join(destination, tc.toNode, "example", tc.target, engine.ManifestFile))
			if err != nil {
				t.Fatal(err)
			}
			var manifest engine.DeployManifest
			if err := yaml.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest.Node != tc.toNode || manifest.Instance != tc.target || len(manifest.Networks) != 1 || manifest.Networks[0].Address != tc.address || len(manifest.Ports) != tc.ports {
				t.Fatalf("migration lost container facts: %+v", manifest)
			}
		})
	}
}
