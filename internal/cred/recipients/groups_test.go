package recipients

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCreateGroup(t *testing.T) {
	root := t.TempDir()
	write(t, root, "hosts/host-a.json", `{"keys":[]}`)
	file, err := CreateGroup(root, "g-team", []string{"host-a"})
	if err != nil || filepath.ToSlash(file) != "groups/g-team.json" {
		t.Fatalf("%s: %v", file, err)
	}
	draft, err := ReadGroup(root, "G-TEAM")
	if err != nil || !reflect.DeepEqual(draft.Hosts, []string{"host-a"}) {
		t.Fatalf("%+v: %v", draft, err)
	}
	for _, name := range []string{"g-team", "G-Team", "../g-escape", "team"} {
		if _, err := CreateGroup(root, name, nil); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	write(t, root, "groups/g-broken.json", `{`)
	if _, err := CreateGroup(root, "G-BROKEN", nil); err == nil {
		t.Fatal("overwrote invalid file")
	}
	if _, err := CreateGroup(root, "g-missing", []string{"missing"}); err == nil {
		t.Fatal("accepted missing host")
	}
	if _, err := CreateGroup(root, "g-nested", []string{"g-team"}); err == nil {
		t.Fatal("accepted nested group")
	}
	if _, err := CreateGroup(root, "g-empty", nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, GroupsDir, "g-empty.json"))
	if !strings.Contains(string(data), `"hosts": []`) {
		t.Fatalf("empty group: %s", data)
	}
}

func TestGroupEditPreservesRawMembersAndDetectsChanges(t *testing.T) {
	root := t.TempDir()
	write(t, root, "hosts/host-a.json", `{"keys":[]}`)
	write(t, root, "hosts/broken.json", `{`)
	original := []string{"HOST-A", "missing", "broken", "host-a"}
	write(t, root, "groups/g-team.json", `{"hosts":["HOST-A","missing","broken","host-a"]}`)
	draft, err := ReadGroup(root, "g-team")
	if err != nil || !reflect.DeepEqual(draft.Hosts, original) {
		t.Fatalf("%+v: %v", draft, err)
	}
	path := filepath.Join(root, GroupsDir, "g-team.json")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveGroup(root, draft, draft.Hosts); err != nil {
		t.Fatal(err)
	}
	back, err := ReadGroup(root, "g-team")
	if err != nil || !reflect.DeepEqual(back.Hosts, original) {
		t.Fatalf("members lost: %+v %v", back, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Fatal("file permissions changed")
	}
	if _, err := SaveGroup(root, back, []string{"new-missing"}); err == nil {
		t.Fatal("added unavailable member")
	}
	if _, err := SaveGroup(root, back, []string{"HOST-A", "broken", "host-a"}); err != nil {
		t.Fatal(err)
	}
	newer := `{"hosts":["missing"]}`
	write(t, root, "groups/g-team.json", newer)
	if _, err := SaveGroup(root, back, nil); err == nil {
		t.Fatal("overwrote external edit")
	}
	if _, err := RenameGroup(root, back, "g-other"); err == nil {
		t.Fatal("renamed external edit")
	}
	data, _ := os.ReadFile(path)
	if string(data) != newer {
		t.Fatal("external edit changed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveGroup(root, back, nil); err == nil {
		t.Fatal("recreated deleted group")
	}
}

func TestGroupRenameKeepsBytesAndRefusesCollisions(t *testing.T) {
	root := t.TempDir()
	original := "{\n  \"hosts\": [\"missing\", \"missing\"]\n}\n"
	write(t, root, "groups/g-team.json", original)
	draft, err := ReadGroup(root, "g-team")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "groups/g-taken.json", `{`)
	for _, name := range []string{"g-team", "G-TAKEN", "bad", "../g-escape"} {
		if _, err := RenameGroup(root, draft, name); err == nil {
			t.Errorf("accepted rename to %q", name)
		}
	}
	if _, err := RenameGroup(root, draft, "g-other"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, GroupsDir, "g-other.json"))
	if string(data) != original {
		t.Fatal("rename changed file bytes")
	}
	if _, err := os.Stat(filepath.Join(root, GroupsDir, "g-team.json")); !os.IsNotExist(err) {
		t.Fatal("old name remains")
	}
	draft, err = ReadGroup(root, "g-other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenameGroup(root, draft, "G-OTHER"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, GroupsDir, "G-OTHER.json")); err != nil || string(data) != original {
		t.Fatalf("case rename: %s %v", data, err)
	}
}

func TestGroupInvalidFilesAreNotEditable(t *testing.T) {
	for _, body := range []string{`{`, `{"hosts":[],"unknown":true}`, `{"hosts":[]} {}`} {
		root := t.TempDir()
		write(t, root, "groups/g-bad.json", body)
		if _, err := ReadGroup(root, "g-bad"); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	root := t.TempDir()
	write(t, root, "groups/g-team.json", `{"hosts":[]}`)
	write(t, root, "groups/G-TEAM.json", `{"hosts":[]}`)
	entries, _ := os.ReadDir(filepath.Join(root, GroupsDir))
	if len(entries) == 2 {
		if _, err := ReadGroup(root, "g-team"); err == nil {
			t.Fatal("accepted ambiguous group")
		}
	}
}

func TestGroupSymlinkCannotBeMutated(t *testing.T) {
	root := t.TempDir()
	write(t, root, "groups/g-target.json", `{"hosts":[]}`)
	path := filepath.Join(root, GroupsDir, "g-link.json")
	if err := os.Symlink("g-target.json", path); err != nil {
		t.Skip(err)
	}
	if _, err := ReadGroup(root, "g-link"); err == nil {
		t.Fatal("accepted symlink for mutation")
	}
	if _, err := CreateGroup(root, "G-LINK", nil); err == nil {
		t.Fatal("replaced symlink")
	}
}
