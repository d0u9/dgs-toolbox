package recipients

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddKey(t *testing.T) {
	root := t.TempDir()
	first, second := ageKey(t), ed25519Key(t)

	file, err := AddKey(root, "Laptop", first, Meta{Description: "Main age key"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.ToSlash(file) != "hosts/Laptop.json" {
		t.Errorf("file %s", file)
	}
	// Same host ignoring case; an SSH comment is not written.
	if _, err := AddKey(root, "laptop", second+" me@laptop", Meta{Description: "  SSH key  "}); err != nil {
		t.Fatal(err)
	}
	folder := load(t, root)
	if len(folder.Problems) != 0 || len(folder.Hosts) != 1 {
		t.Fatalf("folder %+v %q", folder.Hosts, problems(folder))
	}
	host := folder.Hosts[0]
	if host.Name != "Laptop" || len(host.Keys) != 2 || host.Keys[0].Key != first || host.Keys[1].Key != second || host.Keys[1].Description != "SSH key" || host.Keys[1].Comment != "me@laptop" {
		t.Errorf("host %+v", host)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, HostsDir)); len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}

	for name, test := range map[string]struct {
		host, key, description, fragment string
	}{
		"duplicate":   {"nas", first, "x", "already listed under Laptop"},
		"group name":  {"g-nas", ageKey(t), "x", "kept for groups"},
		"bad name":    {"has space", ageKey(t), "x", "may use only"},
		"description": {"nas", ageKey(t), " ", "description is required"},
		"bad key":     {"nas", "nope", "x", "not an age or SSH public key"},
	} {
		if _, err := AddKey(root, test.host, test.key, Meta{Description: test.description}); err == nil || !strings.Contains(err.Error(), test.fragment) {
			t.Errorf("%s: %v", name, err)
		}
	}

	write(t, root, "hosts/broken.json", `{`)
	if _, err := AddKey(root, "Broken", ageKey(t), Meta{Description: "x"}); err == nil || !strings.Contains(err.Error(), "has an error") {
		t.Errorf("broken host: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "hosts", "broken.json")); string(data) != "{" {
		t.Error("broken host file was rewritten")
	}
}

func TestRemoveKey(t *testing.T) {
	root := t.TempDir()
	shared, other := ageKey(t), ageKey(t)
	for _, host := range []string{"a", "b"} {
		if _, err := AddKey(root, host, map[string]string{"a": shared, "b": other}[host], Meta{Description: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	// A key listed twice is only a warning; write the second listing by hand.
	write(t, root, "hosts/c.json", hostJSON([2]string{shared, "copy"}, [2]string{ageKey(t), "kept"}))

	changed, err := RemoveKey(root, shared)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(slashes(changed), ",") != "hosts/a.json,hosts/c.json" {
		t.Errorf("changed %v", changed)
	}
	folder := load(t, root)
	a, _ := folder.Host("a")
	c, _ := folder.Host("c")
	b, _ := folder.Host("b")
	if len(a.Keys) != 0 || len(c.Keys) != 1 || c.Keys[0].Description != "kept" || len(b.Keys) != 1 {
		t.Errorf("hosts a %+v b %+v c %+v", a, b, c)
	}
	hasProblem(t, folder, Warning, "hosts/a.json", "no keys")
	if changed, err := RemoveKey(root, shared); err != nil || len(changed) != 0 {
		t.Errorf("second remove: %v %v", changed, err)
	}
}

func slashes(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.ToSlash(p)
	}
	return out
}

func TestRemoveFromGroups(t *testing.T) {
	root := t.TempDir()
	write(t, root, "hosts/nas.json", hostJSON([2]string{ageKey(t), "x"}))
	write(t, root, "hosts/vps.json", hostJSON([2]string{ageKey(t), "x"}))
	write(t, root, "groups/g-all.json", `{"hosts":["NAS","vps","gone"]}`)
	write(t, root, "groups/g-web.json", `{"hosts":["vps"]}`)
	changed, err := RemoveFromGroups(root, "nas")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(slashes(changed), ",") != "groups/g-all.json" {
		t.Errorf("changed %v", changed)
	}
	data, _ := os.ReadFile(filepath.Join(root, "groups", "g-all.json"))
	if !strings.Contains(string(data), `"vps"`) || !strings.Contains(string(data), `"gone"`) || strings.Contains(strings.ToLower(string(data)), "nas") {
		t.Errorf("g-all:\n%s", data)
	}
}

func TestMetaIsKeptAndChecked(t *testing.T) {
	root := t.TempDir()
	meta := Meta{Description: "mine", Origin: OriginGenerated, Added: "2026-09-15", PrivateKey: "~/.config/age/laptop.agekey"}
	if _, err := AddKey(root, "laptop", ageKey(t), meta); err != nil {
		t.Fatal(err)
	}
	// Adding a second key rewrites the file; the first key's metadata survives.
	if _, err := AddKey(root, "laptop", ageKey(t), Meta{Description: "second", Origin: OriginAdded}); err != nil {
		t.Fatal(err)
	}
	host, _ := load(t, root).Host("laptop")
	if len(host.Keys) != 2 || host.Keys[0].Meta != meta || host.Keys[1].Origin != OriginAdded {
		t.Errorf("keys %+v", host.Keys)
	}
	for name, bad := range map[string]Meta{
		"origin": {Description: "x", Origin: "stolen"},
		"date":   {Description: "x", Added: "15/09/2026"},
	} {
		if _, err := AddKey(root, "laptop", ageKey(t), bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	write(t, root, "hosts/hand.json", `{"keys":[{"public_key":"`+ageKey(t)+`","description":"x","added":"yesterday"}]}`)
	hasProblem(t, load(t, root), Error, "hosts/hand.json", "not a YYYY-MM-DD date")
}
