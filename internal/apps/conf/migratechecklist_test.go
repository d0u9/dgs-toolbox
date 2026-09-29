package conf

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestMigrationProcedureNamesRenderedUninstallAndInstallPaths(t *testing.T) {
	root, _ := examplesRoot(t)
	l, err := load(root)
	if err != nil {
		t.Fatal(err)
	}
	rep := &migrationReport{OldID: "nas", NewID: "nas"}
	rep.Procedure = buildMigrationProcedure(l, l, "nas", "nas", root, "", map[string]string{"node": "from=nas,to=nas"}, nil)
	var out bytes.Buffer
	rep.writeProcedure(&out, nil)
	got := out.String()
	for _, want := range []string{
		`(cd "$M/old/nas/samba/samba-nas" && ./uninstall.sh)`,
		`(cd "$M/new/nas/samba/samba-nas" && ./install.sh)`,
		"**Scenario: relocate this machine.**",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in procedure:\n%s", want, got)
		}
	}
	if strings.Contains(got, "no generated uninstall.sh") {
		t.Fatalf("uninstall manifest was not read:\n%s", got)
	}
	for ni := range l.inv.Nodes {
		if l.inv.Nodes[ni].ID == "nas" {
			l.inv.Nodes[ni].Instances[0].Runtime = "" // host runtime: deploy/ is not rendered
		}
	}
	rep.Procedure = buildMigrationProcedure(l, l, "nas", "nas", root, "", map[string]string{"node": "from=nas,to=nas"}, nil)
	out.Reset()
	rep.writeProcedure(&out, nil)
	if !strings.Contains(out.String(), "no generated uninstall.sh") {
		t.Fatalf("host runtime was given a container uninstall script:\n%s", out.String())
	}
}

func TestMigrationComposeFactsReadsContainersAndMounts(t *testing.T) {
	containers, mounts, problem := migrationComposeFacts([]exportFile{{Path: "n/freshrss/rss/compose.yaml", Bytes: []byte(`services:
  rss:
    container_name: rss
    volumes:
      - data:/var/www/FreshRSS/data
      - /srv/rss/opml:/opml:ro
      - /tmp/anonymous
      - type: tmpfs
        target: /cache
      - type: bind
        source: /srv/rss/extensions
        target: /extensions
`)}})
	if problem != "" {
		t.Fatal(problem)
	}
	want := []migrationMount{{"volume", "/var/www/FreshRSS/data"}, {"bind", "/opml"}, {"bind", "/extensions"}}
	if len(containers) != 1 || containers[0] != migrationNamedContainer("rss") || fmt.Sprint(mounts) != fmt.Sprint(want) {
		t.Fatalf("containers = %v, mounts = %v", containers, mounts)
	}
}

func TestMigrationComposeFactsFindsUnnamedContainerByLabel(t *testing.T) {
	containers, _, problem := migrationComposeFacts([]exportFile{{Path: "compose.yaml", Bytes: []byte("services:\n  archive:\n    image: x\n")}})
	if problem != "" || len(containers) != 1 || containers[0].Ref != `"$(docker ps -aq --filter label=com.docker.compose.service=archive)"` {
		t.Fatalf("containers = %+v, problem = %q", containers, problem)
	}
}

func TestMigrationProcedureImportsOnlyWhenReplacing(t *testing.T) {
	run := migrationRuntime{Service: "freshrss", Instance: "rss", Script: true, Containers: []migrationContainer{migrationNamedContainer("rss")}, Mounts: []migrationMount{{"volume", "/data"}}}
	rep := &migrationReport{OldID: "network-4-linux-01", NewID: "network-4-linux-02"}
	rep.Procedure = migrationProcedure{Scenario: migrationReplace, Old: []migrationRuntime{run}, New: []migrationRuntime{run}, Pair: map[string]string{"rss": "rss"}}
	var out bytes.Buffer
	rep.writeProcedure(&out, nil)
	got := out.String()
	for _, want := range []string{
		`docker inspect -f '{{range .Mounts}}{{.Type}}|{{.Source}}|{{.Destination}}{{"\n"}}{{end}}' 'rss' > "$M/data/rss/rss.mounts"`,
		`sudo tar -C "$(awk -F'|' '$3=="/data"{print $2}' *.mounts)" -czpf data.tgz .`,
		`sudo tar -C "$(awk -F'|' '$3=="/data"{print $2}' *.new-mounts)" -xzpf data.tgz`,
		"docker start 'rss'  # installed in phase 4",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in procedure:\n%s", want, got)
		}
	}
	rep.Procedure.Scenario = migrationRelocate
	out.Reset()
	rep.writeProcedure(&out, nil)
	if strings.Contains(out.String(), "-xzpf") || !strings.Contains(out.String(), `(cd "$M/new/network-4-linux-02/freshrss/rss" && ./install.sh)`) {
		t.Fatalf("relocation imports data or does not install:\n%s", out.String())
	}
}
