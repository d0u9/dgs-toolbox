package tree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testDefinition struct {
	Name  string `yaml:"name"`
	About string `yaml:"about,omitempty"`
}

func TestDefinitionStoreLifecycle(t *testing.T) {
	s := testStoreAt(t.TempDir())
	k := DefinitionKind[testDefinition]{Dir: "outlines", Name: func(v testDefinition) string { return v.Name }, Validate: func(_ *Store, v testDefinition) error {
		if v.About == "invalid" {
			return errors.New("invalid draft")
		}
		return nil
	}}
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.FixedZone("test", 3600))
	for _, name := range []string{"bob", "alice"} {
		if err := SaveDefinition(s, k, "", testDefinition{Name: name}, now); err != nil {
			t.Fatal(err)
		}
	}
	list, err := ListDefinitions(s, k)
	if err != nil || len(list) != 2 || list[0].Name != "alice" {
		t.Fatalf("list: %+v, %v", list, err)
	}
	before, err := s.DefinitionBytes(k.Dir, "alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, draft := range []testDefinition{{Name: "alice"}, {Name: "carol", About: "invalid"}} {
		if err := SaveDefinition(s, k, "", draft, now); err == nil {
			t.Fatal("accepted collision/invalid draft")
		}
	}
	if err := SaveDefinition(s, k, "missing", testDefinition{Name: "carol"}, now); err == nil {
		t.Fatal("accepted missing previous")
	}
	if err := SaveDefinition(s, k, "alice", testDefinition{Name: "carol"}, now); err != nil {
		t.Fatal(err)
	}
	archived := filepath.Join(s.root, TrashDir, k.Dir, "alice-20261007T110000Z.yaml")
	got, err := os.ReadFile(archived)
	if err != nil || string(got) != string(before) {
		t.Fatalf("trash bytes: %q, %v", got, err)
	}
	if _, err := os.Stat(s.DefinitionPath(k.Dir, "alice")); !os.IsNotExist(err) {
		t.Fatalf("source: %v", err)
	}
	if err := SaveDefinition(s, k, "", testDefinition{Name: "alice", About: "second"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := TrashDefinition(s, k, "alice", now); err == nil {
		t.Fatal("accepted trash collision")
	}
	got, err = os.ReadFile(archived)
	if err != nil || string(got) != string(before) {
		t.Fatal("trash overwritten")
	}
	if _, err := ReadDefinition(s, k, "alice"); err != nil {
		t.Fatal("collision lost source", err)
	}
	if err := s.WriteDefinition(k.Dir, "wrong", []byte("name: alice\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDefinition(s, k, "wrong"); err == nil {
		t.Fatal("accepted filename mismatch")
	}
	if err := s.WriteDefinition(k.Dir, "unknown", []byte("name: unknown\nunknown: key\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDefinition(s, k, "unknown"); err == nil {
		t.Fatal("accepted unknown key")
	}
	for _, name := range []string{"../alice", "", "/alice"} {
		if _, err := TrashDefinition(s, k, name, now); err == nil {
			t.Fatal("accepted unsafe name")
		}
	}
	if err := s.WriteDefinition("../outside", "alice", before); err == nil {
		t.Fatal("accepted unsafe folder")
	}
	files, err := os.ReadDir(filepath.Dir(s.DefinitionPath(k.Dir, "alice")))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.Contains(file.Name(), "dgs-part") {
			t.Fatal("temporary file remains")
		}
	}
}
